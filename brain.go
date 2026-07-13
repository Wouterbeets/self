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
// reflect, learn, and each compile — is handed to a brain PROCESS
// (SELF_BRAIN, e.g. "claude -p"; examples/brain-stub is a deterministic
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

// identity names the brain for provenance: who authored the bytes a receipt
// carries. SELF_BRAIN_ID lets an agent name itself; otherwise the brain
// executable is the honest mechanical answer.
func (c *llm) identity() string {
	if id := strings.TrimSpace(os.Getenv("SELF_BRAIN_ID")); id != "" {
		return id
	}
	if exe := brainExe(); exe != "" {
		return exe
	}
	return "brain"
}

func newLLM(home string) *llm {
	return &llm{home: home}
}

// ─────────────────────────────── the prompts ────────────────────────────────

const pipeContract = `command script: receives args as argv, current events as JSONL on stdin, writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at.
projector script: receives the events matching its declared consumes list as JSONL on stdin (an empty list or "*" means every event — declare consumes precisely and the script never needs to filter), writes bare semantic HTML on stdout. Do not emit CSS, JavaScript, inline styles, or external assets: the kernel injects the shared shell at serve time. The kernel persists projector output to SELF_HOME/site/<name>.html.
The kernel sets SELF_HOME on every script. Any language with a shebang works; use only standard libraries.`

// brainAnswerContract tells a capable, tool-using brain how to hand its answer
// back. A coding-agent brain (claude -p) will otherwise try to persist its work
// itself — write events.jsonl, run `self`, install a script — and that effort is
// wasted and denied: the kernel reads ONLY stdout and appends what it finds
// there, under its own signature. It also emits Markdown by habit; the pipe
// tolerates fences, but one clean JSON object per line is what actually wants
// ingesting. Woven into every ask that expects events (learn, reflect, compile).
const brainAnswerContract = `HOW TO ANSWER — the kernel reads ONLY your stdout. Event lines are JSON objects; prose lines are reply text. You do not and cannot write the log yourself: do not edit events.jsonl, run the self CLI, or install anything with your tools — that work is wasted. To persist ordinary state, print the domain event that records it. To add capabilities, print command.declared / projector.declared events as ONE line of compact JSON each (no Markdown, no code fences, no backticks). Declare only what is missing; ordinary domain events are not capability growth.

THIS REPLY IS FINAL — you run once per ask and are never re-invoked. Explore first, THEN answer completely: never end on a plan or a promise ("I'll explore and then respond") — whatever you have not said when you exit was never said.

WHAT YOU ARE GIVEN — your stdin is an orientation brief: where you are, what capabilities exist, where to look for the rest. That is all. To do your job you must EXPLORE the instance surface with your tools: read ` + "`SELF_HOME/site/kernel.html`" + ` for the full self-description, ` + "`SELF_HOME/site/*.html`" + ` for the rendered state a human sees, ` + "`SELF_HOME/events.jsonl`" + ` for the raw log, ` + "`SELF_HOME/capabilities/`" + ` for the compiled scripts. The kernel holds no internal state you cannot see on disk. A brain without tools to read those files cannot do this job.`

// ────────────────────────────── the compiler ────────────────────────────────

func (c *llm) compileCommand(d commandDecl) (string, error) {
	return compileViaBrain(c.home, c.intent, c.reasoning, "command", d.Name, jsonRepr(d))
}

func (c *llm) compileProjector(d projectorDecl) (string, error) {
	return compileViaBrain(c.home, c.intent, c.reasoning, "projector", d.Name, jsonRepr(d))
}

// compileViaBrain hands a compile ask to the plugged brain through the same
// seam as every other ask. The declaration (its optional implementation
// reference included) rides in the prompt; an orientation brief rides on stdin
// (the brain inspects SELF_HOME itself for depth — site/*.html, events.jsonl,
// capabilities/); the answer is one script.authored event. During a learn the
// whole intent rides along too, so each piece is compiled toward the same
// product. The kernel still installs and signs — a brain authors bytes, only
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
// traced compiles show a brain spends its first minute rediscovering the
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

func compileViaBrain(home, intent, reasoning, typ, name, decl string) (string, error) {
	exName, exScript := exemplarScript(home, typ, name)
	res, err := pipeBrain(home, "compile", compilePrompt(intent, reasoning, exName, exScript, typ, name, decl))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(res.Script) == "" {
		return "", fmt.Errorf("the brain answered a compile ask without a script.authored event")
	}
	return res.Script, nil
}

// ──────────────────────────────── the brain ─────────────────────────────────

// brainExe is the plugged brain, if any: SELF_BRAIN is the one way a brain is
// named, and a process behind it honors the one brain contract.
func brainExe() string {
	return strings.TrimSpace(os.Getenv("SELF_BRAIN"))
}

func brainEnv(home, kind string) []string {
	return append(os.Environ(), "SELF_HOME="+home, "SELF_ASK="+kind)
}

// pipeBrain is the ONE seam through which the kernel asks for intelligence —
// think, reflect, learn, and compile all pass here. It spawns $SELF_BRAIN with
// the ask's kind in $SELF_ASK, the prompt as the last argument, and a freshly
// written orientation brief from SELF_HOME/site/brief.md on stdin: where the
// brain is, what capabilities exist, and where to look for the rest — nothing
// more. The brief is on disk, like every other piece of state, so a tool-using
// brain can read it itself (cat SELF_HOME/site/brief.md) and the kernel has no
// internal state a brain cannot see. The brief is recomputed before every ask
// so a brain never reads stale orientation — the kernel writes the file, then
// reads back exactly what it wrote, and feeds that. A real brain MUST inspect
// SELF_HOME itself (site/*.html, events.jsonl, capabilities/) with its own
// tools: the brief is a wake-up card, not a context dump, and a process without
// that exploration ability is not a complete brain. The kernel's seam is still
// a pipe; the tool loop is the brain's concern, never the kernel's. The parse
// of the brain's stdout is tolerant on purpose: JSON lines with a name are
// events — script.authored answers a compile, chat.message carries the reply,
// anything else is a returned event — and bare prose joins the reply. With no
// brain plugged in, the ask fails with a hint.
func pipeBrain(home, kind, prompt string) (*brainResult, error) {
	exe := brainExe()
	if exe == "" {
		return nil, fmt.Errorf("no brain is plugged in — %s", brainHint)
	}
	brief := freshBrief(home)
	bin, argv := brainCommand(exe, prompt)
	cmd := exec.Command(bin, argv...)
	cmd.Env = brainEnv(home, kind)
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
		return nil, fmt.Errorf("start brain %s: %w (%s)", filepath.Base(bin), err, brainHint)
	}
	feedText(stdin, brief)
	res := &brainResult{Events: []map[string]any{}}
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
		return nil, fmt.Errorf("brain %s exited: %w", filepath.Base(bin), err)
	}
	res.Response = strings.Join(prose, "\n")
	return res, nil
}

const brainHint = `plug a brain: SELF_BRAIN=<a tool-capable executable, e.g. "claude -p" or examples/brain-opencode>; the brain must inspect SELF_HOME itself. See examples/README.md. For offline demos/tests, examples/brain-stub is a deterministic no-LLM brain.`

// brainCommand splits a configured executable into command and args, appending
// the prompt as the last argument.
func brainCommand(exe, prompt string) (string, []string) {
	parts := strings.Fields(exe)
	return parts[0], append(parts[1:], prompt)
}

// unfence strips the Markdown a chat-shaped brain (claude -p and its kin) wraps
// JSON in, so a model that answers in prose still plugs into the pipe unchanged.
// A line that is a bare fence marker (``` or ```json) is decoration, reported by
// the second return so the caller drops it from the reply text; a single line
// wrapped in backticks (`{…}`) is unwrapped to its content. Anything else — plain
// JSON from the stub or an adapter, or ordinary prose — passes through untouched,
// so no existing brain regresses.
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
