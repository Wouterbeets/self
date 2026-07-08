// self — a local-first, event-sourced runtime with LLM-generated capabilities.
//
// One append-only event log (events.jsonl) is the only truth. Every view is a
// pure replay of it, rendered as HTML that you and your agent read identically.
// Capabilities are standalone scripts the kernel pipes events through, and code
// is never shipped — a brain process (SELF_BRAIN) authors every script from a
// declaration, for this receiver; the kernel holds no model of its own. A
// running capability can declare new capabilities and the
// kernel compiles them on the spot (the strange loop). Every compile is logged
// as a script.compiled receipt signed with a per-home secret; only kernel-signed
// receipts ever install, so `self rehydrate` rebuilds the whole instance from
// the log alone — an instance is just events.jsonl + .secret.
//
// This file is the whole kernel.
package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// ───────────────────────────── events & the log ─────────────────────────────

type Event struct {
	ID         string          `json:"id"`
	Seq        int             `json:"seq"`
	Name       string          `json:"name"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

func newEvent(name string, payload json.RawMessage) Event {
	b := make([]byte, 16)
	rand.Read(b)
	return Event{ID: hex.EncodeToString(b), Name: name, OccurredAt: time.Now().UTC(), Payload: payload}
}

func logPath(home string) string { return filepath.Join(home, "events.jsonl") }

func readEvents(home string) ([]Event, error) {
	f, err := os.Open(logPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var events []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse event: %w", err)
		}
		events = append(events, e)
	}
	return events, sc.Err()
}

func appendEvent(home string, e *Event) error {
	if err := os.MkdirAll(home, 0755); err != nil {
		return err
	}
	// Assigning seq is read-last-then-append: without a lock, two writers (a
	// server POST and a CLI `run`) can read the same tail and collide. An
	// advisory lock on the log serializes the whole critical section, so the
	// single-writer property holds even across processes.
	unlock, err := lockLog(home)
	if err != nil {
		return err
	}
	defer unlock()
	prior, err := readEvents(home)
	if err != nil {
		return err
	}
	e.Seq = 1
	if len(prior) > 0 {
		e.Seq = prior[len(prior)-1].Seq + 1
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(logPath(home), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, string(line))
	return err
}

// lockLog takes an exclusive advisory (flock) lock on the log file and returns
// a release function. The lock coordinates only between appendEvent callers;
// readers use their own descriptors and are unaffected.
func lockLog(home string) (func(), error) {
	lf, err := os.OpenFile(logPath(home), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		lf.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
		lf.Close()
	}, nil
}

// ─────────────────────── provenance: the signed install ─────────────────────
//
// The loop carries specs, never code: anything may append a script.compiled to
// the log, but only a receipt signed with this home's secret ever installs —
// provenance is intrinsic to the receipt, not enforced by who may write it. A
// forged receipt is inert. The secret lives in SELF_HOME/.secret (0600, never
// in the log), like an ssh host key: per-home, so you can inherit another
// node's declarations but never its binaries.

func loadSecret(home string) ([]byte, error) {
	p := filepath.Join(home, ".secret")
	if data, err := os.ReadFile(p); err == nil {
		if key, err := hex.DecodeString(strings.TrimSpace(string(data))); err == nil && len(key) > 0 {
			return key, nil
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, []byte(hex.EncodeToString(key)), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

type receipt struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Script string `json:"script"`
	// By is the provenance of the bytes: which brain authored this compile —
	// a model at an endpoint, a stub, a named agent. The signature covers it,
	// so authorship can no more be forged or relabeled than the script itself.
	By  string `json:"by,omitempty"`
	Sig string `json:"sig"`
}

// sign binds the receipt's fields so none can be relabeled — one capability's
// bytes can't install under another's name, and authorship can't be moved.
// One formula: domain-separated and length-prefixed, so no concatenation of
// adjacent fields can collide. Instances signed under older formulas re-earn
// their receipts by declaring again — experimental means free to break.
func sign(secret []byte, typ, name, script, by string) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("self.receipt.v2\x00"))
	for _, field := range []string{typ, name, script, by} {
		fmt.Fprintf(m, "%d:", len(field))
		m.Write([]byte(field))
	}
	return hex.EncodeToString(m.Sum(nil))
}

func appendReceipt(home, typ, name, script, by string) error {
	secret, err := loadSecret(home)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(receipt{typ, name, script, by, sign(secret, typ, name, script, by)})
	e := newEvent("script.compiled", payload)
	return appendEvent(home, &e)
}

func verifiedReceipt(secret []byte, payload json.RawMessage) (receipt, bool) {
	var r receipt
	if json.Unmarshal(payload, &r) != nil || r.Sig == "" || r.Script == "" || r.Name == "" {
		return r, false
	}
	return r, hmac.Equal([]byte(sign(secret, r.Type, r.Name, r.Script, r.By)), []byte(r.Sig))
}

// scriptPath is where a capability's executable lives: a directory per
// capability with the script at <name>/run, so the script tree mirrors the
// page tree and a nested name (finances/bills) needs no special case — the
// parent's directory simply holds both its own run and its children.
func scriptPath(home, typ, name string) (string, error) {
	switch typ {
	case "command":
		return filepath.Join(home, "capabilities", "commands", name, "run"), nil
	case "projector":
		return filepath.Join(home, "capabilities", "projectors", name, "run"), nil
	}
	return "", fmt.Errorf("unknown capability type %q", typ)
}

// validCapabilityName admits nested names (finances/bills): slash-separated
// segments, no traversal, no backslash, nothing hidden. A nested projector
// renders to a nested page — progressive unfolding for whoever explores the
// surface: the parent page links down, the front page stays small.
func validCapabilityName(name string) bool {
	if name == "" || strings.Contains(name, `\`) {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, ".") {
			return false
		}
	}
	return true
}

func installScript(home, typ, name, script string) error {
	if !validCapabilityName(name) {
		return fmt.Errorf("unsafe capability name %q", name)
	}
	p, err := scriptPath(home, typ, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(script), 0755)
}

// retirement is the payload of a capability.retired tombstone: which
// (type, name) leaves the derived surface. Events are never deleted —
// retiring only changes what the folds install, list, and render, so the
// whole history stays replayable and a later re-declaration revives the
// capability.
type retirement struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func parseRetirement(payload json.RawMessage) (retirement, bool) {
	var d retirement
	if json.Unmarshal(payload, &d) != nil {
		return d, false
	}
	if d.Type != "command" && d.Type != "projector" {
		return d, false
	}
	if !validCapabilityName(d.Name) {
		return d, false
	}
	return d, true
}

// rehydrate rebuilds the instance from the log alone: the latest kernel-signed
// script.compiled receipt per capability installs verbatim, then every
// projection re-renders. No LLM, no network — a home is events.jsonl + .secret.
func rehydrate(home string) error {
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	secret, err := loadSecret(home)
	if err != nil {
		return err
	}
	// Keyed by (type, name): a command and a projector may share a name, and
	// the latest receipt of each must install — not the latest of either.
	// A capability.retired tombstone deletes its key: whatever was compiled
	// before it stays in the log but out of the fold. A receipt appended
	// after the tombstone reinstalls — the fold is ordered, so revival is
	// just declaring again.
	latest := map[string]receipt{}
	seen := map[string]bool{}
	var order []string
	installed := 0
	for _, e := range events {
		switch e.Name {
		case "script.compiled":
			r, ok := verifiedReceipt(secret, e.Payload)
			if !ok {
				continue
			}
			key := r.Type + "/" + r.Name
			if !seen[key] {
				seen[key] = true
				order = append(order, key)
			}
			latest[key] = r
		case "capability.retired":
			if d, ok := parseRetirement(e.Payload); ok {
				delete(latest, d.Type+"/"+d.Name)
			}
		}
	}
	// capabilities/ and site/ are derived state: rebuild them from nothing so
	// a rehydrate is also a migration — stale files from an older layout (or
	// from receipts later superseded) cannot linger and shadow the log.
	os.RemoveAll(filepath.Join(home, "capabilities"))
	os.RemoveAll(filepath.Join(home, "site"))
	for _, key := range order {
		r, live := latest[key]
		if !live {
			continue // retired after its last receipt — the tombstone wins
		}
		if err := installScript(home, r.Type, r.Name, r.Script); err != nil {
			return err
		}
		installed++
	}
	refreshSite(home)
	fmt.Fprintf(os.Stderr, "self: rehydrated %d capabilit(ies) from the log\n", installed)
	return nil
}

// ───────────────────────────── the pipe contract ────────────────────────────
//
// Compiled scripts are standalone executables in any language. A command reads
// args as argv and the current events as JSONL on stdin, and writes new events
// as JSONL on stdout ({name, payload} per line; the kernel assigns the rest). A
// projector reads all events on stdin and writes HTML on stdout; the kernel
// persists it to site/<name>.html. The kernel sets SELF_HOME on every script.

func feedEvents(stdin io.WriteCloser, events []Event) {
	go func() {
		enc := json.NewEncoder(stdin)
		for i := range events {
			enc.Encode(events[i])
		}
		stdin.Close()
	}()
}

// feedText writes a plain-text string to a process's stdin and closes it. Used
// for the brain, which receives an orientation brief — not the raw log — so it
// reads where to look, then explores SELF_HOME itself with its own tools
// instead of being force-fed a firehose of events.
func feedText(stdin io.WriteCloser, text string) {
	go func() {
		io.WriteString(stdin, text)
		stdin.Close()
	}()
}

// stateBrief is the kernel's wake-up card for a brain: pure orientation, not a
// log digest. It tells the brain where it is, what capabilities exist, and
// where to look for the rest — and nothing else. The brain is expected to
// explore SELF_HOME itself: read site/kernel.html for the full
// self-description, site/*.html for the rendered state a human sees,
// events.jsonl for the raw log, capabilities/ for the compiled scripts. The
// kernel holds no internal state a brain cannot see on disk.
//
// A consequence: a brain that cannot inspect files under SELF_HOME — a plain
// stdin/stdout API adapter with no tools — cannot do the job. The kernel's
// seam is still a pipe, but a real brain needs a tool loop on its side of it.
// The kernel does not sandbox or supply tools; isolating the brain's
// exploration is the brain's own concern (a coding agent already has its own).
//
// The kernel materializes the brief to SELF_HOME/site/brief.md (see
// renderBriefFile) so it is explorable on disk like every other piece of
// state. Markdown on purpose — readable as plain text to a brain, to `cat`, and
// served verbatim as text/plain like any other .md file under site/.
// declaredCaps replays the log into the currently declared commands and
// projectors, each in first-declared order. The shared walk behind both the
// orientation brief and the kernel index — the log is the only source, so both
// see exactly the same capabilities in the same order. A capability.retired
// tombstone delists its target; a declaration after the tombstone lists it
// again, freshly ordered.
func declaredCaps(events []Event) (commands map[string]commandDecl, cmdOrder []string, projectors map[string]projectorDecl, projOrder []string) {
	commands = map[string]commandDecl{}
	projectors = map[string]projectorDecl{}
	drop := func(order []string, name string) []string {
		out := order[:0]
		for _, n := range order {
			if n != name {
				out = append(out, n)
			}
		}
		return out
	}
	for _, e := range events {
		switch e.Name {
		case "command.declared":
			var d commandDecl
			if json.Unmarshal(e.Payload, &d) == nil && d.Name != "" {
				if _, ok := commands[d.Name]; !ok {
					cmdOrder = append(cmdOrder, d.Name)
				}
				commands[d.Name] = d
			}
		case "projector.declared":
			var d projectorDecl
			if json.Unmarshal(e.Payload, &d) == nil && d.Name != "" {
				if _, ok := projectors[d.Name]; !ok {
					projOrder = append(projOrder, d.Name)
				}
				projectors[d.Name] = d
			}
		case "capability.retired":
			d, ok := parseRetirement(e.Payload)
			if !ok {
				continue
			}
			switch d.Type {
			case "command":
				delete(commands, d.Name)
				cmdOrder = drop(cmdOrder, d.Name)
			case "projector":
				delete(projectors, d.Name)
				projOrder = drop(projOrder, d.Name)
			}
		}
	}
	return commands, cmdOrder, projectors, projOrder
}

func stateBrief(home string) string {
	events, err := readEvents(home)
	if err != nil {
		// a corrupt log is the kernel's failure, not the brain's; surface it
		return fmt.Sprintf("# self — orientation brief\n\nInstance: `%s`\n\n**ERROR reading the log:** %s\n", home, err)
	}
	commands, cmdOrder, projectors, projOrder := declaredCaps(events)

	oneLine := func(s string) string {
		return strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# self — orientation brief\n\n")
	fmt.Fprintf(&b, "Instance: `%s`\n\n", home)
	if len(events) == 0 {
		b.WriteString("Empty log. Grow a seed: `self grow <seed>` (try `seeds/journal`).\n")
		return b.String()
	}

	fmt.Fprintf(&b, "## read this first\n\n")
	fmt.Fprintf(&b, "- `site/kernel.html` — the instance's full self-description (capabilities, the pipe contract, where things live).\n")
	fmt.Fprintf(&b, "- `site/*.html` — rendered state, the same pages a human sees.\n")
	fmt.Fprintf(&b, "- `events.jsonl` — the whole append-only log (the only truth).\n")
	fmt.Fprintf(&b, "- `capabilities/` — the compiled scripts currently installed.\n\n")

	if len(projOrder) > 0 {
		b.WriteString("## projections (current state)\n\n")
		for _, n := range projOrder {
			d := projectors[n]
			consumes := strings.Join(d.Consumes, ", ")
			if consumes == "" {
				consumes = "—"
			}
			fmt.Fprintf(&b, "- `/%s` — %s (consumes %s) → `site/%s.html`\n",
				n, oneLine(d.Description), consumes, n)
		}
		b.WriteString("\n")
	}
	if len(cmdOrder) > 0 {
		b.WriteString("## commands (verbs — `self run <name> …`)\n\n")
		for _, n := range cmdOrder {
			d := commands[n]
			fmt.Fprintf(&b, "- `%s` — %s (emits %s)\n", n, oneLine(d.Description), d.Event.Name)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%d events in the log. Explore for the rest.\n", len(events))
	return b.String()
}

// renderBriefFile writes the orientation brief to SELF_HOME/site/brief.md,
// the kernel-resident surface a brain reads. Called alongside renderKernelHTML
// whenever the log changes, and re-run immediately before every brain ask (see
// freshBrief) so a brain never reads stale orientation. Served verbatim as
// text/plain like any other .md file under site/.
func renderBriefFile(home string) {
	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	os.WriteFile(filepath.Join(siteDir, "brief.md"), []byte(stateBrief(home)), 0644)
}

// freshBrief writes the orientation brief to disk and returns the exact bytes
// the kernel just wrote. Used by pipeBrain so the brain is always fed the
// current state of the instance — never a cached file that could grow stale if
// the log changed outside the normal refresh path (e.g. a CLI `run` between a
// serve request and a brain call). The disk is the source; the brain can read
// the same file itself to explore. Write then read back would be redundant —
// stateBrief is deterministic, so the bytes written are the bytes returned.
func freshBrief(home string) string {
	brief := stateBrief(home)
	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	os.WriteFile(filepath.Join(siteDir, "brief.md"), []byte(brief), 0644)
	return brief
}

// pipeProcess runs an executable as a Unix pipeline node — the one shape the
// kernel uses to talk to any outside process, a compiled command or the brain.
func pipeProcess(home, bin string, argv []string) ([]Event, error) {
	current, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, argv...)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home)
	cmd.Dir = home
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", filepath.Base(bin), err)
	}
	feedEvents(stdin, current)

	var out []Event
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var partial struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &partial); err != nil {
			return nil, fmt.Errorf("%s output parse error: %w", filepath.Base(bin), err)
		}
		if partial.Name == "" {
			return nil, fmt.Errorf("%s output missing event name: %s", filepath.Base(bin), line)
		}
		out = append(out, newEvent(partial.Name, partial.Payload))
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("%s exited: %w", filepath.Base(bin), err)
	}
	return out, nil
}

func runCommand(home, command string, args []string) ([]Event, error) {
	bin, _ := scriptPath(home, "command", command)
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("command %q not found (grow a seed that declares it)", command)
	}
	evs, err := pipeProcess(home, bin, args)
	if err != nil {
		return nil, err
	}
	return evs, ingest(home, evs)
}

// ingest appends the events a process emitted, compiles any declarations among
// them (the strange loop), honors any retirements, and re-renders every
// projection. Projections are pure replays, so re-running them all is always
// correct.
func ingest(home string, evs []Event) error {
	for i := range evs {
		if err := appendEvent(home, &evs[i]); err != nil {
			return err
		}
	}
	c := newLLM(home)
	if n := compileDeclarations(c, home, evs); n > 0 {
		fmt.Fprintf(os.Stderr, "self: self-improved — %d capabilit(ies) compiled\n", n)
	}
	if n := applyRetirements(home, evs); n > 0 {
		fmt.Fprintf(os.Stderr, "self: retired %d capabilit(ies)\n", n)
	}
	refreshSite(home)
	return nil
}

// applyRetirements honors capability.retired tombstones on the live path the
// way rehydrate honors them on replay: the installed script and any rendered
// page are removed at once, so disk never drifts from the log. The events all
// stay — a retired capability is one re-declaration away from coming back.
func applyRetirements(home string, evs []Event) int {
	n := 0
	for _, e := range evs {
		if e.Name != "capability.retired" {
			continue
		}
		d, ok := parseRetirement(e.Payload)
		if !ok {
			continue
		}
		p, err := scriptPath(home, d.Type, d.Name)
		if err != nil {
			continue
		}
		os.Remove(p)
		os.Remove(filepath.Dir(p)) // succeeds only when empty — a nested child's dirs survive
		if d.Type == "projector" {
			os.Remove(filepath.Join(home, "site", d.Name+".html"))
		}
		n++
	}
	return n
}

type commandDecl struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Params      map[string]string `json:"params"`
	Event       struct {
		Name   string            `json:"name"`
		Fields map[string]string `json:"fields"`
	} `json:"event"`
	// Implementation is an optional reference the compiler verifies and adapts —
	// never installed as-is, so precision from the seed author and receiver
	// adaptation both survive.
	Implementation string `json:"implementation,omitempty"`
	Revision       struct {
		Request     string `json:"request,omitempty"`
		FromReceipt string `json:"from_receipt,omitempty"`
	} `json:"revision,omitempty"`
}

type projectorDecl struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Consumes       []string `json:"consumes"`
	Implementation string   `json:"implementation,omitempty"`
	Revision       struct {
		Request     string `json:"request,omitempty"`
		FromReceipt string `json:"from_receipt,omitempty"`
	} `json:"revision,omitempty"`
}

// compileDeclarations is the strange-loop hook: every command.declared /
// projector.declared among evs is compiled by the LLM into a script authored
// for this receiver, installed, and logged as a signed receipt. Declaring IS
// creating — this runs at grow time and at run time alike, so a capability (or
// the brain) grows new capabilities just by emitting declarations.
func compileDeclarations(c *llm, home string, evs []Event) int {
	n := 0
	for _, e := range evs {
		var typ, name, script string
		var err error
		switch e.Name {
		case "command.declared":
			var d commandDecl
			if json.Unmarshal(e.Payload, &d) != nil || d.Name == "" {
				continue
			}
			typ, name = "command", d.Name
			fmt.Fprintf(os.Stderr, "self: compiling command %q…\n", name)
			script, err = c.compileCommand(d)
		case "projector.declared":
			var d projectorDecl
			if json.Unmarshal(e.Payload, &d) != nil || d.Name == "" {
				continue
			}
			typ, name = "projector", d.Name
			fmt.Fprintf(os.Stderr, "self: compiling projector %q…\n", name)
			script, err = c.compileProjector(d)
		default:
			continue
		}
		if err == nil {
			err = installScript(home, typ, name, script)
		}
		if err == nil {
			err = appendReceipt(home, typ, name, script, c.identity())
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "self: %s %q failed: %s\n", typ, name, err)
			continue
		}
		n++
	}
	return n
}

// ─────────────────────────────── projections ────────────────────────────────

// runProjection replays the whole log through a projector script and returns
// the HTML it emits. Run it twice, get the same page — a pure function of the log.
func runProjection(home, name string) ([]byte, error) {
	bin, _ := scriptPath(home, "projector", name)
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("projection %q not found", name)
	}
	events, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home)
	cmd.Dir = home
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	feedEvents(stdin, events)
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("projection %q exited: %w", name, err)
	}
	return out.Bytes(), nil
}

func projectToSite(home, name string) error {
	page, err := runProjection(home, name)
	if err != nil {
		return err
	}
	out := filepath.Join(home, "site", name+".html")
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	return os.WriteFile(out, page, 0644)
}

func refreshProjections(home string) {
	root := filepath.Join(home, "capabilities", "projectors")
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "run" {
			return nil
		}
		name, _ := filepath.Rel(root, filepath.Dir(p)) // the dir is the name; nesting nests
		if err := projectToSite(home, name); err != nil {
			fmt.Fprintf(os.Stderr, "self: projection %q failed: %s\n", name, err)
		}
		return nil
	})
}

// refreshSite writes every kernel-resident view of state and re-runs every
// declared projector. Call this whenever the log changes: it keeps disk in
// lockstep with the log so a brain (or a human, or an external agent) reading
// files under SELF_HOME/site/ sees current state, never a stale view. There is
// no internal state the kernel renders into a brain prompt that is not on disk.
// The brief is written LAST, after the projections, so a brain that reads the
// brief and then follows its pointers to site/*.html always finds pages at
// least as fresh as the brief that sent it there.
func refreshSite(home string) {
	renderKernelHTML(home)
	refreshProjections(home)
	renderBriefFile(home)
}

// ──────────────────────────────── the brain ─────────────────────────────────
//
// The kernel holds no model — not even a fake one. Every ask — think,
// heartbeat, grow, and each compile — is handed to a brain PROCESS
// (SELF_BRAIN, e.g. "claude -p"; examples/brain-stub is a deterministic
// offline one for demos and tests), which explores and writes scripts with
// its own tools; the kernel only installs and signs what comes back. The llm
// value carries just enough to route a compile: the home it runs against,
// and — during a grow — the whole intent plus the orchestrator's stated
// reasoning, woven into each compile so no piece is authored in a dark room.
// The reasoning travels in-band, through the prompt and the log, never
// through a session store outside the log.

type llm struct {
	home      string
	intent    string
	reasoning string
}

// identity names the brain for provenance: who authored the bytes a receipt
// carries. SELF_BRAIN_ID lets an agent name itself; otherwise the brain
// executable is the honest mechanical answer.
func (c *llm) identity() string {
	if id := strings.TrimSpace(os.Getenv("SELF_BRAIN_ID")); id != "" {
		return id
	}
	if exe := brainExe(); exe != "" {
		return exe
	}
	return "brain"
}

func newLLM(home string) *llm {
	return &llm{home: home}
}

// ─────────────────────────────── the prompts ────────────────────────────────

const pipeContract = `command script: receives args as argv, current events as JSONL on stdin, writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at.
projector script: receives all events as JSONL on stdin, writes bare semantic HTML on stdout. Do not emit CSS, JavaScript, inline styles, or external assets: the kernel injects the shared shell at serve time. The kernel persists projector output to SELF_HOME/site/<name>.html.
The kernel sets SELF_HOME on every script. Any language with a shebang works; use only standard libraries.`

// brainAnswerContract tells a capable, tool-using brain how to hand its answer
// back. A coding-agent brain (claude -p) will otherwise try to persist its work
// itself — write events.jsonl, run `self`, install a script — and that effort is
// wasted and denied: the kernel reads ONLY stdout and appends what it finds
// there, under its own signature. It also emits Markdown by habit; the pipe
// tolerates fences, but one clean JSON object per line is what actually wants
// ingesting. Woven into every ask that expects events (grow, heartbeat, compile).
const brainAnswerContract = `HOW TO ANSWER — the kernel reads ONLY your stdout. Event lines are JSON objects; prose lines are reply text. You do not and cannot write the log yourself: do not edit events.jsonl, run the self CLI, or install anything with your tools — that work is wasted. To persist ordinary state, print the domain event that records it. To add capabilities, print command.declared / projector.declared events as ONE line of compact JSON each (no Markdown, no code fences, no backticks). Declare only what is missing; ordinary domain events are not capability growth.

THIS REPLY IS FINAL — you run once per ask and are never re-invoked. Explore first, THEN answer completely: never end on a plan or a promise ("I'll explore and then respond") — whatever you have not said when you exit was never said.

WHAT YOU ARE GIVEN — your stdin is an orientation brief: where you are, what capabilities exist, where to look for the rest. That is all. To do your job you must EXPLORE the instance surface with your tools: read ` + "`SELF_HOME/site/kernel.html`" + ` for the full self-description, ` + "`SELF_HOME/site/*.html`" + ` for the rendered state a human sees, ` + "`SELF_HOME/events.jsonl`" + ` for the raw log, ` + "`SELF_HOME/capabilities/`" + ` for the compiled scripts. The kernel holds no internal state you cannot see on disk. A brain without tools to read those files cannot do this job.`

// ────────────────────────────── the compiler ────────────────────────────────

func (c *llm) compileCommand(d commandDecl) (string, error) {
	return compileViaBrain(c.home, c.intent, c.reasoning, "command", d.Name, jsonRepr(d))
}

func (c *llm) compileProjector(d projectorDecl) (string, error) {
	return compileViaBrain(c.home, c.intent, c.reasoning, "projector", d.Name, jsonRepr(d))
}

// compileViaBrain hands a compile ask to the plugged brain through the same
// seam as every other ask. The declaration (its optional implementation
// reference included) rides in the prompt; an orientation brief rides on stdin
// (the brain inspects SELF_HOME itself for depth — site/*.html, events.jsonl,
// capabilities/); the answer is one script.authored event. During a grow the
// whole intent rides along too, so each piece is compiled toward the same
// product. The kernel still installs and signs — a brain authors bytes, only
// the kernel makes them real.
// compilePrompt is the text of a compile ask: author one script honoring the
// pipe contract, test it with your own tools, and hand back exactly one
// script.authored line — the kernel does the installing and signing.
func compilePrompt(intent, reasoning, exemplarName, exemplarScript, typ, name, decl string) string {
	prompt := fmt.Sprintf(`COMPILE one capability for this instance. Author a complete executable script (any language with a shebang, standard libraries only) honoring the pipe contract, adapted to this instance's state (a brief on your stdin; the full log is at SELF_HOME/events.jsonl if you need it).

%s

DECLARATION (%s %q):
%s

If the declaration carries an "implementation", it is a reference script: verify it and adapt it here — never copy it blindly. If it also carries a "revision", preserve the existing behavior except for that requested change. Use your own tools freely to write and TEST the script by execution before answering — but do not install it, edit events.jsonl, or run the self CLI: the kernel installs and signs the script from the one line you print, and nothing else. If this is a projector, emit only bare semantic HTML: no CSS, no JavaScript, no inline style attributes, no external assets.

Answer with ONE line of JSON and nothing else — no Markdown, no code fence:
{"name":"script.authored","payload":{"script":"<the full script>"}}`, pipeContract, typ, name, decl)
	if strings.TrimSpace(exemplarScript) != "" {
		prompt = "Here is a recently compiled " + typ + " as an idiom/reference for this instance. Learn its style and pipe-contract shape, but do not copy it blindly.\n\n--- EXEMPLAR " + exemplarName + " ---\n" + exemplarScript + "\n--- END EXEMPLAR ---\n\n" + prompt
	}
	if strings.TrimSpace(reasoning) != "" {
		prompt = "The ORCHESTRATOR that declared this capability explored the instance and explained its plan below (it is also in the log as grow.orchestrated). Compile in line with it.\n\n--- ORCHESTRATOR'S REASONING ---\n" + reasoning + "\n--- END REASONING ---\n\n" + prompt
	}
	if strings.TrimSpace(intent) != "" {
		prompt = "This capability is one part of a product with the following INTENT. Compile it so the whole intent is served.\n\n--- INTENT ---\n" + intent + "\n--- END INTENT ---\n\n" + prompt
	}
	return prompt
}

// exemplarScript returns the source of the most recently compiled capability
// of the same type (excluding the one being compiled, so a recompile is never
// anchored to its own broken past). Read from the log's verified receipts —
// traced compiles show a brain spends its first minute rediscovering the
// instance's idiom from disk; handing it one exemplar removes that phase.
func exemplarScript(home, typ, name string) (exName, exScript string) {
	events, err := readEvents(home)
	if err != nil {
		return "", ""
	}
	secret, err := loadSecret(home)
	if err != nil {
		return "", ""
	}
	for _, e := range events {
		if e.Name != "script.compiled" {
			continue
		}
		if r, ok := verifiedReceipt(secret, e.Payload); ok && r.Type == typ && r.Name != name {
			exName, exScript = r.Name, r.Script
		}
	}
	const cap = 4096
	if len(exScript) > cap {
		exScript = exScript[:cap] + "\n… (truncated)"
	}
	return exName, exScript
}

func compileViaBrain(home, intent, reasoning, typ, name, decl string) (string, error) {
	exName, exScript := exemplarScript(home, typ, name)
	res, err := pipeBrain(home, "compile", compilePrompt(intent, reasoning, exName, exScript, typ, name, decl))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(res.Script) == "" {
		return "", fmt.Errorf("the brain answered a compile ask without a script.authored event")
	}
	return res.Script, nil
}

// ──────────────────────────────── the brain ─────────────────────────────────

type brainResult struct {
	Response string
	Events   []map[string]any
	Script   string // a compile ask's answer, from a script.authored event
}

// brainExe is the plugged brain, if any: SELF_BRAIN is the one way a brain is
// named, and a process behind it honors the one brain contract.
func brainExe() string {
	return strings.TrimSpace(os.Getenv("SELF_BRAIN"))
}

func brainEnv(home, kind string) []string {
	return append(os.Environ(), "SELF_HOME="+home, "SELF_ASK="+kind)
}

// agent runs one brain conversation with the three powers: read (bash), act
// (run, over the given capability catalog), grow (declare). user may be a JSON
// array of {role, content} turns, so a chat surface can hand the brain real
// turn-based history.
// applyEvents appends events the brain returned and runs any capability
// declarations among them through the strange loop.
func applyEvents(home string, res *brainResult) {
	var evs []Event
	for _, d := range res.Events {
		name, _ := d["name"].(string)
		payload, _ := json.Marshal(d["payload"])
		if name == "" || string(payload) == "null" {
			continue
		}
		e := newEvent(name, payload)
		if err := appendEvent(home, &e); err != nil {
			fmt.Fprintf(os.Stderr, "self: append brain event: %s\n", err)
			return
		}
		evs = append(evs, e)
	}
	if len(evs) > 0 {
		c := newLLM(home)
		c.reasoning = strings.TrimSpace(res.Response)
		n := compileDeclarations(c, home, evs)
		fmt.Fprintf(os.Stderr, "self: grew %d capabilit(ies)\n", n)
		if r := applyRetirements(home, evs); r > 0 {
			fmt.Fprintf(os.Stderr, "self: retired %d capabilit(ies)\n", r)
		}
		refreshSite(home)
	}
}

// ─────────────────────────────── kernel.html ────────────────────────────────

// renderKernelHTML writes the kernel's self-description — capabilities, paths,
// the pipe contract — to site/kernel.html: the page a human lands on and the
// first context a brain reads. Like everything in site/, it is a replay of the log.
func renderKernelHTML(home string) {
	events, err := readEvents(home)
	if err != nil {
		return
	}
	commands, cmdOrder, projectors, projOrder := declaredCaps(events)
	// grownBy is provenance: the latest kernel-signed receipt's By per capability.
	// Verified, not merely read — an unsigned or forged by-line never renders.
	grownBy := map[string]string{}
	if secret, _ := loadSecret(home); secret != nil {
		for _, e := range events {
			if e.Name != "script.compiled" {
				continue
			}
			if r, ok := verifiedReceipt(secret, e.Payload); ok && r.By != "" {
				grownBy[r.Type+"/"+r.Name] = r.By
			}
		}
	}

	esc := html.EscapeString
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>self</title></head><body>\n")
	b.WriteString("<h1>self</h1>\n")
	b.WriteString("<p class=\"muted\">a local-first, event-sourced runtime with LLM-generated capabilities</p>\n")
	b.WriteString("<p>One append-only event log is the only state. Everything here — the capabilities, the projections, this page — is a deterministic replay of that log; humans and agents read the same rendered result. Every path below is a plain file.</p>\n")
	b.WriteString(orientationHTML)

	b.WriteString("<h2>commands</h2>\n")
	if len(cmdOrder) == 0 {
		b.WriteString("<p class=\"muted\">None yet — grow a seed: <code>self grow seeds/chat</code>.</p>\n")
	}
	for _, n := range cmdOrder {
		d := commands[n]
		b.WriteString("<article class=\"card\"><h3>" + esc(d.Name) + "</h3><p>" + esc(d.Description) + "</p>")
		b.WriteString("<p class=\"muted\">produces <code>" + esc(d.Event.Name) + "</code>")
		if len(d.Params) > 0 {
			b.WriteString(" · args " + esc(jsonRepr(d.Params)))
		}
		b.WriteString(" · <code>self run " + esc(d.Name) + " …</code>")
		if by := grownBy["command/"+d.Name]; by != "" {
			b.WriteString(" · grown by " + esc(by))
		}
		b.WriteString("</p></article>\n")
	}

	b.WriteString("<h2>projections</h2>\n")
	if len(projOrder) == 0 {
		b.WriteString("<p class=\"muted\">None yet.</p>\n")
	}
	for _, n := range projOrder {
		d := projectors[n]
		b.WriteString("<article class=\"card\"><h3><a href=\"/" + esc(d.Name) + "\">/" + esc(d.Name) + "</a></h3><p>" + esc(d.Description) + "</p>")
		b.WriteString("<p class=\"muted\">consumes <code>" + esc(strings.Join(d.Consumes, ", ")) + "</code>")
		if by := grownBy["projector/"+d.Name]; by != "" {
			b.WriteString(" · grown by " + esc(by))
		}
		b.WriteString("</p></article>\n")
	}

	b.WriteString("<h2>where I live</h2>\n<table><tr><th>what</th><th>path</th></tr>")
	for _, row := range [][2]string{
		{"the only truth", filepath.Join(home, "events.jsonl")},
		{"compiled commands", filepath.Join(home, "capabilities", "commands")},
		{"compiled projectors", filepath.Join(home, "capabilities", "projectors")},
		{"materialized HTML", filepath.Join(home, "site")},
	} {
		b.WriteString("<tr><td>" + esc(row[0]) + "</td><td><code>" + esc(row[1]) + "</code></td></tr>")
	}
	b.WriteString("</table>\n")

	b.WriteString("<h2>the pipe contract</h2>\n<pre>" + esc(pipeContract) + "</pre>\n")
	b.WriteString("<h2>the events I act on</h2>\n<p><code>command.declared</code> / <code>projector.declared</code> compile into capabilities (the strange loop, at grow time and run time). <code>script.compiled</code> is a compile receipt signed with my <code>.secret</code> — anyone may append one, but only a kernel-signed receipt ever installs; <code>self rehydrate</code> rebuilds my whole instance from them. <code>capability.retired</code> takes a capability off the derived surface — script and page — while every event stays; a later re-declaration revives it.</p>\n")
	b.WriteString("</body></html>\n")

	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	os.WriteFile(filepath.Join(siteDir, "kernel.html"), []byte(b.String()), 0644)
}

// ─────────────────────────────── the surface ────────────────────────────────

// The shell is the one shared enrichment the kernel injects at serve time —
// theme and feel layered over projections that stay bare semantic HTML on
// disk. The split of responsibilities is the design system: the log is the
// truth, the projection is the state, the shell is the feel. The shell knows
// the class vocabulary, never the events; strip it (self show, curl, lynx,
// rehydrate) and every page still works, because every affordance underneath
// is a plain HTML form.
//
// The feel is swappable. What is fixed is the class vocabulary and the
// structural rules below — the contract the projections and shellScript are
// written against. A *theme* changes none of that: it is only a skin, a set of
// CSS custom properties (palette, fonts, radii, border weight, shadow) that the
// structural layer reads through var(). So switching designs never renames a
// class or rewrites a rule; every projection keeps working unchanged and only
// the feel moves. Themes are picked at serve time and carry no state into the
// log — presentation, like prefers-color-scheme; the bare HTML on disk stays
// theme-agnostic, so rehydrate and self show are untouched.

const shellMeta = `<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark">`

const defaultTheme = "grove"

// A theme is a skin: CSS custom properties (palette, fonts, radii, borders) the
// structural layer reads through var(), plus—optionally—a few extra rules for a
// feel variables alone can't carry. It is injected AFTER the structural CSS, so
// its variables resolve and its rules layer on top; it never renames a class or
// changes what a projection emits.
type theme struct {
	label string
	css   string
}

//go:embed themes/*.css
var themeFS embed.FS

// themes and themeOrder are built once from the embedded themes/*.css files —
// the design set ships inside the binary, so serving needs no files on disk and
// the offline guarantees hold. Each file's base name is the theme id; the
// default (grove) lists first, the rest alphabetically. Add a design by dropping
// a .css into themes/ and rebuilding — nothing structural changes.
var themes, themeOrder = loadThemes()

func loadThemes() (map[string]theme, []string) {
	m := map[string]theme{}
	var extra []string
	entries, _ := themeFS.ReadDir("themes")
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".css") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".css")
		data, err := themeFS.ReadFile("themes/" + e.Name())
		if err != nil || name == "" {
			continue
		}
		m[name] = theme{label: themeLabel(name), css: string(data)}
		if name != defaultTheme {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	return m, append([]string{defaultTheme}, extra...)
}

// themeLabel is the picker's display name for a theme id: its capitalized form.
func themeLabel(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// structuralCSS is the fixed half of the shell: the class vocabulary and every
// layout rule, written entirely against var()s a theme supplies. It never
// mentions a literal color, font, or radius — that is what makes the embedded
// themes/*.css skins interchangeable.
//
//go:embed shell/structural.css
var structuralCSS string

// validTheme reports whether name is a known design; selection paths accept
// only known names, so the injected picker links can never smuggle
// arbitrary CSS in.
func validTheme(name string) bool { _, ok := themes[name]; return ok }

// themeCSS assembles the full <style> for one design: the shared structural
// rules, then the theme's CSS (its variables, and any extra rules that layer on
// top). Unknown names fall back to the default.
func themeCSS(name string) string {
	t, ok := themes[name]
	if !ok {
		t = themes[defaultTheme]
	}
	return shellMeta + "<style>" + structuralCSS + "\n" + t.css + "\n</style>"
}

// pickTheme resolves the design for one request: an explicit ?theme= wins,
// then the SELF_THEME instance default, then the built-in default. Two
// mechanisms, no remembered state — a theme is presentation for one request,
// like prefers-color-scheme, never something the server holds for you.
func pickTheme(r *http.Request) string {
	if t := r.URL.Query().Get("theme"); validTheme(t) {
		return t
	}
	if t := strings.TrimSpace(os.Getenv("SELF_THEME")); validTheme(t) {
		return t
	}
	return defaultTheme
}

// themePicker is the one bit of DOM the shell adds to the body: a small fixed
// switcher of plain links. It works with no JS (each link is a GET that the
// server themes and remembers), and it is styled by the active theme itself, so
// it always matches the page it sits on.
func themePicker(current string) string {
	var b strings.Builder
	b.WriteString(`<nav class="self-themes" aria-label="page design">`)
	for _, name := range themeOrder {
		if name == current {
			b.WriteString(`<a href="?theme=` + name + `" aria-current="true">` + themes[name].label + `</a>`)
		} else {
			b.WriteString(`<a href="?theme=` + name + `">` + themes[name].label + `</a>`)
		}
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// shellScript is the reactive half of the shell: progressive enhancement
// only, injected at serve time and never persisted. The state machine is
// untouched — every interaction is still form → command → events → replay;
// the script changes how the round-trip FEELS, not what it is. It may show
// intent in flight (a pending turn, a thinking brain) but never claims
// state: when the round-trip lands, the page is re-fetched and the log's
// replay wins. Liveness is the same idea watched from outside — the byte
// length of /events is the cursor; when the log grows, re-replay.
//
//go:embed shell/shell.js
var shellScriptBody string

// orientationHTML is the kernel index's static "if you are an LLM" briefing —
// self-description copy, kept as data beside the shell it renders with.
//
//go:embed shell/orientation.html
var orientationHTML string

// shellScript wraps the embedded progressive-enhancement JS as an injectable
// <script> element.
var shellScript = "<script>" + shellScriptBody + "</script>"

// siteNav is the human way around an instance: one bar of plain links,
// injected by the kernel on every served page, listing every declared
// projection plus the kernel's own surfaces (brief, events). Projectors stay
// bare — explorability is chrome, so it belongs to the shell, not to every
// script. Like everything served, it is a replay of the log: declared
// projections in declaration order.
func siteNav(home, current string) string {
	events, err := readEvents(home)
	if err != nil {
		return ""
	}
	_, _, _, projOrder := declaredCaps(events)
	link := func(href, label string) string {
		esc := html.EscapeString(label)
		// a nested page (finances/bills) marks its top-level entry (finances)
		if label == current || strings.HasPrefix(current, label+"/") {
			return `<a href="` + href + `" aria-current="true">` + esc + `</a>`
		}
		return `<a href="` + href + `">` + esc + `</a>`
	}
	var b strings.Builder
	b.WriteString(`<nav class="self-nav" aria-label="instance"><a class="self-brand" href="/">self</a>`)
	for _, n := range projOrder {
		if strings.Contains(n, "/") {
			continue // nested pages unfold from their parents, not the nav
		}
		b.WriteString(link("/"+n, n))
	}
	b.WriteString(link("/brief", "brief"))
	b.WriteString(link("/events", "events"))
	b.WriteString(`</nav>`)
	return b.String()
}

// siteFile resolves a path under SELF_HOME/site/ to a file by name, looking for
// <name>.html, <name>.md, and <name>.txt in order. It returns the file path and
// the matched extension, or "" if no such file. Used by the server and by
// `self show` so a brain (or human) can reach any on-disk artifact by bare name.
func siteFile(home, name string) (path, ext string) {
	for _, e := range []string{".html", ".md", ".txt"} {
		p := filepath.Join(home, "site", name+e)
		if fileExists(p) {
			return p, e
		}
	}
	return "", ""
}

// serveSiteFile writes a site file to an HTTP response, dispatching by
// extension: .html goes through the shell (themed, progressive-enhanced), while
// .md and .txt are served verbatim as text/plain — plain text is honest about
// what it is, and the kernel renders no markup it did not itself emit.
func serveSiteFile(w http.ResponseWriter, r *http.Request, home, current, path, ext string) {
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if ext == ".html" {
		writePage(w, r, home, current, data)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

func injectShell(page []byte, theme, nav string) []byte {
	head := themeCSS(theme) + shellScript
	if i := bytes.Index(page, []byte("<head>")); i >= 0 {
		i += len("<head>")
		page = append(page[:i:i], append([]byte(head), page[i:]...)...)
	} else {
		page = append([]byte(head), page...)
	}
	if nav != "" {
		if i := bytes.Index(page, []byte("<body>")); i >= 0 {
			i += len("<body>")
			page = append(page[:i:i], append([]byte(nav), page[i:]...)...)
		} else {
			page = append([]byte(nav), page...)
		}
	}
	picker := themePicker(theme)
	if j := bytes.LastIndex(page, []byte("</body>")); j >= 0 {
		return append(page[:j:j], append([]byte(picker), page[j:]...)...)
	}
	return append(page, []byte(picker)...)
}

// writePage sends an on-disk projection through the shell for one request:
// resolve the design and inject theme + script + nav + picker. This is the
// only place a theme touches a response; nothing is written back to the log,
// to disk, or to the client. current names the page being served so the nav
// can mark it.
func writePage(w http.ResponseWriter, r *http.Request, home, current string, page []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(injectShell(page, pickTheme(r), siteNav(home, current)))
}

// cmdServe serves the instance: every page re-rendered against current events,
// every affordance a plain HTML form. The injected shell layers feel on top —
// pending turns, live re-replay, theme — but carries no state and grants no
// power: strip it and every page still works, because the forms do.
func cmdServe(home string) error {
	refreshSite(home)

	mux := http.NewServeMux()

	// GET /            → kernel.html (my identity), or a welcome projection if grown
	// GET /<name>      → that projection, re-run live
	// anything else    → static site/ files
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.Trim(r.URL.Path, "/"), ".html")
		if name != "" && !validCapabilityName(name) {
			http.Error(w, "not found", 404)
			return
		}
		if name == "" {
			if p, _ := scriptPath(home, "projector", "welcome"); fileExists(p) {
				name = "welcome"
			} else {
				name = "kernel"
			}
		}
		if name == "kernel" {
			renderKernelHTML(home)
			renderBriefFile(home)
			page, err := os.ReadFile(filepath.Join(home, "site", "kernel.html"))
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writePage(w, r, home, name, page)
			return
		}
		if p, _ := scriptPath(home, "projector", name); fileExists(p) {
			page, err := runProjection(home, name)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writePage(w, r, home, name, page)
			return
		}
		// Any on-disk site artifact by bare name: brief, kernel, etc.
		// .html goes through the shell; .md and .txt are served verbatim as
		// text/plain. A brain (or human, or external agent) can reach any
		// kernel-resident surface by name.
		if p, ext := siteFile(home, name); p != "" {
			if name == "brief" {
				renderBriefFile(home) // always fresh when served
				p, ext = siteFile(home, "brief")
			}
			serveSiteFile(w, r, home, name, p, ext)
			return
		}
		http.FileServer(http.Dir(filepath.Join(home, "site"))).ServeHTTP(w, r)
	})

	// GET /events → the raw log
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, logPath(home))
	})

	// POST /run/<command> → run a capability from the browser. A form's field
	// values become positional args in document order (names are for humans;
	// order is the contract); then Post/Redirect/Get back to the page.
	mux.HandleFunc("/run/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		command := strings.TrimPrefix(r.URL.Path, "/run/")
		if !validCapabilityName(command) {
			http.Error(w, "not found", 404)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var args []string
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			for _, pair := range strings.Split(string(body), "&") {
				if pair == "" {
					continue
				}
				_, v, _ := strings.Cut(pair, "=")
				v = strings.ReplaceAll(v, "+", " ")
				if dec, err := url.QueryUnescape(v); err == nil {
					v = dec
				}
				args = append(args, v)
			}
		} else if msg := strings.TrimSpace(string(body)); msg != "" {
			args = []string{msg}
		}
		if _, err := runCommand(home, command, args); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if ref := r.Header.Get("Referer"); ref != "" {
			http.Redirect(w, r, ref, http.StatusSeeOther)
			return
		}
		fmt.Fprint(w, "ok")
	})

	// Loopback by default: the write path (/run/<command>) has no auth, and
	// local-first means local. SELF_BIND is the whole bind address, host or
	// host:port (default 127.0.0.1:7777) — 0.0.0.0 opens it to the network
	// for anyone who knowingly wants that.
	addr := envOr("SELF_BIND", "127.0.0.1")
	if !strings.Contains(addr, ":") {
		addr += ":7777"
	}
	fmt.Fprintf(os.Stderr, "self: serving at http://%s (home %s)\n", addr, home)
	fmt.Fprintf(os.Stderr, "  /              my identity — capabilities, paths, contract\n")
	fmt.Fprintf(os.Stderr, "  /<projection>  a projection, re-rendered live\n")
	fmt.Fprintf(os.Stderr, "  /run/<command> run a capability (plain HTML forms)\n")
	fmt.Fprintf(os.Stderr, "  /events        the raw event log\n")
	return http.ListenAndServe(addr, mux)
}

// cmdGrow grows a seed: a directory with intent.md (the genotype — prose
// intent, not a parts-list) and optionally seed.jsonl (initial content events,
// the initial deposit). The orchestrator reads the intent, explores the
// instance, and declares the decomposition that realizes it here; each piece is
// then compiled with the whole intent woven in. Same intent, different instance,
// different decomposition.
// growPrompt frames the orchestration ask: decompose the intent into declared
// capabilities, and hand them back the one way the kernel accepts them.
// thinkPrompt wraps a think ask with the answer contract. A think is
// report-only — the kernel returns brain-authored events to the caller instead
// of ingesting them — but the brain still needs to know its stdout is the only
// channel: without the contract, a tool-capable brain wastes its session trying
// to persist its work itself (edit the log, run the CLI) and gets denied. Every
// event-expecting ask carries the same guidance; this was the one naked ask left.
func thinkPrompt(prompt string) string {
	return prompt + "\n\n" + brainAnswerContract
}

func growPrompt(intent string) string {
	return "Grow the capabilities that realize this product: declare each one by emitting a command.declared / projector.declared event, then summarize in one line.\n\n" +
		brainAnswerContract + "\n\n--- INTENT ---\n" + intent + "\n--- END INTENT ---"
}

// parseDeposit reads a seed.jsonl initial deposit into events.
func parseDeposit(raw []byte) ([]Event, error) {
	var evs []Event
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse seed.jsonl: %w", err)
		}
		evs = append(evs, e)
	}
	return evs, nil
}

// readSeedSource resolves a grow reference — a directory on disk — to its
// intent and initial deposit.
func readSeedSource(ref string) (name, intent string, deposit []Event, err error) {
	data, e := os.ReadFile(filepath.Join(ref, "intent.md"))
	if e != nil {
		return "", "", nil, fmt.Errorf("a seed is a directory with an intent.md: %w", e)
	}
	name, intent = filepath.Base(ref), strings.TrimSpace(string(data))
	if raw, e := os.ReadFile(filepath.Join(ref, "seed.jsonl")); e == nil {
		deposit, err = parseDeposit(raw)
	}
	return name, intent, deposit, err
}

func cmdGrow(home, ref string) error {
	name, intent, deposit, err := readSeedSource(ref)
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{"name": name, "intent": intent})
	ie := newEvent("intent.declared", payload)
	if err := appendEvent(home, &ie); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "self: orchestrating %q from intent…\n", name)
	res, err := pipeBrain(home, "grow", growPrompt(intent))
	if err != nil {
		return fmt.Errorf("orchestrate %q: %w (growing needs a brain — %s)", name, err, brainHint)
	}
	c := newLLM(home)
	c.intent = intent
	if len(res.Events) == 0 {
		return fmt.Errorf("the orchestrator declared nothing for %q", name)
	}

	// The orchestrator's stated reasoning is provenance: log it, so the chain
	// from intent to script survives in the log (and in any seed sharing it),
	// and weave it into each compile of this grow so every piece is authored
	// with the plan in view — in-band continuity, never a session store.
	if r := strings.TrimSpace(res.Response); r != "" {
		c.reasoning = r
		rp, _ := json.Marshal(map[string]any{"seed": name, "reasoning": r})
		re := newEvent("grow.orchestrated", rp)
		if err := appendEvent(home, &re); err != nil {
			return err
		}
	}

	var declEvents []Event
	for _, d := range res.Events {
		n, _ := d["name"].(string)
		p, _ := json.Marshal(d["payload"])
		if (n != "command.declared" && n != "projector.declared") || string(p) == "null" {
			continue
		}
		e := newEvent(n, p)
		if err := appendEvent(home, &e); err != nil {
			return err
		}
		declEvents = append(declEvents, e)
	}
	grown := compileDeclarations(c, home, declEvents)

	// The initial deposit: content laid once, so the surface has
	// something to render from the first moment.
	for _, e := range deposit {
		fresh := newEvent(e.Name, e.Payload)
		if err := appendEvent(home, &fresh); err != nil {
			return err
		}
	}

	rp, _ := json.Marshal(map[string]any{"seed": name, "capabilities": grown})
	se := newEvent("seed.planted", rp)
	if err := appendEvent(home, &se); err != nil {
		return err
	}
	refreshSite(home)
	fmt.Printf("grew %q: %d capabilit(ies) from intent — %s\n", name, grown, res.Response)
	return nil
}

// ──────────────── sharing — intent and evidence between instances ────────────
//
// A seed is a verbatim slice of the sender's log: every declaration of one
// capability (the intent, re-teachings and dead ends included) and every
// kernel-signed receipt for it (the evidence). The log's own format is the
// wire format. Code never crosses: adopt records the whole seed inside a
// single capability.adopted event — foreign receipts ride there, where
// rehydrate never looks, inert by construction — then re-declares the
// capability so the strange loop authors bytes for THIS instance, through its own
// compiler, signed by its own key. The sender's latest script rides only as
// the reference a seed author already gets; the compiler adapts, never copies.

// declName returns the capability a declaration event declares, or "".
func declName(e Event) (typ, name string) {
	if e.Name != "command.declared" && e.Name != "projector.declared" {
		return "", ""
	}
	var d struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(e.Payload, &d) != nil {
		return "", ""
	}
	return strings.TrimSuffix(e.Name, ".declared"), d.Name
}

func cmdShare(home, name string) error {
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	secret, err := loadSecret(home)
	if err != nil {
		return err
	}
	var seed []Event
	hasDecl := false
	for _, e := range events {
		if _, n := declName(e); n == name {
			seed, hasDecl = append(seed, e), true
		} else if e.Name == "script.compiled" {
			if r, ok := verifiedReceipt(secret, e.Payload); ok && r.Name == name {
				seed = append(seed, e)
			}
		}
	}
	if !hasDecl {
		return fmt.Errorf("no declaration for %q in this log — nothing to share (code never crosses; the declaration is what does)", name)
	}
	enc := json.NewEncoder(os.Stdout)
	for i := range seed {
		enc.Encode(seed[i])
	}
	// The sender remembers giving: if it is not an event, it did not happen.
	payload, _ := json.Marshal(map[string]any{"name": name, "events": len(seed)})
	e := newEvent("capability.shared", payload)
	if err := appendEvent(home, &e); err != nil {
		return err
	}
	refreshSite(home)
	fmt.Fprintf(os.Stderr, "self: shared %q — %d event(s) of intent and evidence\n", name, len(seed))
	return nil
}

// receiptCount counts locally-verified script.compiled receipts for one
// capability. It is the "did a compile actually land" signal: script files
// are derived state and may predate a failed recompile, but a receipt only
// exists if this instance signed fresh bytes.
func receiptCount(home, typ, name string) int {
	secret, err := loadSecret(home)
	if err != nil {
		return 0
	}
	events, err := readEvents(home)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range events {
		if e.Name != "script.compiled" {
			continue
		}
		if r, ok := verifiedReceipt(secret, e.Payload); ok && r.Type == typ && r.Name == name {
			n++
		}
	}
	return n
}

func parseCapabilityTarget(target string) (typ, name string, err error) {
	typ, name, ok := strings.Cut(strings.TrimSpace(target), "/")
	if !ok || name == "" {
		return "", "", fmt.Errorf("target must be command/<name> or projector/<name>")
	}
	if typ != "command" && typ != "projector" {
		return "", "", fmt.Errorf("target type must be command or projector")
	}
	if !validCapabilityName(name) {
		return "", "", fmt.Errorf("unsafe capability name %q", name)
	}
	return typ, name, nil
}

func latestCapabilitySource(home, typ, name string) (decl json.RawMessage, script, receiptID string, err error) {
	events, err := readEvents(home)
	if err != nil {
		return nil, "", "", err
	}
	secret, err := loadSecret(home)
	if err != nil {
		return nil, "", "", err
	}
	for _, e := range events {
		if t, n := declName(e); t == typ && n == name {
			decl = e.Payload
			continue
		}
		if e.Name != "script.compiled" {
			continue
		}
		if r, ok := verifiedReceipt(secret, e.Payload); ok && r.Type == typ && r.Name == name {
			script = r.Script
			receiptID = e.ID
		}
	}
	if decl == nil {
		return nil, "", "", fmt.Errorf("no declaration for %s/%s", typ, name)
	}
	if strings.TrimSpace(script) == "" {
		return nil, "", "", fmt.Errorf("no verified script receipt for %s/%s", typ, name)
	}
	return decl, script, receiptID, nil
}

func cmdRevise(home, target string, words []string) error {
	typ, name, err := parseCapabilityTarget(target)
	if err != nil {
		return err
	}
	request := strings.TrimSpace(strings.Join(words, " "))
	if request == "" {
		return fmt.Errorf("usage: self revise %s/%s <change request>", typ, name)
	}
	declPayload, script, receiptID, err := latestCapabilitySource(home, typ, name)
	if err != nil {
		return err
	}
	var decl map[string]any
	if err := json.Unmarshal(declPayload, &decl); err != nil {
		return fmt.Errorf("latest declaration for %s/%s is not an object: %w", typ, name, err)
	}
	decl["implementation"] = script
	decl["revision"] = map[string]any{"request": request, "from_receipt": receiptID}
	updatedDecl, _ := json.Marshal(decl)
	revisionPayload, _ := json.Marshal(map[string]any{"type": typ, "name": name, "request": request, "from_receipt": receiptID})
	before := receiptCount(home, typ, name)
	if err := ingest(home, []Event{
		newEvent("capability.revision.requested", revisionPayload),
		newEvent(typ+".declared", updatedDecl),
	}); err != nil {
		return err
	}
	if receiptCount(home, typ, name) <= before {
		return fmt.Errorf("revision for %s/%s was recorded, but the compile produced no signed receipt", typ, name)
	}
	fmt.Printf("revised %s/%s — compiled a fresh signed receipt\n", typ, name)
	return nil
}

// cmdRetire appends a capability.retired tombstone. Deletion in an
// event-sourced instance is a fold rule, not an erasure: every declaration and
// receipt stays in the log, but the script comes off disk, a projector's page
// leaves site/, and the brief, kernel index, and rehydrate all stop seeing the
// capability. Re-declaring it later revives it — the fold is ordered.
func cmdRetire(home, target string) error {
	typ, name, err := parseCapabilityTarget(target)
	if err != nil {
		return err
	}
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	commands, _, projectors, _ := declaredCaps(events)
	declared := false
	switch typ {
	case "command":
		_, declared = commands[name]
	case "projector":
		_, declared = projectors[name]
	}
	if !declared {
		return fmt.Errorf("nothing to retire: %s/%s is not currently declared", typ, name)
	}
	payload, _ := json.Marshal(retirement{Type: typ, Name: name})
	if err := ingest(home, []Event{newEvent("capability.retired", payload)}); err != nil {
		return err
	}
	fmt.Printf("retired %s/%s — the log keeps its history; re-declare to revive\n", typ, name)
	return nil
}

func cmdAdopt(home, path string) error {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	var seed []Event
	var typ, name string
	var declPayload json.RawMessage
	reference := ""
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var e Event
		if json.Unmarshal([]byte(line), &e) != nil || e.Name == "" {
			return fmt.Errorf("not a seed — want event JSONL, one {name, payload} per line")
		}
		seed = append(seed, e)
		if t, n := declName(e); n != "" { // the latest declaration is what grows here
			typ, name, declPayload = t, n, e.Payload
		}
		if e.Name == "script.compiled" { // the latest script is reference, never install
			var r receipt
			if json.Unmarshal(e.Payload, &r) == nil && r.Script != "" {
				reference = r.Script
			}
		}
	}
	if declPayload == nil {
		return fmt.Errorf("the seed carries no declaration — nothing can grow from it")
	}
	if err := ensureHome(home); err != nil {
		return err
	}
	if reference != "" {
		var m map[string]any
		if err := json.Unmarshal(declPayload, &m); err != nil {
			return fmt.Errorf("the seed's declaration is not an object: %w", err)
		}
		m["implementation"] = reference
		declPayload, _ = json.Marshal(m)
	}
	ap, _ := json.Marshal(map[string]any{"type": typ, "name": name, "seed": seed})
	before := receiptCount(home, typ, name)
	if err := ingest(home, []Event{
		newEvent("capability.adopted", ap),
		newEvent(typ+".declared", declPayload),
	}); err != nil {
		return err
	}
	// A stale script from an earlier receipt can outlive a failed recompile,
	// so "the file exists" proves nothing. The honest signal is the log: this
	// adopt succeeded only if it minted a fresh signed receipt.
	if receiptCount(home, typ, name) <= before {
		return fmt.Errorf("adopted %q into the log, but the compile produced no signed receipt (any script on disk is from an earlier receipt) — wire a brain and declare it again", name)
	}
	fmt.Printf("adopted %q — re-authored by this instance's own compiler, signed by its own key\n", name)
	return nil
}

func cmdRun(home, command string, args []string) error {
	evs, err := runCommand(home, command, args)
	if err != nil {
		return err
	}
	for _, e := range evs {
		fmt.Printf("appended seq %d %s\n", e.Seq, e.Name)
	}
	return nil
}

// cmdThink asks the brain and prints {response, events} JSON. The brain
// is a PROCESS the kernel pipes the log to — $SELF_BRAIN is any program honoring
// the contract (prompt as last arg, event JSONL out). think appends nothing:
// the caller owns that.
func cmdThink(home, prompt string) error {
	if prompt == "" {
		data, _ := io.ReadAll(os.Stdin)
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		return fmt.Errorf("usage: self think <prompt> (or pipe it on stdin)")
	}
	res, err := pipeBrain(home, "think", thinkPrompt(prompt))
	if err != nil {
		return fmt.Errorf("brain: %w", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"response": res.Response, "events": res.Events, "declarations": res.Events})
}

// pipeBrain is the ONE seam through which the kernel asks for intelligence —
// think, heartbeat, grow, and compile all pass here. It spawns $SELF_BRAIN with
// the ask's kind in $SELF_ASK, the prompt as the last argument, and a freshly
// written orientation brief from SELF_HOME/site/brief.md on stdin: where the
// brain is, what capabilities exist, and where to look for the rest — nothing
// more. The brief is on disk, like every other piece of state, so a tool-using
// brain can read it itself (cat SELF_HOME/site/brief.md) and the kernel has no
// internal state a brain cannot see. The brief is recomputed before every ask
// so a brain never reads stale orientation — the kernel writes the file, then
// reads back exactly what it wrote, and feeds that. A real brain MUST inspect
// SELF_HOME itself (site/*.html, events.jsonl, capabilities/) with its own
// tools: the brief is a wake-up card, not a context dump, and a process without
// that exploration ability is not a complete brain. The kernel's seam is still
// a pipe; the tool loop is the brain's concern, never the kernel's. The parse
// of the brain's stdout is tolerant on purpose: JSON lines with a name are
// events — script.authored answers a compile, chat.message carries the reply,
// anything else is a returned event — and bare prose joins the reply. With no
// brain plugged in, the ask fails with a hint.
func pipeBrain(home, kind, prompt string) (*brainResult, error) {
	exe := brainExe()
	if exe == "" {
		return nil, fmt.Errorf("no brain is plugged in — %s", brainHint)
	}
	brief := freshBrief(home)
	bin, argv := brainCommand(exe, prompt)
	cmd := exec.Command(bin, argv...)
	cmd.Env = brainEnv(home, kind)
	cmd.Dir = home
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start brain %s: %w (%s)", filepath.Base(bin), err, brainHint)
	}
	feedText(stdin, brief)
	res := &brainResult{Events: []map[string]any{}}
	var prose []string
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		content, fence := unfence(line)
		if fence { // a bare ``` / ```json marker: pure decoration, not reply text
			continue
		}
		var e struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal([]byte(content), &e) != nil || e.Name == "" {
			prose = append(prose, line)
			continue
		}
		switch e.Name {
		case "chat.message":
			var p struct{ Role, Content string }
			if json.Unmarshal(e.Payload, &p) == nil && p.Role == "assistant" {
				prose = append(prose, p.Content)
			}
		case "script.authored":
			var p struct{ Script string }
			if json.Unmarshal(e.Payload, &p) == nil {
				res.Script = p.Script
			}
		default:
			res.Events = append(res.Events, map[string]any{"name": e.Name, "payload": json.RawMessage(e.Payload)})
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("brain %s exited: %w", filepath.Base(bin), err)
	}
	res.Response = strings.Join(prose, "\n")
	return res, nil
}

const brainHint = `plug a brain: SELF_BRAIN=<a tool-capable executable, e.g. "claude -p" or examples/brain-opencode>; the brain must inspect SELF_HOME itself. See examples/README.md. For offline demos/tests, examples/brain-stub is a deterministic no-LLM brain.`

// brainCommand splits a configured executable into command and args, appending
// the prompt as the last argument.
func brainCommand(exe, prompt string) (string, []string) {
	parts := strings.Fields(exe)
	return parts[0], append(parts[1:], prompt)
}

// unfence strips the Markdown a chat-shaped brain (claude -p and its kin) wraps
// JSON in, so a model that answers in prose still plugs into the pipe unchanged.
// A line that is a bare fence marker (``` or ```json) is decoration, reported by
// the second return so the caller drops it from the reply text; a single line
// wrapped in backticks (`{…}`) is unwrapped to its content. Anything else — plain
// JSON from the stub or an adapter, or ordinary prose — passes through untouched,
// so no existing brain regresses.
func unfence(line string) (content string, fence bool) {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "```") {
		return "", true
	}
	if len(t) >= 2 && strings.HasPrefix(t, "`") && strings.HasSuffix(t, "`") {
		return strings.TrimSpace(strings.Trim(t, "`")), false
	}
	return t, false
}

// cmdHeartbeat is one self-improvement cycle: the brain reads what changed
// since its last beat, explores, and — if warranted — declares one small
// improvement, which compiles through the strange loop.
func cmdHeartbeat(home string) error {
	prior, _ := readEvents(home)
	hb := newEvent("self.heartbeat", json.RawMessage(`{}`))
	if err := appendEvent(home, &hb); err != nil {
		return err
	}
	prompt := `This is a self-improvement heartbeat. Explore your instance — capabilities, recent events, projections — and choose ONE small, high-value improvement: a missing capability, a clearer projection, a drift to fix. If warranted, declare it (emit command.declared / projector.declared); if nothing is worth changing, say so plainly and declare nothing. Keep it minimal.` +
		"\n\n" + brainAnswerContract + heartbeatContext(prior)
	res, err := pipeBrain(home, "heartbeat", prompt)
	if err != nil {
		return err
	}
	applyEvents(home, res)
	fmt.Println(res.Response)
	return nil
}

// heartbeatContext hands the brain the events since its last beat — capped,
// minus kernel bookkeeping receipts — so a beat reacts to what changed instead
// of exploring from scratch.
func heartbeatContext(events []Event) string {
	last := -1
	for i, e := range events {
		if e.Name == "self.heartbeat" {
			last = i
		}
	}
	var acts []Event
	for _, e := range events[last+1:] {
		if e.Name == "script.compiled" || e.Name == "script.verified" {
			continue
		}
		acts = append(acts, e)
	}
	if len(acts) == 0 {
		return ""
	}
	if len(acts) > 40 {
		acts = acts[len(acts)-40:]
	}
	var b strings.Builder
	b.WriteString("\n\nSince your last heartbeat, these things happened in this instance:\n")
	for _, e := range acts {
		payload := strings.TrimSpace(string(e.Payload))
		if len(payload) > 140 {
			payload = payload[:140] + "…"
		}
		fmt.Fprintf(&b, "  seq %d  %s  %s\n", e.Seq, e.Name, payload)
	}
	b.WriteString("\nResponding to what changed is welcome, but optional.")
	return b.String()
}

func cmdShow(home, name string) error {
	if name == "kernel" {
		renderKernelHTML(home)
		renderBriefFile(home)
		page, err := os.ReadFile(filepath.Join(home, "site", "kernel.html"))
		if err != nil {
			return err
		}
		os.Stdout.Write(page)
		return nil
	}
	if name == "brief" {
		renderBriefFile(home)
		data, err := os.ReadFile(filepath.Join(home, "site", "brief.md"))
		if err != nil {
			return err
		}
		os.Stdout.Write(data)
		return nil
	}
	// a live projector takes precedence over a stale on-disk file of the
	// same name — projectors are the log's pure replay, re-run live.
	if p, _ := scriptPath(home, "projector", name); fileExists(p) {
		page, err := runProjection(home, name)
		if err != nil {
			return err
		}
		os.Stdout.Write(page)
		return nil
	}
	// bare name → on-disk artifact (.html, .md, .txt) under site/, if present
	if p, _ := siteFile(home, name); p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		// Write verbatim — the same bytes the server serves. .md and .txt are
		// plain text; .html is the projection's own markup.
		os.Stdout.Write(data)
		return nil
	}
	return fmt.Errorf("projection %q not found", name)
}

// ─────────────────────────────────── main ───────────────────────────────────

func homeDir() string {
	if v := os.Getenv("SELF_HOME"); v != "" {
		// Scripts run with cwd = home, so a relative home would silently break
		// every exec. Absolute, always.
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return v
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// ensureHome initializes a bare instance on first contact: a signing key and a first
// event. Everything else grows.
func ensureHome(home string) error {
	if _, err := loadSecret(home); err != nil {
		return err
	}
	events, err := readEvents(home)
	if err != nil || len(events) > 0 {
		return err
	}
	e := newEvent("kernel.initialized", json.RawMessage(`{}`))
	if err := appendEvent(home, &e); err != nil {
		return err
	}
	renderKernelHTML(home)
	renderBriefFile(home)
	fmt.Fprintf(os.Stderr, "self: new home %s\n", home)
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, usageText())
}

func usageText() string {
	return `self — a local-first, event-sourced runtime with LLM-generated capabilities

One append-only event log + projections as deterministic replays. A minimal
kernel; every capability is generated from a declaration and installed under
a signed receipt.

usage: self [command] [args]

  self                 rehydrate the instance from the log, then serve it (the default)
  self grow <seed>     grow a seed's intent into capabilities (needs a brain)
  self run <cmd> ...   run a capability — append events, refresh projections
  self think "..."     ask the brain; returns {response, events} JSON
  self heartbeat       one self-improvement cycle (the brain reflects & grows)
  self show <name>     render a projection to stdout
  self rehydrate       rebuild capabilities/ + site/ from the log's signed receipts (no LLM)
  self share <cap>     print a seed to stdout — the capability's declarations and
                       receipts, a verbatim slice of this log
  self adopt <seed>    re-grow a shared capability here ("-" reads stdin) — this
                       instance's own compiler re-authors it; foreign bytes never install
  self revise <target> <request>
                       edit an installed local capability with its current script as context
  self retire <target> retire a capability — its script and page leave the
                       surface; the log keeps every event, re-declaring revives
  self protocol        print the brain + capability wire protocol

environment:
  SELF_HOME         the instance — a dir holding events.jsonl + .secret
                    (default: current working directory; set it in your shell rc
                    to pin a shared instance, e.g. export SELF_HOME=~/.self)

  plug a brain (one seam; think, heartbeat, grow, and compile all pass through it):
  SELF_BRAIN        a tool-capable executable, e.g. "claude -p" or
                    examples/brain-opencode — it gets the ask's kind in
                    $SELF_ASK, the prompt as its last argument, and an
                    orientation brief on stdin; it answers in event JSONL,
                    prose tolerated. The brain must inspect SELF_HOME itself
                    (site/*.html, events.jsonl, capabilities/) with its own
                    tools. See examples/README.md. examples/brain-stub is a
                    deterministic offline brain for demos/tests;
                    examples/brain-openai is a reference adapter that
                    illustrates the wire shape but is incomplete by spec
                    (no tool loop).
  SELF_BRAIN_ID     provenance by-line signed into script.compiled receipts
                    (default: the brain executable)
  SELF_THEME        default page design when serving: grove | micro | paper |
                    spec (default grove); a ?theme= link or the on-page picker
                    overrides it per viewer. Presentation only — never logged.
`
}

func protocolText() string {
	return `self protocol — the wire contracts

Brain process contract

  The same seam handles think, heartbeat, grow, and compile.

  SELF_BRAIN   executable to spawn, optionally with args. A brain MUST be able to
              inspect files under SELF_HOME (site/*.html, events.jsonl,
              capabilities/) with its own tools — a plain stdin/stdout adapter
              with no file access cannot do the job. Coding-agent brains
              (opencode run, claude -p) already have such tools.
  SELF_ASK     request kind: think | heartbeat | grow | compile
  argv         the prompt is passed as the last argument
  stdin        an orientation brief (plain text): where the brain is, what
               capabilities exist, and where to look for the rest. The brain is
               expected to explore SELF_HOME itself for depth — this is a
               wake-up card, not a context dump.
  stdout       event JSONL; non-JSON lines are tolerated as prose reply text

Brain reply events

  chat.message        prose reply for think:
                      {"name":"chat.message","payload":{"role":"assistant","content":"..."}}

  command.declared    declare a command capability; the kernel compiles it:
                      {"name":"command.declared","payload":{"name":"note","description":"...","params":{"text":"string"},"event":{"name":"note.added","fields":{"text":"string"}}}}

  projector.declared  declare a projection; the kernel compiles it:
                      {"name":"projector.declared","payload":{"name":"notes","description":"...","consumes":["note.added"]}}

  script.authored     answer to SELF_ASK=compile only:
                      {"name":"script.authored","payload":{"script":"#!/bin/sh\n..."}}

  capability.retired  retire a capability: its script and page leave the derived
                      surface; the log keeps all history and a re-declaration
                      revives it:
                      {"name":"capability.retired","payload":{"type":"projector","name":"notes"}}

Compiled capability contract

  command script      argv are command args; stdin is the current event log JSONL;
                      stdout is new event JSONL: {"name":"event.name","payload":{...}}
                      the kernel assigns id, seq, and occurred_at, appends the
                      events, then re-renders all projections.

  projector script    stdin is the full event log JSONL; stdout is HTML.
                      The kernel writes it to SELF_HOME/site/<name>.html.

  environment         SELF_HOME is set for every compiled script.

Declarations cross instance boundaries; runnable code does not. A generated
script installs only after the local kernel signs a script.compiled receipt with
SELF_HOME/.secret and the current SELF_BRAIN_ID.
`
}

func commandHelp(cmd string) (string, bool) {
	switch cmd {
	case "grow":
		return "usage: self grow <seed-dir>\n\nRead <seed-dir>/intent.md, ask the brain to declare capabilities, compile them, and install signed receipts.\n", true
	case "run":
		return "usage: self run <command> [args...]\n\nRun an installed command capability. Its emitted events are appended, then projections re-render.\n", true
	case "think":
		return "usage: self think <prompt>\n       self think < prompt.txt\n\nAsk the brain through the SELF_BRAIN protocol. Prints {response, events} JSON and appends nothing.\n", true
	case "heartbeat":
		return "usage: self heartbeat\n\nAppend a heartbeat event, ask the brain for one small improvement, and compile any declarations it emits.\n", true
	case "show":
		return "usage: self show <projection>\n\nRender a projection to stdout by replaying the current log. Use 'kernel' for the instance index.\n", true
	case "rehydrate":
		return "usage: self rehydrate\n\nRebuild capabilities/ and site/ from events.jsonl + .secret without a brain.\n", true
	case "share":
		return "usage: self share <capability>\n\nPrint the capability's declarations and receipts as a JSONL seed.\n", true
	case "adopt":
		return "usage: self adopt <seed.jsonl>\n       self adopt - < seed.jsonl\n\nRecord a shared seed and re-generate its capability locally; foreign code never installs.\n", true
	case "revise":
		return "usage: self revise command/<name> <change request>\n       self revise projector/<name> <change request>\n\nRecord a local revision request, then recompile the installed capability with its latest declaration and verified script as context.\n", true
	case "retire":
		return "usage: self retire command/<name>\n       self retire projector/<name>\n\nAppend a capability.retired tombstone: the installed script (and a projector's page) come off disk, the brief and kernel index stop listing it, and rehydrate honors the tombstone. Events are never deleted — re-declaring the capability revives it.\n", true
	case "protocol":
		return protocolText(), true
	}
	return "", false
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" || arg == "help" {
			return true
		}
	}
	return false
}

func main() {
	home := homeDir()
	if len(os.Args) < 2 {
		err := ensureHome(home)
		if err == nil {
			err = rehydrate(home)
		}
		if err == nil {
			err = cmdServe(home)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "self: %s\n", err)
			os.Exit(1)
		}
		return
	}

	cmd, args := os.Args[1], os.Args[2:]

	var err error
	if cmd != "help" && wantsHelp(args) {
		if text, ok := commandHelp(cmd); ok {
			fmt.Fprint(os.Stdout, text)
			return
		}
	}

	switch cmd {
	case "grow":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self grow <seed-dir>")
		} else {
			err = cmdGrow(home, args[0])
		}
	case "run":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self run <command> [args...]")
		} else {
			err = cmdRun(home, args[0], args[1:])
		}
	case "think":
		err = cmdThink(home, strings.Join(args, " "))
	case "heartbeat":
		err = cmdHeartbeat(home)
	case "show":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self show <projection>")
		} else {
			err = cmdShow(home, args[0])
		}
	case "rehydrate":
		err = rehydrate(home)
	case "share":
		if len(args) != 1 {
			err = fmt.Errorf("usage: self share <capability>  (the seed prints to stdout)")
		} else {
			err = cmdShare(home, args[0])
		}
	case "adopt":
		if len(args) != 1 {
			err = fmt.Errorf("usage: self adopt <seed.jsonl>")
		} else {
			err = cmdAdopt(home, args[0])
		}
	case "revise":
		if len(args) < 2 {
			err = fmt.Errorf("usage: self revise command/<name> <change request>")
		} else {
			err = cmdRevise(home, args[0], args[1:])
		}
	case "retire":
		if len(args) != 1 {
			err = fmt.Errorf("usage: self retire command/<name> | projector/<name>")
		} else {
			err = cmdRetire(home, args[0])
		}
	case "protocol":
		fmt.Fprint(os.Stdout, protocolText())
	case "help", "--help", "-h":
		if len(args) == 0 {
			usage()
		} else if text, ok := commandHelp(args[0]); ok {
			fmt.Fprint(os.Stdout, text)
		} else {
			err = fmt.Errorf("unknown help topic %q", args[0])
		}
	default:
		fmt.Fprintf(os.Stderr, "self: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "self: %s\n", err)
		os.Exit(1)
	}
}

// ──────────────────────────────── small bits ────────────────────────────────

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func jsonRepr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
