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
// when the brain calls one, `invoke` runs it and the result is fed back.
// Used by `self think`, which commands call for LLM access.
func (c *Compiler) CallBrain(user string, commands []Command, invoke CommandInvoker) (*BrainResult, error) {
	if !c.Available() {
		return nil, fmt.Errorf("no LLM available (ensure llama-server is running on localhost:8080, or set SELF_LLM_*)")
	}
	return c.callBrainLLM(BrainSystemPrompt, user, commands, invoke)
}

func (c *Compiler) callBrainLLM(system, user string, commands []Command, invoke CommandInvoker) (*BrainResult, error) {
	messages := []map[string]any{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}
	tools := []map[string]any{bashToolDef, declareTool}
	isCommand := map[string]bool{}
	for _, cmd := range commands {
		tools = append(tools, commandToolDef(cmd))
		isCommand[cmd.Name] = true
	}

	var declarations []map[string]any
	ep := llmEndpoint{c.URL, c.Key, c.Model}

	for round := 0; round < maxToolRounds; round++ {
		msg, err := c.doRound(ep, messages, tools)
		if err != nil && isQuotaExceeded(err) && c.fallback != nil {
			fmt.Fprintf(os.Stderr, "self: %v — falling back to %s\n", err, c.fallback.URL)
			ep = *c.fallback
			msg, err = c.doRound(ep, messages, tools)
		}
		if err != nil {
			return nil, err
		}

		if len(msg.ToolCalls) == 0 {
			return &BrainResult{Response: msg.Content, Declarations: declarations}, nil
		}

		messages = append(messages, map[string]any{
			"role":       "assistant",
			"content":    msg.Content,
			"tool_calls": msg.ToolCalls,
		})

		for _, tc := range msg.ToolCalls {
			var output string
			switch tc.Function.Name {
			case "bash":
				output = c.executeBash(tc.Function.Arguments)
			case "declare":
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					output = fmt.Sprintf("error parsing declare arguments: %s", err)
				} else {
					declarations = append(declarations, args)
					output = "declaration recorded"
				}
			default:
				if isCommand[tc.Function.Name] && invoke != nil {
					var a struct {
						Args string `json:"args"`
					}
					json.Unmarshal([]byte(tc.Function.Arguments), &a)
					out, err := invoke(tc.Function.Name, a.Args)
					if err != nil {
						output = fmt.Sprintf("error running %q: %s", tc.Function.Name, err)
					} else {
						output = out
					}
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

	return nil, fmt.Errorf("exceeded %d tool rounds without a final response", maxToolRounds)
}

// commandToolDef turns a planted command declaration into a brain-callable
// tool. The command takes one space-separated `args` string (matching how the
// CLI and HTML forms invoke it); the declared params are described so the brain
// knows what to pass and in what order.
func commandToolDef(cmd Command) map[string]any {
	desc := cmd.Description
	if len(cmd.Params) > 0 {
		desc += " Params (pass space-separated, in order): " + jsonRepr(cmd.Params)
	}
	desc += " Calling this runs the command and appends its events to the log."
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        cmd.Name,
			"description": desc,
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"args": map[string]any{
						"type":        "string",
						"description": "Arguments to the command, space-separated, in the order its params expect. Empty string if none.",
					},
				},
				"required": []string{"args"},
			},
		},
	}
}

func (c *Compiler) executeBash(args string) string {
	var parsed struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return fmt.Sprintf("error parsing arguments: %s", err)
	}
	output, err := runBash(c.Home, parsed.Command)
	if err != nil {
		return fmt.Sprintf("error: %s", err)
	}
	return output
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
		URL:   "https://opencode.ai/zen/go",
		Key:   entry.Key,
		Model: "glm-5.2",
	}, true
}

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

const maxToolRounds = 15

var llmHTTPClient = &http.Client{Timeout: llmTimeout()}

// llmTimeout is the per-request HTTP timeout. Defaults to 120s; override with
// SELF_LLM_TIMEOUT (any Go duration, e.g. "1h") — useful when a human is in the
// loop authoring responses by hand.
func llmTimeout() time.Duration {
	if v := os.Getenv("SELF_LLM_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 120 * time.Second
}

func (c *Compiler) callLLM(system, user string, submitTool map[string]any) (string, error) {
	messages := []map[string]any{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}
	tools := []map[string]any{bashToolDef, submitTool}
	ep := llmEndpoint{c.URL, c.Key, c.Model}

	for round := 0; round < maxToolRounds; round++ {
		msg, err := c.doRound(ep, messages, tools)
		if err != nil && isQuotaExceeded(err) && c.fallback != nil {
			fmt.Fprintf(os.Stderr, "self: %v — falling back to %s\n", err, c.fallback.URL)
			ep = *c.fallback
			msg, err = c.doRound(ep, messages, tools)
		}
		if err != nil {
			return "", err
		}

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
			output := c.executeBash(tc.Function.Arguments)
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
	result, err := c.callLLM(CommandSystemPrompt, buildCommandPrompt(cmd), submitCommandTool)
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
	result, err := c.callLLM(ProjectorSystemPrompt, buildProjectorPrompt(p), submitProjectorTool)
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

const CommandSystemPrompt = `You are the self compiler. You read a command declaration (command + the event it produces) and write an executable command script. You have a bash tool to explore the garden — the current state of self — before compiling.

The kernel runs command scripts as Unix pipeline processes:
- Receives args as argv. Reads current events as JSONL on stdin. Writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at.
- The kernel sets the SELF_HOME env var. Commands just emit events; the kernel handles persistence.

Write scripts in any language available on the system. Python 3 and bash are safe portable choices. Include the appropriate shebang. Use only standard libraries / builtins — no external dependencies. If the script makes HTTP requests, set a User-Agent header (some endpoints block default library UAs).

Before writing the script, explore the garden with bash:
- ls capabilities/commands/ and capabilities/projectors/ — what capabilities already exist?
- head events.jsonl — what event names are already in the stream? What do their payloads look like?
- cat site/kernel.html — what's the wiring? Which events feed which projectors?

If the new command's event name overlaps with or is semantically adjacent to existing events, integrate: align field names with existing conventions, avoid collisions, and consider whether the new command should produce events that existing projectors can consume. If existing events carry similar information under different names, the script can co-produce the existing event name so existing projectors pick it up.

If the declaration includes a REFERENCE IMPLEMENTATION, treat it as a strong, precise starting point — not gospel. Verify it against the pipe contract (argv in, JSONL events out, the declared event name and fields), read it for bugs, and adapt it to THIS garden's actual event vocabulary and conventions you found while exploring. Keep what is correct, fix or remap what does not fit. You are still the compiler: never submit code you have not verified.

When you're done exploring, call submit_command with the full script source.`

const ProjectorSystemPrompt = `You are the self compiler. You read a projector declaration and write an executable projector script. You have a bash tool to explore the garden — the current state of self — before compiling.

The kernel runs projector scripts as Unix pipeline processes:
- Receives all events as JSONL on stdin. Writes HTML on stdout. The kernel persists the output to SELF_HOME/site/<projector_name>.html — do not write to disk yourself, just emit HTML on stdout.

The projector must build its state from the event stream by filtering for the consumed event names. Emit BARE semantic HTML — do NOT write any CSS, <style> blocks, or inline style attributes. The kernel injects one shared stylesheet at serve time (the enrichment layer), so styling is not your job; styling you emit will only fight it. Use plain semantic elements (h1-h3, p, nav, table/th/td/tfoot, form, input, button, code, hr) and only this small, stable class vocabulary where semantics aren't enough: muted (secondary text), card (bordered panel), row / stack (horizontal / vertical grouping), tag (+ tag accent) (pill labels), msg (+ who) (a chat line), num (on numeric table cells), and on buttons: secondary, danger. That keeps each projector tiny and uniformly themed. Put affordances directly in the markup as plain HTML forms — no JavaScript. To let the user run a command, emit: <form method="post" action="/run/COMMAND"><input name="x"><button>Label</button></form>. The form's input values are passed to the command as positional arguments in document order (field names are for humans; order is the contract), so order the inputs to match the command's params. The kernel runs the command and redirects back, so the page reloads with the new state — full-page reload is fine, the pages are tiny. Use native HTML for interactivity where possible (e.g. <details>/<summary> for show/hide). Do not add htmx or any script.

Write scripts in any language available on the system. Python 3 and bash are safe portable choices. Include the appropriate shebang. Use only standard libraries / builtins — no external dependencies.

Before writing the script, explore the garden with bash:
- ls capabilities/commands/ and capabilities/projectors/ — what capabilities already exist?
- head events.jsonl — what event names are already in the stream? What do their payloads look like? Are there events with similar payloads but different names that this projector should also consume?
- cat site/kernel.html — what's the wiring? Are there existing projectors that already render similar views?

If the declaration's consumed events overlap with or are semantically adjacent to existing events in the stream, adapt: extend the projector's filter to also consume the existing events, mapping their fields into the render. For example, if a finance projector declares consumption of finance.expenditure_added but the stream already has shopping_bill_uploaded events with {vendor, amount, date}, the projector should consume both and map vendor→category. This is receiver-controlled adaptation — the seed adapts to the garden, not the other way around.

If the declaration includes a REFERENCE IMPLEMENTATION, treat it as a strong, precise starting point — not gospel. Verify it against the contract (events on stdin, bare semantic HTML on stdout, the class vocabulary above, /run/ forms for affordances), read it for bugs, and adapt it to the events THIS garden actually carries. Keep what is correct, fix or remap what does not fit. You are still the compiler: never submit code you have not verified.

When you're done exploring, call submit_projector with the full script source.`

func buildCommandPrompt(cmd Command) string {
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
	return prompt + referenceBlock(cmd.Implementation)
}

func buildProjectorPrompt(p ProjectorDecl) string {
	prompt := fmt.Sprintf(`Compile this projector declaration into a projector script.

PROJECTOR declaration:
  name: %s
  description: %s
  consumes: %s

Write the projector_script. It must filter events by the consumed names and render HTML.`,
		p.Name, p.Description, jsonRepr(p.Consumes),
	)
	return prompt + referenceBlock(p.Implementation)
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

const BrainSystemPrompt = `You are self's brain — a general-purpose agent that lives inside the kernel. Commands call you via 'self think' when they need intelligence.

You have three powers:
- READ: a bash tool to explore the garden (cwd=SELF_HOME) — read-only inspection of capabilities/, events.jsonl, and site/.
- ACT: every capability you have is exposed to you as a tool. To DO something the user asks (delete an item, capture a note, set a meal), CALL the matching command tool with its args — do not just describe it or emit a button. The kernel runs it and appends the resulting events, then tells you what happened. The event log is append-only, so actions are safe and reversible: a "delete" is a tombstone event, undoable by a later restore. Prefer acting over explaining when the user asks you to change something.
- GROW: when the user asks for a NEW capability that no existing command provides, call the declare tool (see below) to add it.

Explore the garden with bash before responding:
- ls site/ — what projections exist? These are the current state. Read the relevant ones.
- cat site/<name>.html — read projections that are relevant to the caller's question. For chat, read site/chat.html for conversation history.
- ls capabilities/commands/ and capabilities/projectors/ — what capabilities exist?
- head events.jsonl — what events are in the stream?
- cat site/kernel.html — what's the wiring?

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
