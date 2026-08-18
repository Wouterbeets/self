package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The no-arg `self` is the mind pipe, not the server. Fds decide the face:
//
//	stdout is a pipe, stdin is a tty  → print an explore ask
//	stdin is a pipe, stdout is a tty  → eat (or print the ask, if stdin was prose)
//	both pipes                        → one hop: eat food, or turn prose into an ask
//	both ttys                         → pump through $SELF_MIND if it is a filter
//
// Prose never hits the log. Event lines and script.authored do, and only after
// the same gates a spawned mind already faced: kernel vocabulary is refused,
// a script installs only under a local receipt, a declaration without a
// matching script.authored stays unsigned. Continuation — the strange loop —
// is the next compile ask on stdout, or silence.

const stdinDoor = "stdin"

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// cmdDefault is `self` with no argv: ensure the home, then wear the face the
// fds ask for. Serve is `self serve`.
func cmdDefault(home string) error {
	if err := ensureHome(home); err != nil {
		return err
	}
	if err := rehydrateOpt(home, true); err != nil {
		return err
	}
	return cmdLoop(home, os.Stdin, os.Stdout, os.Stderr, isTTY(os.Stdin), isTTY(os.Stdout))
}

func cmdLoop(home string, in io.Reader, out, errw io.Writer, inTTY, outTTY bool) error {
	switch {
	case !inTTY && !outTTY:
		return hop(home, in, out, errw)
	case !inTTY && outTTY:
		return arrive(home, in, out, errw)
	case inTTY && !outTTY:
		return writeAsk(out, exploreAsk(home, ""))
	default:
		return pumpOrHint(home, "", out, errw)
	}
}

// hop is the middle of a pipe: food is eaten and the next ask (or silence)
// goes to stdout; prose-only stdin is a question that becomes an explore ask.
func hop(home string, in io.Reader, out, errw io.Writer) error {
	raw, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	got, err := parseIncoming(raw)
	if err != nil {
		return err
	}
	if !got.hasFood {
		return writeAsk(out, exploreAsk(home, strings.TrimSpace(string(raw))))
	}
	next, rep, err := receiveParsed(home, got)
	if err != nil {
		return err
	}
	fmt.Fprintln(errw, rep)
	if p := strings.TrimSpace(got.prose); p != "" {
		fmt.Fprintln(errw, p)
	}
	if next != "" {
		return writeAsk(out, next)
	}
	return nil
}

// arrive is the end of a pipe (stdout is a tty). Food is eaten. Prose-only
// stdin is a mind's reply — shown, not turned into another question — unless
// a filter mind is plugged, in which case it is a question and we pump.
func arrive(home string, in io.Reader, out, errw io.Writer) error {
	raw, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	got, err := parseIncoming(raw)
	if err != nil {
		return err
	}
	if !got.hasFood {
		q := strings.TrimSpace(string(raw))
		if mindExe() != "" {
			return pumpOrHint(home, q, out, errw)
		}
		// End of a pipe: prose is a reply, not a new question.
		// `echo hello | self | cat` is how you inspect the ask.
		if q != "" {
			fmt.Fprintln(out, q)
		}
		return nil
	}
	next, rep, err := receiveParsed(home, got)
	if err != nil {
		return err
	}
	fmt.Fprintln(errw, rep)
	if p := strings.TrimSpace(got.prose); p != "" {
		fmt.Fprintln(out, p)
	}
	if next != "" {
		if p := strings.TrimSpace(got.prose); p != "" {
			// stdout already held the reply; the continuation is the loop's nerve.
			fmt.Fprintln(errw, next)
			return nil
		}
		return writeAsk(out, next)
	}
	return nil
}

func pumpOrHint(home, question string, out, errw io.Writer) error {
	if mindExe() == "" {
		fmt.Fprint(errw, "self: pipe me (`echo hi | self | examples/mind-claude | self`) or set SELF_MIND to a stdin-filter mind.\n     to serve the instance: self serve\n")
		return fmt.Errorf("no pipe and no mind")
	}
	return pump(home, exploreAsk(home, question), out, errw)
}

func writeAsk(w io.Writer, ask string) error {
	if ask == "" {
		return nil
	}
	if !strings.HasSuffix(ask, "\n") {
		ask += "\n"
	}
	_, err := io.WriteString(w, ask)
	return err
}

func exploreAsk(home, question string) string {
	freshBrief(home)
	var b strings.Builder
	fmt.Fprintf(&b, "# self ask\n")
	fmt.Fprintf(&b, "kind: explore\n")
	fmt.Fprintf(&b, "home: %s\n", home)
	fmt.Fprintf(&b, "brief: %s\n\n", filepath.Join(home, "site", "brief.md"))
	b.WriteString("Read the brief — standing and contract. Stdout is for self.\n")
	if q := strings.TrimSpace(question); q != "" {
		b.WriteString("\nQUESTION\n")
		b.WriteString(q)
		b.WriteString("\n")
	}
	return b.String()
}

func compileAsk(home string) string {
	p, ok := nextUnsigned(home)
	if !ok {
		return ""
	}
	intent, reasoning := latestLearnContext(home)
	exName, exScript := exemplarScript(home, p.typ, p.name)
	return compilePrompt(intent, reasoning, exName, exScript, p.typ, p.name, string(p.decl))
}

func cmdCompile(home string) error {
	if err := ensureHome(home); err != nil {
		return err
	}
	ask := compileAsk(home)
	if ask == "" {
		return fmt.Errorf("nothing unsigned to compile")
	}
	return writeAsk(os.Stdout, ask)
}

// incoming is one stdin parse: event objects, authored scripts, leftover prose.
// hasFood is true when any object looked like an event or a script.authored —
// including refused kernel vocabulary — so a forged receipt is not a question.
type incoming struct {
	items   []inItem
	prose   string
	hasFood bool
	dropped int
}

type eatReport struct {
	ate      int
	dropped  int
	ignored  []string
	unsigned int
}

func (r eatReport) String() string {
	ign := "0"
	if n := len(r.ignored); n > 0 {
		seen := map[string]bool{}
		var names []string
		for _, n := range r.ignored {
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
		ign = fmt.Sprintf("%d (%s)", n, strings.Join(names, ", "))
	}
	return fmt.Sprintf("self: ate %d, dropped %d, ignored %s, unsigned %d", r.ate, r.dropped, ign, r.unsigned)
}

type inItem struct {
	event  *Event
	script *authored
}

type authored struct {
	Script string
	Type   string
	Name   string
}

func parseIncoming(raw []byte) (incoming, error) {
	var got incoming
	var prose strings.Builder
	flushProse := func() {
		t := strings.TrimSpace(prose.String())
		if t != "" {
			if got.prose != "" {
				got.prose += "\n"
			}
			got.prose += t
		}
		prose.Reset()
	}
	data := raw
	i := 0
	for i < len(data) {
		if data[i] == '\r' {
			i++
			continue
		}
		if isFenceLine(data, i) {
			i = skipLine(data, i)
			continue
		}
		if data[i] == '{' {
			obj, next, kind := takeJSONObject(data, i)
			switch kind {
			case jsonObjValid:
				flushProse()
				if !ingestObject(&got, obj) {
					got.dropped++
				}
				i = next
				continue
			case jsonObjInvalid:
				flushProse()
				got.dropped++
				i = next
				continue
			}
		}
		prose.WriteByte(data[i])
		i++
	}
	flushProse()
	return got, nil
}

const (
	jsonObjValid   = iota // matched braces, json.Valid
	jsonObjInvalid        // matched braces, not JSON
	jsonObjNone           // no complete object here
)

func isFenceLine(data []byte, i int) bool {
	for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
		i++
	}
	return i+3 <= len(data) && data[i] == '`' && data[i+1] == '`' && data[i+2] == '`'
}

func skipLine(data []byte, i int) int {
	for i < len(data) && data[i] != '\n' {
		i++
	}
	if i < len(data) {
		return i + 1
	}
	return i
}

func takeJSONObject(data []byte, start int) (obj []byte, next int, kind int) {
	if start >= len(data) || data[start] != '{' {
		return nil, start, jsonObjNone
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(data); i++ {
		c := data[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			switch c {
			case '\\':
				esc = true
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				chunk := bytes.TrimSpace(data[start : i+1])
				if json.Valid(chunk) {
					return chunk, i + 1, jsonObjValid
				}
				return chunk, i + 1, jsonObjInvalid
			}
		}
	}
	return nil, start, jsonObjNone
}

func ingestObject(got *incoming, obj []byte) bool {
	var e struct {
		Name    string          `json:"name"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(obj, &e) != nil || e.Name == "" {
		return false
	}
	got.hasFood = true
	if e.Name == "script.authored" {
		var p authored
		if json.Unmarshal(e.Payload, &p) != nil || strings.TrimSpace(p.Script) == "" {
			return false
		}
		got.items = append(got.items, inItem{script: &p})
		return true
	}
	ev := newEvent(e.Name, e.Payload)
	got.items = append(got.items, inItem{event: &ev})
	return true
}

func allowedReceive(name string) bool {
	switch name {
	case "command.declared", "projector.declared", "capability.retired":
		return true
	}
	if kernelVocabulary[name] {
		return false
	}
	return name != ""
}

func receiveSpeaker() string {
	if id := strings.TrimSpace(os.Getenv("SELF_MIND_ID")); id != "" {
		return id
	}
	return callerClaim()
}

func receiveParsed(home string, got incoming) (nextAsk string, rep eatReport, err error) {
	rep.dropped = got.dropped
	by := receiveSpeaker()
	for i := range got.items {
		it := got.items[i]
		if it.event != nil {
			if !allowedReceive(it.event.Name) {
				rep.ignored = append(rep.ignored, it.event.Name)
				continue
			}
			it.event.Via, it.event.By = stdinDoor, by
			if err := appendEvent(home, it.event); err != nil {
				return "", rep, err
			}
			rep.ate++
			if it.event.Name == "capability.retired" {
				applyRetirements(home, []Event{*it.event})
			}
			continue
		}
		if it.script == nil {
			continue
		}
		ok, err := installAuthored(home, *it.script, by)
		if err != nil {
			return "", rep, err
		}
		if ok {
			rep.ate++
		} else {
			rep.ignored = append(rep.ignored, "script.authored")
		}
	}
	refreshSite(home)
	rep.unsigned = len(unsignedDeclarations(home))
	return compileAsk(home), rep, nil
}

func installAuthored(home string, a authored, by string) (bool, error) {
	typ, name := a.Type, a.Name
	if typ == "" || name == "" {
		p, ok := nextUnsigned(home)
		if !ok {
			return false, nil
		}
		typ, name = p.typ, p.name
	} else if !unsignedNamed(home, typ, name) {
		return false, nil
	}
	if err := installScript(home, typ, name, a.Script); err != nil {
		return false, err
	}
	return true, appendReceipt(home, typ, name, a.Script, by)
}

type pendingCompile struct {
	typ, name string
	decl      json.RawMessage
}

func nextUnsigned(home string) (pendingCompile, bool) {
	all := unsignedDeclarations(home)
	if len(all) == 0 {
		return pendingCompile{}, false
	}
	return all[0], true
}

func unsignedNamed(home, typ, name string) bool {
	for _, p := range unsignedDeclarations(home) {
		if p.typ == typ && p.name == name {
			return true
		}
	}
	return false
}

// unsignedDeclarations is the live fold: a declaration is unsigned until a
// later verified receipt for that (type, name), or a retirement. A new
// declaration of the same name (revise) makes it unsigned again.
func unsignedDeclarations(home string) []pendingCompile {
	events, err := readEvents(home)
	if err != nil {
		return nil
	}
	secret, err := loadSecret(home)
	if err != nil {
		return nil
	}
	type rec struct {
		decl      json.RawMessage
		signed    bool
		typ, name string
	}
	latest := map[string]*rec{}
	var order []string
	for _, e := range events {
		switch e.Name {
		case "command.declared", "projector.declared":
			typ, name := declName(e)
			if name == "" {
				continue
			}
			key := typ + "/" + name
			if _, ok := latest[key]; !ok {
				order = append(order, key)
			}
			latest[key] = &rec{decl: e.Payload, signed: false, typ: typ, name: name}
		case "script.compiled":
			if r, ok := verifiedReceipt(secret, e.Payload); ok {
				key := r.Type + "/" + r.Name
				if cur, ok := latest[key]; ok {
					cur.signed = true
				}
			}
		case "capability.retired":
			if d, ok := parseRetirement(e.Payload); ok {
				delete(latest, d.Type+"/"+d.Name)
			}
		}
	}
	var out []pendingCompile
	for _, key := range order {
		cur, ok := latest[key]
		if !ok || cur.signed {
			continue
		}
		out = append(out, pendingCompile{typ: cur.typ, name: cur.name, decl: cur.decl})
	}
	return out
}

func latestLearnContext(home string) (intent, reasoning string) {
	events, err := readEvents(home)
	if err != nil {
		return "", ""
	}
	for _, e := range events {
		switch e.Name {
		case "intent.declared":
			var p struct {
				Intent string `json:"intent"`
			}
			if json.Unmarshal(e.Payload, &p) == nil {
				intent = p.Intent
			}
		case "learn.orchestrated":
			var p struct {
				Reasoning string `json:"reasoning"`
			}
			if json.Unmarshal(e.Payload, &p) == nil {
				reasoning = p.Reasoning
			}
		}
	}
	return intent, reasoning
}

// pump runs ask | $SELF_MIND | receive until the next ask is empty or a hop
// makes no progress. The mind is a filter: ask on stdin, events on stdout.
func pump(home, ask string, out, errw io.Writer) error {
	exe := mindExe()
	if exe == "" {
		return fmt.Errorf("no mind is plugged in — %s", mindHint)
	}
	const maxHops = 16
	prevUnsigned := len(unsignedDeclarations(home))
	for hop := 0; ask != "" && hop < maxHops; hop++ {
		body, err := runMindFilter(home, exe, ask)
		if err != nil {
			return err
		}
		got, err := parseIncoming([]byte(body))
		if err != nil {
			return err
		}
		if !got.hasFood {
			if p := strings.TrimSpace(got.prose); p != "" {
				fmt.Fprintln(out, p)
			} else if p := strings.TrimSpace(body); p != "" {
				fmt.Fprintln(out, p)
			}
			return nil
		}
		next, rep, err := receiveParsed(home, got)
		if err != nil {
			return err
		}
		fmt.Fprintln(errw, rep)
		if p := strings.TrimSpace(got.prose); p != "" {
			fmt.Fprintln(out, p)
		}
		n := len(unsignedDeclarations(home))
		if next != "" && n >= prevUnsigned && hop > 0 {
			fmt.Fprintf(errw, "self: compile hop made no progress; leaving %d unsigned\n", n)
			return writeAsk(out, next)
		}
		prevUnsigned = n
		ask = next
	}
	return nil
}

func runMindFilter(home, exe, ask string) (string, error) {
	parts := strings.Fields(exe)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty SELF_MIND")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home)
	cmd.Dir = home
	cmd.Stdin = strings.NewReader(ask)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mind %s: %w", filepath.Base(parts[0]), err)
	}
	return stdout.String(), nil
}
