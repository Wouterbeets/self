// self — a local-first, event-sourced runtime, and to the shell a filter with a
// memory. One append-only log is the only authoritative state; every capability
// and every view is a deterministic replay of it. The kernel holds no model and
// spawns nothing: intelligence enters through a shell pipe, where the mind is
// whatever process you put beside it.
//
//	self "add a mood tracker" | claude -p | self hear
//
// An ask arrives as argv, so the first self situates it against the instance's
// own state and appends nothing. The mind does durable work through installed
// commands and prints events. `self hear` lands them: events append, and
// authored scripts install under receipts the kernel signs with a key only it
// holds. A declaration without a script stays pending and rides the next
// prompt, so the loop converges — that is the strange loop, one shell pass at a
// time.
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
		st, err := loadState(home)
		if err != nil {
			return err
		}
		if len(args) < 1 {
			_, err = io.WriteString(out, listCaps(st, kindCommand))
			return err
		}
		// No ensureSecret here. Minting a key on this path meant `self run`
		// dropped a .secret in whatever directory it was called from — and, on a
		// real instance whose key went missing, forged a fresh one, hiding the
		// only honest diagnostic there is.
		evs, err := runCommand(home, st, args[0], args[1:], doorCLI, callerClaim())
		if err != nil {
			return fmt.Errorf("%w\n%s", err, strings.TrimRight(listCaps(st, kindCommand), "\n"))
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
		if len(args) == 0 {
			_, err = io.WriteString(out, listCaps(st, kindView))
			return err
		}
		if len(args) != 1 {
			return fmt.Errorf("usage: self view <name>\n%s", strings.TrimRight(listCaps(st, kindView), "\n"))
		}
		page, err := runView(home, st, args[0])
		if err != nil {
			return fmt.Errorf("%w\n%s", err, strings.TrimRight(listCaps(st, kindView), "\n"))
		}
		_, err = out.Write(page)
		return err

	case "learn":
		if len(args) != 1 {
			return fmt.Errorf("usage: self learn <account-dir>")
		}
		return cmdLearn(home, args[0], out)

	case "work":
		return cmdWork(home, args, out)

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
			return fmt.Errorf("unknown verb %q — verbs: hear brief run view work learn give rehydrate help; to ask, quote it: self %q", verb, verb)
		}
		return cmdSituate(home, strings.Join(append([]string{verb}, args...), " "), out)
	}
}

// ───────────────────────────────── the brief ────────────────────────────────

// brief is the state card: home, inventory, standing refusals. Facts only, one
// screen. The contract lives in PROTOCOL.md and there is exactly one copy of
// it. This is the read an agent starts from, and every line of it is a replay
// of the log.
func brief(home string, st *state) string {
	var b strings.Builder
	caller := callerClaim()
	if caller == "" {
		caller = "unset — SELF_CALLER"
	}
	fmt.Fprintf(&b, "%s  %d events  caller %s\n", home, len(st.Events), caller)
	if len(st.Events) > 0 && st.Key == nil {
		b.WriteString("no .secret — no receipt verifies\n")
	}
	b.WriteByte('\n')
	b.WriteString(listCaps(st, kindCommand, kindView))
	if len(st.Work) > 0 {
		b.WriteByte('\n')
		b.WriteString(listWork(st))
	}
	if len(st.Reject) > 0 {
		b.WriteByte('\n')
		for _, r := range st.Reject {
			where := strings.Trim(r.Type+"/"+r.Name, "/")
			if where == "" {
				where = "(unnamed)"
			}
			fmt.Fprintf(&b, "refused  %s  seq %d  %s\n", where, r.Seq, oneLine(r.Reason))
		}
	}
	return b.String()
}

type capRow struct{ kind, name, status, rest string }

// listCaps is the index a verb without a name prints: aligned columns, no
// prose. `self view` lists views; `self run` lists commands; the brief lists
// both.
func listCaps(st *state, kinds ...string) string {
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	var rows []capRow
	if want[kindCommand] {
		cmds := st.list(kindCommand)
		if len(cmds) == 0 {
			rows = append(rows, capRow{kind: "cmd", name: "—"})
		}
		for _, c := range cmds {
			rows = append(rows, capRow{"cmd", c.Name, capStatus(c), capRest(c)})
		}
	}
	if want[kindView] {
		for _, c := range st.list(kindView) {
			rows = append(rows, capRow{"view", c.Name, capStatus(c), capRest(c)})
		}
		if st.cap(kindView, "log") == nil {
			rows = append(rows, capRow{"view", "log", "built-in", "every event"})
		}
	}
	return formatCapRows(rows)
}

func listWork(st *state) string {
	if len(st.Work) == 0 {
		return "work  —\n"
	}
	var b strings.Builder
	for _, w := range st.Work {
		fmt.Fprintf(&b, "work  seq %-4d  %s\n", w.Seq, trunc(oneLine(w.Text), 80))
	}
	return b.String()
}

func cmdWork(home string, args []string, out io.Writer) error {
	st, err := loadState(home)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		_, err = io.WriteString(out, listWork(st))
		return err
	}
	if args[0] == "done" {
		if len(args) != 2 {
			return fmt.Errorf("usage: self work done <seq>\n%s", strings.TrimRight(listWork(st), "\n"))
		}
		var seq int
		if _, err := fmt.Sscanf(args[1], "%d", &seq); err != nil || seq < 1 {
			return fmt.Errorf("usage: self work done <seq>\n%s", strings.TrimRight(listWork(st), "\n"))
		}
		open := false
		for _, w := range st.Work {
			if w.Seq == seq {
				open = true
				break
			}
		}
		if !open {
			return fmt.Errorf("no open work seq %d\n%s", seq, strings.TrimRight(listWork(st), "\n"))
		}
		payload, _ := json.Marshal(map[string]int{"seq": seq})
		e := newEvent("work.done", payload)
		e.Via, e.By = doorCLI, callerClaim()
		if err := appendEvents(home, []Event{e}); err != nil {
			return err
		}
		fmt.Fprintf(out, "done  seq %d\n", seq)
		return nil
	}
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		return fmt.Errorf("usage: self work <text…>")
	}
	payload, _ := json.Marshal(map[string]string{"text": text})
	e := newEvent("work.queued", payload)
	e.Via, e.By = doorCLI, callerClaim()
	// appendEvents assigns Seq into the slice it is given, so read the event
	// back from there: a copy would carry seq 0 and the next prompt would
	// print a line that is not the line in the log.
	batch := []Event{e}
	if err := appendEvents(home, batch); err != nil {
		return err
	}
	fmt.Fprintf(out, "work  seq %d  %s\n", batch[0].Seq, trunc(oneLine(text), 80))
	return nil
}

func capStatus(c *capability) string {
	if c.Pending() && c.Receipt != nil {
		return "stale"
	}
	if c.Pending() {
		return "pending"
	}
	return "ok"
}

func capRest(c *capability) string {
	rest := oneLine(c.Decl.Description)
	if c.Type == kindView {
		if tag := consumesTag(c.Decl.Consumes); tag != "" {
			rest += "  " + tag
		}
	}
	if c.Pending() {
		rest += fmt.Sprintf("  seq %d", c.DeclSeq)
	}
	return rest
}

func consumesTag(cs []string) string {
	if len(cs) == 0 {
		return ""
	}
	return "[" + strings.Join(cs, ",") + "]"
}

func formatCapRows(rows []capRow) string {
	nw := 8
	for _, r := range rows {
		if n := utf8.RuneCountInString(r.name); n > nw {
			nw = n
		}
	}
	if nw > 20 {
		nw = 20
	}
	var b strings.Builder
	for _, r := range rows {
		name := r.name
		if utf8.RuneCountInString(name) > 20 {
			name = trunc(name, 20)
		}
		if r.status == "" {
			fmt.Fprintf(&b, "%-4s  %s\n", r.kind, name)
			continue
		}
		fmt.Fprintf(&b, "%-4s  %-*s  %-8s  %s\n", r.kind, nw, name, r.status, trunc(r.rest, 56))
	}
	return b.String()
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
