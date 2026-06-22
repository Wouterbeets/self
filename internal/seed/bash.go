package seed

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	bashTimeout  = 10 * time.Second
	bashMaxOutput = 10000
)

var destructiveRe = regexp.MustCompile(`(?m)\b(rm|rmdir|mv|cp|mkdir|chmod|chown|chattr|ln|mkfifo|mknod|dd|tee|install|touch|curl|wget|nc|ncat|ssh|scp|sftp|rsync|telnet|ftp|socat|netcat|python|python3|perl|ruby|node|lua|php|bash|sh|zsh|dash|ksh|fish|exec|eval|source)\b`)

var sedInPlaceRe = regexp.MustCompile(`\bsed\b[^|]*\s-i\b`)

// runBash executes a read-only bash command with cwd set to home.
// It enforces read-only access via restricted bash (-r) and a denylist
// of destructive, network, and interpreter commands. This is defense in
// depth, not a perfect sandbox — the LLM is cooperative, the risk is
// prompt injection from seed content.
func runBash(home, command string) (string, error) {
	if destructiveRe.MatchString(command) {
		return "", fmt.Errorf("command blocked: write/network/interpreter operations not allowed")
	}
	if sedInPlaceRe.MatchString(command) {
		return "", fmt.Errorf("command blocked: sed -i (in-place edit) not allowed")
	}
	if strings.ContainsAny(command, "<>") {
		return "", fmt.Errorf("command blocked: redirection not allowed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), bashTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "--norc", "--noprofile", "-r", "-c", command)
	cmd.Dir = home

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %s", bashTimeout)
		}
		if stderrStr != "" {
			return "", fmt.Errorf("%s", stderrStr)
		}
		return "", err
	}

	out := stdout.String()
	if len(out) > bashMaxOutput {
		out = out[:bashMaxOutput] + fmt.Sprintf("\n... (truncated at %d bytes)", bashMaxOutput)
	}
	return out, nil
}

// bashToolDef is the OpenAI-compatible tool schema passed to the LLM
// so it can explore the garden at compile time.
var bashToolDef = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name":        "bash",
		"description": "Run a read-only bash command to explore the ks garden. Working directory is KS_HOME. Use ls, cat, grep, head, find, wc, awk, jq, sort, cut, tr, strings, file, stat to inspect existing commands (registry/commands/), projectors (registry/projectors/), events (events.jsonl), and wiring (site/kernel.html). Write operations, network access, redirection, and interpreters are blocked.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The bash command to run",
				},
			},
			"required": []string{"command"},
		},
	},
}

var submitCommandTool = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name":        "submit_command",
		"description": "Submit the compiled command script. Call this when you've finished exploring the garden and are ready to return the script.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command_script": map[string]any{
					"type":        "string",
					"description": "The full command script source code, including shebang.",
				},
			},
			"required": []string{"command_script"},
		},
	},
}

var submitProjectorTool = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name":        "submit_projector",
		"description": "Submit the compiled projector script. Call this when you've finished exploring the garden and are ready to return the script.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projector_script": map[string]any{
					"type":        "string",
					"description": "The full projector script source code, including shebang.",
				},
			},
			"required": []string{"projector_script"},
		},
	},
}

var declareTool = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name":        "declare",
		"description": "Declare a new capability to add to the garden. Call this once per capability when the user asks for a new command or projector.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Event name: command.declared or projector.declared",
				},
				"payload": map[string]any{
					"type":        "object",
					"description": "The declaration payload. For command.declared: {name, description, params, event: {name, fields}}. For projector.declared: {name, description, consumes}.",
				},
			},
			"required": []string{"name", "payload"},
		},
	},
}
