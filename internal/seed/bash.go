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
	bashTimeout   = 10 * time.Second
	bashMaxOutput = 10000
)

// allowedInspectors is the allowlist of read-only commands the LLM may run to
// explore the garden. Every command head in a pipeline must be on this list.
// It contains only tools with no shell-exec or file-write escape — which is
// why sed and awk are absent (sed's w/W commands and GNU `e` flag, and awk's
// system(), are escapes). An allowlist fails closed: unknown commands are
// denied rather than slipping through a gap the denylist forgot.
var allowedInspectors = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "wc": true,
	"grep": true, "egrep": true, "fgrep": true, "rg": true,
	"sort": true, "uniq": true, "cut": true, "tr": true, "comm": true,
	"diff": true, "find": true, "stat": true, "file": true, "strings": true,
	"basename": true, "dirname": true, "echo": true, "printf": true,
	"jq": true, "nl": true, "tac": true, "rev": true, "fold": true,
	"column": true, "od": true, "xxd": true, "cksum": true,
	"md5sum": true, "sha1sum": true, "sha256sum": true,
	"pwd": true, "date": true, "seq": true, "true": true, "test": true,
}

// findDangerousRe catches find's action flags that write or execute. find is
// otherwise a read-only inspector worth allowing. -fprint0 needs its own
// alternative: a trailing \b won't fire between "fprint" and the digit "0"
// (both word characters), so a bare "fprint" alternative misses -fprint0.
var findDangerousRe = regexp.MustCompile(`-(exec|execdir|ok|okdir|delete|fprintf|fprint0|fprint|fls)\b`)

// sortWriteRe catches sort's output-redirect flag in any spelling: -o, a short
// bundle ending in o (-uo), --output, or --output=FILE. sort never needs to
// write a file for inspection — output goes to stdout — so this is pure escape.
var sortWriteRe = regexp.MustCompile(`^(-[a-zA-Z]*o|--output(=.*)?)$`)

// runBash executes a read-only bash command with cwd set to home. Access is
// gated by an allowlist (allowedInspectors): every command head in the line
// must be allowlisted, and command substitution, redirection, and find's
// write/exec actions are rejected. This is defense in depth, not a perfect
// jail — the LLM is cooperative; the real risk is prompt injection from seed
// content — but unlike a denylist it fails closed.
func runBash(home, command string) (string, error) {
	if strings.Contains(command, "$(") || strings.ContainsRune(command, '`') {
		return "", fmt.Errorf("command blocked: command substitution not allowed")
	}
	if strings.ContainsAny(command, "<>") {
		return "", fmt.Errorf("command blocked: redirection not allowed")
	}
	if findDangerousRe.MatchString(command) {
		return "", fmt.Errorf("command blocked: find write/exec actions not allowed")
	}
	for _, seg := range commandSegments(command) {
		head, args := segmentHead(seg)
		base := head[strings.LastIndexByte(head, '/')+1:]
		if !allowedInspectors[base] {
			return "", fmt.Errorf("command blocked: %q is not an allowed read-only inspector", base)
		}
		if err := checkWriteOperands(base, args); err != nil {
			return "", err
		}
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

// commandSegments splits a command line on top-level pipeline/sequence
// operators (| ; & and newline, outside quotes) and returns the fields of each
// segment. The caller resolves each segment's head and inspects its arguments.
func commandSegments(command string) [][]string {
	var segs [][]string
	var seg strings.Builder
	inSingle, inDouble := false, false

	flush := func() {
		if fields := strings.Fields(seg.String()); len(fields) > 0 {
			segs = append(segs, fields)
		}
		seg.Reset()
	}

	for _, c := range command {
		switch {
		case inSingle:
			seg.WriteRune(c)
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			seg.WriteRune(c)
			if c == '"' {
				inDouble = false
			}
		case c == '\'':
			inSingle = true
			seg.WriteRune(c)
		case c == '"':
			inDouble = true
			seg.WriteRune(c)
		case c == '|' || c == ';' || c == '&' || c == '\n':
			flush()
		default:
			seg.WriteRune(c)
		}
	}
	flush()
	return segs
}

// segmentHead returns the program a segment would run and the args that follow
// it, skipping leading VAR=value assignment prefixes (so `FOO=bar cat x`
// resolves to `cat` with args [x]).
func segmentHead(fields []string) (head string, args []string) {
	i := 0
	for i < len(fields) && !strings.HasPrefix(fields[i], "-") && strings.ContainsRune(fields[i], '=') {
		i++
	}
	if i >= len(fields) {
		i = 0
	}
	return fields[i], fields[i+1:]
}

// checkWriteOperands rejects the file-write escapes of allowlisted tools that
// are otherwise read-only. find's write actions are caught by findDangerousRe;
// the remaining writers are sort (-o/--output) and uniq (a second file operand
// is its OUTPUT). Without this, "look but not touch" leaks: sort -o FILE and
// uniq IN OUT both overwrite an arbitrary path under KS_HOME.
func checkWriteOperands(base string, args []string) error {
	switch base {
	case "sort":
		for _, a := range args {
			if sortWriteRe.MatchString(a) {
				return fmt.Errorf("command blocked: sort -o/--output writes a file")
			}
		}
	case "uniq":
		// uniq [OPTION]... [INPUT [OUTPUT]] — a second file operand is the
		// write target. Count non-flag operands, skipping the value consumed
		// by the short options that take one (-f -s -w).
		operands, skipNext := 0, false
		for _, a := range args {
			if skipNext {
				skipNext = false
				continue
			}
			if strings.HasPrefix(a, "-") && a != "-" {
				if a == "-f" || a == "-s" || a == "-w" {
					skipNext = true
				}
				continue
			}
			operands++
		}
		if operands >= 2 {
			return fmt.Errorf("command blocked: uniq with an output-file operand writes a file")
		}
	}
	return nil
}

// bashToolDef is the OpenAI-compatible tool schema passed to the LLM
// so it can explore the garden at compile time.
var bashToolDef = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name":        "bash",
		"description": "Run a read-only bash command to explore the ks garden. Working directory is KS_HOME. Allowed inspectors: ls, cat, head, tail, grep, rg, find, wc, jq, sort, uniq, cut, tr, strings, file, stat, diff (and similar read-only tools); pipelines of these are fine. Use them to inspect commands (registry/commands/), projectors (registry/projectors/), events (events.jsonl), and wiring (site/kernel.html). Anything not on the allowlist is blocked: interpreters (python, awk, sed, perl), writes, network, redirection, command substitution, and find -exec/-delete.",
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
