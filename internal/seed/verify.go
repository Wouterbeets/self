package seed

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// VerifyResult is the outcome of running a compiled script against a
// declaration's examples.
type VerifyResult struct {
	Ran      int      // examples attempted
	Passed   int      // examples that satisfied every ExpectContains
	Failures []string // human-readable, one per failed example
}

// OK reports whether every example passed.
func (r VerifyResult) OK() bool { return r.Ran > 0 && r.Passed == r.Ran }

// Summary is a one-line human description of the result.
func (r VerifyResult) Summary() string {
	if r.Ran == 0 {
		return "no examples"
	}
	return fmt.Sprintf("%d/%d examples passed", r.Passed, r.Ran)
}

const verifyTimeout = 10 * time.Second

// VerifyScript runs a freshly compiled script against examples and reports
// which passed. Each example feeds Args as argv and Events as JSONL on stdin
// (the same pipe contract the kernel uses to run commands and projectors), then
// asserts the script's stdout contains every ExpectContains substring. The
// script is written to a temp file and executed in isolation — nothing touches
// the live garden. kind is "command" or "projector"; it is informational here,
// since both obey the same stdin/stdout contract.
//
// This is the conformance gate: examples define what a capability MUST do
// independent of how it is implemented, so a binary the receiver recompiled to
// its own vocabulary is held to the seed author's contract rather than to the
// compiler's good intentions.
func VerifyScript(script, kind string, examples []Example) (VerifyResult, error) {
	var res VerifyResult
	if len(examples) == 0 {
		return res, nil
	}

	tmp, err := os.CreateTemp("", "self-verify-*")
	if err != nil {
		return res, err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.WriteString(script); err != nil {
		tmp.Close()
		return res, err
	}
	tmp.Close()
	if err := os.Chmod(path, 0755); err != nil {
		return res, err
	}

	for i, ex := range examples {
		res.Ran++
		out, runErr := runExample(path, ex)
		label := ex.Note
		if label == "" {
			label = fmt.Sprintf("example %d", i+1)
		}
		if runErr != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("%s: script error: %v", label, runErr))
			continue
		}
		var problems []string
		var missing []string
		for _, want := range ex.ExpectContains {
			if !strings.Contains(out, want) {
				missing = append(missing, want)
			}
		}
		if len(missing) > 0 {
			problems = append(problems, fmt.Sprintf("missing %v", missing))
		}
		if order := checkOrder(out, ex.ExpectOrder); order != "" {
			problems = append(problems, order)
		}
		if len(problems) > 0 {
			res.Failures = append(res.Failures,
				fmt.Sprintf("%s: %s", label, strings.Join(problems, "; ")))
			continue
		}
		res.Passed++
	}
	return res, nil
}

// runExample executes the script once with the example's argv and JSONL stdin,
// returning its stdout. A timeout guards against a hanging script.
func runExample(path string, ex Example) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, ex.Args...)
	// Run in a temp cwd so a misbehaving script can't read or write the garden.
	if wd, err := os.MkdirTemp("", "self-verify-cwd-*"); err == nil {
		cmd.Dir = wd
		defer os.RemoveAll(wd)
	}

	var stdin bytes.Buffer
	for _, e := range ex.Events {
		stdin.Write([]byte(strings.TrimSpace(string(e))))
		stdin.WriteByte('\n')
	}
	cmd.Stdin = &stdin

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return stdout.String(), fmt.Errorf("timed out after %s", verifyTimeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return stdout.String(), fmt.Errorf("%v: %s", err, firstLine(msg))
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// checkOrder reports a problem string if the wanted substrings are not all
// present in out in the given order (each at or after the previous one's
// position), or "" if they are. Empty want is vacuously ordered.
func checkOrder(out string, want []string) string {
	last := 0
	for _, w := range want {
		idx := strings.Index(out[last:], w)
		if idx < 0 {
			if strings.Contains(out, w) {
				return fmt.Sprintf("out of order: %q", w)
			}
			return fmt.Sprintf("missing (ordered) %q", w)
		}
		last += idx + len(w)
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
