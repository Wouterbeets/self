package main

import (
	"cmp"
	"context"
	"encoding/json"
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

// parseLoopOptions validates only final values: an invalid environment
// default is fine when a flag overrides it.
func parseLoopOptions(args []string) (opts loopOptions, err error) {
	env := func(name, fallback string) string { return cmp.Or(os.Getenv("SELF_LOOP_"+name), fallback) }
	flags := flag.NewFlagSet("loop", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ask := flags.String("ask", os.Getenv("SELF_LOOP_ASK"), "")
	maxPasses := flags.String("max-passes", env("MAX_PASSES", "12"), "")
	settle := flags.String("settle", env("SETTLE", "2"), "")
	timeout := flags.String("timeout", env("TIMEOUT", "30m"), "")
	if err := flags.Parse(args); err != nil {
		return opts, fmt.Errorf("loop options: %w — %s", err, loopUsage)
	}
	opts.Ask, opts.Mind = *ask, flags.Args()
	if opts.MaxPasses, err = positiveInt(*maxPasses, "--max-passes"); err != nil {
		return opts, err
	}
	if opts.Settle, err = positiveInt(*settle, "--settle"); err != nil {
		return opts, err
	}
	if opts.Timeout, err = time.ParseDuration(*timeout); err != nil || opts.Timeout <= 0 {
		return opts, fmt.Errorf("--timeout needs a positive Go duration such as 30m or 45s")
	}
	if len(opts.Mind) == 0 {
		mind := os.Getenv("SELF_LOOP_MIND")
		if mind == "" {
			return opts, fmt.Errorf("no mind configured — pass one after -- or set SELF_LOOP_MIND")
		}
		opts.Mind = []string{"sh", "-c", mind}
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
	fmt.Fprintf(&b, "Pass %d of this self; at most %d more (limit %d).", pass, maxPasses-pass, maxPasses)
	fmt.Fprintf(&b, "\nThis pass ends after %s. Record results and artifact references before then.", timeout)
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
		} else if evs, _, _, _ := wire(string(stdout)); len(evs) > 0 {
			// Only this successful mind's stdout may settle its loop, never a peer's event.
			last := evs[len(evs)-1]
			var result struct{ Reason string }
			if last.Name == "loop.settled" && json.Unmarshal(last.Payload, &result) == nil && strings.TrimSpace(result.Reason) != "" {
				fmt.Fprintf(diag, "self loop: settled on pass %d: %s\n", pass, result.Reason)
				return nil
			}
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
