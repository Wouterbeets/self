package main

import (
	"context"
	"errors"
	"flag"
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
returns the event wire on stdout. Diagnostics go to stderr. Options stop at --
or the first positional argument; the rest is the mind command and its argv.
Use -- to make that boundary explicit. SELF_LOOP_MIND is a shell string run as:
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
	opts := loopOptions{Ask: os.Getenv("SELF_LOOP_ASK")}
	flags := flag.NewFlagSet("loop", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // the caller owns diagnostics
	flags.StringVar(&opts.Ask, "ask", opts.Ask, "")
	for name, fallback := range map[string]string{"max-passes": "12", "settle": "2", "timeout": "30m"} {
		if value := os.Getenv("SELF_LOOP_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))); value != "" {
			fallback = value
		}
		flags.String(name, fallback, "")
	}
	if err := flags.Parse(args); err != nil {
		return opts, fmt.Errorf("loop options: %w — %s", err, loopUsage)
	}
	opts.Mind = flags.Args()
	// Validate after applying CLI overrides, so an overridden environment
	// default cannot reject an otherwise valid invocation.
	for _, option := range []struct {
		name string
		dst  *int
	}{
		{"max-passes", &opts.MaxPasses}, {"settle", &opts.Settle},
	} {
		value, err := positiveInt(flags.Lookup(option.name).Value.String(), "--"+option.name)
		if err != nil {
			return opts, err
		}
		*option.dst = value
	}
	var err error
	opts.Timeout, err = positiveDuration(flags.Lookup("timeout").Value.String(), "--timeout")
	if err != nil {
		return opts, err
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

// loopAsk is the ask a waking receives: a line of facts the kernel alone knows
// — which waking this is, how many remain, what woke the body, whether the last
// waking was quiet — and then the loop layer from PROTOCOL.md. The facts are
// what changes between passes when the mind changes nothing, so a body is never
// woken twice into an identical prompt and told nothing is asked of it.
func loopAsk(pass, maxPasses, quiet, settle int, timeout time.Duration, nudge string) string {
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
	// The mind cannot know how long a waking lasts; the kernel does. A waking
	// that built for nine minutes and appended nothing left nothing.
	fmt.Fprintf(&b, "\nThis waking ends after %s. Only what is appended by then persists; a declaration left pending is safe, a script still on disk is not.", timeout)
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
		_, err := fmt.Fprintln(out, loopUsage)
		return err
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
		prompt := situate(home, before, loopAsk(pass, opts.MaxPasses, quiet, opts.Settle, opts.Timeout, opts.Ask))
		fmt.Fprintf(diag, "self loop: waking %d/%d\n", pass, opts.MaxPasses)

		ctx, cancel := context.WithTimeout(sigCtx, opts.Timeout)
		cmd := exec.CommandContext(ctx, opts.Mind[0], opts.Mind[1:]...)
		// Tool-capable minds must act on the same body that produced their
		// situated prompt. Pin the already-resolved home even when the caller
		// selected it implicitly through cwd rather than SELF_HOME.
		cmd.Env = append(os.Environ(), "SELF_HOME="+home)
		cmd.Stdin, cmd.Stderr = strings.NewReader(prompt), diag
		// The mind is a tree — a wrapper, a model process, the shells it spawns
		// — so it gets its own process group and the whole group is killed
		// together. Killing only the wrapper left grandchildren holding stdout,
		// and Wait sat on that pipe long after the deadline. SIGKILL also stops
		// children that ignore SIGTERM; Cmd's fallback only kills the leader.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		cmd.WaitDelay = 5 * time.Second
		stdout, err := cmd.Output()
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
		if err := cmdHear(home, stdout, out); err != nil {
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
