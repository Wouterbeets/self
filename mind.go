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
	if id := strings.TrimSpace(os.Getenv("SELF_MIND_ID")); id != "" {
		return id
	}
	if exe := mindExe(); exe != "" {
		return exe
	}
	return "mind"
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
// wasted and denied: the kernel reads ONLY this process's stdout and acts on
// what it finds there. How an adapter produces that stdout (tools, HTTP, MCP)
// is adapter-local; the receive contract is not. Woven into learn/reflect and
// as the shared core of think (think adds a report-only rider).
const mindAnswerContract = `HOW TO ANSWER — when the kernel spawned you, it reads ONLY this process's stdout after you exit. Event lines are one compact JSON object each ({"name":"…","payload":{…}}); every other line is reply prose. No Markdown, no code fences, no backticks around JSON. You do not and cannot write the log yourself: do not edit events.jsonl, do not install under capabilities/, and do not run the self CLI to "finish" this ask — that work is wasted. To persist ordinary state when this ask expects it, print the domain event on stdout. To add capabilities, print command.declared / projector.declared the same way. Declare only what is missing; ordinary domain events are not new capabilities.

THIS REPLY IS FINAL — you run once per ask and are never re-invoked. Explore first, THEN answer completely: never end on a plan or a promise ("I'll explore and then respond") — whatever you have not said when you exit was never said.

WHAT YOU ARE GIVEN — your stdin is an orientation brief (also at SELF_HOME/site/brief.md): where you are, how you act, what commands and projections exist, where depth lives. That is enough to act. Explore the instance with your tools as needed: site/*.html, events.jsonl, capabilities/. site/kernel.html is optional depth (full index and the compiled-capability pipe). The kernel holds no internal state you cannot see on disk. A mind without tools to read those files cannot do this job.`

// mindThinkContract is the think-specific rider: report-only, nothing appended.
const mindThinkContract = `THIS ASK IS REPORT-ONLY (think) — the kernel returns your stdout to the caller and does not append events from it. Answer in prose. Do not emit domain events or declarations to "save" anything; nothing you print is ingested into the log.`

// ────────────────────────────── the compiler ────────────────────────────────

func (c *llm) compileCommand(d commandDecl) (string, error) {
	return compileViaMind(c.home, c.intent, c.reasoning, "command", d.Name, jsonRepr(d))
}

func (c *llm) compileProjector(d projectorDecl) (string, error) {
	return compileViaMind(c.home, c.intent, c.reasoning, "projector", d.Name, jsonRepr(d))
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

func compileViaMind(home, intent, reasoning, typ, name, decl string) (string, error) {
	exName, exScript := exemplarScript(home, typ, name)
	res, err := pipeMind(home, "compile", compilePrompt(intent, reasoning, exName, exScript, typ, name, decl))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(res.Script) == "" {
		return "", fmt.Errorf("the mind answered a compile ask without a script.authored event")
	}
	return res.Script, nil
}

// ──────────────────────────────── the mind ─────────────────────────────────

// mindExe is the plugged mind, if any: SELF_MIND is the one way a mind is
// named, and a process behind it honors the one mind contract.
func mindExe() string {
	return strings.TrimSpace(os.Getenv("SELF_MIND"))
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
	exe := mindExe()
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
	res := &mindResult{Events: []map[string]any{}}
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
