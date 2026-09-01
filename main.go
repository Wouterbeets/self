// self — a local-first, event-sourced runtime, and to the shell a filter with a
// memory. One append-only log is the only authoritative state; every capability
// and every view is a deterministic replay of it. The kernel holds no resident
// model: intelligence is whatever process a caller pipes beside it or names to
// `self loop`.
//
//	self "add a mood tracker" | claude -p | self hear
//
// An ask arrives as argv, so the first self situates it against the instance's
// own state and appends nothing. The mind does durable work through installed
// commands and prints events. `self hear` lands them: events append, and
// authored scripts install under receipts the kernel signs with a key only it
// holds. A declaration without a script stays pending and rides the next
// prompt. `self loop` repeats complete situated turns until one leaves the
// authoritative log unchanged.
//
// Reads project. Writes append. Orientation is a read.
//
// PROTOCOL.md is the contract, embedded here and printed by `self help`. It is
// the only place the wire is described: comments in this package point at it
// rather than restating it, because six hand-synced copies of one contract is
// how the previous kernel came to contradict itself inside a single brief.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const cliUsage = `self — local-first event-sourced runtime

Usage:
  self [ask...]                 situate an ask; bare self presents the naked surface
  self hear                     ingest event JSONL or authored scripts from stdin
  self brief                    show capabilities, pending work, and refusals
  self run <command> [args...]  execute a command capability and append its events
  self view <name> [args...]    replay a pure view; built-in log is always available
  self loop [opts] [-- mind...] run situated turns to an unchanged-state fixed point
  self learn <account-dir>      deposit an account and print its learning prompt
  self give <selector> <dir>    write an event or capability account
  self rehydrate                rebuild derived capability files from the log
  self completion <shell>       print a completion script (zsh|bash|fish)
  self help                     print the complete protocol

Loop:
  self loop --help
  SELF_LOOP_MIND='<shell command>' self loop

Environment:
  SELF_HOME, SELF_CALLER, SELF_LOOP_MIND, SELF_LOOP_ASK, SELF_LOOP_MAX_PASSES, SELF_LOOP_TIMEOUT`

func main() {
	home := homeDir()
	args := os.Args[1:]
	verb := ""
	if len(args) > 0 {
		verb, args = args[0], args[1:]
	}

	err := dispatch(home, verb, args, os.Stdout)
	if err != nil {
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
		st, err := loadState(home)
		if err != nil {
			return err
		}
		if len(args) < 1 {
			// No name is a question about what is runnable, not a
			// failure. Answer it with the names this log actually knows.
			fmt.Fprintln(out, "usage: self view <name> [args...]")
			fmt.Fprintln(out)
			fmt.Fprint(out, viewUsage(st))
			return nil
		}
		name := args[0]
		// A name the log does not know at all — no view, no command by that name,
		// and not the built-in log — is a guess. Point the reader at what is real
		// rather than sending them to check `self brief` for the list.
		if name != "log" && st.cap(kindView, name) == nil && st.cap(kindCommand, name) == nil {
			fmt.Fprintf(out, "no view %q in this log\n\n", name)
			fmt.Fprint(out, viewUsage(st))
			return nil
		}
		page, err := runView(home, st, name, args[1:]...)
		if err != nil {
			return err
		}
		_, err = out.Write(page)
		return err

	case "loop":
		return cmdLoop(home, args, out, os.Stderr)

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

	case "completion":
		if len(args) != 1 {
			return fmt.Errorf("usage: self completion <zsh|bash|fish>")
		}
		script, err := completionScript(args[0])
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, script)
		return err

	case "__complete": // the machine face the shims call; PROTOCOL.md: Completion
		return cmdComplete(home, args, out)

	case "-h", "--help":
		_, err := io.WriteString(out, cliUsage+"\n")
		return err

	case "help":
		_, err := io.WriteString(out, protocolDoc)
		return err

	default:
		// Not a verb, so it is an ask — `self what is going on` reads as well as
		// `self "what is going on"`. One bare word is the exception: it is far
		// more likely a mistyped verb than a question, and silently answering a
		// typo with a prompt would hide it.
		if len(args) == 0 && !strings.ContainsAny(verb, " \t\n") {
			return fmt.Errorf("unknown verb %q — verbs: hear brief run view loop learn give rehydrate completion help; to ask a question, quote it: self %q", verb, verb)
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

	b.WriteString("\n## views — `self view <name> [args…]`\n\n")
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
	if st.capabilitiesReady() {
		b.WriteString("\nnothing pending, nothing refused.\n")
	}

	b.WriteString("\n## where\n\n")
	b.WriteString("`events.jsonl` the log, authoritative · `cap/` installed scripts, derived · `.secret` the signing key\n")
	b.WriteString("`self help` the protocol · `self view log` what happened lately\n")
	return b.String()
}

// viewUsage lists the views this instance can run, so a reader who asks for a
// view by a name the log does not know — or asks for none at all — is pointed
// at what actually exists instead of guessing. It mirrors the view section of
// `brief`, because the two answer the same question: what can I read here?
func viewUsage(st *state) string {
	var b strings.Builder
	b.WriteString("views on this instance — `self view <name> [args…]`:\n")
	views := st.list(kindView)
	for _, c := range views {
		consumes := strings.Join(c.Decl.Consumes, ", ")
		if consumes == "" {
			consumes = "the whole log"
		}
		fmt.Fprintf(&b, "- %s — %s (consumes %s)%s\n", c.Name, oneLine(c.Decl.Description), consumes, pendingMark(c))
	}
	if st.cap(kindView, "log") == nil {
		b.WriteString("- log — every event, one line each (built in; a declared view named `log` shadows it)\n")
	}
	if len(views) == 0 && st.cap(kindView, "log") == nil {
		b.WriteString("\n(no declared views yet — only the built-in log; `self help` shows how to author one)\n")
	}
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
