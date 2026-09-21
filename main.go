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
  self                        reconnect with your persistent self for the current task
  self prompt [ask...]         print a prompt for an event-producing mind
  self <ask...>                shorthand for self prompt <ask...>
  self hear [--after <head>]     ingest event JSONL or authored scripts from stdin
  self brief [name]             the state; with a name or intent/<name>, full detail
  self run <command> [args...]  execute a command capability and append its events
  self view <name> [args...]    replay a pure view; built-in log is always available
  self loop [opts] [-- mind...] run a mind until the log stops changing (quiet passes in a row)
  self learn [--into <intent>] <account-dir>  deposit and interpret an account
  self watch [opts] [prefix]   wait for events without appending
  self give <selector> <dir>    write an event or capability account
  self rehydrate                rebuild derived capability files from the log
  self completion <shell>       print a completion script (zsh|bash|fish)
  self help                     print the complete protocol

Loop:
  self loop --help
  SELF_LOOP_MIND='<shell command>' self loop

Environment:
  SELF_HOME, SELF_CALLER, SELF_LOOP_MIND, SELF_LOOP_ASK, SELF_LOOP_MAX_PASSES, SELF_LOOP_SETTLE, SELF_LOOP_TIMEOUT`

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
	limits := map[string]int{"": 0, "brief": 1, "rehydrate": 0, "help": 0, "-h": 0, "--help": 0}
	if max, fixed := limits[verb]; fixed && len(args) > max {
		return fmt.Errorf("self %s accepts at most %d argument(s); see self --help", verb, max)
	}
	switch verb {
	case "":
		return cmdOrient(home, out)

	case "prompt":
		return cmdSituate(home, strings.Join(args, " "), out)

	case "hear":
		if len(args) != 0 && (len(args) != 2 || args[0] != "--after") {
			return fmt.Errorf("usage: self hear [--after <head-id|empty>]")
		}
		if len(args) > 0 {
			args = args[1:]
		}
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		return cmdHear(home, input, out, args...)

	case "watch":
		return cmdWatch(home, args, out)

	case "brief":
		st, err := loadState(home)
		if err != nil {
			return err
		}
		if len(args) > 0 {
			page, err := briefOne(st, args[0])
			if err != nil {
				return err
			}
			_, err = io.WriteString(out, page)
			return err
		}
		_, err = io.WriteString(out, brief(home, st))
		return err

	case "run", "view":
		// Discovery only reads state: never mint a missing instance key here.
		st, err := loadState(home)
		if err != nil {
			return err
		}
		typ, arg := kindView, "name"
		if verb == "run" {
			typ, arg = kindCommand, "command"
		}
		if len(args) == 0 {
			_, err := fmt.Fprintf(out, "usage: self %s <%s> [args...]\n\n%s", verb, arg, capabilityList(st, typ))
			return err
		}
		name, rest := args[0], args[1:]
		if unknown := unfoldMissing(st, typ, name); unknown != "" {
			io.WriteString(os.Stderr, unknown)
		}
		said := &tally{w: os.Stderr}
		if verb == "view" {
			page, err := runViewDiag(home, st, name, said, rest...)
			if err != nil {
				io.WriteString(os.Stderr, unfoldFailed(st, typ, name, said.n == 0))
				return err
			}
			_, err = out.Write(page)
			return err
		}
		evs, err := runCommand(home, st, name, rest, doorCLI, callerClaim(), said)
		if err != nil {
			io.WriteString(os.Stderr, unfoldFailed(st, typ, name, said.n == 0))
			return err
		}
		for _, e := range evs {
			if _, err := fmt.Fprintf(out, "%d\t%s\t%s\n", e.Seq, e.Name, trunc(compact(e.Payload), 160)); err != nil {
				return err
			}
		}
		return nil

	case "loop":
		return cmdLoop(home, args, out, os.Stderr)

	case "learn":
		if len(args) == 3 && args[0] == "--into" && validIntentName(args[1]) {
			return cmdLearn(home, args[2], out, args[1])
		}
		if len(args) != 1 {
			return fmt.Errorf("usage: self learn [--into <intent-name>] <account-dir>")
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

	case "__complete":
		return cmdComplete(home, args, out)

	case "-h", "--help":
		_, err := io.WriteString(out, cliUsage+"\n")
		return err

	case "help":
		_, err := io.WriteString(out, protocolDoc)
		return err

	default:
		// One bare unknown word is a mistyped verb, not an ask.
		if len(args) == 0 && !strings.ContainsAny(verb, " \t\n") {
			fmt.Fprintf(os.Stderr, "%s\n\n", cliUsage)
			return fmt.Errorf("unknown verb %q — to make an ask, use: self prompt %q", verb, verb)
		}
		return cmdSituate(home, strings.Join(append([]string{verb}, args...), " "), out)
	}
}

func brief(home string, st *state) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# self — %s\n\n", home)

	caller := callerClaim()
	if caller == "" {
		caller = `unset — export SELF_CALLER="<who you are>" so your writes are attributable`
	}
	fmt.Fprintf(&b, "log: %d events    caller: %s\n", len(st.Events), caller)
	fmt.Fprintf(&b, "head: %s\n", head(st.Events))
	if anyClipped(st) {
		b.WriteString("lines below are clipped; `self brief <name>` prints one declaration in full\n")
	}
	if len(st.Events) > 0 && st.Key == nil {
		b.WriteString("\n**no .secret beside this log** — no receipt can verify, so this instance has no capabilities.\n")
	}

	b.WriteString("\n## commands — `self run <name> [args…]`\n\n")
	b.WriteString(capabilityList(st, kindCommand))
	b.WriteString("\n## views — `self view <name> [args…]`\n\n")
	b.WriteString(capabilityList(st, kindView))

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
	b.WriteString(intentBrief(st))

	if st.capabilitiesReady() && len(st.openIntents()) == 0 {
		b.WriteString("\nnothing pending, nothing refused.\n")
	}

	b.WriteString("\n## where\n\n")
	b.WriteString("`events.jsonl` the log, authoritative · `cap/` installed scripts, derived · `.secret` the signing key\n")
	b.WriteString("`self help` the protocol · `self view log` what happened lately · `self brief <name>` one capability in full\n")
	return b.String()
}

func unfoldMissing(st *state, typ, name string) string {
	if st.cap(typ, name) != nil || st.cap(otherKind(typ), name) != nil {
		return ""
	}
	if typ == kindView && name == "log" {
		return ""
	}
	return fmt.Sprintf("self: no %s %q in this log. What there is:\n\n%s\n", typ, name, capabilityList(st, typ))
}

func unfoldFailed(st *state, typ, name string, silent bool) string {
	c := st.cap(typ, name)
	if c == nil || c.Receipt == nil {
		return ""
	}
	if silent {
		if page, err := briefOne(st, typ+"/"+name); err == nil {
			return "\n" + page
		}
	}
	return fmt.Sprintf("\nself: `self brief %s` — what %s is for, and what its arguments mean\n", name, c.key())
}

type tally struct {
	w io.Writer
	n int
}

func (t *tally) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	t.n += n
	return n, err
}

func otherKind(typ string) string {
	if typ == kindView {
		return kindCommand
	}
	return kindView
}

func briefOne(st *state, selector string) (string, error) {
	if name, ok := strings.CutPrefix(selector, "work/"); ok {
		return intentDetail(st, name)
	}
	if name, ok := strings.CutPrefix(selector, "intent/"); ok {
		return intentDetail(st, name)
	}
	var found []*capability
	if typ, name, qualified := strings.Cut(selector, "/"); qualified && (typ == kindCommand || typ == kindView) {
		if c := st.cap(typ, name); c != nil {
			found = append(found, c)
		}
	} else {
		for _, typ := range []string{kindCommand, kindView} {
			if c := st.cap(typ, selector); c != nil {
				found = append(found, c)
			}
		}
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no capability %q in this log — `self brief` lists what there is", selector)
	}
	var b strings.Builder
	for i, c := range found {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "# %s\n\n%s\n", c.key(), strings.TrimSpace(c.Decl.Description))
		if c.Type == kindView {
			consumes := strings.Join(c.Decl.Consumes, ", ")
			if consumes == "" {
				consumes = "the whole log"
			}
			fmt.Fprintf(&b, "\nconsumes: %s\n", consumes)
		}
		fmt.Fprintf(&b, "declared at seq %d", c.DeclSeq)
		if c.Receipt != nil {
			fmt.Fprintf(&b, " · installed at seq %d", c.RcptSeq)
		}
		if c.Pending() {
			b.WriteString(" · " + strings.Trim(oneLine(pendingMark(c)), "*() "))
		}
		b.WriteString("\n")
		if c.Reject != nil {
			fmt.Fprintf(&b, "refused at seq %d: %s\n", c.Reject.Seq, oneLine(c.Reject.Reason))
		}
	}
	return b.String(), nil
}

func capabilityList(st *state, typ string) string {
	var b strings.Builder
	caps := st.list(typ)
	for _, c := range caps {
		fmt.Fprintf(&b, "- %s — %s%s\n", c.Name, c.Decl.summary(), pendingMark(c))
	}
	if typ == kindView && st.cap(kindView, "log") == nil {
		fmt.Fprintf(&b, "- log — the last %d events; `--all` for every one (built in, shadowable)\n", builtinLogTail)
	}
	if len(caps) == 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "(no declared %ss yet — `self help` shows how to author one)\n", typ)
	}
	return b.String()
}

func anyClipped(st *state) bool {
	for _, c := range st.Caps {
		if strings.HasSuffix(c.Decl.summary(), "…") {
			return true
		}
	}
	return false
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

func oneLine(s string) string {
	if s := oneLineOrEmpty(s); s != "" {
		return s
	}
	return "(no description)"
}

func oneLineOrEmpty(s string) string { return strings.Join(strings.Fields(s), " ") }

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	if sp := strings.LastIndexByte(s[:cut], ' '); sp > n/2 {
		cut = sp
	}
	return strings.TrimRight(s[:cut], " ,;:.—-") + "…"
}

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
