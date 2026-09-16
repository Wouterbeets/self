package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

//go:embed PROTOCOL.md
var protocolDoc string

func protocolLayer(name string) string {
	begin := "<!-- prompt:" + name + ":begin -->"
	end := "<!-- prompt:" + name + ":end -->"
	_, rest, ok := strings.Cut(protocolDoc, begin)
	if !ok {
		return ""
	}
	body, _, ok := strings.Cut(rest, end)
	if !ok {
		return ""
	}
	return strings.TrimSpace(body)
}

// errRefused is distinct so the loop can continue after a refusal.
var errRefused = errors.New("authored script(s) refused")

const defaultAsk = `No specific ask. Orient from this instance, explore its views, and act only if something warrants durable action. Silence is valid.`

func cmdOrient(home string, out io.Writer) error {
	st, err := loadState(home)
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, protocolLayer("core")+"\n\n"+protocolLayer("session")+"\n\n"+brief(home, st))
	return err
}

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
	return nil
}

func cmdHear(home string, input []byte, out io.Writer) error {
	evs, scripts, prose, err := wire(string(input))
	if err != nil {
		return err
	}
	if len(evs) == 0 && len(scripts) == 0 {
		if hint := wireHint(input); hint != "" {
			fmt.Fprintf(os.Stderr, "self: heard no events — %s\n", hint)
		} else if strings.TrimSpace(string(input)) != "" {
			fmt.Fprintf(os.Stderr, "self: heard no events — passed %d line(s) through and wrote nothing\n", len(prose))
		}
		_, err := out.Write(input)
		return err
	}
	by := callerClaim()
	for i := range evs {
		evs[i].Via, evs[i].By = doorHear, by
	}
	return ingest(home, evs, scripts, prose, by, out)
}

type authored struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Script string `json:"script"`
}

func wire(body string) (evs []Event, scripts []authored, prose []string, err error) {
	all, err := lines(body)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, line := range all {
		probe, ok := eventLine(line)
		if !ok {
			prose = append(prose, line)
			continue
		}
		if probe.Name == "script.authored" {
			var a authored
			json.Unmarshal(probe.Payload, &a)
			scripts = append(scripts, a)
			continue
		}
		evs = append(evs, newEvent(probe.Name, probe.Payload))
	}
	return evs, scripts, prose, nil
}

func wireHint(input []byte) string {
	var whole wireLine
	if json.Unmarshal(input, &whole) != nil {
		return ""
	}
	switch {
	case whole.Name != "" && whole.Payload == nil:
		return `that object has a "name" but no "payload" — the wire needs both keys.`
	case whole.Name == "":
		return `that object has no "name" — the wire needs a lowercase dotted name and a payload.`
	case !validEventName(whole.Name):
		return fmt.Sprintf("%q is not a lowercase dotted event name (like note.added).", whole.Name)
	case bytes.Contains(bytes.TrimSpace(input), []byte("\n")):
		return "the body is ONE pretty-printed JSON object, and the wire is one object per line. Re-emit it compact (jq -c, or json.dumps without indent)."
	}
	return ""
}

type wireLine struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

func eventLine(line string) (wireLine, bool) {
	for _, candidate := range []string{line, strings.TrimSpace(strings.Trim(line, "`"))} {
		var w wireLine
		if json.Unmarshal([]byte(candidate), &w) == nil && validEventName(w.Name) && w.Payload != nil {
			return w, true
		}
	}
	return wireLine{}, false
}

const lineLimit = 64 * 1024 * 1024

func lines(body string) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 1024*1024), lineLimit)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			out = append(out, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading the wire: %w (nothing was heard — a line longer than %dMB cannot be one)", err, lineLimit/(1024*1024))
	}
	return out, nil
}

// ingest is the shared commit path for validated input. Entry points retain
// their parsing policy and assign provenance before handing over the batch.
// Reports are written after unlocking; callers may discard them without losing
// declaration, installation, or retirement handling.
func ingest(home string, evs []Event, scripts []authored, prose []string, by string, out io.Writer) error {
	if len(evs) == 0 && len(scripts) == 0 && len(prose) == 0 {
		return nil
	}
	key, err := ensureSecret(home)
	if err != nil {
		return err
	}
	// Buffer the report and write it after the lock: flock has no timeout, and
	// a slow stdout consumer would otherwise block every other writer.
	var report bytes.Buffer
	err = func() error {
		unlock, lerr := lockLog(home)
		if lerr != nil {
			return lerr
		}
		defer unlock()
		return ingestLocked(home, key, evs, scripts, prose, by, &report)
	}()
	if _, werr := out.Write(report.Bytes()); werr != nil && err == nil {
		return werr
	}
	return err
}

func ingestLocked(home string, key []byte, evs []Event, scripts []authored, prose []string, by string, out io.Writer) error {
	if err := appendLocked(home, evs); err != nil {
		return err
	}

	events, err := readEvents(home)
	if err != nil {
		return err
	}
	st := replay(events, key)
	warnDroppedDeclarations(st, evs)

	var installed, refused []string
	for _, a := range scripts {
		r, installErr := install(st, a, by)
		name, result := "script.installed", any(r)
		if installErr != nil {
			name = "script.rejected"
			result = rejection{Type: a.Type, Name: a.Name, Reason: installErr.Error(), Excerpt: trunc(a.Script, excerptCap)}
		}
		payload, _ := json.Marshal(result)
		e := newEvent(name, payload)
		e.Via = doorKernel
		batch := []Event{e}
		if err := appendLocked(home, batch); err != nil {
			return err
		}
		st.apply(batch)
		if installErr != nil {
			refused = append(refused, fmt.Sprintf("%s/%s: %s", a.Type, a.Name, installErr))
			continue
		}
		c := st.cap(r.Type, r.Name)
		if _, err := materialize(home, st, r.Type, r.Name); err != nil {
			fmt.Fprintf(os.Stderr, "self: installed %s but could not write cap/: %s (a later run re-derives it)\n", c.key(), err)
		}
		installed = append(installed, c.key())
	}

	retired := applyRetirements(home, st, evs)

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
		fmt.Fprintf(os.Stderr, "self: IGNORED %d line(s) — not events. First: %q\n",
			len(prose), trunc(prose[0], 120))
		for _, line := range prose {
			fmt.Fprintf(os.Stderr, "self:   ignored | %s\n", trunc(line, 200))
		}
		fmt.Fprintf(os.Stderr, "self: a line is an event only with a dotted lowercase name AND a payload key (see self help)\n")
	}
	if p := st.pending(); len(p) > 0 {
		names := make([]string, 0, len(p))
		for _, c := range p {
			names = append(names, c.key())
		}
		fmt.Fprintf(out, "pending: %s\n", strings.Join(names, ", "))
	}
	if len(refused) > 0 {
		return fmt.Errorf("%d %w", len(refused), errRefused)
	}
	return nil
}

func warnDroppedDeclarations(st *state, evs []Event) {
	for _, e := range evs {
		typ, ok := strings.CutSuffix(e.Name, ".declared")
		if !ok || (typ != kindCommand && typ != kindView) {
			continue
		}
		var d decl
		if json.Unmarshal(e.Payload, &d) == nil && st.cap(typ, d.Name) != nil {
			continue
		}
		fmt.Fprintf(os.Stderr, "self: %s at seq %d declares no usable name — it is in the log but names no capability, so any script for it will be refused as undeclared\n", e.Name, e.Seq)
	}
}

func install(st *state, a authored, by string) (receipt, error) {
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
	if !strings.HasPrefix(a.Script, "#!") {
		return receipt{}, fmt.Errorf("script has no shebang: its first line must name an interpreter, like #!/bin/sh or #!/usr/bin/env python3")
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

// applyRetirements unlinks from replayed state, not from the tombstone alone:
// a later declaration in the same body revives the capability.
func applyRetirements(home string, st *state, evs []Event) []string {
	var out []string
	for _, e := range evs {
		if e.Name != "capability.retired" {
			continue
		}
		var t struct{ Type, Name string }
		if json.Unmarshal(e.Payload, &t) != nil || !validCapability(t.Type, t.Name) {
			continue
		}
		if st.cap(t.Type, t.Name) != nil {
			continue
		}
		if err := os.Remove(linkPath(home, t.Type, t.Name)); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "self: retired %s/%s but could not remove its link: %s (self rehydrate retries cleanup)\n", t.Type, t.Name, err)
		}
		out = append(out, t.Type+"/"+t.Name)
	}
	return out
}

func situate(home string, st *state, ask string) string {
	var b strings.Builder
	b.WriteString(protocolLayer("core"))
	b.WriteString("\n\n")
	b.WriteString(protocolLayer("execution"))
	b.WriteString("\n\n")
	b.WriteString(brief(home, st))
	b.WriteString("\n## Intent\n\n")
	b.WriteString(protocolLayer("intent"))
	b.WriteString(pendingSection(st))
	b.WriteString("\n## The ask\n\n")
	b.WriteString(strings.TrimSpace(ask))
	b.WriteString("\n")
	return b.String()
}

func pendingSection(st *state) string {
	pending := st.pending()
	if len(pending) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Pending — declared, awaiting a script\n\n")
	b.WriteString(protocolLayer("growth"))
	b.WriteString("\n")
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
