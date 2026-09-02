package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type loopOptions struct {
	MaxPasses int
	Settle    int
	Timeout   time.Duration
	Ask       string
	Mind      []string
}

const loopUsage = `usage: self loop [--ask TEXT] [--max-passes N] [--settle N] [--timeout DURATION] [-- <mind> [args...]]

Wake a mind on this body repeatedly until the body rests: a waking that leaves
the log unchanged is quiet, and --settle quiet wakings in a row end the loop.

Options:
  --ask TEXT        what woke this body; pass one starts there and every later
                    waking still sees it
  --max-passes N   at most N wakings (default 12); fail only if the last one still
                    changed state
  --settle N       quiet wakings in a row before the body rests (default 2); the
                    last of them is asked plainly whether there is anything else
  --timeout D      fail when one mind process exceeds D (examples: 45s, 10m)
  -h, --help       show this help

Environment defaults:
  SELF_LOOP_MIND         shell command used when no mind argv follows --
  SELF_LOOP_ASK          default ask
  SELF_LOOP_MAX_PASSES   default 12
  SELF_LOOP_SETTLE       default 2
  SELF_LOOP_TIMEOUT      default 30m

Each waking is told which number it is and how many remain. A refused script
does not end the loop: the refusal is recorded and its reason rides the next
waking. The mind is executed directly, without a shell. It inherits the caller's
working directory and environment, receives the situated prompt on stdin, and
returns the event wire on stdout. Diagnostics go to stderr. Use -- before the
mind command. SELF_LOOP_MIND is necessarily a shell string and runs as:
sh -c "$SELF_LOOP_MIND".`

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
	opts := loopOptions{MaxPasses: 12, Settle: 2, Timeout: 30 * time.Minute, Ask: os.Getenv("SELF_LOOP_ASK")}
	for _, e := range []struct {
		key string
		dst *int
	}{{"SELF_LOOP_MAX_PASSES", &opts.MaxPasses}, {"SELF_LOOP_SETTLE", &opts.Settle}} {
		if value := os.Getenv(e.key); value != "" {
			parsed, err := positiveInt(value, e.key)
			if err != nil {
				return opts, err
			}
			*e.dst = parsed
		}
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
		case "--settle":
			value, err := positiveInt(args[1], "--settle")
			if err != nil {
				return opts, err
			}
			opts.Settle = value
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

// loopAsk is the ask a waking receives: a line of facts the kernel alone knows
// — which waking this is, how many remain, what woke the body, whether the last
// waking was quiet — and then the loop layer from PROTOCOL.md. The facts are
// what changes between passes when the mind changes nothing, so a body is never
// woken twice into an identical prompt and told nothing is asked of it.
func loopAsk(pass, maxPasses, quiet, settle int, nudge string) string {
	var b strings.Builder
	remaining := maxPasses - pass
	switch remaining {
	case 0:
		fmt.Fprintf(&b, "Waking %d of this body, and the last: whatever you leave is what remains.", pass)
	case 1:
		fmt.Fprintf(&b, "Waking %d of this body; at most one more before it rests.", pass)
	default:
		fmt.Fprintf(&b, "Waking %d of this body; at most %d more before it rests.", pass, remaining)
	}
	if nudge = strings.TrimSpace(nudge); nudge != "" {
		fmt.Fprintf(&b, "\nWhat woke this body: %s", nudge)
		if pass == 1 {
			b.WriteString("\nStart there.")
		}
	}
	if quiet > 0 {
		if quiet+1 >= settle {
			fmt.Fprintf(&b, "\nThe last waking appended nothing, so this body is about to rest. This is the last waking unless something is appended. Anything else?")
		} else {
			fmt.Fprintf(&b, "\nThe last %d waking(s) appended nothing; after %d quiet in a row this body rests.", quiet, settle)
		}
	}
	b.WriteString("\n\n")
	b.WriteString(protocolLayer("loop"))
	return b.String()
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
	// Name the body before the first waking. A shell that exports SELF_HOME
	// globally wakes that instance, not the cwd, and a nudge meant for a scratch
	// body landing on a real one should be visible before the mind acts.
	fmt.Fprintf(diag, "self loop: body %s\n", home)
	// A signal to the loop ends the waking too. Without this, killing `self loop`
	// left the mind running as an orphan: still writing to the body through its
	// own `self run` calls, its final answer going to a closed pipe.
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	quiet := 0
	for pass := 1; pass <= opts.MaxPasses; pass++ {
		before, err := loadState(home)
		if err != nil {
			return err
		}
		prompt := situate(home, before, loopAsk(pass, opts.MaxPasses, quiet, opts.Settle, opts.Ask))
		fmt.Fprintf(diag, "self loop: waking %d/%d\n", pass, opts.MaxPasses)

		ctx, cancel := context.WithTimeout(sigCtx, opts.Timeout)
		cmd := exec.CommandContext(ctx, opts.Mind[0], opts.Mind[1:]...)
		// Tool-capable minds must act on the same body that produced their
		// situated prompt. Pin the already-resolved home even when the caller
		// selected it implicitly through cwd rather than SELF_HOME.
		cmd.Env = withEnv(os.Environ(), "SELF_HOME", home)
		cmd.Stdin = bytes.NewBufferString(prompt)
		var stdout bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, diag
		// The mind is a tree — a wrapper, a model process, the shells it spawns
		// — so it gets its own process group and the whole group is killed
		// together. Killing only the wrapper left grandchildren holding stdout,
		// and Wait sat on that pipe long after the deadline.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
		cmd.WaitDelay = 5 * time.Second
		err = cmd.Run()
		cancel()
		if sigCtx.Err() != nil {
			return fmt.Errorf("interrupted on waking %d — the mind was stopped with the loop; whatever it appended stands", pass)
		}
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("loop mind exceeded %s on waking %d", opts.Timeout, pass)
		}
		if err != nil {
			return fmt.Errorf("loop mind exited on waking %d: %w", pass, err)
		}
		// A refused script is recorded as script.rejected and its reason rides
		// the next waking. Ending the loop here would be the one way a mind
		// could never learn from the refusal it just earned.
		if err := cmdHear(home, stdout.Bytes(), out); err != nil {
			if !errors.Is(err, errRefused) {
				return fmt.Errorf("hearing waking %d: %w", pass, err)
			}
			fmt.Fprintf(diag, "self loop: waking %d: %v — the reason rides the next waking\n", pass, err)
		}

		after, err := loadState(home)
		if err != nil {
			return err
		}
		if stateRevision(before) == stateRevision(after) {
			quiet++
			if quiet >= opts.Settle {
				fmt.Fprintf(diag, "self loop: converged after %d waking(s) — %d quiet in a row, authoritative state unchanged\n", pass, quiet)
				return nil
			}
			fmt.Fprintf(diag, "self loop: waking %d changed nothing (%d of %d quiet before the body rests)\n", pass, quiet, opts.Settle)
			continue
		}
		quiet = 0
		fmt.Fprintf(diag, "self loop: waking %d changed authoritative state (%d -> %d events)\n", pass, len(before.Events), len(after.Events))
	}
	if quiet > 0 {
		// The cap arrived on a quiet waking: the log did not move, so this is a
		// rest, not a failure — the body simply ran out of wakings to be asked in.
		fmt.Fprintf(diag, "self loop: rested at --max-passes %d — the last waking changed nothing\n", opts.MaxPasses)
		return nil
	}
	return fmt.Errorf("loop reached --max-passes %d while authoritative state was still changing", opts.MaxPasses)
}
