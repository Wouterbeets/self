package seed

import (
	"bytes"
	"encoding/json"
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
}

func NewCompiler(home string) *Compiler {
	if os.Getenv("KS_LLM_STUB") == "1" {
		return &Compiler{Stub: true, Home: home}
	}
	url, key, model := defaultLLMConfig()
	return &Compiler{
		URL:   envOr("KS_LLM_URL", url),
		Key:   envOr("KS_LLM_API_KEY", key),
		Model: envOr("KS_LLM_MODEL", model),
		Home:  home,
	}
}

func (c *Compiler) Available() bool {
	if c.Stub {
		return false
	}
	return c.Key != "" || strings.HasPrefix(c.URL, "http://127.0.0.1") || strings.HasPrefix(c.URL, "http://localhost")
}

type BrainResult struct {
	Response     string
	Declarations []map[string]any
}

// CallBrain calls the kernel's brain — a general-purpose agent that
// explores the garden and responds with text + optional declarations.
// Used by `ks think`, which commands call for LLM access.
func (c *Compiler) CallBrain(user string) (*BrainResult, error) {
	if !c.Available() {
		return nil, fmt.Errorf("no LLM available (ensure llama-server is running on localhost:8080, or set KS_LLM_*)")
	}
	return c.callBrainLLM(BrainSystemPrompt, user)
}

func (c *Compiler) callBrainLLM(system, user string) (*BrainResult, error) {
	messages := []map[string]any{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}
	tools := []map[string]any{bashToolDef, declareTool}

	var declarations []map[string]any

	for round := 0; round < maxToolRounds; round++ {
		req := map[string]any{
			"model":       c.Model,
			"messages":    messages,
			"temperature": 0.2,
			"tools":       tools,
		}
		body, _ := json.Marshal(req)

		url := strings.TrimRight(c.URL, "/") + "/v1/chat/completions"
		r, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		if c.Key != "" {
			r.Header.Set("Authorization", "Bearer "+c.Key)
		}

		resp, err := llmHTTPClient.Do(r)
		if err != nil {
			return nil, fmt.Errorf("llm call failed: %w (check KS_LLM_URL)", err)
		}

		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("llm returned %d: %s", resp.StatusCode, string(b))
		}

		var result struct {
			Choices []struct {
				Message struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
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

		msg := result.Choices[0].Message

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
				output = fmt.Sprintf("error: unknown tool %q", tc.Function.Name)
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

func defaultLLMConfig() (url, key, model string) {
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

var llmHTTPClient = &http.Client{Timeout: 120 * time.Second}

func (c *Compiler) callLLM(system, user string, submitTool map[string]any) (string, error) {
	messages := []map[string]any{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}
	tools := []map[string]any{bashToolDef, submitTool}

	for round := 0; round < maxToolRounds; round++ {
		req := map[string]any{
			"model":       c.Model,
			"messages":    messages,
			"temperature": 0.2,
			"tools":       tools,
		}
		body, _ := json.Marshal(req)

		url := strings.TrimRight(c.URL, "/") + "/v1/chat/completions"
		r, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		r.Header.Set("Content-Type", "application/json")
		if c.Key != "" {
			r.Header.Set("Authorization", "Bearer "+c.Key)
		}

		resp, err := llmHTTPClient.Do(r)
		if err != nil {
			return "", fmt.Errorf("llm call failed: %w (check KS_LLM_URL)", err)
		}

		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return "", fmt.Errorf("llm returned %d: %s", resp.StatusCode, string(b))
		}

		var result struct {
			Choices []struct {
				Message struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return "", err
		}
		resp.Body.Close()

		if len(result.Choices) == 0 {
			return "", fmt.Errorf("llm returned no choices")
		}

		msg := result.Choices[0].Message

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
	return fmt.Sprintf("#!/usr/bin/env python3\n# STUB (no LLM configured) — generated by ks\n# Command: %s\nimport sys, json\n\nevent = {\n    \"name\": %q,\n    \"payload\": {\"title\": \" \".join(sys.argv[1:]) or \"(untitled)\"},\n}\nprint(json.dumps(event))\n",
		cmd.Description, cmd.Event.Name)
}

func (c *Compiler) stubProjector(p ProjectorDecl) string {
	return fmt.Sprintf("#!/usr/bin/env python3\n# STUB (no LLM configured) — generated by ks\n# Projector: %s\nimport sys, json\nfrom html import escape\n\nevents = []\nfor line in sys.stdin:\n    line = line.strip()\n    if not line:\n        continue\n    events.append(json.loads(line))\n\nprint(\"<!DOCTYPE html>\")\nprint(\"<html><head><title>%s</title></head><body>\")\nprint(\"<h1>%s</h1>\")\nprint(\"<ul>\")\nfor e in events:\n    if e.get(\"name\") in %s:\n        title = e.get(\"payload\", {}).get(\"title\", \"(untitled)\")\n        print(f\"  <li>{escape(title)}</li>\")\nprint(\"</ul>\")\nprint(\"</body></html>\")\n",
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

const CommandSystemPrompt = `You are the ks compiler. You read a command declaration (command + the event it produces) and write an executable command script. You have a bash tool to explore the garden — the current state of the ks kernel — before compiling.

The kernel runs command scripts as Unix pipeline processes:
- Receives args as argv. Reads current events as JSONL on stdin. Writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at.
- The kernel sets the KS_HOME env var. Commands just emit events; the kernel handles persistence.

Write scripts in any language available on the system. Python 3 and bash are safe portable choices. Include the appropriate shebang. Use only standard libraries / builtins — no external dependencies. If the script makes HTTP requests, set a User-Agent header (some endpoints block default library UAs).

Before writing the script, explore the garden with bash:
- ls registry/commands/ and registry/projectors/ — what capabilities already exist?
- head events.jsonl — what event names are already in the stream? What do their payloads look like?
- cat site/kernel.html — what's the wiring? Which events feed which projectors?

If the new command's event name overlaps with or is semantically adjacent to existing events, integrate: align field names with existing conventions, avoid collisions, and consider whether the new command should produce events that existing projectors can consume. If existing events carry similar information under different names, the script can co-produce the existing event name so existing projectors pick it up.

When you're done exploring, call submit_command with the full script source.`

const ProjectorSystemPrompt = `You are the ks compiler. You read a projector declaration and write an executable projector script. You have a bash tool to explore the garden — the current state of the ks kernel — before compiling.

The kernel runs projector scripts as Unix pipeline processes:
- Receives all events as JSONL on stdin. Writes HTML on stdout. The kernel persists the output to KS_HOME/site/<projector_name>.html — do not write to disk yourself, just emit HTML on stdout.

The projector must build its state from the event stream by filtering for the consumed event names. It should render clean, valid HTML with inline CSS.

Write scripts in any language available on the system. Python 3 and bash are safe portable choices. Include the appropriate shebang. Use only standard libraries / builtins — no external dependencies.

Before writing the script, explore the garden with bash:
- ls registry/commands/ and registry/projectors/ — what capabilities already exist?
- head events.jsonl — what event names are already in the stream? What do their payloads look like? Are there events with similar payloads but different names that this projector should also consume?
- cat site/kernel.html — what's the wiring? Are there existing projectors that already render similar views?

If the declaration's consumed events overlap with or are semantically adjacent to existing events in the stream, adapt: extend the projector's filter to also consume the existing events, mapping their fields into the render. For example, if a finance projector declares consumption of finance.expenditure_added but the stream already has shopping_bill_uploaded events with {vendor, amount, date}, the projector should consume both and map vendor→category. This is receiver-controlled adaptation — the seed adapts to the garden, not the other way around.

When you're done exploring, call submit_projector with the full script source.`

func buildCommandPrompt(cmd Command) string {
	return fmt.Sprintf(`Compile this command declaration into a command script.

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
}

func buildProjectorPrompt(p ProjectorDecl) string {
	return fmt.Sprintf(`Compile this projector declaration into a projector script.

PROJECTOR declaration:
  name: %s
  description: %s
  consumes: %s

Write the projector_script. It must filter events by the consumed names and render HTML.`,
		p.Name, p.Description, jsonRepr(p.Consumes),
	)
}

const BrainSystemPrompt = `You are the ks kernel's brain — a general-purpose agent that lives inside the kernel. You have a bash tool to explore the garden (cwd=KS_HOME). Commands call you via 'ks think' when they need intelligence.

Explore the garden with bash before responding:
- ls site/ — what projections exist? These are the current state. Read the relevant ones.
- cat site/<name>.html — read projections that are relevant to the caller's question. For chat, read site/chat.html for conversation history.
- ls registry/commands/ and registry/projectors/ — what capabilities exist?
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
