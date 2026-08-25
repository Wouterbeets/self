package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type loopOptions struct {
	MaxPasses int
	Timeout   time.Duration
	Ask       string
	Mind      []string
}

const loopUsage = `usage: self loop [--ask TEXT] [--max-passes N] [--timeout DURATION] [-- <mind> [args...]]

Run naked situated turns until one complete turn leaves authoritative state unchanged.

Options:
  --ask TEXT        explicit objective for pass one; later passes are naked
  --max-passes N   fail if state changes for N consecutive passes
  --timeout D      fail when one mind process exceeds D (examples: 45s, 10m)
  -h, --help       show this help

Environment defaults:
  SELF_LOOP_MIND         shell command used when no mind argv follows --
  SELF_LOOP_ASK          default first-pass objective
  SELF_LOOP_MAX_PASSES   default 12
  SELF_LOOP_TIMEOUT      default 30m

The mind is executed directly, without a shell. It inherits the caller's working
directory and environment, receives the situated prompt on stdin, and returns the
event wire on stdout. Diagnostics go to stderr. Use -- before the mind command.
SELF_LOOP_MIND is necessarily a shell string and runs as: sh -c "$SELF_LOOP_MIND".`

func positiveInt(value, source string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("%s needs a positive integer", source)
	}
	return parsed, nil
}

func positiveDuration(value, source string) (time.Duration, error) {
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s needs a positive Go duration such as 30m or 45s", source)
	}
	return parsed, nil
}

func parseLoopOptions(args []string) (loopOptions, error) {
	opts := loopOptions{MaxPasses: 12, Timeout: 30 * time.Minute, Ask: os.Getenv("SELF_LOOP_ASK")}
	if value := os.Getenv("SELF_LOOP_MAX_PASSES"); value != "" {
		parsed, err := positiveInt(value, "SELF_LOOP_MAX_PASSES")
		if err != nil {
			return opts, err
		}
		opts.MaxPasses = parsed
	}
	if value := os.Getenv("SELF_LOOP_TIMEOUT"); value != "" {
		parsed, err := positiveDuration(value, "SELF_LOOP_TIMEOUT")
		if err != nil {
			return opts, err
		}
		opts.Timeout = parsed
	}
	for len(args) > 0 {
		if args[0] == "--" {
			opts.Mind = args[1:]
			break
		}
		if len(args) < 2 {
			return opts, fmt.Errorf("%s", loopUsage)
		}
		switch args[0] {
		case "--ask":
			opts.Ask = args[1]
		case "--max-passes":
			value, err := positiveInt(args[1], "--max-passes")
			if err != nil {
				return opts, err
			}
			opts.MaxPasses = value
		case "--timeout":
			value, err := positiveDuration(args[1], "--timeout")
			if err != nil {
				return opts, err
			}
			opts.Timeout = value
		default:
			return opts, fmt.Errorf("unknown loop option %q — %s", args[0], loopUsage)
		}
		args = args[2:]
	}
	if len(opts.Mind) == 0 {
		if mind := os.Getenv("SELF_LOOP_MIND"); mind != "" {
			opts.Mind = []string{"sh", "-c", mind}
		} else {
			return opts, fmt.Errorf("no mind configured — pass one after -- or set SELF_LOOP_MIND")
		}
	}
	return opts, nil
}

// stateRevision is deliberately kernel-private. The log is append-only, so its
// length and final immutable identity change on every authoritative append.
// Drivers should use `self loop`, not learn this representation.
func stateRevision(st *state) string {
	if len(st.Events) == 0 {
		return "empty"
	}
	last := st.Events[len(st.Events)-1]
	return fmt.Sprintf("%d:%d:%s", len(st.Events), last.Seq, last.ID)
}

func withEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			out = append(out, item)
		}
	}
	return append(out, prefix+value)
}

func cmdLoop(home string, args []string, out, diag io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(out, loopUsage)
		return nil
	}
	opts, err := parseLoopOptions(args)
	if err != nil {
		return err
	}
	for pass := 1; pass <= opts.MaxPasses; pass++ {
		before, err := loadState(home)
		if err != nil {
			return err
		}
		ask := defaultAsk
		if pass == 1 && strings.TrimSpace(opts.Ask) != "" {
			ask = opts.Ask
		}
		prompt := situate(home, before, ask)
		fmt.Fprintf(diag, "self loop: pass %d/%d\n", pass, opts.MaxPasses)

		ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		cmd := exec.CommandContext(ctx, opts.Mind[0], opts.Mind[1:]...)
		// Tool-capable minds must act on the same body that produced their
		// situated prompt. Pin the already-resolved home even when the caller
		// selected it implicitly through cwd rather than SELF_HOME.
		cmd.Env = withEnv(os.Environ(), "SELF_HOME", home)
		cmd.Stdin = bytes.NewBufferString(prompt)
		var stdout bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, diag
		err = cmd.Run()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("loop mind exceeded %s on pass %d", opts.Timeout, pass)
		}
		if err != nil {
			return fmt.Errorf("loop mind exited on pass %d: %w", pass, err)
		}
		if err := cmdHear(home, stdout.Bytes(), out); err != nil {
			return fmt.Errorf("hearing loop pass %d: %w", pass, err)
		}

		after, err := loadState(home)
		if err != nil {
			return err
		}
		if stateRevision(before) == stateRevision(after) {
			fmt.Fprintf(diag, "self loop: converged after %d pass(es) — authoritative state unchanged\n", pass)
			return nil
		}
		fmt.Fprintf(diag, "self loop: pass %d changed authoritative state (%d -> %d events)\n", pass, len(before.Events), len(after.Events))
	}
	return fmt.Errorf("loop reached --max-passes %d while authoritative state was still changing", opts.MaxPasses)
}
