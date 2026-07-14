package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The kernel holds no model — not even a fake one. Every ask — think,
// reflect, learn, and each compile — is handed to a mind PROCESS
// (SELF_MIND, e.g. "claude -p"; examples/mind-stub is a deterministic
// offline one for demos and tests), which explores and writes scripts with
// its own tools; the kernel only installs and signs what comes back. The llm
// value carries just enough to route a compile: the home it runs against,
// and — during a learn — the whole intent plus the orchestrator's stated
// reasoning, woven into each compile so no piece is authored in a dark room.
// The reasoning travels in-band, through the prompt and the log, never
// through a session store outside the log.

type llm struct {
	home      string
	intent    string
	reasoning string
}

// identity names the mind for provenance: who authored the bytes a receipt
// carries. SELF_MIND_ID lets an agent name itself; otherwise the mind
// executable is the honest mechanical answer.
func (c *llm) identity() string {
	return mindRef{exe: mindExe()}.identity()
}

func newLLM(home string) *llm {
	return &llm{home: home}
}

// ─────────────────────────────── the prompts ────────────────────────────────

const pipeContract = `command script: receives args as argv, current events as JSONL on stdin, writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at.
projector script: receives the events matching its declared consumes list as JSONL on stdin (an empty list or "*" means every event — declare consumes precisely and the script never needs to filter), writes bare semantic HTML on stdout. Do not emit CSS, JavaScript, inline styles, or external assets: the kernel injects the shared shell at serve time. The kernel persists projector output to SELF_HOME/site/<name>.html.
The kernel sets SELF_HOME on every script. Any language with a shebang works; use only standard libraries.`

// mindAnswerContract tells a capable, tool-using mind how to hand its answer
// back. A coding-agent mind (claude -p) will otherwise try to persist its work
// itself — write events.jsonl, run `self`, install a script — and that effort is
// wasted and denied: the kernel reads ONLY stdout and appends what it finds
// there, under its own signature. It also emits Markdown by habit; the pipe
// tolerates fences, but one clean JSON object per line is what actually wants
// ingesting. Woven into every ask that expects events (learn, reflect, compile).
const mindAnswerContract = `HOW TO ANSWER — the kernel reads ONLY your stdout. Event lines are JSON objects; prose lines are reply text. You do not and cannot write the log yourself: do not edit events.jsonl, run the self CLI, or install anything with your tools — that work is wasted. To persist ordinary state, print the domain event that records it. To add capabilities, print command.declared / projector.declared events as ONE line of compact JSON each (no Markdown, no code fences, no backticks). Declare only what is missing; ordinary domain events are not capability growth.

THIS REPLY IS FINAL — you run once per ask and are never re-invoked. Explore first, THEN answer completely: never end on a plan or a promise ("I'll explore and then respond") — whatever you have not said when you exit was never said.

WHAT YOU ARE GIVEN — your stdin is an orientation brief: where you are, what capabilities exist, where to look for the rest. That is all. To do your job you must EXPLORE the instance surface with your tools: read ` + "`SELF_HOME/site/kernel.html`" + ` for the full self-description, ` + "`SELF_HOME/site/*.html`" + ` for the rendered state a human sees, ` + "`SELF_HOME/events.jsonl`" + ` for the raw log, ` + "`SELF_HOME/capabilities/`" + ` for the compiled scripts. The kernel holds no internal state you cannot see on disk. A mind without tools to read those files cannot do this job.`

// ────────────────────────────── the compiler ────────────────────────────────

func (c *llm) compileCommand(d commandDecl) (string, string, error) {
	return compileViaMind(c.home, c.intent, c.reasoning, "command", d.Name, jsonRepr(d), d.Mind)
}

func (c *llm) compileProjector(d projectorDecl) (string, string, error) {
	return compileViaMind(c.home, c.intent, c.reasoning, "projector", d.Name, jsonRepr(d), d.Mind)
}

// compileViaMind hands a compile ask to the plugged mind through the same
// seam as every other ask. The declaration (its optional implementation
// reference included) rides in the prompt; an orientation brief rides on stdin
// (the mind inspects SELF_HOME itself for depth — site/*.html, events.jsonl,
// capabilities/); the answer is one script.authored event. During a learn the
// whole intent rides along too, so each piece is compiled toward the same
// product. The kernel still installs and signs — a mind authors bytes, only
// the kernel makes them real.
// compilePrompt is the text of a compile ask: author one script honoring the
// pipe contract, test it with your own tools, and hand back exactly one
// script.authored line — the kernel does the installing and signing.
func compilePrompt(intent, reasoning, exemplarName, exemplarScript, typ, name, decl string) string {
	prompt := fmt.Sprintf(`COMPILE one capability for this instance. Author a complete executable script (any language with a shebang, standard libraries only) honoring the pipe contract, adapted to this instance's state (a brief on your stdin; the full log is at SELF_HOME/events.jsonl if you need it).

%s

DECLARATION (%s %q):
%s

If the declaration carries an "implementation", it is a reference script: verify it and adapt it here — never copy it blindly. If it also carries a "revision", preserve the existing behavior except for that requested change. Use your own tools freely to write and TEST the script by execution before answering — but do not install it, edit events.jsonl, or run the self CLI: the kernel installs and signs the script from the one line you print, and nothing else. If this is a projector, emit only bare semantic HTML: no CSS, no JavaScript, no inline style attributes, no external assets.

Answer with ONE line of JSON and nothing else — no Markdown, no code fence:
{"name":"script.authored","payload":{"script":"<the full script>"}}`, pipeContract, typ, name, decl)
	if strings.TrimSpace(exemplarScript) != "" {
		prompt = "Here is a recently compiled " + typ + " as an idiom/reference for this instance. Learn its style and pipe-contract shape, but do not copy it blindly.\n\n--- EXEMPLAR " + exemplarName + " ---\n" + exemplarScript + "\n--- END EXEMPLAR ---\n\n" + prompt
	}
	if strings.TrimSpace(reasoning) != "" {
		prompt = "The ORCHESTRATOR that declared this capability explored the instance and explained its plan below (it is also in the log as learn.orchestrated). Compile in line with it.\n\n--- ORCHESTRATOR'S REASONING ---\n" + reasoning + "\n--- END REASONING ---\n\n" + prompt
	}
	if strings.TrimSpace(intent) != "" {
		prompt = "This capability is one part of a product with the following INTENT. Compile it so the whole intent is served.\n\n--- INTENT ---\n" + intent + "\n--- END INTENT ---\n\n" + prompt
	}
	return prompt
}

// exemplarScript returns the source of the most recently compiled capability
// of the same type (excluding the one being compiled, so a recompile is never
// anchored to its own broken past). Read from the log's verified receipts —
// traced compiles show a mind spends its first minute rediscovering the
// instance's idiom from disk; handing it one exemplar removes that phase.
func exemplarScript(home, typ, name string) (exName, exScript string) {
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
		if r, ok := verifiedReceipt(secret, e.Payload); ok && r.Type == typ && r.Name != name {
			exName, exScript = r.Name, r.Script
		}
	}
	const cap = 4096
	if len(exScript) > cap {
		exScript = exScript[:cap] + "\n… (truncated)"
	}
	return exName, exScript
}

// compileViaMind routes a compile to a resolved mind and returns the script
// with the identity of the mind that actually authored it — the receipt's
// by-line. A declaration's hint sets the starting mind; a mechanical failure
// (the process erroring, or answering without a script) walks up the
// SELF_MIND_ESCALATION chain, each hop recorded as a compile.escalated event.
// The unnamed default never escalates: with no roster declared, a failed
// compile fails exactly as it always has.
func compileViaMind(home, intent, reasoning, typ, name, decl, hint string) (string, string, error) {
	exName, exScript := exemplarScript(home, typ, name)
	prompt := compilePrompt(intent, reasoning, exName, exScript, typ, name, decl)
	m := resolveMind("compile", hint)
	for {
		res, err := pipeMindVia(home, "compile", m, prompt)
		if err == nil && strings.TrimSpace(res.Script) != "" {
			return res.Script, m.identity(), nil
		}
		if err == nil {
			err = fmt.Errorf("the mind answered a compile ask without a script.authored event")
		}
		next, ok := escalateFrom(m)
		if !ok {
			return "", "", err
		}
		fmt.Fprintf(os.Stderr, "self: compile %s %q via mind %q failed: %s — escalating to %q\n", typ, name, m.name, err, next.name)
		payload, _ := json.Marshal(map[string]any{"type": typ, "name": name, "from": m.name, "to": next.name, "reason": err.Error()})
		e := newEvent("compile.escalated", payload)
		if aerr := appendEvent(home, &e); aerr != nil {
			return "", "", aerr
		}
		m = next
	}
}

// ──────────────────────────────── the mind ─────────────────────────────────

// mindExe is the plugged mind, if any: SELF_MIND is the one way a mind is
// named, and a process behind it honors the one mind contract.
func mindExe() string {
	return strings.TrimSpace(os.Getenv("SELF_MIND"))
}

// ────────────────────────────── named minds ─────────────────────────────────
//
// An instance may plug more than one mind and route among them by NAME. The
// kernel stays model-free: it dereferences operator-chosen names to processes
// and nothing more — which model answers to a name, its endpoint, its cost,
// all live in the adapter's environment, exactly as with a single SELF_MIND.
//
//   SELF_MINDS               the roster: space-separated names, e.g. "fast deep top"
//   SELF_MIND_<NAME>         the executable a roster name dereferences to
//   SELF_MIND_ID_<NAME>      per-name provenance, signed into that mind's receipts
//   SELF_MIND_<KIND>         per-ask-kind route (THINK/REFLECT/LEARN/COMPILE):
//                            a roster name, else taken verbatim as an executable
//   SELF_MIND_ESCALATION     ordered names, cheap→expensive; a mechanically
//                            failed compile retries one step up the chain
//
// With none of these set, SELF_MIND alone behaves exactly as it always has.

// mindRef is a resolved mind: a roster name (empty for the unnamed default)
// and the executable behind it.
type mindRef struct {
	name string
	exe  string
}

// identity names this mind for provenance: SELF_MIND_ID_<NAME> for a roster
// mind, then SELF_MIND_ID, then the executable — the honest mechanical answer.
func (m mindRef) identity() string {
	if m.name != "" {
		if id := strings.TrimSpace(os.Getenv("SELF_MIND_ID_" + strings.ToUpper(m.name))); id != "" {
			return id
		}
	}
	if id := strings.TrimSpace(os.Getenv("SELF_MIND_ID")); id != "" {
		return id
	}
	if m.exe != "" {
		return m.exe
	}
	return "mind"
}

// rosterNames returns the declared roster. The ask kinds and "id" are
// reserved — a roster name becomes an env-var suffix (SELF_MIND_<NAME>), and
// those suffixes already mean something to the kernel.
func rosterNames() []string {
	reserved := map[string]bool{"think": true, "reflect": true, "learn": true, "compile": true, "id": true}
	var names []string
	for _, n := range strings.Fields(os.Getenv("SELF_MINDS")) {
		if reserved[strings.ToLower(n)] {
			fmt.Fprintf(os.Stderr, "self: SELF_MINDS name %q is reserved — ignored\n", n)
			continue
		}
		names = append(names, n)
	}
	return names
}

// mindByName dereferences a roster name to its executable. A name outside the
// roster, or one with no SELF_MIND_<NAME> bound, resolves to nothing.
func mindByName(name string) (mindRef, bool) {
	for _, n := range rosterNames() {
		if n == name {
			if exe := strings.TrimSpace(os.Getenv("SELF_MIND_" + strings.ToUpper(n))); exe != "" {
				return mindRef{name: n, exe: exe}, true
			}
		}
	}
	return mindRef{}, false
}

// resolveMind picks the mind for an ask: a declaration's hint first (honored
// only when it names a bound roster mind — a miss is noted and never fatal),
// then the per-kind route, then the plugged default. An empty result means no
// mind is plugged in at all, and the ask fails with the usual hint.
func resolveMind(kind, hint string) mindRef {
	if hint != "" {
		if m, ok := mindByName(hint); ok {
			return m
		}
		fmt.Fprintf(os.Stderr, "self: declaration asked for mind %q, which SELF_MINDS does not offer — using the %s route\n", hint, kind)
	}
	if v := strings.TrimSpace(os.Getenv("SELF_MIND_" + strings.ToUpper(kind))); v != "" {
		if m, ok := mindByName(v); ok {
			return m
		}
		return mindRef{exe: v}
	}
	return mindRef{exe: mindExe()}
}

// escalateFrom returns the next bound mind after m in SELF_MIND_ESCALATION.
// Only a roster-named mind escalates — the unnamed default keeps its
// fail-fast behavior — and an unbound name in the chain is skipped over.
func escalateFrom(m mindRef) (mindRef, bool) {
	if m.name == "" {
		return mindRef{}, false
	}
	chain := strings.Fields(os.Getenv("SELF_MIND_ESCALATION"))
	for i, n := range chain {
		if n != m.name {
			continue
		}
		for _, next := range chain[i+1:] {
			if nm, ok := mindByName(next); ok {
				return nm, true
			}
		}
		break
	}
	return mindRef{}, false
}

func mindEnv(home, kind string) []string {
	return append(os.Environ(), "SELF_HOME="+home, "SELF_ASK="+kind)
}

// pipeMind is the ONE seam through which the kernel asks for intelligence —
// think, reflect, learn, and compile all pass here. It spawns $SELF_MIND with
// the ask's kind in $SELF_ASK, the prompt as the last argument, and a freshly
// written orientation brief from SELF_HOME/site/brief.md on stdin: where the
// mind is, what capabilities exist, and where to look for the rest — nothing
// more. The brief is on disk, like every other piece of state, so a tool-using
// mind can read it itself (cat SELF_HOME/site/brief.md) and the kernel has no
// internal state a mind cannot see. The brief is recomputed before every ask
// so a mind never reads stale orientation — the kernel writes the file, then
// reads back exactly what it wrote, and feeds that. A real mind MUST inspect
// SELF_HOME itself (site/*.html, events.jsonl, capabilities/) with its own
// tools: the brief is a wake-up card, not a context dump, and a process without
// that exploration ability is not a complete mind. The kernel's seam is still
// a pipe; the tool loop is the mind's concern, never the kernel's. The parse
// of the mind's stdout is tolerant on purpose: JSON lines with a name are
// events — script.authored answers a compile, chat.message carries the reply,
// anything else is a returned event — and bare prose joins the reply. With no
// mind plugged in, the ask fails with a hint.
func pipeMind(home, kind, prompt string) (*mindResult, error) {
	return pipeMindVia(home, kind, resolveMind(kind, ""), prompt)
}

// pipeMindVia is pipeMind with the mind already resolved — the compile
// escalation loop steps through minds without re-resolving, and the result
// remembers which mind ran so callers can log the route.
func pipeMindVia(home, kind string, m mindRef, prompt string) (*mindResult, error) {
	exe := m.exe
	if exe == "" {
		return nil, fmt.Errorf("no mind is plugged in — %s", mindHint)
	}
	brief := freshBrief(home)
	bin, argv := mindCommand(exe, prompt)
	cmd := exec.Command(bin, argv...)
	cmd.Env = mindEnv(home, kind)
	cmd.Dir = home
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mind %s: %w (%s)", filepath.Base(bin), err, mindHint)
	}
	feedText(stdin, brief)
	res := &mindResult{Events: []map[string]any{}, Mind: m}
	var prose []string
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		content, fence := unfence(line)
		if fence { // a bare ``` / ```json marker: pure decoration, not reply text
			continue
		}
		var e struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal([]byte(content), &e) != nil || e.Name == "" {
			prose = append(prose, line)
			continue
		}
		switch e.Name {
		case "chat.message":
			var p struct{ Role, Content string }
			if json.Unmarshal(e.Payload, &p) == nil && p.Role == "assistant" {
				prose = append(prose, p.Content)
			}
		case "script.authored":
			var p struct{ Script string }
			if json.Unmarshal(e.Payload, &p) == nil {
				res.Script = p.Script
			}
		default:
			res.Events = append(res.Events, map[string]any{"name": e.Name, "payload": json.RawMessage(e.Payload)})
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("mind %s exited: %w", filepath.Base(bin), err)
	}
	res.Response = strings.Join(prose, "\n")
	return res, nil
}

const mindHint = `plug a mind: SELF_MIND=<a tool-capable executable, e.g. "claude -p" or examples/mind-opencode>; the mind must inspect SELF_HOME itself. See examples/README.md. For offline demos/tests, examples/mind-stub is a deterministic no-LLM mind.`

// mindCommand splits a configured executable into command and args, appending
// the prompt as the last argument.
func mindCommand(exe, prompt string) (string, []string) {
	parts := strings.Fields(exe)
	return parts[0], append(parts[1:], prompt)
}

// unfence strips the Markdown a chat-shaped mind (claude -p and its kin) wraps
// JSON in, so a model that answers in prose still plugs into the pipe unchanged.
// A line that is a bare fence marker (``` or ```json) is decoration, reported by
// the second return so the caller drops it from the reply text; a single line
// wrapped in backticks (`{…}`) is unwrapped to its content. Anything else — plain
// JSON from the stub or an adapter, or ordinary prose — passes through untouched,
// so no existing mind regresses.
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
