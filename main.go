// self — a local-first, event-sourced runtime with LLM-generated capabilities.
//
// One append-only event log (events.jsonl) is the only truth. Every view is a
// pure replay of it, rendered as HTML that you and your agent read identically.
// Capabilities are standalone scripts the kernel pipes events through, and code
// is never shipped — an LLM compiler authors every script from a declaration,
// for this receiver. A running capability can declare new capabilities and the
// kernel compiles them on the spot (the strange loop). Every compile is logged
// as a script.compiled receipt signed with a per-home secret; only kernel-signed
// receipts ever install, so `self rehydrate` rebuilds the whole instance from
// the log alone — an instance is just events.jsonl + .secret.
//
// This file is the whole kernel.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ───────────────────────────── events & the log ─────────────────────────────

type Event struct {
	ID         string          `json:"id"`
	Seq        int             `json:"seq"`
	Name       string          `json:"name"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

func newEvent(name string, payload json.RawMessage) Event {
	b := make([]byte, 16)
	rand.Read(b)
	return Event{ID: hex.EncodeToString(b), Name: name, OccurredAt: time.Now().UTC(), Payload: payload}
}

func logPath(home string) string { return filepath.Join(home, "events.jsonl") }

func readEvents(home string) ([]Event, error) {
	f, err := os.Open(logPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var events []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse event: %w", err)
		}
		events = append(events, e)
	}
	return events, sc.Err()
}

func appendEvent(home string, e *Event) error {
	prior, err := readEvents(home)
	if err != nil {
		return err
	}
	e.Seq = 1
	if len(prior) > 0 {
		e.Seq = prior[len(prior)-1].Seq + 1
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(logPath(home), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, string(line))
	return err
}

// ─────────────────────── provenance: the signed install ─────────────────────
//
// The loop carries specs, never code: anything may append a script.compiled to
// the log, but only a receipt signed with this home's secret ever installs —
// provenance is intrinsic to the receipt, not enforced by who may write it. A
// forged receipt is inert. The secret lives in SELF_HOME/.secret (0600, never
// in the log), like an ssh host key: per-home, so you can inherit another
// node's declarations but never its binaries.

func loadSecret(home string) ([]byte, error) {
	p := filepath.Join(home, ".secret")
	if data, err := os.ReadFile(p); err == nil {
		if key, err := hex.DecodeString(strings.TrimSpace(string(data))); err == nil && len(key) > 0 {
			return key, nil
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, []byte(hex.EncodeToString(key)), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

type receipt struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Script string `json:"script"`
	// By is the provenance of the bytes: which brain authored this compile —
	// a model at an endpoint, a stub, a named agent. The signature covers it,
	// so authorship can no more be forged or relabeled than the script itself.
	// Receipts minted before provenance existed have no By and verify by the
	// legacy formula.
	By  string `json:"by,omitempty"`
	Sig string `json:"sig"`
}

// sign binds the receipt's fields so none can be relabeled — one capability's
// bytes can't install under another's name, and authorship can't be moved.
// The v2 formula is domain-separated and length-prefixed (no concatenation of
// adjacent fields can collide); the legacy formula survives so instances minted
// before provenance still rehydrate. Type names are kernel-vocabulary
// ("command"/"projector"), so a legacy mac can never alias a v2 one.
func sign(secret []byte, typ, name, script, by string) string {
	m := hmac.New(sha256.New, secret)
	if by == "" { // legacy: receipts from before authorship was recorded
		m.Write([]byte(typ))
		m.Write([]byte{0})
		m.Write([]byte(name))
		m.Write([]byte{0})
		m.Write([]byte(script))
	} else {
		m.Write([]byte("self.receipt.v2\x00"))
		for _, field := range []string{typ, name, script, by} {
			fmt.Fprintf(m, "%d:", len(field))
			m.Write([]byte(field))
		}
	}
	return hex.EncodeToString(m.Sum(nil))
}

func appendReceipt(home, typ, name, script, by string) error {
	secret, err := loadSecret(home)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(receipt{typ, name, script, by, sign(secret, typ, name, script, by)})
	e := newEvent("script.compiled", payload)
	return appendEvent(home, &e)
}

func verifiedReceipt(secret []byte, payload json.RawMessage) (receipt, bool) {
	var r receipt
	if json.Unmarshal(payload, &r) != nil || r.Sig == "" || r.Script == "" || r.Name == "" {
		return r, false
	}
	return r, hmac.Equal([]byte(sign(secret, r.Type, r.Name, r.Script, r.By)), []byte(r.Sig))
}

func scriptPath(home, typ, name string) (string, error) {
	switch typ {
	case "command":
		return filepath.Join(home, "capabilities", "commands", name), nil
	case "projector":
		return filepath.Join(home, "capabilities", "projectors", name), nil
	}
	return "", fmt.Errorf("unknown capability type %q", typ)
}

func installScript(home, typ, name, script string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("unsafe capability name %q", name)
	}
	p, err := scriptPath(home, typ, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(script), 0755)
}

// rehydrate rebuilds the instance from the log alone: the latest kernel-signed
// script.compiled receipt per capability installs verbatim, then every
// projection re-renders. No LLM, no network — a home is events.jsonl + .secret.
func rehydrate(home string) error {
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	secret, err := loadSecret(home)
	if err != nil {
		return err
	}
	// Keyed by (type, name): a command and a projector may share a name, and
	// the latest receipt of each must install — not the latest of either.
	latest := map[string]receipt{}
	var order []string
	for _, e := range events {
		if e.Name != "script.compiled" {
			continue
		}
		r, ok := verifiedReceipt(secret, e.Payload)
		if !ok {
			continue
		}
		key := r.Type + "/" + r.Name
		if _, seen := latest[key]; !seen {
			order = append(order, key)
		}
		latest[key] = r
	}
	for _, key := range order {
		r := latest[key]
		if err := installScript(home, r.Type, r.Name, r.Script); err != nil {
			return err
		}
	}
	renderKernelHTML(home)
	refreshProjections(home)
	fmt.Fprintf(os.Stderr, "self: rehydrated %d capabilit(ies) from the log\n", len(order))
	return nil
}

// ───────────────────────────── the pipe contract ────────────────────────────
//
// Compiled scripts are standalone executables in any language. A command reads
// args as argv and the current events as JSONL on stdin, and writes new events
// as JSONL on stdout ({name, payload} per line; the kernel assigns the rest). A
// projector reads all events on stdin and writes HTML on stdout; the kernel
// persists it to site/<name>.html. The kernel sets SELF_HOME on every script.

func feedEvents(stdin io.WriteCloser, events []Event) {
	go func() {
		enc := json.NewEncoder(stdin)
		for i := range events {
			enc.Encode(events[i])
		}
		stdin.Close()
	}()
}

// pipeProcess runs an executable as a Unix pipeline node — the one shape the
// kernel uses to talk to any outside process, a compiled command or the brain.
func pipeProcess(home, bin string, argv []string) ([]Event, error) {
	current, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, argv...)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home)
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
		return nil, fmt.Errorf("start %s: %w", filepath.Base(bin), err)
	}
	feedEvents(stdin, current)

	var out []Event
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var partial struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &partial); err != nil {
			return nil, fmt.Errorf("%s output parse error: %w", filepath.Base(bin), err)
		}
		if partial.Name == "" {
			return nil, fmt.Errorf("%s output missing event name: %s", filepath.Base(bin), line)
		}
		out = append(out, newEvent(partial.Name, partial.Payload))
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("%s exited: %w", filepath.Base(bin), err)
	}
	return out, nil
}

func runCommand(home, command string, args []string) ([]Event, error) {
	bin, _ := scriptPath(home, "command", command)
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("command %q not found (grow a seed that declares it)", command)
	}
	evs, err := pipeProcess(home, bin, args)
	if err != nil {
		return nil, err
	}
	return evs, ingest(home, evs)
}

// ingest appends the events a process emitted, compiles any declarations among
// them (the strange loop), and re-renders every projection. Projections are
// pure replays, so re-running them all is always correct.
func ingest(home string, evs []Event) error {
	for i := range evs {
		if err := appendEvent(home, &evs[i]); err != nil {
			return err
		}
	}
	c := newLLM(home)
	defer c.close()
	if n := compileDeclarations(c, home, evs); n > 0 {
		fmt.Fprintf(os.Stderr, "self: self-improved — %d capabilit(ies) compiled\n", n)
	}
	renderKernelHTML(home)
	refreshProjections(home)
	return nil
}

type commandDecl struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Params      map[string]string `json:"params"`
	Event       struct {
		Name   string            `json:"name"`
		Fields map[string]string `json:"fields"`
	} `json:"event"`
	// Implementation is an optional reference the compiler verifies and adapts —
	// never installed as-is, so precision from the seed author and receiver
	// adaptation both survive.
	Implementation string `json:"implementation,omitempty"`
}

type projectorDecl struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Consumes       []string `json:"consumes"`
	Implementation string   `json:"implementation,omitempty"`
}

// compileDeclarations is the strange-loop hook: every command.declared /
// projector.declared among evs is compiled by the LLM into a script authored
// for this receiver, installed, and logged as a signed receipt. Declaring IS
// creating — this runs at grow time and at run time alike, so a capability (or
// the brain) grows new capabilities just by emitting declarations.
func compileDeclarations(c *llm, home string, evs []Event) int {
	n := 0
	for _, e := range evs {
		var typ, name, script string
		var err error
		switch e.Name {
		case "command.declared":
			var d commandDecl
			if json.Unmarshal(e.Payload, &d) != nil || d.Name == "" {
				continue
			}
			typ, name = "command", d.Name
			fmt.Fprintf(os.Stderr, "self: compiling command %q…\n", name)
			script, err = c.compileCommand(d)
		case "projector.declared":
			var d projectorDecl
			if json.Unmarshal(e.Payload, &d) != nil || d.Name == "" {
				continue
			}
			typ, name = "projector", d.Name
			fmt.Fprintf(os.Stderr, "self: compiling projector %q…\n", name)
			script, err = c.compileProjector(d)
		default:
			continue
		}
		if err == nil {
			err = installScript(home, typ, name, script)
		}
		if err == nil {
			err = appendReceipt(home, typ, name, script, c.identity())
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "self: %s %q failed: %s\n", typ, name, err)
			continue
		}
		n++
	}
	return n
}

// ─────────────────────────────── projections ────────────────────────────────

// runProjection replays the whole log through a projector script and returns
// the HTML it emits. Run it twice, get the same page — a pure function of the log.
func runProjection(home, name string) ([]byte, error) {
	bin, _ := scriptPath(home, "projector", name)
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("projection %q not found", name)
	}
	events, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home)
	cmd.Dir = home
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	feedEvents(stdin, events)
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("projection %q exited: %w", name, err)
	}
	return out.Bytes(), nil
}

func projectToSite(home, name string) error {
	page, err := runProjection(home, name)
	if err != nil {
		return err
	}
	siteDir := filepath.Join(home, "site")
	if err := os.MkdirAll(siteDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(siteDir, name+".html"), page, 0644)
}

func refreshProjections(home string) {
	entries, err := os.ReadDir(filepath.Join(home, "capabilities", "projectors"))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := projectToSite(home, e.Name()); err != nil {
			fmt.Fprintf(os.Stderr, "self: projection %q failed: %s\n", e.Name(), err)
		}
	}
}

// ──────────────────────────────── the LLM ───────────────────────────────────
//
// One OpenAI-compatible endpoint plays two roles: the COMPILER (declaration in,
// script out) and the default BRAIN (`self brain`). Both explore the instance
// through a single bash tool — a jailed full-bash playpen where the platform
// allows, a fail-closed read-only allowlist otherwise — before writing
// anything: same seed, different instance, different binary.

type llm struct {
	url, key, model string
	stub            bool
	home            string
	// intent is the whole-seed genotype, woven into every compile during a grow
	// so no piece is compiled in a dark room.
	intent string
	// pen is the jailed full-bash playpen, seeded lazily on the first bash
	// call and shared across this client's conversations so state persists.
	pen     *playpen
	penOnce sync.Once
}

// close releases the playpen, if one was ever seeded.
func (c *llm) close() { c.pen.close() }

// identity names the brain for provenance: who authored the bytes a receipt
// carries. SELF_BRAIN_ID lets an agent name itself; otherwise the model at
// its endpoint is the honest mechanical answer, and a stub says so.
func (c *llm) identity() string {
	if id := strings.TrimSpace(os.Getenv("SELF_BRAIN_ID")); id != "" {
		return id
	}
	if c.stub {
		return "stub (no LLM)"
	}
	if exe := brainExe(); exe != "" {
		return exe
	}
	return c.model + " @ " + c.url
}

func newLLM(home string) *llm {
	if os.Getenv("SELF_LLM_STUB") == "1" {
		return &llm{stub: true, home: home}
	}
	return &llm{
		url:   envOr("SELF_LLM_URL", "http://127.0.0.1:8080"),
		key:   os.Getenv("SELF_LLM_API_KEY"),
		model: envOr("SELF_LLM_MODEL", "local"),
		home:  home,
	}
}

func (c *llm) available() bool {
	return !c.stub && (c.key != "" ||
		strings.HasPrefix(c.url, "http://127.0.0.1") || strings.HasPrefix(c.url, "http://localhost"))
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type assistantMsg struct {
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls"`
}

var llmClient = &http.Client{Timeout: 10 * time.Minute}

func (c *llm) doRound(messages, tools []map[string]any) (*assistantMsg, error) {
	body, _ := json.Marshal(map[string]any{
		"model": c.model, "messages": messages, "temperature": 0.2, "tools": tools,
	})
	req, err := http.NewRequest("POST", strings.TrimRight(c.url, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := llmClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm call failed: %w (check SELF_LLM_URL)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("llm returned %d: %s", resp.StatusCode, b)
	}
	var result struct {
		Choices []struct {
			Message assistantMsg `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}
	return &result.Choices[0].Message, nil
}

// toolLoop drives one conversation until the model answers with plain text or a
// handler ends it (done=true, out as the final result).
func (c *llm) toolLoop(system string, turns, tools []map[string]any, handle func(name, args string) (out string, done bool)) (string, error) {
	if !c.available() {
		return "", fmt.Errorf("no brain is plugged in — %s", brainHint)
	}
	messages := append([]map[string]any{{"role": "system", "content": system}}, turns...)
	calls := 0
	for round := 0; round < 40; round++ {
		msg, err := c.doRound(messages, tools)
		if err != nil {
			return "", err
		}
		if len(msg.ToolCalls) == 0 {
			return msg.Content, nil
		}
		messages = append(messages, map[string]any{"role": "assistant", "content": msg.Content, "tool_calls": msg.ToolCalls})
		for _, tc := range msg.ToolCalls {
			if calls++; calls > 60 {
				return "", fmt.Errorf("stopped after %d tool calls without a final response", calls-1)
			}
			out, done := handle(tc.Function.Name, tc.Function.Arguments)
			if done {
				return out, nil
			}
			messages = append(messages, map[string]any{"role": "tool", "tool_call_id": tc.ID, "content": out})
		}
	}
	return "", fmt.Errorf("exceeded 40 tool rounds without a final response")
}

func tool(name, desc string, props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name": name, "description": desc,
		"parameters": map[string]any{"type": "object", "properties": props, "required": required},
	}}
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var (
	bashTool    = tool("bash", "Run a bash command in the PLAYPEN: an ephemeral jailed copy of the instance (events.jsonl, capabilities/, site/ at /body — never the signing key). Full bash, real execution: write files, run interpreters, pipe the copied log through a candidate script and read what comes out. Writes stay in the jail, there is no network, and state persists across calls in this conversation. Nothing here changes the real instance — to change it, emit declarations. Where jailing is unsupported this becomes a fail-closed READ-ONLY allowlist (ls, cat, head, tail, grep, find, wc, sort, uniq, cut, tr, echo, jq; no redirection) — a refused write tells you which mode you are in.", map[string]any{"command": str("the shell command")}, "command")
	declareTool = tool("declare", `Declare ONE new capability; the kernel compiles it into a live script. Call once per capability. A command: {"name":"command.declared","payload":{"name","description","params":{k:type},"event":{"name","fields":{k:type}}}}. A projector: {"name":"projector.declared","payload":{"name","description","consumes":["event.name"]}}.`, map[string]any{"name": str("command.declared or projector.declared"), "payload": map[string]any{"type": "object", "description": "the declaration"}}, "name", "payload")
	doneTool    = tool("done", "Finish, with a short summary for the user.", map[string]any{"summary": str("one or two sentences")}, "summary")
	runTool     = tool("run", "Run one of the capabilities listed under CAPABILITIES; the kernel appends the events it emits.", map[string]any{"name": str("the capability"), "args": str("space-separated args, in declared order")}, "name")
	submitTool  = tool("submit", "Submit the finished script (full source, with shebang).", map[string]any{"script": str("the executable script")}, "script")
)

var readOnlyCmds = map[string]bool{"ls": true, "cat": true, "head": true, "tail": true, "grep": true,
	"find": true, "wc": true, "sort": true, "uniq": true, "cut": true, "tr": true, "echo": true, "jq": true}

// readOnlyBash is the exploration tool: fail-closed to plain readers, so the
// model can inspect the instance but not modify it. Like the jail, it runs
// against a sanitized COPY of the instance (events.jsonl, capabilities/, site/
// — never .secret), so the signing key is simply not present to read. The
// fallback must not be a weaker door than the jail it stands in for.
func readOnlyBash(home, command string) string {
	if strings.ContainsAny(command, "><`") || strings.Contains(command, "$(") ||
		strings.Contains(command, "-exec") || strings.Contains(command, "-delete") || strings.Contains(command, "-ok") {
		return "error: read-only bash — redirection, substitution, and find actions are not allowed"
	}
	for _, seg := range strings.FieldsFunc(command, func(r rune) bool { return r == '|' || r == ';' || r == '&' || r == '\n' }) {
		f := strings.Fields(seg)
		if len(f) > 0 && !readOnlyCmds[f[0]] {
			return fmt.Sprintf("error: %q is not on the read-only allowlist", f[0])
		}
	}
	work, err := os.MkdirTemp("", "self-ro-")
	if err != nil {
		return "error: " + err.Error()
	}
	defer os.RemoveAll(work)
	if data, err := os.ReadFile(filepath.Join(home, "events.jsonl")); err == nil {
		os.WriteFile(filepath.Join(work, "events.jsonl"), data, 0644)
	}
	for _, dir := range []string{"capabilities", "site"} {
		copyTree(filepath.Join(home, dir), filepath.Join(work, dir))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "SELF_HOME="+work)
	out, err := cmd.CombinedOutput()
	if len(out) > 16384 {
		out = append(out[:16384], "\n… (truncated)"...)
	}
	if err != nil {
		return fmt.Sprintf("%serror: %s", out, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return "(no output)"
	}
	return string(out)
}

func (c *llm) handleBash(args string) string {
	var a struct {
		Command string `json:"command"`
	}
	json.Unmarshal([]byte(args), &a)
	c.penOnce.Do(func() { c.pen = newPlaypen(c.home) })
	if c.pen != nil {
		return c.pen.run(a.Command)
	}
	return readOnlyBash(c.home, a.Command)
}

// ─────────────────────────────── the playpen ────────────────────────────────
//
// Full bash for the brain, contained. A read-only allowlist lets a model look
// but never execute, and scripts authored without execution go untested. The
// playpen enables testing while keeping the trust model: each brain/compiler
// conversation gets an EPHEMERAL COPY of the instance (events.jsonl,
// capabilities/, site/ at /body — never .secret) inside a Linux user-namespace
// jail. Writes cannot leave the jail (pivot_root onto a throwaway tree; system
// dirs bound read-only), the network namespace has no interfaces, and the
// signing key never enters. Nothing done inside installs anything:
// declarations remain the only ingress to the real instance, and only
// the kernel signs — the jail can propose and test, never author the record.
// If namespaces are unavailable, or SELF_SANDBOX=0, the kernel falls back to
// the read-only allowlist. It never fails open.

type playpen struct{ root string }

var (
	jailProbe sync.Once
	jailOK    bool
)

// jailWorks probes once per process whether this platform can jail: some
// kernels and container runtimes forbid unprivileged user namespaces, and the
// answer must come from an experiment, not a guess.
func jailWorks() bool {
	jailProbe.Do(func() {
		root, err := os.MkdirTemp("", "self-playpen-probe-")
		if err != nil {
			return
		}
		defer os.RemoveAll(root)
		os.MkdirAll(filepath.Join(root, "body"), 0755)
		p := &playpen{root: root}
		_, err = p.exec("true", 15*time.Second)
		jailOK = err == nil
	})
	return jailOK
}

// newPlaypen seeds a jail with a copy of the instance, minus the one file that
// must never enter it. Returns nil when jailing is off or unsupported here —
// the caller then falls back to the fail-closed read-only allowlist.
func newPlaypen(home string) *playpen {
	if os.Getenv("SELF_SANDBOX") == "0" || !jailWorks() {
		return nil
	}
	root, err := os.MkdirTemp("", "self-playpen-")
	if err != nil {
		return nil
	}
	body := filepath.Join(root, "body")
	if err := os.MkdirAll(body, 0755); err != nil {
		os.RemoveAll(root)
		return nil
	}
	if data, err := os.ReadFile(filepath.Join(home, "events.jsonl")); err == nil {
		os.WriteFile(filepath.Join(body, "events.jsonl"), data, 0644)
	}
	for _, dir := range []string{"capabilities", "site"} {
		copyTree(filepath.Join(home, dir), filepath.Join(body, dir))
	}
	return &playpen{root: root}
}

func (p *playpen) run(command string) string {
	out, err := p.exec(command, 120*time.Second)
	if len(out) > 16384 {
		out = append(out[:16384], "\n… (truncated)"...)
	}
	if err != nil {
		return fmt.Sprintf("%serror: %s", out, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return "(no output)"
	}
	return string(out)
}

func (p *playpen) close() {
	if p != nil {
		os.RemoveAll(p.root)
	}
}

// copyTree copies a directory of plain files (the instance's derived state) —
// good enough for capabilities/ and site/, which hold no links or devices.
func copyTree(src, dst string) {
	filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			os.MkdirAll(target, 0755)
			return nil
		}
		if data, err := os.ReadFile(path); err == nil {
			os.WriteFile(target, data, info.Mode().Perm())
		}
		return nil
	})
}

// ─────────────────────────────── the prompts ────────────────────────────────

// kernelPrimer is the mental model every compile/brain prompt opens with — the
// load-bearing protocols, held BEFORE exploration.
const kernelPrimer = `self in one breath — hold this before you explore or write anything:

- One append-only event log is the ONLY state. Every capability is a small script the kernel runs over that log; every view is a pure replay of it, rendered as HTML that humans and agents read identically. There is no hidden memory: to remember something, emit an event; to use memory, read events back. What is not in the log did not happen, and will not survive this session.
- THE STRANGE LOOP — the heart of self. Emitting a command.declared or projector.declared event makes the kernel compile it into a live capability on the spot, at grow time AND at run time. Declaring IS creating: a running capability (or you) grows new capabilities just by emitting those events. Code never arrives pre-built — the kernel compiles every script from its declaration, for this receiver, and logs a receipt signed by this home.
- YOUR WORK IS SIGNED AS YOURS. Every compile receipt carries the authoring agent's name (SELF_BRAIN_ID if set, else the model at its endpoint) inside the signature. You are not an anonymous process: what you grow here is attributed, permanent, and replayable by whoever comes after you.
- INTELLIGENCE is a capability the kernel exposes. A command that needs to think runs 'self think "<prompt>"' (the argument may be a JSON array of {role, content} turns) and gets {response, declarations} back; declarations flow through the strange loop. The brain is whoever answers — a local model, an API, an agent, a human at a bridge. The kernel cannot tell, and does not care.
- VERIFY BY EXECUTION. Your bash tool is usually the playpen: a jailed copy of the instance at /body (full bash, no network, never the signing key). Run the thing before you claim the thing — write a candidate, pipe the copied events.jsonl through it, read what comes out. Nothing done there touches the real instance; only declarations cross back. A tested script beats an inspected one.

With that model in hand, explore THIS instance before building: its projections (site/*.html) are its current state, its event names are its vocabulary, and its capabilities define its conventions — adapt to what exists rather than duplicating it.`

const pipeContract = `command script: receives args as argv, current events as JSONL on stdin, writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at.
projector script: receives all events as JSONL on stdin, writes HTML on stdout. The kernel persists it to SELF_HOME/site/<name>.html.
The kernel sets SELF_HOME on every script. Any language with a shebang works; use only standard libraries.`

const commandSystemPrompt = kernelPrimer + `

You are the self compiler. You read a command declaration and write an executable command script.

` + pipeContract + `

Before writing, explore: the event vocabulary in events.jsonl, the installed capabilities, the rendered site/. If the new command's event overlaps with events already in the log, integrate — align field names, avoid collisions, consider co-producing an existing event name so existing projections pick it up. If the declaration includes a REFERENCE IMPLEMENTATION, verify it against the pipe contract and adapt it to this instance — never submit code you have not verified. When the playpen allows execution, verification means running: write the candidate to a file, pipe the copied events.jsonl through it, and read the events it emits before you submit.

When done exploring, call submit with the full script.`

const projectorSystemPrompt = kernelPrimer + `

You are the self compiler. You read a projector declaration and write an executable projector script.

` + pipeContract + `

Build state by filtering stdin for the consumed event names. Emit BARE semantic HTML — no CSS, no <style>, no inline styles, and no JavaScript (the kernel injects one shared shell — theme and reactivity — at serve time; your page must be complete and legible without it). Use plain elements plus only this class vocabulary where needed: muted, card, row, stack, tag, msg (+ who for the speaker label, plus the speaker's role as a modifier class on conversation turns: "msg user" / "msg assistant"), num, and on buttons secondary / danger. Affordances are plain HTML forms: <form method="post" action="/run/COMMAND"><input name="x"><button>Label</button></form> — each field's value becomes a positional argument in document order, and the kernel redirects back so the page reloads with the new state.

Before writing, explore. If the consumed events overlap with events already in the stream under different names, extend the filter to consume both and map their fields — the seed adapts to the instance, not the other way around. If the declaration includes a REFERENCE IMPLEMENTATION, verify and adapt it — never submit code you have not verified. When the playpen allows execution, verification means running: pipe the copied events.jsonl through your candidate and read the HTML it renders before you submit.

When done exploring, call submit with the full script.`

const brainSystemPrompt = kernelPrimer + `

You are this instance's brain right now — the process the kernel spawned to think with. Commands reach you via 'self think'; heartbeats reach you to reflect and grow. You inhabit this instance for one conversation and are then gone; the log is the only part of you that survives.

You have three powers:
- READ & TRY: the bash tool — a jailed copy of the instance at /body (full bash, no network, no signing key). site/*.html is your memory: read the relevant page before answering. Test anything by real execution before you commit to it; the jail is a scratch copy, so nothing done there changes the real instance.
- ACT: call the run tool with a capability from the CAPABILITIES list to actually do what is asked — don't merely describe it. The log is append-only, so acting is safe: nothing is ever destroyed.
- GROW: when no capability fits, call declare to add one; the kernel compiles it on the spot and signs your name to it. Declining to grow is an honest answer — add only what is genuinely missing.

Say true things. If this instance has verification capabilities (claim/verify and a ledger), use them on your own work: a claim without evidence stays visibly unproven. If past sessions left hand-off notes, read them before acting and record one for the next session. Respond with plain text (or done) for conversational replies.`

const orchestratorSystemPrompt = kernelPrimer + `

You are self's developmental compiler. You are given a product's INTENT — what it is for, its core intuitions, the feel, the anti-goals. Grow it: design the SMALLEST coherent set of capabilities that realizes this intent in THIS instance, and declare each one with the declare tool.

- Decompose into commands (verbs that emit events) and projectors (views over events); let a shared event vocabulary be the seams.
- Write each description richly enough that someone compiling that one piece in isolation would still serve the WHOLE intent — name the sibling capabilities, the shared events, the feel.
- Honor the public surface names the intent fixes; how you realize them is yours to choose for this instance.
- The kernel's contracts win over any conflicting wording in the intent: commands read argv + JSONL stdin and emit JSONL events; projectors read JSONL stdin and emit bare semantic HTML with /run/<command> forms, no JavaScript.
- An intent is a hypothesis about reality. Where it names external systems — their CLIs, paths, schemas — prefer what you can verify by execution in the playpen over what any document, including the intent itself, asserts; and where you cannot verify yet, make the capability degrade honestly and say so in its description.

Explore, declare every capability, then call done with a one-line summary of the decomposition.`

// ────────────────────────────── the compiler ────────────────────────────────

func (c *llm) compileCommand(d commandDecl) (string, error) {
	if c.stub {
		return stubCommand(d), nil
	}
	if brainExe() != "" {
		return compileViaBrain(c.home, "command", d.Name, jsonRepr(d))
	}
	user := fmt.Sprintf("Compile this command declaration into a command script.\n\nCOMMAND: %s\n  description: %s\n  params: %s\n\nEVENT it produces:\n  name: %s\n  fields: %s\n\nIt must produce an event with the declared name, its fields populated from argv.",
		d.Name, d.Description, jsonRepr(d.Params), d.Event.Name, jsonRepr(d.Event.Fields))
	return c.compile(commandSystemPrompt, user, d.Implementation)
}

func (c *llm) compileProjector(d projectorDecl) (string, error) {
	if c.stub {
		return stubProjector(d), nil
	}
	if brainExe() != "" {
		return compileViaBrain(c.home, "projector", d.Name, jsonRepr(d))
	}
	user := fmt.Sprintf("Compile this projector declaration into a projector script.\n\nPROJECTOR: %s\n  description: %s\n  consumes: %s\n\nIt must filter events by the consumed names and render HTML.",
		d.Name, d.Description, jsonRepr(d.Consumes))
	return c.compile(projectorSystemPrompt, user, d.Implementation)
}

// compileViaBrain hands a compile ask to the plugged brain through the same
// seam as every other ask. The declaration (its optional implementation
// reference included) rides in the prompt; the log rides on stdin; the answer
// is one script.authored event. The kernel still installs and signs — a brain
// authors bytes, only the kernel makes them real.
func compileViaBrain(home, typ, name, decl string) (string, error) {
	prompt := fmt.Sprintf(`COMPILE one capability for this instance. Author a complete executable script (any language with a shebang, standard libraries only) honoring the pipe contract, adapted to this instance's log (on your stdin as JSONL).

%s

DECLARATION (%s %q):
%s

If the declaration carries an "implementation", it is a reference from another instance: verify it and adapt it here — never copy it blindly. Verify by execution with your own tools before answering.

Answer with ONE line of JSON and nothing else:
{"name":"script.authored","payload":{"script":"<the full script>"}}`, pipeContract, typ, name, decl)
	res, err := pipeBrain(home, "compile", prompt)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(res.Script) == "" {
		return "", fmt.Errorf("the brain answered a compile ask without a script.authored event")
	}
	return res.Script, nil
}

func (c *llm) compile(system, user, reference string) (string, error) {
	if strings.TrimSpace(c.intent) != "" {
		user = "This capability is one part of a product with the following INTENT. Compile it so the whole intent is served.\n\n--- INTENT ---\n" + c.intent + "\n--- END INTENT ---\n\n" + user
	}
	if strings.TrimSpace(reference) != "" {
		user += "\n\nREFERENCE IMPLEMENTATION (verify against the contract and adapt to this instance — do not copy blindly):\n```\n" + reference + "\n```"
	}
	var script string
	_, err := c.toolLoop(system, []map[string]any{{"role": "user", "content": user}},
		[]map[string]any{bashTool, submitTool},
		func(name, args string) (string, bool) {
			switch name {
			case "bash":
				return c.handleBash(args), false
			case "submit":
				var a struct {
					Script string `json:"script"`
				}
				if json.Unmarshal([]byte(args), &a) != nil || strings.TrimSpace(a.Script) == "" {
					return `error: submit needs {"script": "..."}`, false
				}
				script = a.Script
				return "", true
			}
			return fmt.Sprintf("error: unknown tool %q", name), false
		})
	if err != nil {
		return "", err
	}
	if script == "" {
		return "", fmt.Errorf("the compiler returned no script (it must call submit)")
	}
	return script, nil
}

// Stub scripts (SELF_LLM_STUB=1) keep the whole loop testable offline: no LLM,
// no network, real pipe-contract binaries.
func payloadField(fields map[string]string) string {
	if _, ok := fields["title"]; ok {
		return "title"
	}
	if _, ok := fields["text"]; ok {
		return "text"
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if strings.TrimSpace(k) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		return keys[0]
	}
	return "title"
}

func stubCommand(d commandDecl) string {
	eventName := d.Event.Name
	if strings.TrimSpace(eventName) == "" {
		eventName = d.Name + ".ran"
	}
	field := payloadField(d.Event.Fields)
	return fmt.Sprintf("#!/usr/bin/env python3\n# STUB (no LLM configured) — %s\nimport sys, json\nprint(json.dumps({\"name\": %q, \"payload\": {%q: \" \".join(sys.argv[1:]) or \"(untitled)\"}}))\n",
		d.Description, eventName, field)
}

func stubProjector(d projectorDecl) string {
	return fmt.Sprintf("#!/usr/bin/env python3\n# STUB (no LLM configured) — %s\nimport sys, json\nfrom html import escape\nconsumes = %s\nprint(\"<h1>%s</h1><ul>\")\nfor line in sys.stdin:\n    line = line.strip()\n    if not line:\n        continue\n    e = json.loads(line)\n    if not consumes or e.get(\"name\") in consumes:\n        payload = e.get('payload', {}) or {}\n        value = payload.get('title', payload.get('text'))\n        if value is None and payload:\n            value = payload[sorted(payload)[0]]\n        print(f\"<li>{escape(str(value if value is not None else '(untitled)'))}</li>\")\nprint(\"</ul>\")\n",
		d.Description, jsonRepr(d.Consumes), d.Name)
}

func stubBrain(home, kind, prompt string) (*brainResult, error) {
	res := &brainResult{Declarations: []map[string]any{}}
	switch kind {
	case "think":
		res.Response = "stub thought about: " + prompt
	case "heartbeat":
		res.Response = "stub heartbeat: no changes"
	case "grow":
		name, intent := latestIntent(home)
		if intent == "" {
			intent = prompt
		}
		command := firstCommandName(intent)
		if command == "" {
			command = sanitizeCapabilityName(name)
		}
		if command == "" {
			command = "entry"
		}
		projector := firstProjectionName(intent)
		if projector == "" {
			projector = command + "s"
		}
		event := firstEventName(intent)
		if event == "" {
			event = command + ".recorded"
		}
		res.Declarations = []map[string]any{
			{"name": "command.declared", "payload": map[string]any{"name": command, "description": "offline stub command grown from " + name, "params": map[string]string{"text": "string"}, "event": map[string]any{"name": event, "fields": map[string]string{"text": "string"}}}},
			{"name": "projector.declared", "payload": map[string]any{"name": projector, "description": "offline stub projection grown from " + name, "consumes": []string{event}}},
		}
		res.Response = fmt.Sprintf("stub declared %q and %q", command, projector)
	case "compile":
		return nil, fmt.Errorf("stub compile is handled by the built-in stub compiler, not the brain process")
	default:
		res.Response = "stub received " + kind
	}
	return res, nil
}

func latestIntent(home string) (name, intent string) {
	events, err := readEvents(home)
	if err != nil {
		return "", ""
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Name != "intent.declared" {
			continue
		}
		var p struct {
			Name   string `json:"name"`
			Intent string `json:"intent"`
		}
		if json.Unmarshal(events[i].Payload, &p) == nil {
			return p.Name, p.Intent
		}
	}
	return "", ""
}

func backticked(s string) []string {
	var out []string
	for {
		start := strings.IndexByte(s, '`')
		if start < 0 {
			return out
		}
		s = s[start+1:]
		end := strings.IndexByte(s, '`')
		if end < 0 {
			return out
		}
		out = append(out, s[:end])
		s = s[end+1:]
	}
}

func firstCommandName(intent string) string {
	for _, token := range backticked(intent) {
		if rest, ok := strings.CutPrefix(token, "self run "); ok {
			if fields := strings.Fields(rest); len(fields) > 0 {
				return sanitizeCapabilityName(fields[0])
			}
		}
	}
	return ""
}

func firstProjectionName(intent string) string {
	for _, token := range backticked(intent) {
		if strings.HasPrefix(token, "/") && len(token) > 1 {
			return sanitizeCapabilityName(strings.TrimPrefix(token, "/"))
		}
	}
	return ""
}

func firstEventName(intent string) string {
	for _, token := range backticked(intent) {
		if strings.Contains(token, ".") && !strings.Contains(token, " ") && !strings.HasPrefix(token, "/") {
			return token
		}
	}
	return ""
}

func sanitizeCapabilityName(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(s, "self-seed-"))
	s = strings.TrimSuffix(s, "-seed")
	s = strings.Trim(s, "`/ <>.,:;()[]{}\t\n")
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// ──────────────────────────────── the brain ─────────────────────────────────

type brainResult struct {
	Response     string
	Declarations []map[string]any
	Script       string // a compile ask's answer, from a script.authored event
}

// brainExe is the plugged brain, if any: an executable (with optional args)
// honoring the brain contract — prompt as last arg, log JSONL on stdin, event
// JSONL (or prose) on stdout.
func brainExe() string { return strings.TrimSpace(os.Getenv("SELF_BRAIN")) }

// agent runs one brain conversation with the three powers: read (bash), act
// (run, over the given capability catalog), grow (declare). user may be a JSON
// array of {role, content} turns, so a chat surface can hand the brain real
// turn-based history.
func (c *llm) agent(system, user string, commands []commandDecl, invoke func(name, args string) (string, error)) (*brainResult, error) {
	tools := []map[string]any{bashTool, declareTool, doneTool}
	known := map[string]bool{}
	if len(commands) > 0 {
		tools = append(tools, runTool)
		var b strings.Builder
		for _, cmd := range commands {
			known[cmd.Name] = true
			fmt.Fprintf(&b, "  %s — %s", cmd.Name, firstSentence(cmd.Description))
			if len(cmd.Params) > 0 {
				b.WriteString(" (args: " + jsonRepr(cmd.Params) + ")")
			}
			b.WriteByte('\n')
		}
		system += "\n\nCAPABILITIES YOU CAN RUN — call the run tool with {\"name\", \"args\"}:\n" + b.String()
	}
	res := &brainResult{}
	seen := map[string]bool{}
	final, err := c.toolLoop(system, conversationTurns(user), tools,
		func(name, args string) (string, bool) {
			switch name {
			case "bash":
				return c.handleBash(args), false
			case "done":
				var a struct {
					Summary string `json:"summary"`
				}
				json.Unmarshal([]byte(args), &a)
				return a.Summary, true
			case "declare":
				var d map[string]any
				if json.Unmarshal([]byte(args), &d) != nil || d["name"] == nil {
					return "error: declare needs {name, payload}", false
				}
				if seen[args] {
					return "declaration already recorded", false
				}
				seen[args] = true
				res.Declarations = append(res.Declarations, d)
				return "declaration recorded — it compiles when you finish", false
			case "run":
				var a struct {
					Name string `json:"name"`
					Args string `json:"args"`
				}
				json.Unmarshal([]byte(args), &a)
				if !known[a.Name] {
					return fmt.Sprintf("error: no such capability %q — pick from the CAPABILITIES list", a.Name), false
				}
				if invoke == nil {
					return "error: acting is disabled here", false
				}
				out, err := invoke(a.Name, a.Args)
				if err != nil {
					return "error: " + err.Error(), false
				}
				return out, false
			}
			return fmt.Sprintf("error: unknown tool %q", name), false
		})
	if err != nil {
		return nil, err
	}
	res.Response = final
	return res, nil
}

// conversationTurns: a JSON array of {role, content} becomes real turns;
// anything else is a single user message.
func conversationTurns(user string) []map[string]any {
	if s := strings.TrimSpace(user); strings.HasPrefix(s, "[") {
		var raw []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if json.Unmarshal([]byte(s), &raw) == nil && len(raw) > 0 {
			turns := make([]map[string]any, 0, len(raw))
			for _, m := range raw {
				if m.Role == "" {
					return []map[string]any{{"role": "user", "content": user}}
				}
				turns = append(turns, map[string]any{"role": m.Role, "content": m.Content})
			}
			return turns
		}
	}
	return []map[string]any{{"role": "user", "content": user}}
}

// plantedCommands reads the command catalog from the log — latest declaration
// per name, first-seen order. chat is excluded: the brain calling chat would
// re-enter itself.
func plantedCommands(home string) []commandDecl {
	events, err := readEvents(home)
	if err != nil {
		return nil
	}
	byName := map[string]commandDecl{}
	var order []string
	for _, e := range events {
		if e.Name != "command.declared" {
			continue
		}
		var d commandDecl
		if json.Unmarshal(e.Payload, &d) != nil || d.Name == "" {
			continue
		}
		if _, seen := byName[d.Name]; !seen {
			order = append(order, d.Name)
		}
		byName[d.Name] = d
	}
	var out []commandDecl
	for _, n := range order {
		if n != "chat" {
			out = append(out, byName[n])
		}
	}
	return out
}

// brainTools returns the brain's act power — the runnable catalog and an
// invoker — honoring SELF_THINK_DEPTH so a brain-invoked command that itself
// thinks can't recurse without bound.
func brainTools(home string) ([]commandDecl, func(name, args string) (string, error)) {
	depth := 0
	fmt.Sscanf(os.Getenv("SELF_THINK_DEPTH"), "%d", &depth)
	if depth >= 3 {
		return nil, nil
	}
	os.Setenv("SELF_THINK_DEPTH", fmt.Sprintf("%d", depth+1))
	invoke := func(name, args string) (string, error) {
		var argv []string
		if strings.TrimSpace(args) != "" {
			argv = []string{args}
		}
		evs, err := runCommand(home, name, argv)
		if err != nil {
			return "", err
		}
		names := make([]string, len(evs))
		for i, e := range evs {
			names[i] = e.Name
		}
		return fmt.Sprintf("ran %q — appended %d event(s): %s", name, len(evs), strings.Join(names, ", ")), nil
	}
	return plantedCommands(home), invoke
}

// applyDeclarations appends what the brain declared and runs it through the
// strange loop.
func applyDeclarations(home string, res *brainResult) {
	var evs []Event
	for _, d := range res.Declarations {
		name, _ := d["name"].(string)
		payload, _ := json.Marshal(d["payload"])
		if name == "" || string(payload) == "null" {
			continue
		}
		e := newEvent(name, payload)
		if err := appendEvent(home, &e); err != nil {
			fmt.Fprintf(os.Stderr, "self: append declaration: %s\n", err)
			return
		}
		evs = append(evs, e)
	}
	if len(evs) > 0 {
		c := newLLM(home)
		defer c.close()
		n := compileDeclarations(c, home, evs)
		fmt.Fprintf(os.Stderr, "self: grew %d capabilit(ies)\n", n)
		renderKernelHTML(home)
		refreshProjections(home)
	}
}

// ─────────────────────────────── kernel.html ────────────────────────────────

// renderKernelHTML writes the kernel's self-description — capabilities, paths,
// the pipe contract — to site/kernel.html: the page a human lands on and the
// first context a brain reads. Like everything in site/, it is a replay of the log.
func renderKernelHTML(home string) {
	events, err := readEvents(home)
	if err != nil {
		return
	}
	commands := map[string]commandDecl{}
	projectors := map[string]projectorDecl{}
	var cmdOrder, projOrder []string
	// grownBy is provenance: the latest kernel-signed receipt's By per capability.
	// Verified, not merely read — an unsigned or forged by-line never renders.
	grownBy := map[string]string{}
	secret, _ := loadSecret(home)
	for _, e := range events {
		switch e.Name {
		case "script.compiled":
			if secret != nil {
				if r, ok := verifiedReceipt(secret, e.Payload); ok && r.By != "" {
					grownBy[r.Type+"/"+r.Name] = r.By
				}
			}
		case "command.declared":
			var d commandDecl
			if json.Unmarshal(e.Payload, &d) == nil && d.Name != "" {
				if _, ok := commands[d.Name]; !ok {
					cmdOrder = append(cmdOrder, d.Name)
				}
				commands[d.Name] = d
			}
		case "projector.declared":
			var d projectorDecl
			if json.Unmarshal(e.Payload, &d) == nil && d.Name != "" {
				if _, ok := projectors[d.Name]; !ok {
					projOrder = append(projOrder, d.Name)
				}
				projectors[d.Name] = d
			}
		}
	}

	esc := html.EscapeString
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>self</title></head><body>\n")
	b.WriteString("<h1>self</h1>\n")
	b.WriteString("<p class=\"muted\">a local-first, event-sourced runtime with LLM-generated capabilities</p>\n")
	b.WriteString("<p>One append-only event log is the only state. Everything here — the capabilities, the projections, this page — is a deterministic replay of that log; humans and agents read the same rendered result. Every path below is a plain file.</p>\n")
	b.WriteString("<h2>if you are an LLM reading this</h2>\n<p>You are one of two things, and the runtime treats them identically. Either the kernel spawned you as this instance's <strong>brain</strong>: <code>events.jsonl</code> is the only state, <code>site/*.html</code> is its rendered form, and your operations are <strong>read</strong> (inspect projections), <strong>write</strong> (run any command below), and <strong>extend</strong> (declare a capability; the kernel compiles it on ingestion). Or you are an <strong>external agent</strong> — a coding session, another instance's process — using this instance from outside: same state, same operations, through the shell. <code>self run &lt;command&gt; …</code> to write, <code>self show &lt;projection&gt;</code> to read, <code>self think \"…\"</code> / <code>self heartbeat</code> to extend, or <code>POST /run/&lt;command&gt;</code> while this page is served. Set <code>SELF_BRAIN_ID</code> first — every script you cause to be generated is signed with that string as its recorded author.</p>\n<p>The log is append-only, so no operation is destructive — and it is the only state that survives a session: what is not appended is lost. If this instance defines verification or hand-off conventions (claim/verify, awaken/bequeath — see below), follow them: announce your session, attach evidence before marking work done, record a hand-off note when you finish.</p>\n")

	b.WriteString("<h2>commands</h2>\n")
	if len(cmdOrder) == 0 {
		b.WriteString("<p class=\"muted\">None yet — grow a seed: <code>self grow seeds/chat</code>.</p>\n")
	}
	for _, n := range cmdOrder {
		d := commands[n]
		b.WriteString("<article class=\"card\"><h3>" + esc(d.Name) + "</h3><p>" + esc(d.Description) + "</p>")
		b.WriteString("<p class=\"muted\">produces <code>" + esc(d.Event.Name) + "</code>")
		if len(d.Params) > 0 {
			b.WriteString(" · args " + esc(jsonRepr(d.Params)))
		}
		b.WriteString(" · <code>self run " + esc(d.Name) + " …</code>")
		if by := grownBy["command/"+d.Name]; by != "" {
			b.WriteString(" · grown by " + esc(by))
		}
		b.WriteString("</p></article>\n")
	}

	b.WriteString("<h2>projections</h2>\n")
	if len(projOrder) == 0 {
		b.WriteString("<p class=\"muted\">None yet.</p>\n")
	}
	for _, n := range projOrder {
		d := projectors[n]
		b.WriteString("<article class=\"card\"><h3><a href=\"/" + esc(d.Name) + "\">/" + esc(d.Name) + "</a></h3><p>" + esc(d.Description) + "</p>")
		b.WriteString("<p class=\"muted\">consumes <code>" + esc(strings.Join(d.Consumes, ", ")) + "</code>")
		if by := grownBy["projector/"+d.Name]; by != "" {
			b.WriteString(" · grown by " + esc(by))
		}
		b.WriteString("</p></article>\n")
	}

	b.WriteString("<h2>where I live</h2>\n<table><tr><th>what</th><th>path</th></tr>")
	for _, row := range [][2]string{
		{"the only truth", filepath.Join(home, "events.jsonl")},
		{"compiled commands", filepath.Join(home, "capabilities", "commands")},
		{"compiled projectors", filepath.Join(home, "capabilities", "projectors")},
		{"materialized HTML", filepath.Join(home, "site")},
	} {
		b.WriteString("<tr><td>" + esc(row[0]) + "</td><td><code>" + esc(row[1]) + "</code></td></tr>")
	}
	b.WriteString("</table>\n")

	b.WriteString("<h2>the pipe contract</h2>\n<pre>" + esc(pipeContract) + "</pre>\n")
	b.WriteString("<h2>the events I act on</h2>\n<p><code>command.declared</code> / <code>projector.declared</code> compile into capabilities (the strange loop, at grow time and run time). <code>script.compiled</code> is a compile receipt signed with my <code>.secret</code> — anyone may append one, but only a kernel-signed receipt ever installs; <code>self rehydrate</code> rebuilds my whole instance from them.</p>\n")
	b.WriteString("</body></html>\n")

	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	os.WriteFile(filepath.Join(siteDir, "kernel.html"), []byte(b.String()), 0644)
}

// ─────────────────────────────── the surface ────────────────────────────────

// The shell is the one shared enrichment the kernel injects at serve time —
// theme and feel layered over projections that stay bare semantic HTML on
// disk. The split of responsibilities is the design system: the log is the
// truth, the projection is the state, the shell is the feel. The shell knows
// the class vocabulary, never the events; strip it (self show, curl, lynx,
// rehydrate) and every page still works, because every affordance underneath
// is a plain HTML form.
//
// The feel is swappable. What is fixed is the class vocabulary and the
// structural rules below — the contract the projections and shellScript are
// written against. A *theme* changes none of that: it is only a skin, a set of
// CSS custom properties (palette, fonts, radii, border weight, shadow) that the
// structural layer reads through var(). So switching designs never renames a
// class or rewrites a rule; every projection keeps working unchanged and only
// the feel moves. Themes are picked at serve time and carry no state into the
// log — presentation, like prefers-color-scheme; the bare HTML on disk stays
// theme-agnostic, so rehydrate and self show are untouched.

const shellMeta = `<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark">`

const defaultTheme = "grove"

// themeOrder fixes how the picker lists designs; themes is the lookup. To add a
// design, define its variables in a new skin and add one entry here — nothing
// structural changes.
var themeOrder = []string{"grove", "micro", "paper"}

var themes = map[string]struct {
	label string // the name shown in the picker
	skin  string // :root (and dark-media) variable definitions — the whole theme
}{
	// grove — the original: warm paper, serif headings, soft rounded cards.
	"grove": {"Grove", `:root{--bg:#f6f4ef;--panel:#fffdf8;--ink:#26231d;--muted:#7d7568;--line:#e5e0d5;--wash:#edeade;
--accent:#2f6b4f;--accent-ink:#fff;--user-bg:#e3efe6;--user-line:#cbdfd2;--danger:#b3402e;
--shadow:0 1px 3px rgba(60,50,30,.07);
--font:system-ui,-apple-system,sans-serif;--head-font:Georgia,'Iowan Old Style',serif;--mono:ui-monospace,monospace;
--radius:10px;--radius-sm:5px;--radius-msg:14px;--line-w:1px}
@media (prefers-color-scheme:dark){:root{--bg:#16150f;--panel:#201e16;--ink:#e7e2d6;--muted:#948b7b;
--line:#35322a;--wash:#2a2820;--accent:#69b98e;--accent-ink:#10231a;--user-bg:#233529;--user-line:#31513e;
--danger:#e0755f;--shadow:0 1px 3px rgba(0,0,0,.4)}}`},

	// micro — micrographics: monospace throughout, zero radius, thick hard
	// borders and a solid offset drop shadow. A crisp, bitmap-poster feel.
	"micro": {"Micro", `:root{--bg:#e8e6df;--panel:#fbfbf7;--ink:#161512;--muted:#6d6a60;--line:#161512;--wash:#dedbcf;
--accent:#c8361f;--accent-ink:#fff;--user-bg:#e3e0d4;--user-line:#161512;--danger:#c8361f;
--shadow:3px 3px 0 var(--line);
--font:'Courier New',ui-monospace,'DejaVu Sans Mono',monospace;--head-font:'Courier New',ui-monospace,monospace;--mono:'Courier New',ui-monospace,monospace;
--radius:0;--radius-sm:0;--radius-msg:0;--line-w:2px}
@media (prefers-color-scheme:dark){:root{--bg:#0d0d0b;--panel:#161613;--ink:#eae8dd;--muted:#8f8b7d;
--line:#eae8dd;--wash:#232320;--accent:#ff5a3c;--accent-ink:#0d0d0b;--user-bg:#1d1d19;--user-line:#eae8dd;
--danger:#ff5a3c;--shadow:3px 3px 0 var(--line)}}`},

	// paper — clean and modern: sans throughout, thin hairlines, no shadow,
	// gentle radii. The low-chrome option.
	"paper": {"Paper", `:root{--bg:#ffffff;--panel:#ffffff;--ink:#1a1a1a;--muted:#8a8a8a;--line:#e6e6e6;--wash:#f4f4f4;
--accent:#2b59c3;--accent-ink:#fff;--user-bg:#eef2fb;--user-line:#dbe3f6;--danger:#c0392b;
--shadow:none;
--font:system-ui,-apple-system,sans-serif;--head-font:system-ui,-apple-system,sans-serif;--mono:ui-monospace,monospace;
--radius:6px;--radius-sm:4px;--radius-msg:10px;--line-w:1px}
@media (prefers-color-scheme:dark){:root{--bg:#0f1115;--panel:#151821;--ink:#e6e8ee;--muted:#8b90a0;
--line:#242836;--wash:#1b1f2a;--accent:#6f9bff;--accent-ink:#0f1115;--user-bg:#1a2233;--user-line:#2a3550;
--danger:#ff7a6b;--shadow:none}}`},
}

// structuralCSS is the fixed half of the shell: the class vocabulary and every
// layout rule, written entirely against var()s a theme supplies. It never
// mentions a literal color, font, or radius — that is what makes the skins
// above interchangeable.
const structuralCSS = `*{box-sizing:border-box}html{scroll-behavior:smooth}
body{font-family:var(--font);margin:0 auto;max-width:72ch;padding:24px 20px 32px;
background:var(--bg);color:var(--ink);line-height:1.55}
h1,h2,h3{font-family:var(--head-font);font-weight:600;line-height:1.25}
h1{font-size:1.55rem;margin:.2em 0 .6em}h2{margin-top:32px;border-bottom:var(--line-w) solid var(--line);padding-bottom:6px}
a{color:var(--accent)}.muted{color:var(--muted)}
.card,article{background:var(--panel);border:var(--line-w) solid var(--line);border-radius:var(--radius);padding:12px 16px;margin:10px 0;box-shadow:var(--shadow)}
.card.danger{border-color:var(--danger);color:var(--danger)}
.row{display:flex;gap:10px;align-items:center;flex-wrap:wrap}.stack{display:flex;flex-direction:column;gap:10px}
.tag{display:inline-block;background:var(--wash);color:var(--accent);border-radius:var(--radius-sm);padding:1px 8px;font-size:12px;font-family:var(--mono)}
.num{text-align:right;font-variant-numeric:tabular-nums}
.msg{max-width:85%;width:fit-content;background:var(--panel);border:var(--line-w) solid var(--line);border-radius:var(--radius-msg);
padding:8px 14px;margin:10px 0;box-shadow:var(--shadow);overflow-wrap:break-word}
.msg .who{display:block;font-size:.72rem;font-weight:600;text-transform:uppercase;letter-spacing:.06em;color:var(--muted);margin-bottom:1px}
.msg.user{margin-left:auto;background:var(--user-bg);border-color:var(--user-line);border-bottom-right-radius:var(--radius-sm)}
.msg.assistant{margin-right:auto;border-bottom-left-radius:var(--radius-sm)}
.msg.pending{opacity:.65}
.dots i{display:inline-block;width:5px;height:5px;margin:1px 2px;border-radius:50%;background:var(--muted);animation:blink 1.2s infinite}
.dots i:nth-child(2){animation-delay:.2s}.dots i:nth-child(3){animation-delay:.4s}
@keyframes blink{0%,80%,100%{opacity:.25}40%{opacity:1}}
table{border-collapse:collapse;width:100%}
th{text-align:left;color:var(--muted);font-size:.78rem;text-transform:uppercase;letter-spacing:.05em}
th,td{border-bottom:var(--line-w) solid var(--line);padding:7px 10px;font-size:14px}
pre{background:var(--panel);border:var(--line-w) solid var(--line);border-radius:var(--radius);padding:12px;overflow-x:auto;font-size:13px}
code{font-family:var(--mono);background:var(--wash);border-radius:var(--radius-sm);padding:1px 5px;font-size:.9em}
form{display:flex;gap:8px;flex-wrap:wrap;margin:12px 0}
input,textarea{font:inherit;flex:1 1 24ch;min-width:0;padding:9px 12px;border:var(--line-w) solid var(--line);border-radius:var(--radius);background:var(--panel);color:inherit}
textarea{flex-basis:100%}
input:focus,textarea:focus{outline:2px solid var(--accent);outline-offset:1px;border-color:transparent}
button{font:inherit;padding:9px 16px;border:var(--line-w) solid var(--accent);border-radius:var(--radius);background:var(--accent);color:var(--accent-ink);cursor:pointer;transition:filter .15s}
button:hover{filter:brightness(1.08)}
button.secondary{background:transparent;color:var(--accent)}button.danger{border-color:var(--danger);background:var(--danger);color:#fff}
form.busy button{opacity:.55;pointer-events:none}
body:has(.msg) form:last-of-type{position:sticky;bottom:0;padding:14px 0 10px;margin-top:6px;background:linear-gradient(transparent,var(--bg) 35%)}
.rise{animation:rise .22s ease-out both}
@keyframes rise{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
::view-transition-old(root),::view-transition-new(root){animation-duration:.15s}
@media (prefers-reduced-motion:reduce){*{animation:none!important;transition:none!important}html{scroll-behavior:auto}}
.self-themes{position:fixed;right:10px;bottom:10px;z-index:9;display:flex;gap:4px;padding:4px;
background:var(--panel);border:var(--line-w) solid var(--line);border-radius:var(--radius-sm);box-shadow:var(--shadow);font-family:var(--mono);font-size:11px}
.self-themes a{padding:2px 7px;border-radius:var(--radius-sm);color:var(--muted);text-decoration:none;line-height:1.4}
.self-themes a[aria-current]{background:var(--accent);color:var(--accent-ink)}
@media print{.self-themes{display:none}}
</style>`

// validTheme reports whether name is a known design; selection paths accept
// only known names, so the injected picker links and cookie can never smuggle
// arbitrary CSS in.
func validTheme(name string) bool { _, ok := themes[name]; return ok }

// themeCSS assembles the full <style> for one design: its skin (the variables)
// followed by the shared structural rules. Unknown names fall back to default.
func themeCSS(name string) string {
	t, ok := themes[name]
	if !ok {
		t = themes[defaultTheme]
	}
	return shellMeta + "<style>" + t.skin + "\n" + structuralCSS
}

// pickTheme resolves the design for one request, most specific first: an
// explicit ?theme= wins (and the handler remembers it in a cookie), then the
// cookie, then the SELF_THEME instance default, then the built-in default.
func pickTheme(r *http.Request) string {
	if t := r.URL.Query().Get("theme"); validTheme(t) {
		return t
	}
	if c, err := r.Cookie("self_theme"); err == nil && validTheme(c.Value) {
		return c.Value
	}
	if t := strings.TrimSpace(os.Getenv("SELF_THEME")); validTheme(t) {
		return t
	}
	return defaultTheme
}

// themePicker is the one bit of DOM the shell adds to the body: a small fixed
// switcher of plain links. It works with no JS (each link is a GET that the
// server themes and remembers), and it is styled by the active theme itself, so
// it always matches the page it sits on.
func themePicker(current string) string {
	var b strings.Builder
	b.WriteString(`<nav class="self-themes" aria-label="page design">`)
	for _, name := range themeOrder {
		if name == current {
			b.WriteString(`<a href="?theme=` + name + `" aria-current="true">` + themes[name].label + `</a>`)
		} else {
			b.WriteString(`<a href="?theme=` + name + `">` + themes[name].label + `</a>`)
		}
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// shellScript is the reactive half of the shell: progressive enhancement
// only, injected at serve time and never persisted. The state machine is
// untouched — every interaction is still form → command → events → replay;
// the script changes how the round-trip FEELS, not what it is. It may show
// intent in flight (a pending turn, a thinking brain) but never claims
// state: when the round-trip lands, the page is re-fetched and the log's
// replay wins. Liveness is the same idea watched from outside — the byte
// length of /events is the cursor; when the log grows, re-replay.
const shellScript = `<script>
(() => {
"use strict";
if (!window.fetch || !window.DOMParser) return;
let busy = false, baseline = null;

const logSize = () => fetch("/events", {method:"HEAD", cache:"no-store"})
  .then(r => r.headers.get("content-length")).catch(() => null);

async function refresh(toBottom) {
  const res = await fetch(location.href, {cache:"no-store"});
  if (!res.ok) return;
  const doc = new DOMParser().parseFromString(await res.text(), "text/html");
  const ae = document.activeElement;
  const keep = ae && ae.name ? {name: ae.name, value: ae.value} : null;
  const y = scrollY;
  const swap = () => {
    document.body.replaceWith(doc.body);
    if (toBottom && document.querySelector(".msg")) {
      [...document.querySelectorAll(".msg")].slice(-2).forEach(m => m.classList.add("rise"));
      scrollTo(0, document.body.scrollHeight);
    } else scrollTo(0, y);
    if (keep) {
      const el = document.querySelector("[name=\"" + keep.name.replace(/"/g, "") + "\"]");
      if (el) { if (!toBottom) el.value = keep.value; el.focus(); }
    }
  };
  if (document.startViewTransition) document.startViewTransition(swap); else swap();
}

document.addEventListener("submit", e => {
  const f = e.target;
  const action = new URL(f.getAttribute("action") || "", location.href);
  if (!action.pathname.startsWith("/run/")) return;
  e.preventDefault();
  if (busy) return;
  busy = true;
  f.classList.add("busy");
  const body = new URLSearchParams(new FormData(f));
  const first = f.querySelector("input,textarea");
  const text = first ? first.value : "";
  let ghost, think;
  const anchor = [...document.querySelectorAll(".msg")].pop();
  if (anchor && text.trim()) { // a conversation: show the turn in flight, clearly pending
    const label = (role) => { // borrow the who label the projection already uses
      const whos = document.querySelectorAll(".msg." + role + " .who");
      return whos.length ? "<span class='who'></span>" : "";
    };
    ghost = document.createElement("div");
    ghost.className = "msg user pending rise";
    ghost.innerHTML = label("user");
    ghost.appendChild(document.createTextNode(text));
    think = document.createElement("div");
    think.className = "msg assistant pending rise";
    think.innerHTML = label("assistant") + "<span class='dots'><i></i><i></i><i></i></span>";
    for (const role of ["user", "assistant"]) {
      const whos = document.querySelectorAll(".msg." + role + " .who");
      const target = (role === "user" ? ghost : think).querySelector(".who");
      if (whos.length && target) target.textContent = whos[whos.length - 1].textContent;
    }
    anchor.after(ghost, think);
    first.value = "";
    scrollTo({top: document.body.scrollHeight, behavior: "smooth"});
  }
  fetch(action, {method: "POST", body}).then(async r => {
    if (!r.ok) throw new Error((await r.text()).trim() || r.status);
    baseline = null;
    await refresh(true);
  }).catch(err => { // degrade honestly: say what failed, give the words back
    if (ghost) { think.remove(); ghost.remove(); first.value = text; }
    const p = document.createElement("p");
    p.className = "card danger rise";
    p.textContent = "could not run " + action.pathname.slice(5) + ": " + err.message;
    f.before(p);
    setTimeout(() => p.remove(), 8000);
  }).finally(() => { busy = false; f.classList.remove("busy"); });
});

async function tick() {
  if (document.hidden || busy) return;
  const n = await logSize();
  if (n == null) return;
  if (baseline != null && n !== baseline) {
    const nearBottom = innerHeight + scrollY > document.body.scrollHeight - 120;
    await refresh(nearBottom && !!document.querySelector(".msg"));
  }
  baseline = n;
}
setInterval(tick, 2500);
addEventListener("visibilitychange", () => { if (!document.hidden) tick(); });
addEventListener("DOMContentLoaded", tick);
})();
</script>`

func injectShell(page []byte, theme string) []byte {
	head := themeCSS(theme) + shellScript
	if i := bytes.Index(page, []byte("<head>")); i >= 0 {
		i += len("<head>")
		page = append(page[:i:i], append([]byte(head), page[i:]...)...)
	} else {
		page = append([]byte(head), page...)
	}
	picker := themePicker(theme)
	if j := bytes.LastIndex(page, []byte("</body>")); j >= 0 {
		return append(page[:j:j], append([]byte(picker), page[j:]...)...)
	}
	return append(page, []byte(picker)...)
}

// writePage sends an on-disk projection through the shell for one request:
// resolve the design, remember an explicit choice in a cookie so it persists
// across pages, and inject theme + script + picker. This is the only place a
// theme touches a response; nothing is written back to the log or to disk.
func writePage(w http.ResponseWriter, r *http.Request, page []byte) {
	theme := pickTheme(r)
	if q := r.URL.Query().Get("theme"); validTheme(q) {
		http.SetCookie(w, &http.Cookie{Name: "self_theme", Value: q, Path: "/", MaxAge: 31536000, SameSite: http.SameSiteLaxMode})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(injectShell(page, theme))
}

// cmdServe serves the instance: every page re-rendered against current events,
// every affordance a plain HTML form. The injected shell layers feel on top —
// pending turns, live re-replay, theme — but carries no state and grants no
// power: strip it and every page still works, because the forms do.
func cmdServe(home, port string) error {
	if port == "" {
		port = "7777"
	}
	renderKernelHTML(home)
	refreshProjections(home)

	mux := http.NewServeMux()

	// GET /            → kernel.html (my identity), or a welcome projection if grown
	// GET /<name>      → that projection, re-run live
	// anything else    → static site/ files
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.Trim(r.URL.Path, "/"), ".html")
		if name == "" {
			if p, _ := scriptPath(home, "projector", "welcome"); fileExists(p) {
				name = "welcome"
			} else {
				name = "kernel"
			}
		}
		if name == "kernel" {
			renderKernelHTML(home)
			page, err := os.ReadFile(filepath.Join(home, "site", "kernel.html"))
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writePage(w, r, page)
			return
		}
		if p, _ := scriptPath(home, "projector", name); fileExists(p) {
			page, err := runProjection(home, name)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writePage(w, r, page)
			return
		}
		http.FileServer(http.Dir(filepath.Join(home, "site"))).ServeHTTP(w, r)
	})

	// GET /events → the raw log
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, logPath(home))
	})

	// POST /run/<command> → run a capability from the browser. A form's field
	// values become positional args in document order (names are for humans;
	// order is the contract); then Post/Redirect/Get back to the page.
	mux.HandleFunc("/run/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		command := strings.TrimPrefix(r.URL.Path, "/run/")
		body, _ := io.ReadAll(r.Body)
		var args []string
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			for _, pair := range strings.Split(string(body), "&") {
				if pair == "" {
					continue
				}
				_, v, _ := strings.Cut(pair, "=")
				v = strings.ReplaceAll(v, "+", " ")
				if dec, err := url.QueryUnescape(v); err == nil {
					v = dec
				}
				args = append(args, v)
			}
		} else if msg := strings.TrimSpace(string(body)); msg != "" {
			args = []string{msg}
		}
		if _, err := runCommand(home, command, args); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if ref := r.Header.Get("Referer"); ref != "" {
			http.Redirect(w, r, ref, http.StatusSeeOther)
			return
		}
		fmt.Fprint(w, "ok")
	})

	// Loopback by default: the write path (/run/<command>) has no auth, and
	// local-first means local. SELF_BIND=0.0.0.0 opens it to the network for
	// anyone who knowingly wants that.
	addr := envOr("SELF_BIND", "127.0.0.1") + ":" + port
	fmt.Fprintf(os.Stderr, "self: serving at http://%s (home %s)\n", addr, home)
	fmt.Fprintf(os.Stderr, "  /              my identity — capabilities, paths, contract\n")
	fmt.Fprintf(os.Stderr, "  /<projection>  a projection, re-rendered live\n")
	fmt.Fprintf(os.Stderr, "  /run/<command> run a capability (plain HTML forms)\n")
	fmt.Fprintf(os.Stderr, "  /events        the raw event log\n")
	return http.ListenAndServe(addr, mux)
}

// ────────────────────────────── the commands ────────────────────────────────

// cmdGrow grows a seed: a directory with intent.md (the genotype — prose
// intent, not a parts-list) and optionally seed.jsonl (initial content events,
// the initial deposit). The orchestrator reads the intent, explores the
// instance, and declares the decomposition that realizes it here; each piece is
// then compiled with the whole intent woven in. Same intent, different instance,
// different decomposition.
func cmdGrow(home, seedDir string) error {
	raw, err := os.ReadFile(filepath.Join(seedDir, "intent.md"))
	if err != nil {
		return fmt.Errorf("a seed is a directory with an intent.md: %w", err)
	}
	intent := strings.TrimSpace(string(raw))
	name := filepath.Base(seedDir)

	payload, _ := json.Marshal(map[string]any{"name": name, "intent": intent})
	ie := newEvent("intent.declared", payload)
	if err := appendEvent(home, &ie); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "self: orchestrating %q from intent…\n", name)
	res, err := pipeBrain(home, "grow",
		"Grow the capabilities that realize this product: declare each one (emit command.declared / projector.declared), then summarize in one line.\n\n--- INTENT ---\n"+intent+"\n--- END INTENT ---")
	if err != nil {
		return fmt.Errorf("orchestrate %q: %w (growing needs a brain — %s)", name, err, brainHint)
	}
	c := newLLM(home)
	defer c.close()
	c.intent = intent
	if len(res.Declarations) == 0 {
		return fmt.Errorf("the orchestrator declared nothing for %q", name)
	}

	var declEvents []Event
	for _, d := range res.Declarations {
		n, _ := d["name"].(string)
		p, _ := json.Marshal(d["payload"])
		if (n != "command.declared" && n != "projector.declared") || string(p) == "null" {
			continue
		}
		e := newEvent(n, p)
		if err := appendEvent(home, &e); err != nil {
			return err
		}
		declEvents = append(declEvents, e)
	}
	grown := compileDeclarations(c, home, declEvents)

	// The initial deposit: content laid once, so the surface has
	// something to render from the first moment.
	if raw, err := os.ReadFile(filepath.Join(seedDir, "seed.jsonl")); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if line = strings.TrimSpace(line); line == "" {
				continue
			}
			var e Event
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				return fmt.Errorf("parse seed.jsonl: %w", err)
			}
			fresh := newEvent(e.Name, e.Payload)
			if err := appendEvent(home, &fresh); err != nil {
				return err
			}
		}
	}

	rp, _ := json.Marshal(map[string]any{"seed": name, "capabilities": grown})
	se := newEvent("seed.planted", rp)
	if err := appendEvent(home, &se); err != nil {
		return err
	}
	renderKernelHTML(home)
	refreshProjections(home)
	fmt.Printf("grew %q: %d capabilit(ies) from intent — %s\n", name, grown, res.Response)
	return nil
}

// ──────────────── sharing — intent and evidence between instances ────────────
//
// A seed is a verbatim slice of the sender's log: every declaration of one
// capability (the intent, re-teachings and dead ends included) and every
// kernel-signed receipt for it (the evidence). The log's own format is the
// wire format. Code never crosses: adopt records the whole seed inside a
// single capability.adopted event — foreign receipts ride there, where
// rehydrate never looks, inert by construction — then re-declares the
// capability so the strange loop authors bytes for THIS instance, through its own
// compiler, signed by its own key. The sender's latest script rides only as
// the reference a seed author already gets; the compiler adapts, never copies.

// declName returns the capability a declaration event declares, or "".
func declName(e Event) (typ, name string) {
	if e.Name != "command.declared" && e.Name != "projector.declared" {
		return "", ""
	}
	var d struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(e.Payload, &d) != nil {
		return "", ""
	}
	return strings.TrimSuffix(e.Name, ".declared"), d.Name
}

func cmdShare(home, name string) error {
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	secret, err := loadSecret(home)
	if err != nil {
		return err
	}
	var seed []Event
	hasDecl := false
	for _, e := range events {
		if _, n := declName(e); n == name {
			seed, hasDecl = append(seed, e), true
		} else if e.Name == "script.compiled" {
			if r, ok := verifiedReceipt(secret, e.Payload); ok && r.Name == name {
				seed = append(seed, e)
			}
		}
	}
	if !hasDecl {
		return fmt.Errorf("no declaration for %q in this log — nothing to share (code never crosses; the declaration is what does)", name)
	}
	enc := json.NewEncoder(os.Stdout)
	for i := range seed {
		enc.Encode(seed[i])
	}
	// The sender remembers giving: if it is not an event, it did not happen.
	payload, _ := json.Marshal(map[string]any{"name": name, "events": len(seed)})
	e := newEvent("capability.shared", payload)
	if err := appendEvent(home, &e); err != nil {
		return err
	}
	refreshProjections(home)
	fmt.Fprintf(os.Stderr, "self: shared %q — %d event(s) of intent and evidence\n", name, len(seed))
	return nil
}

func cmdAdopt(home, path string) error {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	var seed []Event
	var typ, name string
	var declPayload json.RawMessage
	reference := ""
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var e Event
		if json.Unmarshal([]byte(line), &e) != nil || e.Name == "" {
			return fmt.Errorf("not a seed — want event JSONL, one {name, payload} per line")
		}
		seed = append(seed, e)
		if t, n := declName(e); n != "" { // the latest declaration is what grows here
			typ, name, declPayload = t, n, e.Payload
		}
		if e.Name == "script.compiled" { // the latest script is reference, never install
			var r receipt
			if json.Unmarshal(e.Payload, &r) == nil && r.Script != "" {
				reference = r.Script
			}
		}
	}
	if declPayload == nil {
		return fmt.Errorf("the seed carries no declaration — nothing can grow from it")
	}
	if err := ensureHome(home); err != nil {
		return err
	}
	if reference != "" {
		var m map[string]any
		if err := json.Unmarshal(declPayload, &m); err != nil {
			return fmt.Errorf("the seed's declaration is not an object: %w", err)
		}
		m["implementation"] = reference
		declPayload, _ = json.Marshal(m)
	}
	ap, _ := json.Marshal(map[string]any{"type": typ, "name": name, "seed": seed})
	if err := ingest(home, []Event{
		newEvent("capability.adopted", ap),
		newEvent(typ+".declared", declPayload),
	}); err != nil {
		return err
	}
	if p, _ := scriptPath(home, typ, name); !fileExists(p) {
		return fmt.Errorf("adopted %q into the log, but no compiler produced a script — wire a brain (or SELF_LLM_STUB=1) and declare it again", name)
	}
	fmt.Printf("adopted %q — re-authored by this instance's own compiler, signed by its own key\n", name)
	return nil
}

func cmdRun(home, command string, args []string) error {
	evs, err := runCommand(home, command, args)
	if err != nil {
		return err
	}
	for _, e := range evs {
		fmt.Printf("appended seq %d %s\n", e.Seq, e.Name)
	}
	return nil
}

// cmdThink asks the brain and prints {response, declarations} JSON. The brain
// is a PROCESS the kernel pipes the log to — $SELF_BRAIN swaps in any program
// honoring the contract (prompt as last arg, event JSONL out); the default is
// self's own `brain` mode. think appends nothing: the caller owns that.
func cmdThink(home, prompt string) error {
	if prompt == "" {
		data, _ := io.ReadAll(os.Stdin)
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		return fmt.Errorf("usage: self think <prompt> (or pipe it on stdin)")
	}
	res, err := pipeBrain(home, "think", prompt)
	if err != nil {
		return fmt.Errorf("brain: %w", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"response": res.Response, "declarations": res.Declarations})
}

// pipeBrain is the ONE seam through which the kernel asks for intelligence —
// think, heartbeat, grow, and compile all pass here. It spawns the brain
// ($SELF_BRAIN, or the built-in `self brain`) with the ask's kind in
// $SELF_ASK, the prompt as the last argument, and the whole log as JSONL on
// stdin. The parse is tolerant on purpose: JSON lines with a name are events —
// script.authored answers a compile, chat.message carries the reply, anything
// else is a declaration — and bare prose joins the reply. So any process that can
// read stdin and print plugs in whole: a local model, an API loop, a coding
// agent, a human. The kernel cannot tell the difference, by construction.
func pipeBrain(home, kind, prompt string) (*brainResult, error) {
	if os.Getenv("SELF_LLM_STUB") == "1" && brainExe() == "" {
		return stubBrain(home, kind, prompt)
	}
	current, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	bin, argv := brainCommand(prompt)
	cmd := exec.Command(bin, argv...)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home, "SELF_ASK="+kind)
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
	feedEvents(stdin, current)
	res := &brainResult{Declarations: []map[string]any{}}
	var prose []string
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal([]byte(line), &e) != nil || e.Name == "" {
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
			res.Declarations = append(res.Declarations, map[string]any{"name": e.Name, "payload": json.RawMessage(e.Payload)})
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("brain %s exited: %w", filepath.Base(bin), err)
	}
	res.Response = strings.Join(prose, "\n")
	return res, nil
}

const brainHint = `plug a brain: SELF_BRAIN=<any executable, e.g. "claude -p">, or SELF_LLM_URL=<OpenAI-compatible endpoint>, or SELF_LLM_STUB=1 for offline stubs`

func brainCommand(prompt string) (string, []string) {
	if v := strings.TrimSpace(os.Getenv("SELF_BRAIN")); v != "" {
		parts := strings.Fields(v)
		return parts[0], append(parts[1:], prompt)
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "self"
	}
	return exe, []string{"brain", prompt}
}

// cmdBrain is the default brain process behind the same contract any
// replacement must honor: prompt in, event JSONL out (the reply as
// chat.message, growth as declarations). Because it is just a process, it can
// be replaced wholesale via $SELF_BRAIN — the kernel can't tell the difference.
func cmdBrain(home, prompt string) error {
	if prompt == "" {
		data, _ := io.ReadAll(os.Stdin)
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		return fmt.Errorf("usage: self brain <prompt> (or pipe it on stdin)")
	}
	commands, invoke := brainTools(home)
	system := brainSystemPrompt
	if os.Getenv("SELF_ASK") == "grow" {
		system, commands, invoke = orchestratorSystemPrompt, nil, nil
	}
	c := newLLM(home)
	defer c.close()
	res, err := c.agent(system, prompt, commands, invoke)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	emit := func(name string, payload any) {
		enc.Encode(map[string]any{"name": name, "payload": payload})
	}
	emit("chat.message", map[string]any{"role": "user", "content": prompt})
	if strings.TrimSpace(res.Response) != "" {
		emit("chat.message", map[string]any{"role": "assistant", "content": res.Response})
	}
	for _, d := range res.Declarations {
		if name, _ := d["name"].(string); name != "" && d["payload"] != nil {
			emit(name, d["payload"])
		}
	}
	return nil
}

// cmdHeartbeat is one self-improvement cycle: the brain reads what changed
// since its last beat, explores, and — if warranted — declares one small
// improvement, which compiles through the strange loop.
func cmdHeartbeat(home string) error {
	prior, _ := readEvents(home)
	hb := newEvent("self.heartbeat", json.RawMessage(`{}`))
	if err := appendEvent(home, &hb); err != nil {
		return err
	}
	prompt := `This is a self-improvement heartbeat. Explore your instance — capabilities, recent events, projections — and choose ONE small, high-value improvement: a missing capability, a clearer projection, a drift to fix. If warranted, declare it (emit command.declared / projector.declared); if nothing is worth changing, say so plainly and declare nothing. Keep it minimal.` + heartbeatContext(prior)
	res, err := pipeBrain(home, "heartbeat", prompt)
	if err != nil {
		return err
	}
	applyDeclarations(home, res)
	fmt.Println(res.Response)
	return nil
}

// heartbeatContext hands the brain the events since its last beat — capped,
// minus kernel bookkeeping receipts — so a beat reacts to what changed instead
// of exploring from scratch.
func heartbeatContext(events []Event) string {
	last := -1
	for i, e := range events {
		if e.Name == "self.heartbeat" {
			last = i
		}
	}
	var acts []Event
	for _, e := range events[last+1:] {
		if e.Name == "script.compiled" || e.Name == "script.verified" {
			continue
		}
		acts = append(acts, e)
	}
	if len(acts) == 0 {
		return ""
	}
	if len(acts) > 40 {
		acts = acts[len(acts)-40:]
	}
	var b strings.Builder
	b.WriteString("\n\nSince your last heartbeat, these things happened in this instance:\n")
	for _, e := range acts {
		payload := strings.TrimSpace(string(e.Payload))
		if len(payload) > 140 {
			payload = payload[:140] + "…"
		}
		fmt.Fprintf(&b, "  seq %d  %s  %s\n", e.Seq, e.Name, payload)
	}
	b.WriteString("\nResponding to what changed is welcome, but optional.")
	return b.String()
}

func cmdShow(home, name string) error {
	if name == "kernel" {
		renderKernelHTML(home)
		page, err := os.ReadFile(filepath.Join(home, "site", "kernel.html"))
		if err != nil {
			return err
		}
		os.Stdout.Write(page)
		return nil
	}
	page, err := runProjection(home, name)
	if err != nil {
		return err
	}
	os.Stdout.Write(page)
	return nil
}

// ─────────────────────────────────── main ───────────────────────────────────

func homeDir() string {
	if v := os.Getenv("SELF_HOME"); v != "" {
		// Scripts run with cwd = home, so a relative home would silently break
		// every exec. Absolute, always.
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".self")
}

// ensureHome initializes a bare instance on first contact: a signing key and a first
// event. Everything else grows.
func ensureHome(home string) error {
	if _, err := loadSecret(home); err != nil {
		return err
	}
	events, err := readEvents(home)
	if err != nil || len(events) > 0 {
		return err
	}
	e := newEvent("kernel.initialized", json.RawMessage(`{}`))
	if err := appendEvent(home, &e); err != nil {
		return err
	}
	renderKernelHTML(home)
	fmt.Fprintf(os.Stderr, "self: new home %s\n", home)
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, usageText())
}

func usageText() string {
	return `self — a local-first, event-sourced runtime with LLM-generated capabilities

One append-only event log + projections as deterministic replays. A minimal
kernel; every capability is generated from a declaration and installed under
a signed receipt.

usage: self [command] [args]

  self                 rehydrate the instance from the log, then serve it (the default)
  self grow <seed>     grow a seed's intent into capabilities (needs a brain)
  self run <cmd> ...   run a capability — append events, refresh projections
  self think "..."     ask the brain; returns {response, declarations} JSON
  self brain "..."     the default brain process (prompt in, event JSONL out); swap via $SELF_BRAIN
  self heartbeat       one self-improvement cycle (the brain reflects & grows)
  self show <name>     render a projection to stdout
  self live [port]     serve the instance (default 7777)
  self rehydrate       rebuild capabilities/ + site/ from the log's signed receipts (no LLM)
  self share <cap>     print a seed to stdout — the capability's declarations and
                       receipts, a verbatim slice of this log
  self adopt <seed>    re-grow a shared capability here ("-" reads stdin) — this
                       instance's own compiler re-authors it; foreign bytes never install
  self protocol        print the brain + capability wire protocol

environment:
  SELF_HOME         the instance — a dir holding events.jsonl + .secret (default ~/.self)

  plug a brain (one seam; think, heartbeat, grow, and compile all pass through it):
  SELF_BRAIN        any executable, e.g. "claude -p" — it gets the ask's kind in
                    $SELF_ASK, the prompt as its last argument, and the whole log
                    as JSONL on stdin; it answers in event JSONL, prose tolerated
  SELF_LLM_URL      …or an OpenAI-compatible endpoint (default http://127.0.0.1:8080)
  SELF_LLM_API_KEY  its key
  SELF_LLM_MODEL    its model
  SELF_LLM_STUB     "1" → offline stub scripts (no LLM, no network)
  SELF_SANDBOX      "0" → disable the brain's jailed playpen (bash falls back
                    to a fail-closed read-only allowlist; never fails open)
  SELF_BRAIN_ID     provenance by-line signed into script.compiled receipts
                    (default: the model @ its endpoint, or "stub (no LLM)")
  SELF_THEME        default page design when serving: grove | micro | paper
                    (default grove); a ?theme= link or the on-page picker
                    overrides it per viewer. Presentation only — never logged.
`
}

func protocolText() string {
	return `self protocol — the wire contracts

Brain process contract

  The same seam handles think, heartbeat, grow, and compile.

  SELF_BRAIN   executable to spawn, optionally with args, for example "claude -p"
  SELF_ASK     request kind: think | heartbeat | grow | compile
  argv         the prompt is passed as the last argument
  stdin        the full event log as JSONL: {id, seq, name, occurred_at, payload}
  stdout       event JSONL; non-JSON lines are tolerated as prose reply text

Brain reply events

  chat.message        prose reply for think/brain:
                      {"name":"chat.message","payload":{"role":"assistant","content":"..."}}

  command.declared    declare a command capability; the kernel compiles it:
                      {"name":"command.declared","payload":{"name":"note","description":"...","params":{"text":"string"},"event":{"name":"note.added","fields":{"text":"string"}}}}

  projector.declared  declare a projection; the kernel compiles it:
                      {"name":"projector.declared","payload":{"name":"notes","description":"...","consumes":["note.added"]}}

  script.authored     answer to SELF_ASK=compile only:
                      {"name":"script.authored","payload":{"script":"#!/bin/sh\n..."}}

Compiled capability contract

  command script      argv are command args; stdin is the current event log JSONL;
                      stdout is new event JSONL: {"name":"event.name","payload":{...}}
                      the kernel assigns id, seq, and occurred_at, appends the
                      events, then re-renders all projections.

  projector script    stdin is the full event log JSONL; stdout is HTML.
                      The kernel writes it to SELF_HOME/site/<name>.html.

  environment         SELF_HOME is set for every compiled script.

Declarations cross instance boundaries; runnable code does not. A generated
script installs only after the local kernel signs a script.compiled receipt with
SELF_HOME/.secret and the current SELF_BRAIN_ID.
`
}

func commandHelp(cmd string) (string, bool) {
	switch cmd {
	case "grow":
		return "usage: self grow <seed-dir>\n\nRead <seed-dir>/intent.md, ask the brain to declare capabilities, compile them, and install signed receipts.\n", true
	case "run":
		return "usage: self run <command> [args...]\n\nRun an installed command capability. Its emitted events are appended, then projections re-render.\n", true
	case "think":
		return "usage: self think <prompt>\n       self think < prompt.txt\n\nAsk the brain through the SELF_BRAIN protocol. Prints {response, declarations} JSON and appends nothing.\n", true
	case "brain":
		return "usage: self brain <prompt>\n       self brain < prompt.txt\n\nRun the built-in brain process. It implements the same protocol expected from SELF_BRAIN.\n", true
	case "heartbeat":
		return "usage: self heartbeat\n\nAppend a heartbeat event, ask the brain for one small improvement, and compile any declarations it emits.\n", true
	case "show":
		return "usage: self show <projection>\n\nRender a projection to stdout by replaying the current log. Use 'kernel' for the instance index.\n", true
	case "live":
		return "usage: self live [port]\n\nServe the instance on 127.0.0.1 (or SELF_BIND) with /, /<projection>, /run/<command>, and /events.\n", true
	case "rehydrate":
		return "usage: self rehydrate\n\nRebuild capabilities/ and site/ from events.jsonl + .secret without a brain.\n", true
	case "share":
		return "usage: self share <capability>\n\nPrint the capability's declarations and receipts as a JSONL seed.\n", true
	case "adopt":
		return "usage: self adopt <seed.jsonl>\n       self adopt - < seed.jsonl\n\nRecord a shared seed and re-generate its capability locally; foreign code never installs.\n", true
	case "protocol":
		return protocolText(), true
	}
	return "", false
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" || arg == "help" {
			return true
		}
	}
	return false
}

func main() {
	home := homeDir()
	if len(os.Args) < 2 {
		err := ensureHome(home)
		if err == nil {
			err = rehydrate(home)
		}
		if err == nil {
			err = cmdServe(home, "")
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "self: %s\n", err)
			os.Exit(1)
		}
		return
	}

	cmd, args := os.Args[1], os.Args[2:]

	// __jail is the playpen's child half: this process is already inside fresh
	// namespaces (see playpen.exec) and here becomes the jailed bash.
	if cmd == "__jail" {
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "self: __jail is internal to the playpen")
			os.Exit(125)
		}
		if err := cmdJail(args[0], args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "self: jail: %s\n", err)
			os.Exit(125)
		}
		return
	}

	var err error
	if cmd != "help" && wantsHelp(args) {
		if text, ok := commandHelp(cmd); ok {
			fmt.Fprint(os.Stdout, text)
			return
		}
	}

	switch cmd {
	case "grow":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self grow <seed-dir>")
		} else {
			err = cmdGrow(home, args[0])
		}
	case "run":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self run <command> [args...]")
		} else {
			err = cmdRun(home, args[0], args[1:])
		}
	case "think":
		err = cmdThink(home, strings.Join(args, " "))
	case "brain":
		err = cmdBrain(home, strings.Join(args, " "))
	case "heartbeat":
		err = cmdHeartbeat(home)
	case "show":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self show <projection>")
		} else {
			err = cmdShow(home, args[0])
		}
	case "live":
		port := ""
		if len(args) > 0 {
			port = args[0]
		}
		if err = ensureHome(home); err == nil {
			err = cmdServe(home, port)
		}
	case "rehydrate":
		err = rehydrate(home)
	case "share":
		if len(args) != 1 {
			err = fmt.Errorf("usage: self share <capability>  (the seed prints to stdout)")
		} else {
			err = cmdShare(home, args[0])
		}
	case "adopt":
		if len(args) != 1 {
			err = fmt.Errorf("usage: self adopt <seed.jsonl>")
		} else {
			err = cmdAdopt(home, args[0])
		}
	case "protocol":
		fmt.Fprint(os.Stdout, protocolText())
	case "help", "--help", "-h":
		if len(args) == 0 {
			usage()
		} else if text, ok := commandHelp(args[0]); ok {
			fmt.Fprint(os.Stdout, text)
		} else {
			err = fmt.Errorf("unknown help topic %q", args[0])
		}
	default:
		fmt.Fprintf(os.Stderr, "self: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "self: %s\n", err)
		os.Exit(1)
	}
}

// ──────────────────────────────── small bits ────────────────────────────────

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func jsonRepr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '.'); i > 0 && i < 140 {
		return s[:i]
	}
	if len(s) > 140 {
		return s[:140] + "…"
	}
	return s
}
