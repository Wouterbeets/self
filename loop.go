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

Run a mind against this self repeatedly until the log stops changing: a pass
that leaves the log unchanged is quiet, and --settle quiet passes in a row end
the loop.

Options:
  --ask TEXT        the ask; pass one starts there and every later pass still
                    sees it
  --max-passes N   at most N passes (default 12); fail only if the last one still
                    changed state
  --settle N       quiet passes in a row before the loop stops (default 2); the
                    last of them is asked plainly whether there is anything else
  --timeout D      fail when one mind process exceeds D (examples: 45s, 10m)
  -h, --help       show this help

Environment defaults:
  SELF_LOOP_MIND         shell command used when no mind argv follows --
  SELF_LOOP_ASK          default ask
  SELF_LOOP_MAX_PASSES   default 12
  SELF_LOOP_SETTLE       default 2
  SELF_LOOP_TIMEOUT      default 30m

Each pass is told which number it is and how many remain. A refused script
does not end the loop: the refusal is recorded and its reason rides the next
pass. The mind is executed directly, without a shell. It inherits the caller's
working directory and environment, receives the prompt on stdin, and
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
	flags.SetOutput(io.Discard)
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

func stateRevision(st *state) string {
	if len(st.Events) == 0 {
		return "empty"
	}
	last := st.Events[len(st.Events)-1]
	return fmt.Sprintf("%d:%d:%s", len(st.Events), last.Seq, last.ID)
}

func loopAsk(pass, maxPasses, quiet, settle int, timeout time.Duration, nudge string) string {
	var b strings.Builder
	remaining := maxPasses - pass
	switch remaining {
	case 0:
		fmt.Fprintf(&b, "Pass %d of this self, and the last: only what you append remains.", pass)
	case 1:
		fmt.Fprintf(&b, "Pass %d of this self; at most one more before the loop stops.", pass)
	default:
		fmt.Fprintf(&b, "Pass %d of this self; at most %d more before the loop stops.", pass, remaining)
	}
	fmt.Fprintf(&b, "\nThis pass ends after %s. Only what is appended by then persists; a declaration left pending is safe, a script still on disk is not.", timeout)
	if nudge = strings.TrimSpace(nudge); nudge != "" {
		fmt.Fprintf(&b, "\nThe ask: %s", nudge)
		if pass == 1 {
			b.WriteString("\nStart there.")
		}
	}
	if quiet > 0 {
		if quiet+1 >= settle {
			fmt.Fprintf(&b, "\nThe last pass appended nothing, so the loop is about to stop. This is the last pass unless something is appended. Anything else?")
		} else {
			fmt.Fprintf(&b, "\nThe last %d pass(es) appended nothing; after %d quiet in a row the loop stops.", quiet, settle)
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
	fmt.Fprintf(diag, "self loop: body %s\n", home)
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	quiet := 0
	for pass := 1; pass <= opts.MaxPasses; pass++ {
		before, err := loadState(home)
		if err != nil {
			return err
		}
		prompt := situate(home, before, loopAsk(pass, opts.MaxPasses, quiet, opts.Settle, opts.Timeout, opts.Ask))
		fmt.Fprintf(diag, "self loop: pass %d/%d\n", pass, opts.MaxPasses)

		ctx, cancel := context.WithTimeout(sigCtx, opts.Timeout)
		cmd := exec.CommandContext(ctx, opts.Mind[0], opts.Mind[1:]...)
		cmd.Env = append(os.Environ(), "SELF_HOME="+home)
		cmd.Stdin, cmd.Stderr = strings.NewReader(prompt), diag
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		cmd.WaitDelay = 5 * time.Second
		stdout, err := cmd.Output()
		cancel()
		if sigCtx.Err() != nil {
			return fmt.Errorf("interrupted on pass %d — the mind was stopped with the loop; whatever it appended stands", pass)
		}
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("loop mind exceeded %s on pass %d", opts.Timeout, pass)
		}
		if err != nil {
			return fmt.Errorf("loop mind exited on pass %d: %w", pass, err)
		}
		if err := cmdHear(home, stdout, out); err != nil {
			if !errors.Is(err, errRefused) {
				return fmt.Errorf("hearing pass %d: %w", pass, err)
			}
			fmt.Fprintf(diag, "self loop: pass %d: %v — the reason rides the next pass\n", pass, err)
		}

		after, err := loadState(home)
		if err != nil {
			return err
		}
		if stateRevision(before) == stateRevision(after) {
			quiet++
			if quiet >= opts.Settle {
				fmt.Fprintf(diag, "self loop: converged after %d pass(es) — %d quiet in a row, authoritative state unchanged\n", pass, quiet)
				return nil
			}
			fmt.Fprintf(diag, "self loop: pass %d changed nothing (%d of %d quiet before the loop stops)\n", pass, quiet, opts.Settle)
			continue
		}
		quiet = 0
		fmt.Fprintf(diag, "self loop: pass %d changed authoritative state (%d -> %d events)\n", pass, len(before.Events), len(after.Events))
	}
	if quiet > 0 {
		fmt.Fprintf(diag, "self loop: stopped at --max-passes %d — the last pass changed nothing\n", opts.MaxPasses)
		return nil
	}
	return fmt.Errorf("loop reached --max-passes %d while authoritative state was still changing", opts.MaxPasses)
}
