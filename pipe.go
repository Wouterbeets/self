package main

// The seam. `self` is a filter and the mind is whatever the shell puts between
// two invocations of it:
//
//	echo "add a mood tracker" | self | claude -p | self
//
// Two faces, and the content of stdin picks which — never a terminal, never a
// flag. The law is that reads project and writes append: situating an ask
// appends nothing, so an agent can orient a hundred times without scarring the
// log. That is the whole dispatcher, and it behaves identically in a terminal,
// a pipe, a script, a sandbox and cron.
//
// The previous kernel decided this with isatty, which meant the documented loop
// misfiled a mind's reply as a question everywhere an agent actually runs, and
// meant no one could know what `self` did without simulating file descriptors.

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// protocolDoc is the contract, embedded so a `self` on PATH describes itself
// with no repo in sight. `self help` prints it whole; the situated prompt
// splices the marked section. One description of the wire, in one place.
//
//go:embed PROTOCOL.md
var protocolDoc string

// wireContract is PROTOCOL.md's own words about the wire, spliced rather than
// restated. Six hand-synced copies of one contract is how the previous kernel
// came to contradict itself inside a single brief.
func wireContract() string {
	_, rest, ok := strings.Cut(protocolDoc, "<!-- prompt:begin -->")
	if !ok {
		return ""
	}
	body, _, _ := strings.Cut(rest, "<!-- prompt:end -->")
	return strings.TrimSpace(body)
}

// defaultAsk is what bare `self` situates: not a face, just the text used when
// nobody supplied one. Pending work is already in the brief, so this does not
// need a priority queue — it needs to point at what is there.
const defaultAsk = `No specific ask. Look at the brief above, then decide.

1. If declarations are pending, author them. That is the work of this pass.
2. If a refusal stands, resolve it: author the capability correctly, or retire it.
3. Otherwise read this instance's views — ` + "`self view <name>`" + ` — and act on
   what they show. Unfinished domain work is real work, and the kernel cannot see
   it: only the views can.
4. If nothing there needs doing, choose ONE small improvement to this instance
   and make it, or print nothing. Silence is a valid turn.`

// ──────────────────────────────── the seam ──────────────────────────────────
//
// Direction is structural, not sniffed. An ask arrives as ARGV; what comes back
// from a mind arrives on STDIN, at `self hear`. Prose alone cannot tell the two
// apart — "what is going on?" and a mind's answer to it are both prose — which
// is exactly why the previous kernel reached for isatty and got the loop wrong
// everywhere an agent runs. So the read face never reads stdin (it would also
// block at the head of a pipeline), and the write face is named.

// situate is the read face: everything a cold mind needs to act, and nothing the
// log does not already hold. Appends nothing, ever.
func cmdSituate(home string, ask string, out io.Writer) error {
	empty := strings.TrimSpace(ask) == ""
	if empty {
		ask = defaultAsk
	}
	st, err := loadState(home)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(out, situate(home, st, ask)); err != nil {
		return err
	}
	if empty && st.quiet() {
		return errQuiet
	}
	return nil
}

// errQuiet is exit code 3: the read succeeded and there was nothing to do. It is
// what lets `while self | mind | self hear; do :; done` terminate.
var errQuiet = fmt.Errorf("nothing pending")

// cmdHear is the write face — the only door a mind's output enters through.
// Event lines land and install; every other line is ignored, echoed, and
// counted. Nothing else is written.
//
// This used to be strict: a body was the wire only if EVERY line was an event,
// on the theory that a mind narrating around JSON must not partially mutate
// state. Driving the real loop killed that theory. Told plainly that stdout is
// the wire, `claude -p` opened with one line — "Printing the six lines to
// stdout now, exactly as the wire requires" — and six perfect events followed.
// Strictness threw all six away. That is the modal behaviour of a chat-trained
// model, and the property strictness protected does not exist here: `hear` is
// only ever invoked to ingest, so there is no reply face for a prose body to be
// mistaken for. Leniency is also less code — a stray fence or a backticked line
// is just a line that is not an event.
func cmdHear(home string, input []byte, out io.Writer) error {
	evs, scripts, prose := wire(string(input))
	if len(evs) == 0 && len(scripts) == 0 {
		if strings.TrimSpace(string(input)) != "" {
			fmt.Fprintf(os.Stderr, "self: heard no events — passed %d line(s) through and wrote nothing\n", len(prose))
		}
		_, err := out.Write(input)
		return err
	}
	return hear(home, evs, scripts, prose, out)
}

// ────────────────────────────────── wire ────────────────────────────────────

// authored is a script a mind wrote, carried on the wire as script.authored. It
// never lands in the log raw: the signed receipt is its record.
type authored struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Script string `json:"script"`
}

// wire splits a mind's output into events, authored scripts, and everything
// else. A line is an event when it is a JSON object with a dotted lowercase
// name AND a payload key. Both halves matter: on the name test alone, a mind
// reporting {"name":"notes","status":"ok"} would land an event called "notes" in
// the authoritative log.
func wire(body string) (evs []Event, scripts []authored, prose []string) {
	for _, line := range lines(body) {
		probe, ok := eventLine(line)
		if !ok {
			prose = append(prose, line)
			continue
		}
		if probe.Name == "script.authored" {
			// Kept even when malformed: a bad authored line is a failure the
			// log must remember, not a line to drop.
			var a authored
			json.Unmarshal(probe.Payload, &a)
			scripts = append(scripts, a)
			continue
		}
		evs = append(evs, newEvent(probe.Name, probe.Payload))
	}
	return evs, scripts, prose
}

type wireLine struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

// eventLine parses one line, retrying without surrounding backticks — models
// wrap single lines in code spans, and a fence line simply is not an event.
func eventLine(line string) (wireLine, bool) {
	for _, candidate := range []string{line, strings.TrimSpace(strings.Trim(line, "`"))} {
		var w wireLine
		if json.Unmarshal([]byte(candidate), &w) == nil && validEventName(w.Name) && w.Payload != nil {
			return w, true
		}
	}
	return wireLine{}, false
}

func lines(body string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// ────────────────────────────────── hear ────────────────────────────────────

// hear is the write door: events land, authored scripts install under signed
// receipts, and the outcome is reported. The whole body is one critical
// section — a declaration and its script arrive in the same breath, and
// resolving declared-ness between them must not race another invocation
// retiring the same capability.
func hear(home string, evs []Event, scripts []authored, prose []string, out io.Writer) error {
	key, err := ensureSecret(home)
	if err != nil {
		return err
	}
	unlock, err := lockLog(home)
	if err != nil {
		return err
	}
	defer unlock()

	by := callerClaim()
	for i := range evs {
		evs[i].Via, evs[i].By = doorHear, by
	}
	if err := appendLocked(home, evs); err != nil {
		return err
	}

	events, err := readEvents(home)
	if err != nil {
		return err
	}
	st := replay(events, key)

	var installed, refused []string
	for _, a := range scripts {
		r, err := install(home, st, a, by)
		if err != nil {
			rej := rejection{Type: a.Type, Name: a.Name, Reason: err.Error(), Excerpt: trunc(a.Script, excerptCap)}
			payload, _ := json.Marshal(rej)
			e := newEvent("script.rejected", payload)
			e.Via = doorKernel // the refusal is the kernel's own act
			// appendLocked assigns Seq into the slice it is given, so read the
			// event back from there: a copy would carry seq 0 and make the
			// capability look eternally pending.
			batch := []Event{e}
			if err := appendLocked(home, batch); err != nil {
				return err
			}
			e = batch[0]
			rej.Seq = e.Seq
			st.Events = append(st.Events, e)
			st.Reject = append(st.Reject, &rej)
			if c := st.cap(a.Type, a.Name); c != nil {
				c.Reject = &rej
			}
			refused = append(refused, fmt.Sprintf("%s/%s: %s", a.Type, a.Name, err))
			continue
		}
		payload, _ := json.Marshal(r)
		e := newEvent("script.installed", payload)
		e.Via = doorKernel // the receipt is the kernel's own act; the author is signed inside
		batch := []Event{e}
		if err := appendLocked(home, batch); err != nil {
			return err
		}
		e = batch[0]
		st.Events = append(st.Events, e)
		c := st.cap(r.Type, r.Name)
		c.Receipt, c.RcptSeq, c.Reject = &r, e.Seq, nil
		st.Reject = dropRejection(st.Reject, c.key())
		if _, err := materialize(home, st, r.Type, r.Name); err != nil {
			return err
		}
		installed = append(installed, c.key())
	}

	retired := applyRetirements(home, evs)

	// The report goes to stdout: this is the last stage of a pipeline, and
	// whether the script installed is the one thing its operator needs.
	if len(evs) > 0 {
		fmt.Fprintf(out, "heard %d event(s): seq %d-%d\n", len(evs), evs[0].Seq, evs[len(evs)-1].Seq)
	}
	for _, k := range installed {
		fmt.Fprintf(out, "installed %s under a signed receipt\n", k)
	}
	for _, r := range refused {
		fmt.Fprintf(out, "REFUSED %s\n", r)
	}
	for _, k := range retired {
		fmt.Fprintf(out, "retired %s\n", k)
	}
	if len(prose) > 0 {
		fmt.Fprintf(os.Stderr, "self: ignored %d line(s) that were not events (echoed below the report)\n", len(prose))
		for _, line := range prose {
			fmt.Fprintln(out, line)
		}
	}
	if p := st.pending(); len(p) > 0 {
		names := make([]string, 0, len(p))
		for _, c := range p {
			names = append(names, c.key())
		}
		fmt.Fprintf(out, "pending: %s\n", strings.Join(names, ", "))
	}
	if len(refused) > 0 {
		return fmt.Errorf("%d authored script(s) refused", len(refused))
	}
	return nil
}

// install is the trust gate. A mind can only ever propose: the capability must
// be declared in this log and not retired, and the kernel signs the bytes with
// its own key. The consumes list is taken from the DECLARATION and signed with
// the script, so what a view was signed against is what it will be fed.
func install(home string, st *state, a authored, by string) (receipt, error) {
	typ, name := strings.TrimSpace(a.Type), strings.TrimSpace(a.Name)
	if typ == "" || name == "" {
		return receipt{}, fmt.Errorf("script.authored needs both type and name")
	}
	if !validCapability(typ, name) {
		return receipt{}, fmt.Errorf("script.authored for an unusable %s name %q (lowercase path segments; a trailing \"run\" segment is reserved)", typ, name)
	}
	if strings.TrimSpace(a.Script) == "" {
		return receipt{}, fmt.Errorf("script.authored carries no script")
	}
	c := st.cap(typ, name)
	if c == nil {
		return receipt{}, fmt.Errorf("%s/%s is not declared in this log — declare it in the same body, before the script", typ, name)
	}
	r := receipt{Type: typ, Name: name, Script: a.Script, By: by}
	if typ == kindView {
		r.Consumes = c.Decl.Consumes
	}
	r.Sig = sign(st.Key, r)
	return r, nil
}

func dropRejection(rs []*rejection, key string) []*rejection {
	out := rs[:0]
	for _, r := range rs {
		if r.Type+"/"+r.Name != key {
			out = append(out, r)
		}
	}
	return out
}

// applyRetirements takes retired capabilities off the readable surface as their
// tombstones land, so disk never claims something the log has ended. Every
// event stays; re-declaring revives it.
func applyRetirements(home string, evs []Event) []string {
	var out []string
	for _, e := range evs {
		if e.Name != "capability.retired" {
			continue
		}
		var t struct{ Type, Name string }
		if json.Unmarshal(e.Payload, &t) != nil || !validCapability(t.Type, t.Name) {
			continue
		}
		unlink(home, t.Type, t.Name)
		out = append(out, t.Type+"/"+t.Name)
	}
	return out
}

// ───────────────────────────────── the prompt ───────────────────────────────

// situate builds the situated prompt: the brief, pending work with the reason
// each previous attempt failed, one exemplar of this instance's idiom, the wire
// contract spliced from PROTOCOL.md, and the ask.
func situate(home string, st *state, ask string) string {
	var b strings.Builder
	b.WriteString("You are the mind of a self instance. This came out of `self`; your stdout goes back into `self`.\n\n")
	b.WriteString(brief(home, st))
	b.WriteString(pendingSection(st))
	b.WriteString("\n")
	b.WriteString(wireContract())
	b.WriteString("\n\n## The ask\n\n")
	b.WriteString(strings.TrimSpace(ask))
	b.WriteString("\n")
	return b.String()
}

// pendingSection is the strange loop's ask. The rejection reason replayed here
// is the only thing that makes script.rejected teach anyone anything: without
// it the refusal is a failure the log remembers and nothing reads.
func pendingSection(st *state) string {
	pending := st.pending()
	if len(pending) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Pending — declared, awaiting a script\n\n")
	b.WriteString("Author each one, test it by running it, and print its script.authored line.\n")
	skip := map[string]bool{}
	for _, c := range pending {
		skip[c.key()] = true
		d, _ := json.Marshal(c.Decl)
		fmt.Fprintf(&b, "\n%s %q declared at seq %d:\n%s\n", c.Type, c.Name, c.DeclSeq, d)
		if c.Reject != nil {
			fmt.Fprintf(&b, "Your previous attempt was REFUSED: %s\nDo not repeat that mistake.\n", c.Reject.Reason)
		}
	}
	if name, script := st.exemplar(skip); script != "" {
		fmt.Fprintf(&b, "\nAn installed capability of this instance, as idiom — learn its shape, do not copy it:\n\n--- %s ---\n%s\n--- end ---\n", name, script)
	}
	return b.String()
}
