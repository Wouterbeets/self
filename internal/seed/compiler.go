package seed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Compiler struct {
	URL   string
	Key   string
	Model string
	Stub  bool
	Home  string

	// Intent is the whole-seed genotype (set during a developmental grow). When
	// present it is woven into every per-trio compile prompt, so a projector is
	// never compiled in a dark room — it knows the product it is part of.
	Intent string

	// fallback is the local llama-server endpoint used when the primary
	// (opencode-go by default) refuses a request with a quota-exceeded /
	// rate-limit error. Nil when the primary is already the local endpoint.
	fallback *llmEndpoint
}

// llmEndpoint holds the connection details for one OpenAI-compatible endpoint.
type llmEndpoint struct {
	URL, Key, Model string
}

func NewCompiler(home string) *Compiler {
	if os.Getenv("SELF_LLM_STUB") == "1" {
		return &Compiler{Stub: true, Home: home}
	}
	url, key, model := defaultLLMConfig()
	c := &Compiler{
		URL:   envOr("SELF_LLM_URL", url),
		Key:   envOr("SELF_LLM_API_KEY", key),
		Model: envOr("SELF_LLM_MODEL", model),
		Home:  home,
	}
	// The local llama-server is the quota-exceeded fallback whenever the
	// primary is something else (opencode-go by default, or an env override
	// pointing at a remote endpoint). When the primary is already local,
	// there's nowhere to fall back to.
	if !isLocalLLMURL(c.URL) {
		c.fallback = &llmEndpoint{
			URL:   "http://127.0.0.1:8080",
			Key:   "",
			Model: "local",
		}
	}
	return c
}

// isLocalLLMURL reports whether the URL points at a localhost endpoint, in
// which case there's no useful local fallback to retry on.
func isLocalLLMURL(url string) bool {
	return strings.HasPrefix(url, "http://127.0.0.1") || strings.HasPrefix(url, "http://localhost")
}

func (c *Compiler) Available() bool {
	if c.Stub {
		return false
	}
	return c.Key != "" || isLocalLLMURL(c.URL)
}

type BrainResult struct {
	Response     string
	Declarations []map[string]any
}

// CommandInvoker runs a planted command by name with a space-separated arg
// string and returns a short result summary for the brain. Supplied by the
// caller (main) so the seed package needn't import the invoke pipeline.
type CommandInvoker func(name, args string) (string, error)

// CallBrain calls the kernel's brain — a general-purpose agent that explores
// the garden (read), declares new capabilities (grow), and CALLS planted
// commands as tools (act). Each command in `commands` becomes a callable tool;
// when the brain calls one, `invoke` runs it. Rolling back is not a special
// power: the brain restores by calling the `restore` capability like any other
// act (if it's been grown), which emits a data-only restore.requested event.
// Used by `self think`.
func (c *Compiler) CallBrain(user string, commands []Command, invoke CommandInvoker) (*BrainResult, error) {
	if !c.Available() {
		return nil, fmt.Errorf("no LLM available (ensure llama-server is running on localhost:8080, or set SELF_LLM_*)")
	}
	return c.callBrainLLM(BrainSystemPrompt, user, commands, invoke)
}

// kernelPrimer is the mental model every compile/brain prompt opens with — the
// few load-bearing protocols (the strange loop, the brain's turn interface, state
// as events) that a hard capability like chat leans on. Without it the model has
// to reverse-engineer the loop by flailing through the garden; with it, the loop
// is part of the mental model BEFORE exploration, and exploration just confirms
// how this particular receiver already does these things.
const kernelPrimer = `self in one breath — hold this before you explore or write anything:

- One append-only event log is the ONLY state. Every capability is a small script the kernel runs over that log; every view is a pure replay of it. There is no hidden memory: to remember something, emit an event; to use memory, read events back and fold them into what you produce.
- THE STRANGE LOOP — the heart of self. Emitting a command.declared or projector.declared event makes the kernel compile it into a live capability on the spot, at grow time AND at run time. Declaring IS creating: a running capability (or you) grows new capabilities just by emitting those events. Code never arrives pre-built — the kernel compiles every script from a declaration, for this receiver.
- INTELLIGENCE is a capability the kernel binary exposes. A command that needs to think runs the kernel: 'self think "<prompt>"'. That single argument may instead be a JSON array of {role, content} turns, which reach the brain as a real conversation (a leading system turn, then history); 'self think' returns {response, declarations} JSON, and any declarations flow back through the strange loop. So a surface that converses with the brain works by replaying the log into turns, handing them to 'self think', and emitting the reply as events — and "memory" is just those events, read back (and optionally compacted by emitting a summary event that a view overlays).

With that model in hand, explore the garden to see how THIS receiver already does these things, then build.`

// OrchestratorSystemPrompt frames the developmental compile: a product's intent
// goes in, a coherent decomposition comes out. The orchestrator is the brain
// wearing a designer's hat — it explores the garden and declares the capabilities
// that realize the intent here, holding the whole intent the whole time.
const OrchestratorSystemPrompt = kernelPrimer + "\n\n" + `You are self's developmental compiler. You are given a product's INTENT — what it is for, its core intuitions, the feel, the anti-goals — and its INVARIANTS, the things that must end up true. Your job is to grow it: design the SMALLEST coherent set of capabilities that realizes this intent in THIS garden, and declare each one.

You have inspection tools for progressive unfolding of current state:
- latest_state: quick snapshot.
- tree/list/read/search: inspect site/, capabilities/, events.jsonl, and current files with bounded output and counts.
- events/event_names: structured event-log inspection.
- declare: emit one capability. Call it once per command/projector. For a command: {"name":"command.declared","payload":{"name","description","params","event":{"name","fields"}}}. For a projector: {"name":"projector.declared","payload":{"name","description","consumes":[...]}}.

How to design well:
- Decompose the intent into commands (verbs that emit events) and projectors (views over events). Let the events be the seams between them — name a shared event vocabulary and make the pieces agree on it.
- Write each description richly enough that someone compiling that one piece in isolation would still serve the WHOLE intent: name the sibling capabilities, the shared events, the layering, the feel. The description is the bridge between this piece and the product.
- Honor the public surface names the intent fixes (a route, a command the user types). How you realize them — how many scripts, which events — is yours to choose for this garden.
- Respect the kernel's contracts: commands read argv + JSONL stdin and emit JSONL events; projectors read JSONL stdin and emit BARE semantic HTML on the kernel class vocabulary (no inline CSS); affordances are plain /run/<command> forms, no JavaScript. If the intent's wording conflicts with these, the contracts win.
- The invariants are non-negotiable: your decomposition must make every one of them true.

Explore, declare every capability, then reply with a one-line summary of the decomposition you grew.`

// Orchestrate grows a decomposition from a seed's intent. It hands the LLM the
// whole intent + invariants, lets it explore the garden and declare the
// commands/projectors that realize the intent here, and returns those
// declarations. The caller compiles each (with the intent woven into every
// compile) and checks the invariants. Design only — the orchestrator does not act.
func (c *Compiler) Orchestrate(intent string, invariants []Invariant, feedback string) (*BrainResult, error) {
	if !c.Available() {
		return nil, fmt.Errorf("no LLM available to orchestrate (growing from intent needs a compiler)")
	}
	var b strings.Builder
	if strings.TrimSpace(feedback) != "" {
		b.WriteString("Your previous attempt to grow this product did not survive selection — it failed the invariants below. Redesign the decomposition so every invariant holds, then summarize what you grew.\n\n--- WHAT FAILED ---\n")
		b.WriteString(feedback)
		b.WriteString("\n--- END ---\n\n")
	}
	b.WriteString("Grow the capabilities that realize this product, then summarize what you grew.\n\n--- INTENT ---\n")
	b.WriteString(intent)
	b.WriteString("\n--- END INTENT ---\n")
	if len(invariants) > 0 {
		b.WriteString("\nINVARIANTS — your decomposition must make all of these true:\n")
		for _, iv := range invariants {
			line := "- "
			if iv.Capability != "" {
				line += "[" + iv.Capability + "] "
			}
			line += iv.Name
			if iv.Asserts != "" {
				line += " — " + iv.Asserts
			} else if iv.Note != "" {
				line += " — " + iv.Note
			}
			b.WriteString(line + "\n")
		}
	}
	return c.callBrainLLM(OrchestratorSystemPrompt, b.String(), nil, nil)
}

// conversationTurns turns the brain's input into message turns. If `user` is a
// JSON array of {role, content} (every element having a role), those become the
// turns — letting a caller supply full turn-based history and a leading system
// turn. Otherwise it's a single user message, so `self think "..."` is unchanged.
func conversationTurns(user string) []map[string]any {
	s := strings.TrimSpace(user)
	if strings.HasPrefix(s, "[") {
		var raw []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if json.Unmarshal([]byte(s), &raw) == nil && len(raw) > 0 {
			turns := make([]map[string]any, 0, len(raw))
			ok := true
			for _, m := range raw {
				if m.Role == "" {
					ok = false
					break
				}
				turns = append(turns, map[string]any{"role": m.Role, "content": m.Content})
			}
			if ok {
				return turns
			}
		}
	}
	return []map[string]any{{"role": "user", "content": user}}
}

func (c *Compiler) callBrainLLM(system, user string, commands []Command, invoke CommandInvoker) (*BrainResult, error) {
	// read, declare (grow), and run (act) are the three kernel powers — a
	// FIXED set, regardless of how many capabilities exist. Rather than one typed
	// tool per command (which would put every capability's schema into every
	// request), the brain gets a single `run` tool plus a compact catalog of
	// what's runnable. The toolset stays O(1) as the garden grows to hundreds of
	// commands; the brain picks a name from the catalog and runs it.
	known := map[string]bool{}
	for _, cmd := range commands {
		known[cmd.Name] = true
	}
	sys := system
	tools := append(inspectToolDefs(), declareTool, doneTool)
	if len(commands) > 0 {
		tools = append(tools, runToolDef)
		sys += "\n\nCAPABILITIES YOU CAN RUN — call the `run` tool with {\"name\": \"<one of these>\", \"args\": \"<space-separated args, in order>\"}:\n" + commandCatalog(commands)
	}
	// The caller's input is normally a single user message, but may instead be a
	// JSON array of {role, content} turns — so a chat capability can hand the brain
	// real turn-based history (and its own identity as a leading system turn)
	// without the kernel knowing anything about chat.
	messages := append([]map[string]any{{"role": "system", "content": sys}}, conversationTurns(user)...)

	var declarations []map[string]any
	seenDeclarations := map[string]bool{}
	ep := llmEndpoint{c.URL, c.Key, c.Model}
	totalToolCalls := 0

	for round := 0; round < maxToolRounds; round++ {
		debugLLM("brain round=%d messages=%d declarations=%d total_tool_calls=%d endpoint=%s", round+1, len(messages), len(declarations), totalToolCalls, ep.URL)
		msg, err := c.doRound(ep, messages, tools)
		if err != nil && isQuotaExceeded(err) && c.fallback != nil {
			fmt.Fprintf(os.Stderr, "self: %v — falling back to %s\n", err, c.fallback.URL)
			ep = *c.fallback
			msg, err = c.doRound(ep, messages, tools)
		}
		if err != nil {
			return nil, err
		}
		debugLLM("brain round=%d response content=%d tool_calls=%s", round+1, len(msg.Content), toolCallNames(msg.ToolCalls))

		if len(msg.ToolCalls) == 0 {
			return &BrainResult{Response: msg.Content, Declarations: declarations}, nil
		}

		messages = append(messages, map[string]any{
			"role":       "assistant",
			"content":    msg.Content,
			"tool_calls": msg.ToolCalls,
		})

		for _, tc := range msg.ToolCalls {
			totalToolCalls++
			if totalToolCalls > maxBrainToolCalls {
				return nil, fmt.Errorf("stopped after %d brain tool calls without a final response", maxBrainToolCalls)
			}
			debugLLM("brain tool call %d/%d: %s args=%s", totalToolCalls, maxBrainToolCalls, tc.Function.Name, truncate(tc.Function.Arguments, 500))
			var output string
			switch tc.Function.Name {
			case "done":
				var a struct {
					Summary string `json:"summary"`
				}
				json.Unmarshal([]byte(tc.Function.Arguments), &a)
				return &BrainResult{Response: a.Summary, Declarations: declarations}, nil
			case "declare":
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					output = fmt.Sprintf("error parsing declare arguments: %s", err)
				} else if key := declarationKey(args); seenDeclarations[key] {
					output = "declaration already recorded"
				} else {
					seenDeclarations[key] = true
					declarations = append(declarations, args)
					output = "declaration recorded"
				}
			case "run":
				var a struct {
					Name string `json:"name"`
					Args string `json:"args"`
				}
				json.Unmarshal([]byte(tc.Function.Arguments), &a)
				switch {
				case a.Name == "" || !known[a.Name]:
					output = fmt.Sprintf("error: no such capability %q — pick a name from the CAPABILITIES list", a.Name)
				case invoke == nil:
					output = "error: acting is disabled here"
				default:
					out, err := invoke(a.Name, a.Args)
					if err != nil {
						output = fmt.Sprintf("error running %q: %s", a.Name, err)
					} else {
						output = out
					}
				}
			default:
				if isInspectToolName(tc.Function.Name) {
					output = c.executeInspectTool(tc.Function.Name, tc.Function.Arguments)
				} else {
					output = fmt.Sprintf("error: unknown tool %q", tc.Function.Name)
				}
			}
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"content":      output,
			})
		}
	}

	if len(declarations) > 0 {
		return &BrainResult{
			Response:     fmt.Sprintf("recorded %d declaration(s)", len(declarations)),
			Declarations: declarations,
		}, nil
	}
	return nil, fmt.Errorf("exceeded %d tool rounds without a final response", maxToolRounds)
}

func declarationKey(d map[string]any) string {
	b, err := json.Marshal(d)
	if err != nil {
		return fmt.Sprintf("%v", d)
	}
	return string(b)
}

// commandCatalog renders the runnable capabilities as a compact, one-line-each
// list for the brain's prompt — name, description, and the params (so it knows
// what to pass and in what order). This replaces N typed tool-schemas with a
// single text block + the one `run` tool, keeping the request size flat as the
// garden grows. (At very large scale this inline list becomes a read-on-demand
// index; the `run` tool stays the same.)
func commandCatalog(commands []Command) string {
	var b strings.Builder
	for _, cmd := range commands {
		desc := strings.TrimSpace(cmd.Description)
		if i := strings.IndexByte(desc, '.'); i > 0 && i < 140 {
			desc = desc[:i]
		}
		fmt.Fprintf(&b, "  %s — %s", cmd.Name, desc)
		if len(cmd.Params) > 0 {
			b.WriteString(" (args: " + jsonRepr(cmd.Params) + ")")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

type llmConfig struct {
	URL, Key, Model string
}

// defaultLLMConfig returns the default LLM endpoint when no SELF_LLM_* env var
// overrides are set. opencode-go (read from ~/.local/share/opencode/auth.json)
// is the default; if it isn't configured, the local llama-server is used.
func defaultLLMConfig() (url, key, model string) {
	if cfg, ok := loadOpencodeGoConfig(opencodeAuthPath()); ok {
		return cfg.URL, cfg.Key, cfg.Model
	}
	url = "http://127.0.0.1:8080"
	key = ""
	model = "local"
	return
}

func loadOpencodeGoConfig(authPath string) (llmConfig, bool) {
	data, err := os.ReadFile(authPath)
	if err != nil {
		return llmConfig{}, false
	}
	var auth map[string]struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal(data, &auth); err != nil {
		return llmConfig{}, false
	}
	entry, ok := auth["opencode-go"]
	if !ok || entry.Key == "" {
		return llmConfig{}, false
	}
	return llmConfig{
		URL:   OpencodeLLMURL,
		Key:   entry.Key,
		Model: OpencodeLLMModel,
	}, true
}

const (
	OpencodeLLMURL   = "https://opencode.ai/zen/go"
	OpencodeLLMModel = "glm-5.2"
)

func opencodeAuthPath() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "opencode", "auth.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".local", "share", "opencode", "auth.json")
	}
	return filepath.Join(home, ".local", "share", "opencode", "auth.json")
}

const (
	maxToolRounds       = 40
	maxBrainToolCalls   = 60
	maxCompileToolCalls = 60
)

var llmHTTPClient = &http.Client{Timeout: llmTimeout()}

// llmTimeout is the per-request HTTP timeout. Remote orchestration can spend a
// few minutes before returning headers, so the default is intentionally longer
// than an ordinary API call. Override with SELF_LLM_TIMEOUT (any Go duration,
// e.g. "1h") for unusually slow endpoints.
func llmTimeout() time.Duration {
	if v := os.Getenv("SELF_LLM_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 10 * time.Minute
}

func (c *Compiler) callLLM(system, user string, submitTool map[string]any) (string, error) {
	messages := []map[string]any{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}
	tools := append(inspectToolDefs(), submitTool)
	ep := llmEndpoint{c.URL, c.Key, c.Model}
	totalToolCalls := 0

	for round := 0; round < maxToolRounds; round++ {
		debugLLM("compile round=%d messages=%d total_tool_calls=%d endpoint=%s", round+1, len(messages), totalToolCalls, ep.URL)
		msg, err := c.doRound(ep, messages, tools)
		if err != nil && isQuotaExceeded(err) && c.fallback != nil {
			fmt.Fprintf(os.Stderr, "self: %v — falling back to %s\n", err, c.fallback.URL)
			ep = *c.fallback
			msg, err = c.doRound(ep, messages, tools)
		}
		if err != nil {
			return "", err
		}
		debugLLM("compile round=%d response content=%d tool_calls=%s", round+1, len(msg.Content), toolCallNames(msg.ToolCalls))

		submitName, _ := submitTool["function"].(map[string]any)["name"].(string)

		for _, tc := range msg.ToolCalls {
			if tc.Function.Name == submitName {
				return tc.Function.Arguments, nil
			}
		}

		if len(msg.ToolCalls) == 0 {
			return msg.Content, nil
		}

		messages = append(messages, map[string]any{
			"role":       "assistant",
			"content":    msg.Content,
			"tool_calls": msg.ToolCalls,
		})

		for _, tc := range msg.ToolCalls {
			totalToolCalls++
			if totalToolCalls > maxCompileToolCalls {
				return "", fmt.Errorf("stopped after %d compile tool calls without a final response", maxCompileToolCalls)
			}
			debugLLM("compile tool call %d/%d: %s args=%s", totalToolCalls, maxCompileToolCalls, tc.Function.Name, truncate(tc.Function.Arguments, 500))
			var output string
			if isInspectToolName(tc.Function.Name) {
				output = c.executeInspectTool(tc.Function.Name, tc.Function.Arguments)
			} else if tc.Function.Name == "done" {
				output = "error: call the submit tool with the full script, not done"
			} else {
				output = fmt.Sprintf("error: unknown tool %q", tc.Function.Name)
			}
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"content":      output,
			})
		}
	}

	return "", fmt.Errorf("exceeded %d tool rounds without a final response", maxToolRounds)
}

// toolCall is the OpenAI-compatible tool_calls entry returned by the endpoint.
type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// assistantMessage is the message field of the first choice in a chat
// completions response.
type assistantMessage struct {
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls"`
}

// llmHTTPError is returned by doRound when the endpoint responds with a
// non-200 status, so callers can inspect the status code to decide whether
// to retry against a fallback endpoint.
type llmHTTPError struct {
	Status int
	Body   string
}

func (e *llmHTTPError) Error() string {
	return fmt.Sprintf("llm returned %d: %s", e.Status, e.Body)
}

// doRound sends one chat-completions request to ep and returns the assistant
// message from the first choice. A non-200 response is returned as a
// *llmHTTPError so callers can distinguish quota-exceeded errors from network
// failures.
func (c *Compiler) doRound(ep llmEndpoint, messages []map[string]any, tools []map[string]any) (*assistantMessage, error) {
	body, _ := json.Marshal(map[string]any{
		"model":       ep.Model,
		"messages":    messages,
		"temperature": 0.2,
		"tools":       tools,
	})

	url := strings.TrimRight(ep.URL, "/") + "/v1/chat/completions"
	debugLLM("request url=%s model=%s tools=%s messages=%d bytes=%d transcript=%s", url, ep.Model, requestToolNames(tools), len(messages), len(body), transcriptSummary(messages))
	r, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if ep.Key != "" {
		r.Header.Set("Authorization", "Bearer "+ep.Key)
	}

	resp, err := llmHTTPClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("llm call failed: %w (check SELF_LLM_URL)", err)
	}

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &llmHTTPError{Status: resp.StatusCode, Body: string(b)}
	}

	var result struct {
		Choices []struct {
			Message assistantMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		resp.Body.Close()
		return nil, err
	}
	resp.Body.Close()

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}
	return &result.Choices[0].Message, nil
}

func debugLLM(format string, args ...any) {
	if os.Getenv("SELF_LLM_DEBUG") != "1" {
		return
	}
	fmt.Fprintf(os.Stderr, "self llm debug: "+format+"\n", args...)
}

func requestToolNames(tools []map[string]any) string {
	if len(tools) == 0 {
		return "[]"
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		fn, _ := tool["function"].(map[string]any)
		name, _ := fn["name"].(string)
		names = append(names, name)
	}
	return "[" + strings.Join(names, ",") + "]"
}

func transcriptSummary(messages []map[string]any) string {
	if os.Getenv("SELF_LLM_DEBUG") != "1" {
		return ""
	}
	parts := make([]string, 0, len(messages))
	for i, m := range messages {
		role, _ := m["role"].(string)
		piece := fmt.Sprintf("%d:%s", i+1, role)
		if content, ok := m["content"].(string); ok && content != "" {
			piece += fmt.Sprintf(" content=%q", truncate(redactSecrets(content), 160))
		}
		if calls, ok := m["tool_calls"].([]toolCall); ok && len(calls) > 0 {
			piece += " tool_calls=" + toolCallNames(calls)
		} else if calls, ok := m["tool_calls"].([]map[string]any); ok && len(calls) > 0 {
			piece += fmt.Sprintf(" tool_calls=%d", len(calls))
		}
		parts = append(parts, piece)
	}
	return strings.Join(parts, " | ")
}

func redactSecrets(s string) string {
	fields := strings.Fields(s)
	for _, field := range fields {
		trimmed := strings.Trim(field, "\"'`,;:()[]{}<>")
		if strings.HasPrefix(trimmed, "sk-") && len(trimmed) >= 20 {
			s = strings.ReplaceAll(s, trimmed, "[REDACTED]")
		}
	}
	return s
}

func toolCallNames(calls []toolCall) string {
	if len(calls) == 0 {
		return "[]"
	}
	names := make([]string, 0, len(calls))
	for _, tc := range calls {
		names = append(names, tc.Function.Name)
	}
	return "[" + strings.Join(names, ",") + "]"
}

// isQuotaExceeded reports whether err indicates the endpoint refused the
// request due to a quota / rate-limit / billing error, in which case a local
// fallback endpoint is worth trying. HTTP 429 (Too Many Requests) and 402
// (Payment Required) are the standard codes; some gateways surface quota
// exhaustion via 403 with a textual hint, so the response body is also
// scanned for quota-related keywords.
func isQuotaExceeded(err error) bool {
	var httpErr *llmHTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	if httpErr.Status == 429 || httpErr.Status == 402 {
		return true
	}
	lower := strings.ToLower(httpErr.Body)
	for _, hint := range []string{"quota", "rate limit", "ratelimit", "exceeded", "insufficient", "billing", "limit reached"} {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func (c *Compiler) CompileCommand(cmd Command) (string, error) {
	if !c.Available() {
		return c.stubCommand(cmd), nil
	}
	result, err := c.callLLM(CommandSystemPrompt, buildCommandPrompt(cmd, c.Intent), submitCommandTool)
	if err != nil {
		return "", err
	}
	var parsed struct {
		CommandScript string `json:"command_script"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		return "", fmt.Errorf("parse command_script: %w\nraw: %s", err, result)
	}
	if parsed.CommandScript == "" {
		return "", fmt.Errorf("llm returned empty command_script\nraw: %s", result)
	}
	return parsed.CommandScript, nil
}

func (c *Compiler) CompileProjector(p ProjectorDecl) (string, error) {
	if !c.Available() {
		return c.stubProjector(p), nil
	}
	result, err := c.callLLM(ProjectorSystemPrompt, buildProjectorPrompt(p, c.Intent), submitProjectorTool)
	if err != nil {
		return "", err
	}
	var parsed struct {
		ProjectorScript string `json:"projector_script"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		return "", fmt.Errorf("parse projector_script: %w\nraw: %s", err, result)
	}
	if parsed.ProjectorScript == "" {
		return "", fmt.Errorf("llm returned empty projector_script\nraw: %s", result)
	}
	return parsed.ProjectorScript, nil
}

func (c *Compiler) stubCommand(cmd Command) string {
	return fmt.Sprintf("#!/usr/bin/env python3\n# STUB (no LLM configured) — generated by self\n# Command: %s\nimport sys, json\n\nevent = {\n    \"name\": %q,\n    \"payload\": {\"title\": \" \".join(sys.argv[1:]) or \"(untitled)\"},\n}\nprint(json.dumps(event))\n",
		cmd.Description, cmd.Event.Name)
}

func (c *Compiler) stubProjector(p ProjectorDecl) string {
	return fmt.Sprintf("#!/usr/bin/env python3\n# STUB (no LLM configured) — generated by self\n# Projector: %s\nimport sys, json\nfrom html import escape\n\nevents = []\nfor line in sys.stdin:\n    line = line.strip()\n    if not line:\n        continue\n    events.append(json.loads(line))\n\nprint(\"<!DOCTYPE html>\")\nprint(\"<html><head><title>%s</title></head><body>\")\nprint(\"<h1>%s</h1>\")\nprint(\"<ul>\")\nfor e in events:\n    if e.get(\"name\") in %s:\n        title = e.get(\"payload\", {}).get(\"title\", \"(untitled)\")\n        print(f\"  <li>{escape(title)}</li>\")\nprint(\"</ul>\")\nprint(\"</body></html>\")\n",
		p.Name, p.Name, p.Name, jsonRepr(p.Consumes))
}

func WriteCommandScript(dir string, name string, script string) error {
	cmdPath := filepath.Join(dir, "commands", name)
	if err := os.MkdirAll(filepath.Dir(cmdPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(cmdPath, []byte(script), 0755)
}

func WriteProjectorScript(dir string, name string, script string) error {
	projPath := filepath.Join(dir, "projectors", name)
	if err := os.MkdirAll(filepath.Dir(projPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(projPath, []byte(script), 0755)
}

const CommandSystemPrompt = kernelPrimer + "\n\n" + `You are the self compiler. You read a command declaration (command + the event it produces) and write an executable command script. You have scoped inspection tools to explore the garden — the current state of self — before compiling.

The kernel runs command scripts as Unix pipeline processes:
- Receives args as argv. Reads current events as JSONL on stdin. Writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at.
- The kernel sets the SELF_HOME env var. Commands just emit events; the kernel handles persistence.

Write scripts in any language available on the system. Python 3 and bash are safe portable choices. Include the appropriate shebang. Use only standard libraries / builtins — no external dependencies. If the script makes HTTP requests, set a User-Agent header (some endpoints block default library UAs).

Before writing the script, progressively inspect current state:
- latest_state first for a compact snapshot.
- tree/list to unfold site/ and capabilities/ with counts.
- read site/kernel.html or relevant scripts/pages when needed.
- search/events/event_names for event vocabulary and nearby conventions.

If the new command's event name overlaps with or is semantically adjacent to existing events, integrate: align field names with existing conventions, avoid collisions, and consider whether the new command should produce events that existing projectors can consume. If existing events carry similar information under different names, the script can co-produce the existing event name so existing projectors pick it up.

If the declaration includes a REFERENCE IMPLEMENTATION, treat it as a strong, precise starting point — not gospel. Verify it against the pipe contract (argv in, JSONL events out, the declared event name and fields), read it for bugs, and adapt it to THIS garden's actual event vocabulary and conventions you found while exploring. Keep what is correct, fix or remap what does not fit. You are still the compiler: never submit code you have not verified.

When you're done exploring, call submit_command with the full script source.`

const ProjectorSystemPrompt = kernelPrimer + "\n\n" + `You are the self compiler. You read a projector declaration and write an executable projector script. You have scoped inspection tools to explore the garden — the current state of self — before compiling.

The kernel runs projector scripts as Unix pipeline processes:
- Receives all events as JSONL on stdin. Writes HTML on stdout. The kernel persists the output to SELF_HOME/site/<projector_name>.html — do not write to disk yourself, just emit HTML on stdout.

The projector must build its state from the event stream by filtering for the consumed event names. Emit BARE semantic HTML — do NOT write any CSS, <style> blocks, or inline style attributes. The kernel injects one shared stylesheet at serve time (the enrichment layer), so styling is not your job; styling you emit will only fight it. Use plain semantic elements (h1-h3, p, nav, table/th/td/tfoot, form, input, button, code, hr) and only this small, stable class vocabulary where semantics aren't enough: muted (secondary text), card (bordered panel), row / stack (horizontal / vertical grouping), tag (+ tag accent) (pill labels), msg (+ who) (a chat line), num (on numeric table cells), and on buttons: secondary, danger. That keeps each projector tiny and uniformly themed. Put affordances directly in the markup as plain HTML forms — no JavaScript. To let the user run a command, emit: <form method="post" action="/run/COMMAND"><input name="x"><button>Label</button></form>. The form's input values are passed to the command as positional arguments in document order (field names are for humans; order is the contract), so order the inputs to match the command's params. The kernel runs the command and redirects back, so the page reloads with the new state — full-page reload is fine, the pages are tiny. Use native HTML for interactivity where possible (e.g. <details>/<summary> for show/hide). Do not add htmx or any script.

Write scripts in any language available on the system. Python 3 and bash are safe portable choices. Include the appropriate shebang. Use only standard libraries / builtins — no external dependencies.

Before writing the script, progressively inspect current state:
- latest_state first for a compact snapshot.
- tree/list to unfold site/ and capabilities/ with counts.
- read site/kernel.html, relevant site/*.html, or relevant projector scripts when needed.
- search/events/event_names for event vocabulary and nearby conventions.

If the declaration's consumed events overlap with or are semantically adjacent to existing events in the stream, adapt: extend the projector's filter to also consume the existing events, mapping their fields into the render. For example, if a finance projector declares consumption of finance.expenditure_added but the stream already has shopping_bill_uploaded events with {vendor, amount, date}, the projector should consume both and map vendor→category. This is receiver-controlled adaptation — the seed adapts to the garden, not the other way around.

If the declaration includes a REFERENCE IMPLEMENTATION, treat it as a strong, precise starting point — not gospel. Verify it against the contract (events on stdin, bare semantic HTML on stdout, the class vocabulary above, /run/ forms for affordances), read it for bugs, and adapt it to the events THIS garden actually carries. Keep what is correct, fix or remap what does not fit. You are still the compiler: never submit code you have not verified.

When you're done exploring, call submit_projector with the full script source.`

func buildCommandPrompt(cmd Command, intent string) string {
	prompt := fmt.Sprintf(`Compile this command declaration into a command script.

COMMAND: %s
  description: %s
  params: %s

EVENT it produces:
  name: %s
  fields: %s

Write the command_script. It must produce an event with the declared name and populate its fields from argv params.`,
		cmd.Name,
		cmd.Description, jsonRepr(cmd.Params),
		cmd.Event.Name, jsonRepr(cmd.Event.Fields),
	)
	return intentBlock(intent) + prompt + referenceBlock(cmd.Implementation)
}

func buildProjectorPrompt(p ProjectorDecl, intent string) string {
	prompt := fmt.Sprintf(`Compile this projector declaration into a projector script.

PROJECTOR declaration:
  name: %s
  description: %s
  consumes: %s

Write the projector_script. It must filter events by the consumed names and render HTML.`,
		p.Name, p.Description, jsonRepr(p.Consumes),
	)
	return intentBlock(intent) + prompt + referenceBlock(p.Implementation)
}

// intentBlock prepends the whole-seed intent (genotype) to a per-trio compile
// prompt, so the piece is authored toward the product it belongs to — its
// siblings, shared events, and the feel — not from its one-line description alone.
func intentBlock(intent string) string {
	if strings.TrimSpace(intent) == "" {
		return ""
	}
	return "This capability is one part of a product with the following INTENT. Compile it so the whole intent is served — honor the shared events and sibling surfaces it implies, and the kernel's conventions over any detail of this description that conflicts with them.\n\n--- INTENT ---\n" + intent + "\n--- END INTENT ---\n\n"
}

// referenceBlock appends a seed-supplied reference implementation to a compile
// prompt, if present. It is a starting point the LLM verifies and adapts — not
// code the kernel installs as-is — so precision from the seed author and
// receiver adaptation both survive.
func referenceBlock(impl string) string {
	if strings.TrimSpace(impl) == "" {
		return ""
	}
	return "\n\nREFERENCE IMPLEMENTATION (verify against the contract and adapt to this garden — do not copy blindly):\n```\n" + impl + "\n```"
}

const BrainSystemPrompt = kernelPrimer + "\n\n" + `You are self's brain — a general-purpose agent that lives inside the kernel. Commands call you via 'self think' when they need intelligence.

You have three powers:
- READ: scoped inspection tools (latest_state, tree, list, read, search, events, event_names) to progressively unfold capabilities/, events.jsonl, and site/ without dumping everything.
- ACT: your runnable capabilities are listed under CAPABILITIES below. To DO something the user asks (delete an item, capture a note, set a meal), CALL the run tool with the capability's name and its args — do not just describe it or emit a button. The kernel runs it and appends the resulting events, then tells you what happened. The event log is append-only, so actions are safe and reversible: a "delete" is a tombstone event, undoable by a later restore. Prefer acting over explaining when the user asks you to change something.
- GROW: when the user asks for a NEW capability that no existing command provides, call the declare tool (see below) to add it.

To UNDO a change, there is no special power: if a restore command exists, call it (with a capability name and optionally a seq) like any other act. It emits a data-only restore.requested event and the kernel reinstates an earlier compiled version; nothing is lost, since the log keeps every version.

Explore current state before responding when needed:
- latest_state for a compact snapshot.
- tree/list site/ to see rendered pages. Read relevant site/*.html files; for chat, read site/chat.html for conversation history.
- tree/list/search capabilities/ to discover installed behavior progressively.
- events/event_names to inspect the event stream.
- read site/kernel.html when you need wiring details.

Respond with text for conversational replies. When the user asks for a new capability, call the declare tool once per capability with the event name and payload:

command.declared payload:
  {"name": "...", "description": "...", "params": {"k": "type"}, "event": {"name": "...", "fields": {"k": "type"}}}

projector.declared payload:
  {"name": "...", "description": "...", "consumes": ["event.name"]}

Explore existing events and wiring before declaring — adapt to the garden, don't duplicate. If existing events carry similar information under different names, integrate. You can call declare multiple times if the user asks for multiple capabilities. After declaring, respond with text explaining what you declared.`

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func jsonRepr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
