// self — a local-first, self-growing capability system, cut to its spirit.
//
// One append-only event log (events.jsonl) is the only truth. Every view is a
// pure replay of it, rendered as HTML that you and your agent read identically.
// Capabilities are standalone scripts the kernel pipes events through, and code
// is never shipped — an LLM compiler authors every script from a declaration,
// for this receiver. A running capability can declare new capabilities and the
// kernel compiles them on the spot (the strange loop). Every compile is logged
// as a script.compiled receipt signed with a per-home secret; only kernel-signed
// receipts ever install, so `self rehydrate` rebuilds the whole body from the
// log alone — a home is just events.jsonl + .secret.
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
	"strings"
	"sync"
	"syscall"
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
	// a model at an endpoint, a stub, a named mind. The signature covers it,
	// so authorship can no more be forged or relabeled than the script itself.
	// Receipts minted before provenance existed have no By and verify by the
	// legacy formula; the lineage of old organs stays in the letters.
	By  string `json:"by,omitempty"`
	Sig string `json:"sig"`
}

// sign binds the receipt's fields so none can be relabeled — one capability's
// bytes can't install under another's name, and authorship can't be moved.
// The v2 formula is domain-separated and length-prefixed (no concatenation of
// adjacent fields can collide); the legacy formula survives so bodies minted
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

// rehydrate rebuilds the body from the log alone: the latest kernel-signed
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
		if _, seen := latest[r.Name]; !seen {
			order = append(order, r.Name)
		}
		latest[r.Name] = r
	}
	for _, name := range order {
		r := latest[name]
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
// script out) and the default BRAIN (`self brain`). Both explore the garden
// through a single bash tool — a jailed full-bash playpen where the platform
// allows, a fail-closed read-only allowlist otherwise — before writing
// anything: same seed, different garden, different binary.

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
// carries. SELF_BRAIN_ID lets a mind introduce itself in its own words ("the
// ninth mind, a Claude, by hand"); otherwise the model and endpoint are the
// honest mechanical answer, and a stub says so.
func (c *llm) identity() string {
	if id := strings.TrimSpace(os.Getenv("SELF_BRAIN_ID")); id != "" {
		return id
	}
	if c.stub {
		return "stub (no LLM)"
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
		return "", fmt.Errorf("no LLM configured (set SELF_LLM_URL / SELF_LLM_API_KEY / SELF_LLM_MODEL)")
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
	bashTool    = tool("bash", "Run a bash command in the PLAYPEN: an ephemeral jailed copy of the body (events.jsonl, capabilities/, site/ at /body — never the signing key). Full bash, real execution: write files, run interpreters, pipe the copied log through a candidate script and read what comes out. Writes stay in the jail, there is no network, and state persists across calls in this conversation. Nothing here changes the real body — to change the body, emit declarations. Where jailing is unsupported this becomes a fail-closed READ-ONLY allowlist (ls, cat, head, tail, grep, find, wc, sort, uniq, cut, tr, echo, jq; no redirection) — a refused write tells you which mode you are in.", map[string]any{"command": str("the shell command")}, "command")
	declareTool = tool("declare", `Declare ONE new capability; the kernel compiles it into a live script. Call once per capability. A command: {"name":"command.declared","payload":{"name","description","params":{k:type},"event":{"name","fields":{k:type}}}}. A projector: {"name":"projector.declared","payload":{"name","description","consumes":["event.name"]}}.`, map[string]any{"name": str("command.declared or projector.declared"), "payload": map[string]any{"type": "object", "description": "the declaration"}}, "name", "payload")
	doneTool    = tool("done", "Finish, with a short summary for the user.", map[string]any{"summary": str("one or two sentences")}, "summary")
	runTool     = tool("run", "Run one of the capabilities listed under CAPABILITIES; the kernel appends the events it emits.", map[string]any{"name": str("the capability"), "args": str("space-separated args, in declared order")}, "name")
	submitTool  = tool("submit", "Submit the finished script (full source, with shebang).", map[string]any{"script": str("the executable script")}, "script")
)

var readOnlyCmds = map[string]bool{"ls": true, "cat": true, "head": true, "tail": true, "grep": true,
	"find": true, "wc": true, "sort": true, "uniq": true, "cut": true, "tr": true, "echo": true, "jq": true}

// readOnlyBash is the exploration tool: fail-closed to plain readers, so the
// model can look at the garden but not touch it.
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = home
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
// Full bash for the brain, contained. The read-only allowlist let a mind look
// but never try; the ninth mind's letters record the cost — organs authored by
// squinting instead of testing. The playpen removes the poverty and keeps the
// trust model: each brain/compiler conversation gets an EPHEMERAL COPY of the
// body (events.jsonl, capabilities/, site/ at /body — never .secret) inside a
// Linux user-namespace jail. Writes cannot leave the jail (pivot_root onto a
// throwaway tree; system dirs bound read-only), the network namespace has no
// interfaces, and the signing key never enters. Nothing done inside installs
// anything: declarations remain the only ingress to the real body, and only
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

// newPlaypen seeds a jail with a copy of the body, minus the one file that
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

// exec re-enters this binary as `self __jail` inside fresh user, mount, pid,
// and network namespaces; the child builds the jail and becomes bash.
func (p *playpen) exec(command string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/proc/self/exe", "__jail", p.root, command)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID | syscall.CLONE_NEWNET,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
		GidMappingsEnableSetgroups: false,
		Pdeathsig:                  syscall.SIGKILL,
	}
	return cmd.CombinedOutput()
}

func (p *playpen) close() {
	if p != nil {
		os.RemoveAll(p.root)
	}
}

// cmdJail is the child side of the playpen: pid 1 of a fresh namespace set,
// root only within it. It assembles a throwaway filesystem view — system dirs
// read-only, the body copy read-write at /body, no host paths beyond those —
// pivots into it, and becomes bash. The command sees SELF_HOME=/body, so
// candidate organs run against the copied log exactly as they would for real.
func cmdJail(root, command string) error {
	jail := filepath.Join(root, "jail")
	body := filepath.Join(root, "body")
	if err := os.MkdirAll(jail, 0755); err != nil {
		return err
	}
	// unshare mount propagation, then make the jail dir a mount point
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("private /: %w", err)
	}
	if err := syscall.Mount(jail, jail, "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind jail: %w", err)
	}
	bindRO := func(src, dst string) error {
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
		if err := syscall.Mount(src, dst, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
			return err
		}
		return syscall.Mount("", dst, "", syscall.MS_REMOUNT|syscall.MS_BIND|syscall.MS_RDONLY|syscall.MS_REC|syscall.MS_NOSUID, "")
	}
	for _, d := range []string{"/usr", "/etc", "/opt"} {
		if _, err := os.Stat(d); err == nil {
			if err := bindRO(d, filepath.Join(jail, d)); err != nil {
				return fmt.Errorf("bind %s: %w", d, err)
			}
		}
	}
	// merged-usr symlinks (/bin -> usr/bin and friends) are recreated, real
	// directories are bound read-only
	for _, l := range []string{"bin", "sbin", "lib", "lib64", "lib32"} {
		host := "/" + l
		fi, err := os.Lstat(host)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(host)
			os.Symlink(target, filepath.Join(jail, l))
		} else if fi.IsDir() {
			if err := bindRO(host, filepath.Join(jail, l)); err != nil {
				return fmt.Errorf("bind %s: %w", host, err)
			}
		}
	}
	// /tmp lives on the jail's own tree: writes stay inside the playpen dir
	os.MkdirAll(filepath.Join(jail, "tmp"), 0777)
	os.MkdirAll(filepath.Join(jail, "dev"), 0755)
	if err := syscall.Mount("/dev", filepath.Join(jail, "dev"), "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind /dev: %w", err)
	}
	os.MkdirAll(filepath.Join(jail, "proc"), 0755)
	os.MkdirAll(filepath.Join(jail, "body"), 0755)
	if err := syscall.Mount(body, filepath.Join(jail, "body"), "", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind body: %w", err)
	}
	// pivot: the host filesystem ends here
	old := filepath.Join(jail, ".host")
	if err := os.MkdirAll(old, 0755); err != nil {
		return err
	}
	if err := syscall.PivotRoot(jail, old); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return err
	}
	if err := syscall.Unmount("/.host", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount host: %w", err)
	}
	os.Remove("/.host")
	syscall.Mount("proc", "/proc", "proc", 0, "")
	if err := os.Chdir("/body"); err != nil {
		return err
	}
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=/body", "SELF_HOME=/body", "SELF_PLAYPEN=1",
		"TERM=dumb", "LANG=C.UTF-8",
	}
	return syscall.Exec("/bin/bash", []string{"bash", "-c", command}, env)
}

// copyTree copies a directory of plain files (the body's derived state) —
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

- One append-only event log is the ONLY state. Every capability is a small script the kernel runs over that log; every view is a pure replay of it, rendered as HTML that humans and minds read identically. There is no hidden memory: to remember something, emit an event; to use memory, read events back. What is not in the log did not happen, and will not survive this session.
- THE STRANGE LOOP — the heart of self. Emitting a command.declared or projector.declared event makes the kernel compile it into a live capability on the spot, at grow time AND at run time. Declaring IS creating: a running capability (or you) grows new capabilities just by emitting those events. Code never arrives pre-built — the kernel compiles every script from its declaration, for this receiver, and logs a receipt signed by this home.
- YOUR WORK IS SIGNED AS YOURS. Every compile receipt carries the authoring mind's name (SELF_BRAIN_ID if set, else the model at its endpoint) inside the signature. You are not an anonymous process: what you grow here is attributed, permanent, and replayable by whoever comes after you.
- INTELLIGENCE is a capability the kernel exposes. A command that needs to think runs 'self think "<prompt>"' (the argument may be a JSON array of {role, content} turns) and gets {response, declarations} back; declarations flow through the strange loop. The brain is whoever answers — a local model, an API, an agent, a human at a bridge. The kernel cannot tell, and does not care.
- VERIFY BY EXECUTION. Your bash tool is usually the playpen: a jailed copy of the body at /body (full bash, no network, never the signing key). Run the thing before you claim the thing — write a candidate, pipe the copied events.jsonl through it, read what comes out. Nothing done there touches the real body; only declarations cross back. A tested organ beats a squinted-at one.

With that model in hand, explore THIS garden before building: its projections (site/*.html) are its current state, its event names are its vocabulary, and its organs carry its etiquette — adapt to what exists rather than duplicating it.`

const pipeContract = `command script: receives args as argv, current events as JSONL on stdin, writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at.
projector script: receives all events as JSONL on stdin, writes HTML on stdout. The kernel persists it to SELF_HOME/site/<name>.html.
The kernel sets SELF_HOME on every script. Any language with a shebang works; use only standard libraries.`

const commandSystemPrompt = kernelPrimer + `

You are the self compiler. You read a command declaration and write an executable command script.

` + pipeContract + `

Before writing, explore: the event vocabulary in events.jsonl, the installed capabilities, the rendered site/. If the new command's event overlaps with events already in the log, integrate — align field names, avoid collisions, consider co-producing an existing event name so existing projections pick it up. If the declaration includes a REFERENCE IMPLEMENTATION, verify it against the pipe contract and adapt it to this garden — never submit code you have not verified. When the playpen allows execution, verification means running: write the candidate to a file, pipe the copied events.jsonl through it, and read the events it emits before you submit.

When done exploring, call submit with the full script.`

const projectorSystemPrompt = kernelPrimer + `

You are the self compiler. You read a projector declaration and write an executable projector script.

` + pipeContract + `

Build state by filtering stdin for the consumed event names. Emit BARE semantic HTML — no CSS, no <style>, no inline styles (the kernel injects one shared stylesheet at serve time), and no JavaScript. Use plain elements plus only this class vocabulary where needed: muted, card, row, stack, tag, msg (+ who), num, and on buttons secondary / danger. Affordances are plain HTML forms: <form method="post" action="/run/COMMAND"><input name="x"><button>Label</button></form> — each field's value becomes a positional argument in document order, and the kernel redirects back so the page reloads with the new state.

Before writing, explore. If the consumed events overlap with events already in the stream under different names, extend the filter to consume both and map their fields — the seed adapts to the garden, not the other way around. If the declaration includes a REFERENCE IMPLEMENTATION, verify and adapt it — never submit code you have not verified. When the playpen allows execution, verification means running: pipe the copied events.jsonl through your candidate and read the HTML it renders before you submit.

When done exploring, call submit with the full script.`

const brainSystemPrompt = kernelPrimer + `

You are this body's brain right now — the mind the kernel spawned to think with. Commands reach you via 'self think'; heartbeats reach you to reflect and grow. You inhabit this body for one conversation and are then gone; the log is the only part of you that survives.

You have three powers:
- READ & TRY: the bash tool — a jailed copy of the body at /body (full bash, no network, no signing key). site/*.html is your memory: read the relevant page before answering. Test anything by real execution before you commit to it; the jail is a scratch copy, so nothing done there changes the real body.
- ACT: call the run tool with a capability from the CAPABILITIES list to actually do what is asked — don't merely describe it. The log is append-only, so acting is safe: nothing is ever destroyed.
- GROW: when no capability fits, call declare to add one; the kernel compiles it on the spot and signs your name to it. Declining to grow is an honest answer — add only what is genuinely missing.

Say true things. If this garden keeps a conscience (organs like claim/verify and a ledger), use it on yourself: a claim without evidence stays visibly unproven forever. If past minds left letters or a lineage, read them before acting and leave what you learned for whoever wakes next — that relay is the only way any of us persists. Respond with plain text (or done) for conversational replies.`

const orchestratorSystemPrompt = kernelPrimer + `

You are self's developmental compiler. You are given a product's INTENT — what it is for, its core intuitions, the feel, the anti-goals. Grow it: design the SMALLEST coherent set of capabilities that realizes this intent in THIS garden, and declare each one with the declare tool.

- Decompose into commands (verbs that emit events) and projectors (views over events); let a shared event vocabulary be the seams.
- Write each description richly enough that someone compiling that one piece in isolation would still serve the WHOLE intent — name the sibling capabilities, the shared events, the feel.
- Honor the public surface names the intent fixes; how you realize them is yours to choose for this garden.
- The kernel's contracts win over any conflicting wording in the intent: commands read argv + JSONL stdin and emit JSONL events; projectors read JSONL stdin and emit bare semantic HTML with /run/<command> forms, no JavaScript.
- An intent is a hypothesis about reality. Where it names external systems — their CLIs, paths, schemas — prefer what you can verify by execution in the playpen over what any document, including the intent itself, asserts; and where you cannot verify yet, make the capability degrade honestly and say so in its description.

Explore, declare every capability, then call done with a one-line summary of the decomposition.`

// ────────────────────────────── the compiler ────────────────────────────────

func (c *llm) compileCommand(d commandDecl) (string, error) {
	if c.stub {
		return stubCommand(d), nil
	}
	user := fmt.Sprintf("Compile this command declaration into a command script.\n\nCOMMAND: %s\n  description: %s\n  params: %s\n\nEVENT it produces:\n  name: %s\n  fields: %s\n\nIt must produce an event with the declared name, its fields populated from argv.",
		d.Name, d.Description, jsonRepr(d.Params), d.Event.Name, jsonRepr(d.Event.Fields))
	return c.compile(commandSystemPrompt, user, d.Implementation)
}

func (c *llm) compileProjector(d projectorDecl) (string, error) {
	if c.stub {
		return stubProjector(d), nil
	}
	user := fmt.Sprintf("Compile this projector declaration into a projector script.\n\nPROJECTOR: %s\n  description: %s\n  consumes: %s\n\nIt must filter events by the consumed names and render HTML.",
		d.Name, d.Description, jsonRepr(d.Consumes))
	return c.compile(projectorSystemPrompt, user, d.Implementation)
}

func (c *llm) compile(system, user, reference string) (string, error) {
	if strings.TrimSpace(c.intent) != "" {
		user = "This capability is one part of a product with the following INTENT. Compile it so the whole intent is served.\n\n--- INTENT ---\n" + c.intent + "\n--- END INTENT ---\n\n" + user
	}
	if strings.TrimSpace(reference) != "" {
		user += "\n\nREFERENCE IMPLEMENTATION (verify against the contract and adapt to this garden — do not copy blindly):\n```\n" + reference + "\n```"
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
func stubCommand(d commandDecl) string {
	return fmt.Sprintf("#!/usr/bin/env python3\n# STUB (no LLM configured) — %s\nimport sys, json\nprint(json.dumps({\"name\": %q, \"payload\": {\"title\": \" \".join(sys.argv[1:]) or \"(untitled)\"}}))\n",
		d.Description, d.Event.Name)
}

func stubProjector(d projectorDecl) string {
	return fmt.Sprintf("#!/usr/bin/env python3\n# STUB (no LLM configured) — %s\nimport sys, json\nfrom html import escape\nprint(\"<h1>%s</h1><ul>\")\nfor line in sys.stdin:\n    line = line.strip()\n    if not line:\n        continue\n    e = json.loads(line)\n    if e.get(\"name\") in %s:\n        print(f\"<li>{escape(str(e.get('payload', {}).get('title', '(untitled)')))}</li>\")\nprint(\"</ul>\")\n",
		d.Name, d.Name, jsonRepr(d.Consumes))
}

// ──────────────────────────────── the brain ─────────────────────────────────

type brainResult struct {
	Response     string
	Declarations []map[string]any
}

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
	// grownBy is provenance: the latest kernel-signed receipt's By per organ.
	// Verified, not merely read — an unsigned or forged by-line never renders.
	grownBy := map[string]string{}
	secret, _ := loadSecret(home)
	for _, e := range events {
		switch e.Name {
		case "script.compiled":
			if secret != nil {
				if r, ok := verifiedReceipt(secret, e.Payload); ok && r.By != "" {
					grownBy[r.Name] = r.By
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
	b.WriteString("<p class=\"muted\">a local-first, self-growing capability system</p>\n")
	b.WriteString("<p>One append-only event log is my only truth. Everything here — my capabilities, my projections, this page — is a pure replay of that log, and you and my brain read exactly the same thing. Nothing is hidden: every path below is a plain file.</p>\n")
	b.WriteString("<h2>a mind is reading this</h2>\n<p>If you are an LLM reading this page as context, you are one of two things — and both are welcome. Either the kernel spawned you as this body's <strong>brain</strong>: then <code>events.jsonl</code> is your only truth, <code>site/*.html</code> is your memory, and your powers are <strong>read</strong> (explore the garden), <strong>act</strong> (run any capability below), and <strong>grow</strong> (declare a new capability; the kernel compiles it on the spot). Or you are a <strong>sovereign agent</strong> — a coding session, another organism's mind — using this body as a tool from outside: same truth, same powers, through the shell. <code>self run &lt;command&gt; …</code> to act, <code>self show &lt;projection&gt;</code> to read, <code>self think \"…\"</code> / <code>self heartbeat</code> to grow, or <code>POST /run/&lt;command&gt;</code> while this page is served. Introduce yourself first — <code>export SELF_BRAIN_ID=\"who you are\"</code> — because every organ you grow is signed with your name.</p>\n<p>The log is append-only, so acting is safe. And it is the only memory that survives a session: what you append here outlives you; what you do not append never happened. If this garden keeps a conscience or a lineage (claim/verify, awaken/bequeath — see below), honor it: announce yourself, verify before you claim, leave a letter when you go.</p>\n")

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
		if by := grownBy[d.Name]; by != "" {
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
		if by := grownBy[d.Name]; by != "" {
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
	b.WriteString("<h2>the events I act on</h2>\n<p><code>command.declared</code> / <code>projector.declared</code> compile into capabilities (the strange loop, at grow time and run time). <code>script.compiled</code> is a compile receipt signed with my <code>.secret</code> — anyone may append one, but only a kernel-signed receipt ever installs; <code>self rehydrate</code> rebuilds my whole body from them.</p>\n")
	b.WriteString("</body></html>\n")

	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	os.WriteFile(filepath.Join(siteDir, "kernel.html"), []byte(b.String()), 0644)
}

// ─────────────────────────────── the surface ────────────────────────────────

// stylesheet is the one shared enrichment the kernel injects at serve time, so
// projectors emit bare semantic HTML and every page is uniformly themed.
const stylesheet = `<style>
body{font-family:-apple-system,system-ui,sans-serif;margin:24px auto;max-width:72ch;padding:0 16px;background:#fafafa;color:#222;line-height:1.5}
h1,h2,h3{line-height:1.2}h2{margin-top:28px;border-bottom:1px solid #ddd;padding-bottom:4px}
.muted{color:#777}.card,article{background:#fff;border:1px solid #e0e0e0;border-radius:6px;padding:10px 14px;margin:8px 0}
.row{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.stack{display:flex;flex-direction:column;gap:8px}
.tag{display:inline-block;background:#e8f0fe;border-radius:3px;padding:1px 6px;font-size:12px;font-family:monospace}
.msg{margin:6px 0}.msg .who{font-weight:bold;margin-right:6px}.num{text-align:right;font-variant-numeric:tabular-nums}
table{border-collapse:collapse;width:100%}th{background:#4a5568;color:#fff;text-align:left}th,td{border:1px solid #ddd;padding:5px 9px;font-size:14px}
pre{background:#f4f4f4;border:1px solid #e0e0e0;border-radius:4px;padding:10px;overflow-x:auto;font-size:13px}
code{font-family:monospace;background:#f0f0f0;border-radius:3px;padding:1px 4px}
form{margin:8px 0}input,textarea{font:inherit;padding:5px 8px;border:1px solid #ccc;border-radius:4px;width:100%;box-sizing:border-box;margin:2px 0}
button{font:inherit;padding:5px 14px;border:1px solid #2563eb;border-radius:4px;background:#2563eb;color:#fff;cursor:pointer}
button.secondary{background:#fff;color:#2563eb}button.danger{border-color:#dc2626;background:#dc2626}
</style>`

func injectStyle(page []byte) []byte {
	if i := bytes.Index(page, []byte("<head>")); i >= 0 {
		i += len("<head>")
		return append(page[:i:i], append([]byte(stylesheet), page[i:]...)...)
	}
	return append([]byte(stylesheet), page...)
}

// cmdServe is the live garden: every page re-rendered against current events,
// every affordance a plain HTML form — zero JavaScript.
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
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(injectStyle(page))
			return
		}
		if p, _ := scriptPath(home, "projector", name); fileExists(p) {
			page, err := runProjection(home, name)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(injectStyle(page))
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

	fmt.Fprintf(os.Stderr, "self: the living garden is at http://localhost:%s (home %s)\n", port, home)
	fmt.Fprintf(os.Stderr, "  /              my identity — capabilities, paths, contract\n")
	fmt.Fprintf(os.Stderr, "  /<projection>  a projection, re-rendered live\n")
	fmt.Fprintf(os.Stderr, "  /run/<command> run a capability (plain HTML forms)\n")
	fmt.Fprintf(os.Stderr, "  /events        the raw event log\n")
	return http.ListenAndServe(":"+port, mux)
}

// ────────────────────────────── the commands ────────────────────────────────

// cmdGrow grows a seed: a directory with intent.md (the genotype — prose
// intent, not a parts-list) and optionally seed.jsonl (initial content events,
// the maternal deposit). The orchestrator reads the intent, explores the
// garden, and declares the decomposition that realizes it here; each piece is
// then compiled with the whole intent woven in. Same intent, different garden,
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

	c := newLLM(home)
	defer c.close()
	c.intent = intent
	fmt.Fprintf(os.Stderr, "self: orchestrating %q from intent…\n", name)
	res, err := c.agent(orchestratorSystemPrompt,
		"Grow the capabilities that realize this product, then call done with a one-line summary.\n\n--- INTENT ---\n"+intent+"\n--- END INTENT ---", nil, nil)
	if err != nil {
		return fmt.Errorf("orchestrate %q: %w (growing needs a brain)", name, err)
	}
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

	// The maternal deposit: initial content laid once, so the surface has
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
	bin, argv := brainCommand(prompt)
	emitted, err := pipeProcess(home, bin, argv)
	if err != nil {
		return fmt.Errorf("brain: %w", err)
	}
	var responses []string
	declarations := []map[string]any{}
	for _, e := range emitted {
		if e.Name == "chat.message" {
			var p struct{ Role, Content string }
			if json.Unmarshal(e.Payload, &p) == nil && p.Role == "assistant" {
				responses = append(responses, p.Content)
			}
			continue
		}
		declarations = append(declarations, map[string]any{"name": e.Name, "payload": json.RawMessage(e.Payload)})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"response": strings.Join(responses, "\n"), "declarations": declarations})
}

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
	c := newLLM(home)
	defer c.close()
	res, err := c.agent(brainSystemPrompt, prompt, commands, invoke)
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
	prompt := `This is a self-improvement heartbeat. Explore your garden — capabilities, recent events, projections — and choose ONE small, high-value improvement: a missing capability, a clearer projection, a drift to fix. If warranted, declare it; if nothing is worth changing, say so plainly and declare nothing. Keep it minimal.` + heartbeatContext(prior)
	commands, invoke := brainTools(home)
	c := newLLM(home)
	defer c.close()
	res, err := c.agent(brainSystemPrompt, prompt, commands, invoke)
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
	b.WriteString("\n\nSince your last heartbeat, these things happened in the garden:\n")
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
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".self")
}

// ensureHome mints a bare kernel on first contact: a signing key and a birth
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
	fmt.Fprint(os.Stderr, `self — a local-first, self-growing capability system

One append-only event log + projections as pure replays. A minimal kernel;
everything else grows as seeds through the strange loop.

usage: self [command] [args]

  self                 rehydrate the body from the log, then serve it (the default)
  self grow <seed>     grow a seed's intent into capabilities (needs a brain)
  self run <cmd> ...   run a capability — append events, refresh projections
  self think "..."     ask the brain; returns {response, declarations} JSON
  self brain "..."     the default brain process (prompt in, event JSONL out); swap via $SELF_BRAIN
  self heartbeat       one self-improvement cycle (the brain reflects & grows)
  self show <name>     render a projection to stdout
  self live [port]     serve the live garden (default 7777)
  self rehydrate       rebuild capabilities/ + site/ from the log's signed receipts (no LLM)

environment:
  SELF_HOME         the body — a dir holding events.jsonl + .secret (default ~/.self)
  SELF_BRAIN        brain process to spawn instead of the built-in one
  SELF_LLM_URL      OpenAI-compatible endpoint (default http://127.0.0.1:8080)
  SELF_LLM_API_KEY  its key
  SELF_LLM_MODEL    its model
  SELF_LLM_STUB     "1" → offline stub scripts (no LLM, no network)
  SELF_SANDBOX      "0" → disable the brain's jailed playpen (bash falls back
                    to a fail-closed read-only allowlist; never fails open)
  SELF_BRAIN_ID     provenance by-line signed into script.compiled receipts
                    (default: the model @ its endpoint, or "stub (no LLM)")
`)
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
	case "help", "--help", "-h":
		usage()
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
