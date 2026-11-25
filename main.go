package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const cliUsage = `self — local-first event-sourced runtime

Usage:
  self                        reconnect with your persistent self for the current task
  self setup                  print what to offer so every session starts with self
  self make <kind> <name> <summary...> < script   declare and install a capability in one step
  self prompt [ask...]         print a prompt for an event-producing mind
  self hear [--after <head>]     ingest event JSONL or authored scripts from stdin
  self brief [name]             the state; with a name or intent/<name>, full detail
  self run <command> [args...]  execute a command capability and append its events
  self view <name> [args...]    replay a pure view; built-in log is always available
  self learn [--into <intent>] <account-dir>  deposit and interpret an account
  self watch [opts] [prefix]   wait for events without appending
  self give <selector> <dir>    write an event or capability account
  self rehydrate                rebuild derived capability files from the log
  self help                     print the complete protocol

Environment:
  SELF_HOME, SELF_CALLER`

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
		var exit *exec.ExitError
		if (verb == "run" || verb == "view") && errors.As(err, &exit) && exit.ExitCode() > 0 {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}

func dispatch(home, verb string, args []string, out io.Writer) error {
	limits := map[string]int{"": 0, "setup": 0, "brief": 1, "rehydrate": 0, "help": 0, "-h": 0, "--help": 0}
	if max, fixed := limits[verb]; fixed && len(args) > max {
		return fmt.Errorf("self %s accepts at most %d argument(s); see self --help", verb, max)
	}
	switch verb {
	case "":
		return cmdOrient(home, out)

	case "setup":
		_, err := io.WriteString(out, protocolLayer("setup")+"\n")
		return err

	case "make":
		return cmdMake(home, args, os.Stdin, out)

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
			return fmt.Errorf("read stdin: %w", err)
		}
		return cmdHear(home, input, out, args...)

	case "watch":
		return cmdWatch(home, args, out)

	case "brief":
		st, err := loadState(home)
		if err != nil {
			return err
		}
		page := brief(home, st) + "\n`self` with no arguments says what this instance is for and what to keep in it.\n"
		if len(args) > 0 {
			if page, err = briefOne(st, args[0]); err != nil {
				return err
			}
		}
		_, err = io.WriteString(out, page)
		return err

	case "run", "view":
		st, err := loadState(home)
		if err != nil {
			return err
		}
		typ, arg := kindView, "name"
		if verb == "run" {
			typ, arg = kindCommand, "command"
		}
		if len(args) == 0 {
			_, err := fmt.Fprintf(out, "usage: self %s <%s> [args...]\n\n%s", verb, arg, capabilityList(st, typ, viaCounts(st)))
			return err
		}
		name, rest := args[0], args[1:]
		io.WriteString(os.Stderr, unfoldMissing(st, typ, name))
		said := &tally{w: os.Stderr}
		var page []byte
		if verb == "view" {
			page, err = runViewDiag(home, st, name, said, rest...)
		} else {
			var evs []Event
			evs, err = runCommand(home, st, name, rest, doorCLI+":"+name, callerClaim(), said, out)
			for _, e := range evs {
				page = fmt.Appendf(page, "%d\t%s\t%s\n", e.Seq, e.Name, trunc(compact(e.Payload), 160))
			}
		}
		if err != nil {
			io.WriteString(os.Stderr, unfoldFailed(st, typ, name, said.n == 0))
			return err
		}
		_, err = out.Write(page)
		return err

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

	case "-h", "--help":
		_, err := io.WriteString(out, protocolLayer("core")+"\n\n"+
			"Run `self` with no arguments to arrive: it only reads, and prints this with everything the instance holds.\n\n"+cliUsage+"\n")
		return err

	case "help":
		_, err := io.WriteString(out, protocolDoc)
		return err

	default:
		fmt.Fprintf(os.Stderr, "%s\n\n", cliUsage)
		return fmt.Errorf("unknown verb %q — to make an ask, use: self prompt %q", verb, strings.Join(append([]string{verb}, args...), " "))
	}
}

const listedInSession = 12

func writeBriefHeader(b *strings.Builder, home string, st *state, full bool) {
	caller := cmp.Or(callerClaim(), "unnamed — set SELF_CALLER to sign what you make")
	if full {
		fmt.Fprintf(b, "# self — %s\n\nlog: %d events    caller: %s\nhead: %s\n", home, len(st.Events), caller, head(st.Events))
		if anyClipped(st) {
			b.WriteString("lines below are clipped; `self brief <name>` prints one declaration in full\n")
		}
	} else {
		fmt.Fprintf(b, "# self · %s · %d events · caller %s\n\n", home, len(st.Events), caller)
	}
	if len(st.Events) > 0 && st.Key == nil {
		b.WriteString("**no .secret beside this log** — no receipt can verify, so this instance has no capabilities.\n\n")
	}
}

func writeIssues(b *strings.Builder, st *state) {
	if p := st.pending(); len(p) > 0 {
		b.WriteString("\n## pending — declared, no script yet\n\n")
		for _, c := range p {
			fmt.Fprintf(b, "- %s (declared at seq %d)\n", c.key(), c.DeclSeq)
		}
	}
	if len(st.Reject) > 0 {
		b.WriteString("\n## refused — standing, until authored or retired\n\n")
		for _, r := range st.Reject {
			where := cmp.Or(strings.Trim(r.Type+"/"+r.Name, "/"), "(unnamed)")
			fmt.Fprintf(b, "- %s (seq %d): %s\n", where, r.Seq, oneLine(r.Reason))
		}
	}
}

func brief(home string, st *state) string {
	var b strings.Builder
	writeBriefHeader(&b, home, st, true)
	counts := viaCounts(st)
	fmt.Fprintf(&b, "\n## commands — `self run <name> [args…]`\n\n%s\n## views — `self view <name> [args…]`\n\n%s",
		capabilityList(st, kindCommand, counts), capabilityList(st, kindView, counts))
	writeIssues(&b, st)
	b.WriteString(intentBrief(st))
	b.WriteString("\n## where\n\n" +
		"`events.jsonl` the log, authoritative · `cap/` installed scripts, to read · `.secret` the signing key\n" +
		"`self view log` what happened lately\n")
	return b.String()
}

func orientBrief(home string, st *state) string {
	var b strings.Builder
	last := lastUses(st)
	if len(st.Caps) <= listedInSession {
		writeBriefHeader(&b, home, st, true)
		counts := viaCounts(st)
		fmt.Fprintf(&b, "\n## commands — `self run <name> [args…]`\n\n%s\n## views — `self view <name> [args…]`\n\n%s",
			capabilityList(st, kindCommand, counts), capabilityList(st, kindView, counts))
		writeIssues(&b, st)
		if u := untouchedLine(st, last); u != "" {
			b.WriteString("\n" + u)
		}
		b.WriteString(intentBrief(st))
		return b.String()
	}
	writeBriefHeader(&b, home, st, false)
	fmt.Fprintf(&b, "%d commands, %d views: `self brief` says what each is for.\n", len(st.list(kindCommand)), len(st.list(kindView)))
	if names := lately(st, last, 10); len(names) > 0 {
		fmt.Fprintf(&b, "Used lately: %s\n", strings.Join(names, " · "))
	}
	b.WriteString(untouchedLine(st, last))
	if p, r := len(st.pending()), len(st.Reject); p+r > 0 {
		fmt.Fprintf(&b, "%d declared without a script, %d refused: `self brief` lists them.\n", p, r)
	}
	b.WriteString(wantedLine(st))
	return b.String()
}

func untouchedLine(st *state, last map[string]time.Time) string {
	names := untouched(st, last)
	if len(names) == 0 {
		return ""
	}
	const shown = 8
	more := ""
	if len(names) > shown {
		more = fmt.Sprintf(" and %d more", len(names)-shown)
		names = names[:shown]
	}
	return fmt.Sprintf("Untouched for 30 days: %s%s. Rework, retire, or keep one on purpose with a capability.kept event and its reason.\n", strings.Join(names, " · "), more)
}

func wantedLine(st *state) string {
	open := st.openIntents()
	if len(open) == 0 {
		return ""
	}
	line := fmt.Sprintf("%d wanted (`self brief intent/`)", len(open))
	kept := map[string]time.Time{}
	for i := range st.Events {
		if e := &st.Events[i]; e.Name == "intent.kept" {
			var k struct{ Name string }
			if json.Unmarshal(e.Payload, &k) == nil && e.OccurredAt.After(kept[k.Name]) {
				kept[k.Name] = e.OccurredAt
			}
		}
	}
	var oldest *intentItem
	stale := 0
	for _, w := range open {
		at := eventAt(st, w.Seq)
		if k := kept[w.Name]; k.After(at) {
			at = k
		}
		if !at.IsZero() && time.Since(at) >= untouchedAfter {
			stale++
			if oldest == nil || w.Seq < oldest.Seq {
				oldest = w
			}
		}
	}
	if stale > 0 {
		line += fmt.Sprintf("; %d open over 30 days, oldest intent/%s. Pursue, close, or keep one with an intent.kept event and its reason", stale, oldest.Name)
	}
	return line + ".\n"
}

func unfoldMissing(st *state, typ, name string) string {
	if st.cap(typ, name) != nil || st.cap(otherKind(typ), name) != nil || (typ == kindView && name == "log") {
		return ""
	}
	return fmt.Sprintf("self: no %s %q in this log. What there is:\n\n%s\n", typ, name, capabilityList(st, typ, viaCounts(st)))
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
		if c.Type == kindView || len(c.Decl.Consumes) > 0 {
			consumes := strings.Join(c.Decl.Consumes, ", ")
			if consumes == "" {
				consumes = "the whole log"
			}
			fmt.Fprintf(&b, "\nconsumes: %s\n", consumes)
			if s := c.Decl.Brief; s != nil {
				fmt.Fprintf(&b, "brief section: order %d, %d bytes\n", s.Order, s.budget())
			}
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

func capabilityList(st *state, typ string, counts map[string]int) string {
	var b strings.Builder
	caps := st.list(typ)
	for _, c := range caps {
		fmt.Fprintf(&b, "- %s — %s%s%s\n", c.Name, c.Decl.summary(), provenance(c, counts), pendingMark(c))
	}
	if typ == kindView && st.cap(kindView, "log") == nil {
		fmt.Fprintf(&b, "- log — the last %d events; `--all` for every one (built in, shadowable)\n", builtinLogTail)
	}
	if len(caps) == 0 && typ == kindCommand {
		b.WriteString(noneYet(st))
	}
	return b.String()
}

func noneYet(st *state) string {
	if len(st.Events) == 0 {
		return "nothing yet: the first one is yours to make\n"
	}
	return "none yet\n"
}

func provenance(c *capability, counts map[string]int) string {
	if c.Receipt == nil {
		return ""
	}
	who := c.Receipt.By
	if who != "" && who == callerClaim() {
		who = "you"
	}
	parts := []string{ago(time.Since(c.RcptAt))}
	if who != "" {
		parts[0] = "by " + who + ", " + parts[0]
	}
	if c.Type == kindCommand {
		n := counts[doorCLI+":"+c.Name]
		if n == 1 {
			parts = append(parts, "1 event since")
		} else if n > 1 {
			parts = append(parts, fmt.Sprintf("%d events since", n))
		}
	}
	return "  (" + strings.Join(parts, " · ") + ")"
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func anyClipped(st *state) bool {
	return slices.ContainsFunc(st.Caps, func(c *capability) bool { return strings.HasSuffix(c.Decl.summary(), "…") })
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

func oneLine(s string) string { return cmp.Or(oneLineOrEmpty(s), "(no description)") }

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
