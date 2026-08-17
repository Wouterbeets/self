package main

// The seam. `self` is a filter, and the mind is whatever the shell puts
// between two invocations of it:
//
//	echo "whats going on today?" | self | claude -p | self
//
// The kernel holds no model and spawns no mind — ever. The first `self` turns
// prose into a situated prompt: the question wrapped with the orientation
// brief, the recent conversation, any pending work, and the answer contract.
// The mind is any process that reads that prompt and prints event JSONL. The
// second `self` hears the mind: event lines are appended to the log, authored
// scripts are installed under locally signed receipts, and the reply passes
// through to the caller. Which face runs is decided by what arrives on stdin:
//
//	prose            → ask   (record self.asked, emit the situated prompt)
//	event lines      → hear  (append, install, reply)
//	nothing (a tty)  → the work prompt: pending compiles, else one reflection
//
// So the strange loop is one shell idiom, applied until quiet:
//
//	self | claude -p | self         # one improvement / compile cycle
//
// A declaration without a script stays pending and is asked again on every
// pass — the loop converges when the mind has nothing left to author.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// ─────────────────────────────── the contracts ──────────────────────────────

// pipeContract is the shape of an installed capability script — what a mind
// must author toward. It rides inside every prompt that asks for a script.
const pipeContract = `command script: receives args as argv, current events as JSONL on stdin, writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at, and provenance (via — the door the invocation came through — and by, the caller's claim); a script cannot set them.
projector script: receives the events matching its declared consumes list as JSONL on stdin (an empty list or "*" means every event — declare consumes precisely and the script never needs to filter), writes bare semantic HTML on stdout. Do not emit CSS, JavaScript, inline styles, or external assets: the kernel injects the shared shell at serve time. The kernel persists projector output to SELF_HOME/site/<name>.html.
The kernel sets SELF_HOME on every script. Any language with a shebang works; use only standard libraries.`

// answerContract closes every prompt the ask face emits. It teaches any mind —
// claude -p, a local model, a shell script — how to speak back through the
// pipe so the second `self` can hear it.
const answerContract = `HOW TO ANSWER — your stdout is piped back into ` + "`self`" + `, which reads it line by line. A line that parses as {"name":"…","payload":{…}} is an event: self appends it to the log verbatim (the kernel stamps id, seq, time, and provenance — never you). Every other line passes through to the caller as prose. One compact JSON object per line; no Markdown, no code fences, no backticks around JSON.

ALWAYS end with your reply as one event line:
{"name":"self.replied","payload":{"text":"<your reply>"}}
The log is the only memory: a reply that is not an event was never said.

To persist ordinary state, prefer installed commands (self run <command> …) or emit the domain event directly. To grow a capability, emit command.declared / projector.declared — and its script, in the same answer or a later pass, as:
{"name":"script.authored","payload":{"type":"command|projector","name":"<capability>","script":"<the full script>"}}
Only the kernel installs and signs; a declaration without a script stays pending, and self will ask again on the next pass. Declaring nothing is a valid outcome.

You are expected to have tools: explore SELF_HOME yourself — site/*.html (rendered state), events.jsonl (the authoritative log), capabilities/ (installed scripts) — before answering. Do not edit events.jsonl and do not install scripts yourself: print events; self does the rest. THIS REPLY IS FINAL — you are not re-invoked, and whatever you have not printed when you exit was never said.`

// ─────────────────────────────── the dispatcher ─────────────────────────────

// cmdPipe is bare `self`: the filter. Its face is chosen by the shape of
// stdin, so the same word composes on either side of a mind.
func cmdPipe(home string) error {
	if err := ensureHome(home); err != nil {
		return err
	}
	if stdinIsTTY() {
		if stdoutIsTTY() {
			// A human at a terminal: orientation, not a prompt.
			fmt.Print(freshBrief(home))
			fmt.Print(pipeStatus(home))
			return nil
		}
		// `self | mind | self` with no piped question: emit the work prompt.
		return emitWork(home, os.Stdout)
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	return pipeFilter(home, string(input), os.Stdout)
}

// pipeFilter routes one stdin body to a face. Split from cmdPipe so tests
// drive the seam without a terminal.
func pipeFilter(home, input string, out io.Writer) error {
	evs, scripts, reply := parseWire(input)
	if len(evs) == 0 && len(scripts) == 0 {
		question := strings.TrimSpace(input)
		if question == "" {
			return emitWork(home, out)
		}
		return emitAsk(home, question, out)
	}
	return hear(home, evs, scripts, reply, out)
}

func stdinIsTTY() bool  { return isTTY(os.Stdin) }
func stdoutIsTTY() bool { return isTTY(os.Stdout) }

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// pipeStatus is the one-glance state a human gets under the brief.
func pipeStatus(home string) string {
	var b strings.Builder
	if pending := pendingDecls(home); len(pending) > 0 {
		fmt.Fprintf(&b, "\n%d declaration(s) pending scripts — run:  self | claude -p | self\n", len(pending))
	}
	b.WriteString("\nthe loop:  echo \"<ask>\" | self | claude -p | self      serve:  self serve\n")
	return b.String()
}

// ──────────────────────────────── the wire ──────────────────────────────────

// authored is a script the mind wrote for a declared capability, carried on
// the wire as a script.authored line. It never lands in the log raw — the
// signed script.compiled receipt is its record.
type authored struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Script string `json:"script"`
}

// parseWire classifies a stdin body line by line: event lines (a JSON object
// with a name), authored scripts (the script.authored event, lifted off the
// wire), and everything else — prose, kept in stream order as the reply. The
// text of a self.replied event joins the reply too: it is both memory and
// message.
func parseWire(input string) (evs []Event, scripts []authored, reply []string) {
	sc := bufio.NewScanner(strings.NewReader(input))
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		content, fence := unfence(line)
		if fence {
			continue
		}
		var e struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal([]byte(content), &e) != nil || e.Name == "" {
			reply = append(reply, line)
			continue
		}
		if e.Name == "script.authored" {
			var a authored
			if json.Unmarshal(e.Payload, &a) == nil && strings.TrimSpace(a.Script) != "" {
				scripts = append(scripts, a)
			} else {
				fmt.Fprintf(os.Stderr, "self: script.authored without a script — dropped\n")
			}
			continue
		}
		if e.Name == "self.replied" {
			var p struct{ Text string }
			if json.Unmarshal(e.Payload, &p) == nil && p.Text != "" {
				reply = append(reply, p.Text)
			}
		}
		evs = append(evs, newEvent(e.Name, e.Payload))
	}
	return evs, scripts, reply
}

// unfence strips the Markdown a chat-shaped mind (claude -p and its kin) wraps
// JSON in, so a model that answers in prose still plugs into the pipe. A line
// that is a bare fence marker (``` or ```json) is decoration; a single line
// wrapped in backticks is unwrapped. Anything else passes through untouched.
func unfence(line string) (content string, fence bool) {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "```") {
		return "", true
	}
	if len(t) >= 2 && strings.HasPrefix(t, "`") && strings.HasSuffix(t, "`") {
		return strings.TrimSpace(strings.Trim(t, "`")), false
	}
	return t, false
}

// ──────────────────────────────── hear ──────────────────────────────────────

// authorClaim names who authored the bytes a receipt carries: SELF_MIND_ID
// when the composer of the pipe set one, else the caller's claim, else the
// door itself. A claim, recorded and signed — never verified.
func authorClaim() string {
	if id := strings.TrimSpace(os.Getenv("SELF_MIND_ID")); id != "" {
		return id
	}
	if by := callerClaim(); by != "" {
		return by
	}
	return "pipe"
}

// hear ingests what came back through the pipe: events land with the pipe's
// provenance, authored scripts install under signed receipts (only for
// capabilities this log has declared), and the reply passes through to out.
func hear(home string, evs []Event, scripts []authored, reply []string, out io.Writer) error {
	if err := ensureHome(home); err != nil {
		return err
	}
	// Append first, install second, render once at the end — a declaration
	// and its authored script often arrive in the same breath, and no
	// projection should replay between the two.
	for i := range evs {
		evs[i].Via, evs[i].By = "pipe", callerClaim()
		if err := appendEvent(home, &evs[i]); err != nil {
			return err
		}
	}
	if n := applyRetirements(home, evs); n > 0 {
		fmt.Fprintf(os.Stderr, "self: retired %d capabilit(ies)\n", n)
	}
	installed := 0
	for _, a := range scripts {
		if err := installAuthored(home, a); err != nil {
			fmt.Fprintf(os.Stderr, "self: %s\n", err)
			continue
		}
		installed++
	}
	if installed > 0 {
		refreshSite(home)
	} else {
		refreshSiteAfter(home, evs)
	}
	for _, line := range reply {
		fmt.Fprintln(out, line)
	}
	fmt.Fprintf(os.Stderr, "self: heard %d event(s), installed %d script(s)\n", len(evs), installed)
	if pending := pendingDecls(home); len(pending) > 0 {
		fmt.Fprintf(os.Stderr, "self: %d declaration(s) pending scripts — run:  self | claude -p | self\n", len(pending))
	}
	return nil
}

// installAuthored installs one authored script, gated the only way anything
// installs: the capability must be declared in this log (and not retired),
// and the kernel signs a script.compiled receipt over the bytes. An authored
// script whose type/name are blank matches the single pending declaration if
// exactly one exists — tolerance for a terse mind, never ambiguity.
func installAuthored(home string, a authored) error {
	typ, name := strings.TrimSpace(a.Type), strings.TrimSpace(a.Name)
	if typ == "" || name == "" {
		pending := pendingDecls(home)
		if len(pending) != 1 {
			return fmt.Errorf("script.authored without type/name matches nothing (pending: %d)", len(pending))
		}
		typ, name = pending[0].Type, pending[0].Name
	}
	if typ != "command" && typ != "projector" {
		return fmt.Errorf("script.authored for unknown type %q", typ)
	}
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	commands, _, projectors, _ := declaredCaps(events)
	declared := false
	switch typ {
	case "command":
		_, declared = commands[name]
	case "projector":
		_, declared = projectors[name]
	}
	if !declared {
		return fmt.Errorf("script.authored for undeclared %s/%s — declare it first", typ, name)
	}
	if err := installScript(home, typ, name, a.Script); err != nil {
		return err
	}
	if err := appendReceipt(home, typ, name, a.Script, authorClaim()); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "self: installed %s/%s under a signed receipt\n", typ, name)
	return nil
}

// ──────────────────────────────── ask ───────────────────────────────────────

// emitAsk records the question — hearing an ask is an experience, and the log
// is the only memory — then emits the situated prompt for whatever mind sits
// downstream.
func emitAsk(home, question string, out io.Writer) error {
	if err := ensureHome(home); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"text": question})
	e := newEvent("self.asked", payload)
	e.Via, e.By = "pipe", callerClaim()
	if err := appendEvent(home, &e); err != nil {
		return err
	}
	refreshSiteAfter(home, []Event{e})
	_, err := io.WriteString(out, situatedPrompt(home, question))
	return err
}

// emitWork is the empty-stdin face: pending compiles if any, else one
// reflection — so bare `self | claude -p | self` always means something, and
// running it repeatedly converges when the mind has nothing left to author or
// improve.
func emitWork(home string, out io.Writer) error {
	if err := ensureHome(home); err != nil {
		return err
	}
	if len(pendingDecls(home)) > 0 {
		_, err := io.WriteString(out, situatedPrompt(home,
			"Author the pending scripts listed above. Emit one script.authored line per declaration; add nothing else unless something is plainly broken."))
		return err
	}
	if seq, content, ok := unansweredChat(home); ok {
		e := newEvent("self.reflected", json.RawMessage(`{}`))
		e.Via, e.By = "pipe", callerClaim()
		if err := appendEvent(home, &e); err != nil {
			return err
		}
		_, err := io.WriteString(out, situatedPrompt(home, fmt.Sprintf(
			"A user message on /chat (seq %d) is still waiting for a reply:\n\"%s\"\nAnswer it: emit one chat.message event with payload role \"assistant\" and content your reply. That reply is the work of this pass — explore the instance if you need context, but do not let any other improvement displace it.", seq, content)))
		return err
	}
	e := newEvent("self.reflected", json.RawMessage(`{}`))
	e.Via, e.By = "pipe", callerClaim()
	if err := appendEvent(home, &e); err != nil {
		return err
	}
	_, err := io.WriteString(out, situatedPrompt(home,
		"This is a self-improvement reflection. Explore this instance — capabilities, recent events, projections — and choose ONE small, high-value improvement: a missing capability, a clearer projection, a drift to fix. If warranted, declare it (command.declared / projector.declared) and author its script (script.authored); if nothing is worth changing, say so plainly and declare nothing. Keep it minimal."))
	return err
}

// situatedPrompt is the ask face's whole output: the ask, situated. Brief,
// conversation, pending work, the ask itself, and the answer contract — a
// complete cold-start context for any mind, built only from the log and its
// replays.
func situatedPrompt(home, ask string) string {
	var b strings.Builder
	b.WriteString("You are the mind of a self instance — a local-first, event-sourced runtime. This prompt came out of `self`; your stdout will be piped back into `self`.\n\n")
	b.WriteString(freshBrief(home))
	b.WriteString(conversationTail(home))
	b.WriteString(pendingSection(home))
	b.WriteString("\n## The ask\n\n")
	b.WriteString(strings.TrimSpace(ask))
	b.WriteString("\n\n## How to answer\n\n")
	b.WriteString(answerContract)
	b.WriteString("\n")
	return b.String()
}

// conversationTail renders the last few pipe exchanges so the loop reads as a
// conversation. Only events that entered through the pipe door count — a
// deposited record cannot speak here: doors are this log's facts, and the
// tail trusts them.
func conversationTail(home string) string {
	events, err := readEvents(home)
	if err != nil {
		return ""
	}
	type turn struct{ who, text string }
	var turns []turn
	for _, e := range events {
		switch e.Name {
		case "self.asked", "self.replied":
			if e.Via != "pipe" {
				continue
			}
			var p struct{ Text string }
			if json.Unmarshal(e.Payload, &p) == nil && p.Text != "" {
				turns = append(turns, turn{e.Name[len("self."):], p.Text})
			}
		case "chat.message":
			// the chat surface is the door users talk through; its turns
			// surface in the tail no matter which CLI door carried them
			var p struct{ Role, Content string }
			if json.Unmarshal(e.Payload, &p) == nil && p.Content != "" {
				who := "self"
				if p.Role == "user" {
					who = "you"
				}
				turns = append(turns, turn{who, p.Content})
			}
		}
	}
	if len(turns) <= 1 {
		return "" // the current ask alone is no conversation
	}
	if len(turns) > 8 {
		turns = turns[len(turns)-8:]
	}
	var b strings.Builder
	b.WriteString("\n## Recent conversation (from the log)\n\n")
	for _, t := range turns {
		text := strings.ReplaceAll(t.text, "\n", " ")
		if len(text) > 300 {
			text = text[:300] + "…"
		}
		fmt.Fprintf(&b, "%s: %s\n", t.who, text)
	}
	return b.String()
}

// pendingDecl is a declared capability with no script yet — or one declared
// anew since its last receipt, which is how revision looks on the log.
type pendingDecl struct {
	Type string
	Name string
	Decl json.RawMessage
}

// pendingDecls replays the log into the capabilities awaiting scripts: the
// latest declaration per live capability, pending when no verified receipt
// postdates it. Derived, like everything — there is no pending-state file.
func pendingDecls(home string) []pendingDecl {
	events, err := readEvents(home)
	if err != nil {
		return nil
	}
	secret, err := loadSecret(home)
	if err != nil {
		return nil
	}
	declSeq := map[string]int{}
	declPayload := map[string]json.RawMessage{}
	receiptSeq := map[string]int{}
	var order []string
	for _, e := range events {
		if typ, name := declName(e); typ != "" {
			key := typ + "/" + name
			if _, seen := declSeq[key]; !seen {
				order = append(order, key)
			}
			declSeq[key], declPayload[key] = e.Seq, e.Payload
			continue
		}
		switch e.Name {
		case "script.compiled":
			if r, ok := verifiedReceipt(secret, e.Payload); ok {
				receiptSeq[r.Type+"/"+r.Name] = e.Seq
			}
		case "capability.retired":
			if d, ok := parseRetirement(e.Payload); ok {
				key := d.Type + "/" + d.Name
				delete(declSeq, key)
				delete(declPayload, key)
				delete(receiptSeq, key)
			}
		}
	}
	var pending []pendingDecl
	done := map[string]bool{} // a retire-then-redeclare lists its key twice
	for _, key := range order {
		seq, live := declSeq[key]
		if !live || done[key] || receiptSeq[key] >= seq {
			continue
		}
		done[key] = true
		typ, name, _ := strings.Cut(key, "/")
		pending = append(pending, pendingDecl{typ, name, declPayload[key]})
	}
	return pending
}

// unansweredChat replays the log for the newest user chat.message with no
// assistant chat.message after it. A user waiting on a reply is work: the
// work face must surface it, because the conversation tail alone cannot
// outrank a model's inclination to report "nothing pending" and stop.
func unansweredChat(home string) (seq int, content string, ok bool) {
	events, err := readEvents(home)
	if err != nil {
		return 0, "", false
	}
	var lastUser *Event
	for i := range events {
		e := &events[i]
		if e.Name != "chat.message" {
			continue
		}
		var p struct{ Role, Content string }
		if json.Unmarshal(e.Payload, &p) != nil {
			continue
		}
		switch p.Role {
		case "user":
			lastUser = e
		case "assistant":
			lastUser = nil
		}
	}
	if lastUser == nil {
		return 0, "", false
	}
	var p struct{ Content string }
	json.Unmarshal(lastUser.Payload, &p)
	return lastUser.Seq, p.Content, true
}

// pendingSection renders the compile asks: one DECLARATION block per pending
// capability, the pipe contract they must honor, and one recently compiled
// script as this instance's idiom.
func pendingSection(home string) string {
	pending := pendingDecls(home)
	if len(pending) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Pending work — declared capabilities awaiting scripts\n\n")
	b.WriteString("For each declaration below, author a complete executable script (any language with a shebang, standard libraries only) honoring this contract, test it by execution with your own tools, and emit one script.authored line (see HOW TO ANSWER). If a declaration carries an \"implementation\", it is a reference: verify and adapt it — never copy it blindly.\n\n")
	b.WriteString(pipeContract)
	b.WriteString("\n")
	for _, p := range pending {
		fmt.Fprintf(&b, "\nDECLARATION (%s %q):\n%s\n", p.Type, p.Name, compactJSON(p.Decl))
	}
	skip := map[string]bool{}
	for _, p := range pending {
		skip[p.Type+"/"+p.Name] = true
	}
	if exName, exScript := exemplarScript(home, skip); exScript != "" {
		b.WriteString("\nA recently compiled capability of this instance, as idiom — learn its shape, do not copy it blindly:\n")
		fmt.Fprintf(&b, "\n--- EXEMPLAR %s ---\n%s\n--- END EXEMPLAR ---\n", exName, exScript)
	}
	return b.String()
}

// exemplarScript returns the most recently compiled script from the log's
// verified receipts, skipping the capabilities being asked about (so a
// recompile is never anchored to its own broken past). Traced compiles show a
// mind spends its first minute rediscovering the instance's idiom from disk;
// handing it one exemplar removes that phase.
func exemplarScript(home string, skip map[string]bool) (exName, exScript string) {
	events, err := readEvents(home)
	if err != nil {
		return "", ""
	}
	secret, err := loadSecret(home)
	if err != nil {
		return "", ""
	}
	for _, e := range events {
		if e.Name != "script.compiled" {
			continue
		}
		if r, ok := verifiedReceipt(secret, e.Payload); ok && !skip[r.Type+"/"+r.Name] {
			exName, exScript = r.Type+"/"+r.Name, r.Script
		}
	}
	const cap = 4096
	if len(exScript) > cap {
		exScript = exScript[:cap] + "\n… (truncated)"
	}
	return exName, exScript
}

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}
