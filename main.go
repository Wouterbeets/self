// self — a local-first, event-sourced runtime, and to the shell a filter with a
// memory. One append-only log is the only authoritative state; every capability
// and every view is a deterministic replay of it. The kernel holds no resident
// model: intelligence is whatever process a caller pipes beside it or names to
// `self loop`.
//
//	self "add a mood tracker" | claude -p | self hear
//
// An ask arrives as argv, so the first self situates it against the instance's
// own state and appends nothing. The mind does durable work through installed
// commands and prints events. `self hear` lands them: events append, and
// authored scripts install under receipts the kernel signs with a key only it
// holds. A declaration without a script stays pending and rides the next
// prompt. `self loop` repeats complete situated turns until one leaves the
// authoritative log unchanged.
//
// Reads project. Writes append. Orientation is a read.
//
// PROTOCOL.md is the contract, embedded here and printed by `self help`. It is
// the only place the wire is described: comments in this package point at it
// rather than restating it, because six hand-synced copies of one contract is
// how the previous kernel came to contradict itself inside a single brief.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const cliUsage = `self — local-first event-sourced runtime

Usage:
  self [ask...]                 situate an ask; bare self presents the naked surface
  self hear                     ingest event JSONL or authored scripts from stdin
  self brief [name]             the surface; with a name, that one capability in full
  self run <command> [args...]  execute a command capability and append its events
  self view <name> [args...]    replay a pure view; built-in log is always available
  self loop [opts] [-- mind...] wake a mind on this body until it rests (quiet wakings in a row)
  self learn <account-dir>      deposit an account and print its learning prompt
  self give <selector> <dir>    write an event or capability account
  self rehydrate                rebuild derived capability files from the log
  self completion <shell>       print a completion script (zsh|bash|fish)
  self help                     print the complete protocol

Loop:
  self loop --help
  SELF_LOOP_MIND='<shell command>' self loop

Environment:
  SELF_HOME, SELF_CALLER, SELF_LOOP_MIND, SELF_LOOP_ASK, SELF_LOOP_MAX_PASSES, SELF_LOOP_SETTLE, SELF_LOOP_TIMEOUT`

func main() {
	home := homeDir()
	args := os.Args[1:]
	verb := ""
	if len(args) > 0 {
		verb, args = args[0], args[1:]
	}

	err := dispatch(home, verb, args, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "self: %s\n", err)
		os.Exit(1)
	}
}

func dispatch(home, verb string, args []string, out io.Writer) error {
	switch verb {
	case "": // the read face: the ask is argv, and stdin is never touched
		return cmdSituate(home, strings.Join(args, " "), out)

	case "hear": // the write face: the one door a mind's output enters through
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		return cmdHear(home, input, out)

	case "brief":
		st, err := loadState(home)
		if err != nil {
			return err
		}
		if len(args) > 0 {
			page, err := briefOne(st, args[0])
			if err != nil {
				return err
			}
			_, err = io.WriteString(out, page)
			return err
		}
		_, err = io.WriteString(out, brief(home, st))
		return err

	case "run", "view":
		// Discovery only reads state: never mint a missing instance key here.
		st, err := loadState(home)
		if err != nil {
			return err
		}
		typ, arg := kindView, "name"
		if verb == "run" {
			typ, arg = kindCommand, "command"
		}
		if len(args) == 0 {
			_, err := fmt.Fprintf(out, "usage: self %s <%s> [args...]\n\n%s", verb, arg, capabilityList(st, typ))
			return err
		}
		name, rest := args[0], args[1:]
		// Diagnostics go to stderr, never to out: stdout is the wire a mind's
		// pipeline parses, and an index printed into it would be read as events.
		if unknown := unfoldMissing(st, typ, name); unknown != "" {
			io.WriteString(os.Stderr, unknown)
		}
		// What the capability itself says, forwarded as it is written and
		// counted: a tool that explained what it wanted does not get a second
		// explanation stapled to it.
		said := &tally{w: os.Stderr}
		if verb == "view" {
			page, err := runViewDiag(home, st, name, said, rest...)
			if err != nil {
				io.WriteString(os.Stderr, unfoldFailed(st, typ, name, said.n == 0))
				return err
			}
			_, err = out.Write(page)
			return err
		}
		evs, err := runCommand(home, st, name, rest, doorCLI, callerClaim(), said)
		if err != nil {
			io.WriteString(os.Stderr, unfoldFailed(st, typ, name, said.n == 0))
			return err
		}
		for _, e := range evs {
			if _, err := fmt.Fprintf(out, "%d\t%s\t%s\n", e.Seq, e.Name, trunc(compact(e.Payload), 160)); err != nil {
				return err
			}
		}
		return nil

	case "loop":
		return cmdLoop(home, args, out, os.Stderr)

	case "learn":
		if len(args) != 1 {
			return fmt.Errorf("usage: self learn <account-dir>")
		}
		return cmdLearn(home, args[0], out)

	case "give":
		if len(args) != 2 {
			return fmt.Errorf("usage: self give <event-prefix | command/<name> | view/<name>> <dir>")
		}
		return cmdGive(home, args[0], args[1])

	case "rehydrate":
		return rehydrate(home)

	case "completion":
		if len(args) != 1 {
			return fmt.Errorf("usage: self completion <zsh|bash|fish>")
		}
		script, err := completionScript(args[0])
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, script)
		return err

	case "__complete": // the machine face the shims call; PROTOCOL.md: Completion
		return cmdComplete(home, args, out)

	case "-h", "--help":
		_, err := io.WriteString(out, cliUsage+"\n")
		return err

	case "help":
		_, err := io.WriteString(out, protocolDoc)
		return err

	default:
		// Not a verb, so it is an ask — `self what is going on` reads as well as
		// `self "what is going on"`. One bare word is the exception: it is far
		// more likely a mistyped verb than a question, and silently answering a
		// typo with a prompt would hide it.
		if len(args) == 0 && !strings.ContainsAny(verb, " \t\n") {
			fmt.Fprintf(os.Stderr, "%s\n\n", cliUsage)
			return fmt.Errorf("unknown verb %q — to ask a question instead, quote it: self %q", verb, verb)
		}
		return cmdSituate(home, strings.Join(append([]string{verb}, args...), " "), out)
	}
}

// ───────────────────────────────── the brief ────────────────────────────────

// brief is the state card: what this instance is, what it can do, what is
// pending, and which authoring attempts stand refused. Facts only — the contract lives in PROTOCOL.md and there
// is exactly one copy of it. This is the read an agent starts from, and every
// line of it is a replay of the log.
func brief(home string, st *state) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# self — %s\n\n", home)

	caller := callerClaim()
	if caller == "" {
		caller = `unset — export SELF_CALLER="<who you are>" so your writes are attributable`
	}
	fmt.Fprintf(&b, "log: %d events    caller: %s\n", len(st.Events), caller)
	// Only where a line was actually cut. The ellipsis is the thing a reader
	// notices, so the way to expand it belongs beside the ellipsis and not
	// forty lines below in `## where` — and on an instance whose summaries all
	// fit, this line would be explaining a truncation nobody can see.
	if anyClipped(st) {
		b.WriteString("lines below are clipped; `self brief <name>` prints one declaration in full\n")
	}
	if len(st.Events) > 0 && st.Key == nil {
		b.WriteString("\n**no .secret beside this log** — no receipt can verify, so this instance has no capabilities.\n")
	}

	b.WriteString("\n## commands — `self run <name> [args…]`\n\n")
	b.WriteString(capabilityList(st, kindCommand))
	b.WriteString("\n## views — `self view <name> [args…]`\n\n")
	b.WriteString(capabilityList(st, kindView))

	if p := st.pending(); len(p) > 0 {
		b.WriteString("\n## pending — declared, no script yet\n\n")
		for _, c := range p {
			fmt.Fprintf(&b, "- %s (declared at seq %d)\n", c.key(), c.DeclSeq)
		}
	}
	if len(st.Reject) > 0 {
		b.WriteString("\n## refused — standing, until authored or retired\n\n")
		for _, r := range st.Reject {
			where := strings.Trim(r.Type+"/"+r.Name, "/")
			if where == "" {
				where = "(unnamed)"
			}
			fmt.Fprintf(&b, "- %s (seq %d): %s\n", where, r.Seq, oneLine(r.Reason))
		}
	}
	if st.capabilitiesReady() {
		b.WriteString("\nnothing pending, nothing refused.\n")
	}

	b.WriteString("\n## where\n\n")
	b.WriteString("`events.jsonl` the log, authoritative · `cap/` installed scripts, derived · `.secret` the signing key\n")
	b.WriteString("`self help` the protocol · `self view log` what happened lately · `self brief <name>` one capability in full\n")
	return b.String()
}

// unfoldMissing is the CLI's answer to a name this log does not hold: the index
// of the names it does, printed before the kernel's own one-line complaint. It
// returns text rather than writing it, so the caller owns the stream — stdout
// is the wire, and an index printed into it would be parsed as events.
func unfoldMissing(st *state, typ, name string) string {
	if st.cap(typ, name) != nil || st.cap(otherKind(typ), name) != nil {
		return "" // it exists, or materialize will redirect to the other kind
	}
	// The built-in log is a view this instance holds without ever having
	// declared one, so it is absent from the capability index the check above
	// reads. It is still a name that resolves.
	if typ == kindView && name == "log" {
		return ""
	}
	return fmt.Sprintf("self: no %s %q in this log. What there is:\n\n%s\n", typ, name, capabilityList(st, typ))
}

// unfoldFailed is the CLI's answer to a capability that ran and refused what it
// was given. "Wrong arguments" and "I do not know this tool's arguments" are one
// moment to the reader, so a non-zero exit is worth the declaration.
//
// How much of it depends on what the script already said. A capability that
// documents itself — `peer` prints its verb table and exits 2 — has answered the
// question, and gets only the pointer to the rationale it cannot print. One that
// failed silently gets the whole declaration, because otherwise the exit code is
// the only thing the reader has.
func unfoldFailed(st *state, typ, name string, silent bool) string {
	c := st.cap(typ, name)
	if c == nil || c.Receipt == nil {
		return "" // never ran: the missing-name index or the kernel's error stands alone
	}
	if silent {
		if page, err := briefOne(st, typ+"/"+name); err == nil {
			return "\n" + page
		}
	}
	return fmt.Sprintf("\nself: `self brief %s` — what %s is for, and what its arguments mean\n", name, c.key())
}

// tally forwards everything written and remembers whether anything was.
type tally struct {
	w io.Writer
	n int
}

func (t *tally) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	t.n += n
	return n, err
}

func otherKind(typ string) string {
	if typ == kindView {
		return kindCommand
	}
	return kindView
}

// briefOne is the drill-down a bounded surface owes its reader. The brief now
// answers only which capability; this answers what it takes and what skipping
// it costs — the whole of Description, unclipped, for the one mind that asked.
// Without it the bound would not be a bound but a deletion.
func briefOne(st *state, selector string) (string, error) {
	var found []*capability
	if typ, name, qualified := strings.Cut(selector, "/"); qualified {
		if typ != kindCommand && typ != kindView {
			return "", fmt.Errorf("a capability selector is command/<name> or view/<name>")
		}
		if c := st.cap(typ, name); c != nil {
			found = append(found, c)
		}
	} else {
		// Unqualified and ambiguous prints both rather than guessing: a command
		// and a view under one name do different things to the log, and that is
		// exactly the distinction a reader is here to resolve.
		for _, typ := range []string{kindCommand, kindView} {
			if c := st.cap(typ, selector); c != nil {
				found = append(found, c)
			}
		}
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no capability %q in this log — `self brief` lists what there is", selector)
	}
	var b strings.Builder
	for i, c := range found {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "# %s\n\n%s\n", c.key(), oneLine(c.Decl.Description))
		if c.Type == kindView {
			consumes := strings.Join(c.Decl.Consumes, ", ")
			if consumes == "" {
				consumes = "the whole log"
			}
			fmt.Fprintf(&b, "\nconsumes: %s\n", consumes)
		}
		fmt.Fprintf(&b, "declared at seq %d", c.DeclSeq)
		if c.Receipt != nil {
			fmt.Fprintf(&b, " · installed at seq %d", c.RcptSeq)
		}
		if c.Pending() {
			b.WriteString(" · " + strings.Trim(oneLine(pendingMark(c)), "*() "))
		}
		b.WriteString("\n")
		if c.Reject != nil {
			fmt.Fprintf(&b, "refused at seq %d: %s\n", c.Reject.Seq, oneLine(c.Reject.Reason))
		}
	}
	return b.String(), nil
}

// capabilityList is the same live index in orientation and command discovery.
func capabilityList(st *state, typ string) string {
	var b strings.Builder
	caps := st.list(typ)
	for _, c := range caps {
		fmt.Fprintf(&b, "- %s — %s%s\n", c.Name, c.Decl.summary(), pendingMark(c))
	}
	if typ == kindView && st.cap(kindView, "log") == nil {
		fmt.Fprintf(&b, "- log — the last %d events, newest last; `--all` for the whole log (built in, shadowable)\n", builtinLogTail)
	}
	if len(caps) == 0 {
		fmt.Fprintf(&b, "\n(no declared %ss yet — `self help` shows how to author one)\n", typ)
	}
	return b.String()
}

// anyClipped reports whether the surface is hiding anything, which is exactly
// when it owes the reader a way through.
func anyClipped(st *state) bool {
	for _, c := range st.Caps {
		if strings.HasSuffix(c.Decl.summary(), "…") {
			return true
		}
	}
	return false
}

func pendingMark(c *capability) string {
	if !c.Pending() {
		return ""
	}
	if c.Receipt != nil {
		return "  *(re-declared, running the older script until re-authored)*"
	}
	return "  *(pending — no script yet)*"
}

// ─────────────────────────────────── util ───────────────────────────────────

func oneLine(s string) string {
	if s := oneLineOrEmpty(s); s != "" {
		return s
	}
	return "(no description)"
}

// oneLineOrEmpty flattens prose to a single line and, unlike oneLine, keeps
// nothing as nothing — so a caller can tell absence from content and choose its
// own fallback rather than rendering a placeholder into a derived field.
func oneLineOrEmpty(s string) string { return strings.Join(strings.Fields(s), " ") }

// firstSentence is the salvage path for a declaration written before summaries
// existed: these descriptions open with usage and the acting verb and only then
// turn to rationale, so the opening sentence is the closest thing to a summary
// already present in the log. A heuristic, and only ever a fallback — a
// declared Summary is never guessed at.
func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

// clip bounds a string like trunc, but retreats to a word boundary first: a
// summary is read by a mind, and a word severed mid-syllable costs more
// attention than the characters it saves. It gives up on the boundary rather
// than the bound if backing up would discard most of the budget.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	if sp := strings.LastIndexByte(s[:cut], ' '); sp > n/2 {
		cut = sp
	}
	return strings.TrimRight(s[:cut], " ,;:.—-") + "…"
}

// trunc cuts to at most n bytes without splitting a rune: a prompt or a report
// carrying half a character is invalid UTF-8, and the prompt is piped straight
// into a model.
func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func compact(raw json.RawMessage) string {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}
