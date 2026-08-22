// self — a local-first, event-sourced runtime, and to the shell a filter with a
// memory. One append-only log is the only authoritative state; every capability
// and every view is a deterministic replay of it. The kernel holds no model and
// spawns nothing: intelligence enters through a shell pipe, where the mind is
// whatever process you put between two invocations of self.
//
//	echo "add a mood tracker" | self | claude -p | self
//
// The first self situates the ask against the instance's own state and appends
// nothing. The mind does durable work through installed commands and prints
// events. The second self hears them: they land, and authored scripts install
// under receipts the kernel signs with a key only it holds. A declaration
// without a script stays pending and rides the next prompt, so the loop
// converges — that is the strange loop, one shell pass at a time.
//
// Reads project. Writes append. Orientation is a read.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

func main() {
	home := homeDir()
	args := os.Args[1:]
	verb := ""
	if len(args) > 0 {
		verb, args = args[0], args[1:]
	}

	err := dispatch(home, verb, args, os.Stdout)
	switch {
	case err == nil:
	case errors.Is(err, errQuiet):
		os.Exit(3) // nothing to do — the loop's convergence signal
	default:
		fmt.Fprintf(os.Stderr, "self: %s\n", err)
		os.Exit(1)
	}
}

func dispatch(home, verb string, args []string, out io.Writer) error {
	switch verb {
	case "": // the read face: the ask is argv, and stdin is never touched
		return cmdSituate(home, strings.Join(args, " "), out)

	case "hear": // the write face: the one door a mind's output enters through
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		return cmdHear(home, input, out)

	case "brief":
		st, err := loadState(home)
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, brief(home, st))
		return err

	case "run":
		if len(args) < 1 {
			return fmt.Errorf("usage: self run <command> [args...]")
		}
		// No ensureSecret here. Minting a key on this path meant `self run`
		// dropped a .secret in whatever directory it was called from — and, on a
		// real instance whose key went missing, forged a fresh one, hiding the
		// only honest diagnostic there is.
		st, err := loadState(home)
		if err != nil {
			return err
		}
		evs, err := runCommand(home, st, args[0], args[1:], doorCLI, callerClaim())
		if err != nil {
			return err
		}
		if len(evs) == 0 {
			fmt.Fprintln(out, "no events")
			return nil
		}
		for _, e := range evs {
			fmt.Fprintf(out, "%d\t%s\t%s\n", e.Seq, e.Name, trunc(compact(e.Payload), 160))
		}
		return nil

	case "view":
		if len(args) != 1 {
			return fmt.Errorf("usage: self view <name>")
		}
		st, err := loadState(home)
		if err != nil {
			return err
		}
		page, err := runView(home, st, args[0])
		if err != nil {
			return err
		}
		_, err = out.Write(page)
		return err

	case "learn":
		if len(args) != 1 {
			return fmt.Errorf("usage: self learn <account-dir>")
		}
		return cmdLearn(home, args[0], out)

	case "give":
		if len(args) != 2 {
			return fmt.Errorf("usage: self give <event-prefix | command/<name> | view/<name>> <dir>")
		}
		return cmdGive(home, args[0], args[1])

	case "rehydrate":
		return rehydrate(home)

	case "help", "-h", "--help":
		_, err := io.WriteString(out, protocolDoc)
		return err

	default:
		// Not a verb, so it is an ask — `self what is going on` reads as well as
		// `self "what is going on"`. One bare word is the exception: it is far
		// more likely a mistyped verb than a question, and silently answering a
		// typo with a prompt would hide it.
		if len(args) == 0 && !strings.ContainsAny(verb, " \t\n") {
			return fmt.Errorf("unknown verb %q — verbs: hear brief run view learn give rehydrate help; to ask a question, quote it: self %q", verb, verb)
		}
		return cmdSituate(home, strings.Join(append([]string{verb}, args...), " "), out)
	}
}

// ───────────────────────────────── the brief ────────────────────────────────

// brief is the state card: what this instance is, what it can do, what is
// pending, and which authoring attempts stand refused. Facts only — the contract lives in PROTOCOL.md and there
// is exactly one copy of it. This is the read an agent starts from, and every
// line of it is a replay of the log.
func brief(home string, st *state) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# self — %s\n\n", home)

	caller := callerClaim()
	if caller == "" {
		caller = `unset — export SELF_CALLER="<who you are>" so your writes are attributable`
	}
	fmt.Fprintf(&b, "log: %d events    caller: %s\n", len(st.Events), caller)
	if len(st.Events) > 0 && st.Key == nil {
		b.WriteString("\n**no .secret beside this log** — no receipt can verify, so this instance has no capabilities.\n")
	}

	cmds, views := st.list(kindCommand), st.list(kindView)

	b.WriteString("\n## commands — `self run <name> [args…]`\n\n")
	if len(cmds) == 0 {
		b.WriteString("none yet\n")
	}
	for _, c := range cmds {
		fmt.Fprintf(&b, "- **%s** — %s%s\n", c.Name, oneLine(c.Decl.Description), pendingMark(c))
	}

	b.WriteString("\n## views — `self view <name>`\n\n")
	for _, c := range views {
		consumes := strings.Join(c.Decl.Consumes, ", ")
		if consumes == "" {
			consumes = "the whole log"
		}
		fmt.Fprintf(&b, "- **%s** — %s (consumes %s)%s\n", c.Name, oneLine(c.Decl.Description), consumes, pendingMark(c))
	}
	if st.cap(kindView, "log") == nil {
		b.WriteString("- **log** — every event, one line each (built in; a declared view named `log` shadows it)\n")
	}

	if p := st.pending(); len(p) > 0 {
		b.WriteString("\n## pending — declared, no script yet\n\n")
		for _, c := range p {
			fmt.Fprintf(&b, "- %s (declared at seq %d)\n", c.key(), c.DeclSeq)
		}
	}
	if len(st.Reject) > 0 {
		b.WriteString("\n## refused — standing, until authored or retired\n\n")
		for _, r := range st.Reject {
			where := strings.Trim(r.Type+"/"+r.Name, "/")
			if where == "" {
				where = "(unnamed)"
			}
			fmt.Fprintf(&b, "- %s (seq %d): %s\n", where, r.Seq, oneLine(r.Reason))
		}
	}
	if st.quiet() {
		b.WriteString("\nnothing pending, nothing refused.\n")
	}

	b.WriteString("\n## where\n\n")
	b.WriteString("`events.jsonl` the log, authoritative · `cap/` installed scripts, derived · `.secret` the signing key\n")
	b.WriteString("`self help` the protocol · `self view log` what happened lately\n")
	return b.String()
}

func pendingMark(c *capability) string {
	if !c.Pending() {
		return ""
	}
	if c.Receipt != nil {
		return "  *(re-declared, running the older script until re-authored)*"
	}
	return "  *(pending — no script yet)*"
}

// ─────────────────────────────────── util ───────────────────────────────────

func oneLine(s string) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
	if s == "" {
		return "(no description)"
	}
	return s
}

// trunc cuts to at most n bytes without splitting a rune: a prompt or a report
// carrying half a character is invalid UTF-8, and the prompt is piped straight
// into a model.
func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func compact(raw json.RawMessage) string {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}
