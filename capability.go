package main

// Capabilities: one replay, one materialization, two ways to run.
//
// Everything derived — what exists, what is pending, what was refused, which
// bytes are trusted — comes out of a single walk over the log in replay().
// There is no second source and no cache to drift.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	kindCommand = "command"
	kindView    = "view"
)

// decl is a declaration: a name and prose. Deliberately schemaless — v1 carried
// params, an event schema, a reference implementation and a revision record,
// none of which anything validated, and the reference implementation was a
// runnable riding the one channel the account protocol exists to keep runnables
// out of. What a command takes and emits belongs in Description, because that
// string is what the next cold mind actually reads.
type decl struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Consumes    []string `json:"consumes,omitempty"` // views only: the events fed on stdin
}

// capability is a live declared capability and whatever the log says about it.
type capability struct {
	Type    string
	Name    string
	Decl    decl
	DeclSeq int
	Receipt *receipt // the latest verified receipt, nil if never installed
	RcptSeq int
	Reject  *rejection // a refusal that still stands for this capability
}

func (c *capability) key() string { return c.Type + "/" + c.Name }

// Pending reports whether this capability is waiting for a script: never
// installed, or re-declared since its last receipt (which is what a revision
// looks like on an append-only log).
func (c *capability) Pending() bool { return c.Receipt == nil || c.RcptSeq < c.DeclSeq }

// rejection is the kernel's testimony that it refused an authored script.
type rejection struct {
	Seq    int    `json:"-"`
	Type   string `json:"type,omitempty"`
	Name   string `json:"name,omitempty"`
	Reason string `json:"reason"`
	// Excerpt is bounded: a refused script never installs, so replay never
	// needs it whole — it is there to diagnose, not to restore.
	Excerpt string `json:"excerpt,omitempty"`
}

const excerptCap = 1024

// state is the log, replayed. Constructed once per invocation.
type state struct {
	Events []Event
	Key    []byte
	Caps   []*capability // live, in first-declared order
	byKey  map[string]*capability
	Reject []*rejection // refusals still standing, in log order
	Work   []*workItem  // open stacked work, in log order
}

// workItem is a line of prose the next pass should do. Not a capability —
// arbitrary work. It rides the prompt like a pending declaration and holds
// the loop open until a matching work.done.
type workItem struct {
	Seq  int
	Text string
}

func loadState(home string) (*state, error) {
	events, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	return replay(events, secret(home)), nil
}

// replay is the only interpreter of the log. Given the events and the key, it
// answers every derived question at once. A nil key means nothing verifies:
// a directory holding a log but no .secret has no capabilities, which is the
// truth about it.
func replay(events []Event, key []byte) *state {
	st := &state{Events: events, Key: key, byKey: map[string]*capability{}}
	rejects := map[string]*rejection{}

	forget := func(k string) {
		delete(st.byKey, k)
		delete(rejects, k)
		for i, c := range st.Caps {
			if c.key() == k {
				st.Caps = append(st.Caps[:i], st.Caps[i+1:]...)
				break
			}
		}
	}
	live := func(typ, name string) *capability {
		k := typ + "/" + name
		if c, ok := st.byKey[k]; ok {
			return c
		}
		c := &capability{Type: typ, Name: name}
		st.byKey[k] = c
		st.Caps = append(st.Caps, c)
		return c
	}

	for _, e := range events {
		switch e.Name {
		case "command.declared", "view.declared":
			typ := strings.TrimSuffix(e.Name, ".declared")
			var d decl
			if json.Unmarshal(e.Payload, &d) != nil || !validCapability(typ, d.Name) {
				continue
			}
			c := live(typ, d.Name)
			c.Decl, c.DeclSeq = d, e.Seq

		case "script.installed":
			// A receipt is the kernel's own testimony, so it counts only
			// through the kernel's own door. Without this gate, echoing an old
			// receipt payload back through `hear` re-installs it — undoing a
			// retirement, or rolling a fixed script back to a broken one — with
			// no key and no declaration, because the signature on a genuine
			// receipt stays valid forever.
			if e.Via != doorKernel {
				continue
			}
			// A receipt for a capability no declaration mentions still
			// installs: rehydrate must be able to rebuild from receipts alone.
			// It can only exist if someone held the key.
			r, ok := verifyReceipt(key, e.Payload)
			if !ok {
				continue
			}
			c := live(r.Type, r.Name)
			c.Receipt, c.RcptSeq = &r, e.Seq
			// A successful install closes the refusal it supersedes — and every
			// refusal that names nothing a declaration or a retirement could
			// ever match. Those cannot be closed on their own key, so without
			// this the instance never reports quiet again and the documented
			// loop spins forever.
			delete(rejects, r.Type+"/"+r.Name)
			for k, rej := range rejects {
				if !validCapability(rej.Type, rej.Name) {
					delete(rejects, k)
				}
			}

		case "script.rejected":
			// The kernel's own testimony only. A refusal arriving through the
			// pipe or a learned account is inert: doors are local facts.
			if e.Via != doorKernel {
				continue
			}
			var r rejection
			if json.Unmarshal(e.Payload, &r) != nil || r.Reason == "" {
				continue
			}
			r.Seq = e.Seq
			rejects[r.Type+"/"+r.Name] = &r

		case "capability.retired":
			var t struct{ Type, Name string }
			if json.Unmarshal(e.Payload, &t) != nil || !validCapability(t.Type, t.Name) {
				continue
			}
			forget(t.Type + "/" + t.Name)

		case "work.queued":
			var p struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(e.Payload, &p) != nil {
				continue
			}
			text := strings.TrimSpace(p.Text)
			if text == "" {
				continue
			}
			st.Work = append(st.Work, &workItem{Seq: e.Seq, Text: text})

		case "work.done":
			var p struct {
				Seq  int    `json:"seq"`
				Text string `json:"text"`
			}
			if json.Unmarshal(e.Payload, &p) != nil {
				continue
			}
			st.Work = closeWork(st.Work, p.Seq, strings.TrimSpace(p.Text))
		}
	}

	for _, c := range st.Caps {
		c.Reject = rejects[c.key()]
	}
	for _, r := range rejects {
		st.Reject = append(st.Reject, r)
	}
	sort.Slice(st.Reject, func(i, j int) bool { return st.Reject[i].Seq < st.Reject[j].Seq })
	return st
}

func (st *state) cap(typ, name string) *capability { return st.byKey[typ+"/"+name] }

func (st *state) list(typ string) []*capability {
	var out []*capability
	for _, c := range st.Caps {
		if c.Type == typ {
			out = append(out, c)
		}
	}
	return out
}

func (st *state) pending() []*capability {
	var out []*capability
	for _, c := range st.Caps {
		if c.Pending() {
			out = append(out, c)
		}
	}
	return out
}

// quiet reports the loop's convergence: nothing declared awaits a script, no
// refusal stands, and no stacked work is open. It is what bare `self` turns
// into exit code 3.
func (st *state) quiet() bool {
	return len(st.pending()) == 0 && len(st.Reject) == 0 && len(st.Work) == 0
}

func closeWork(open []*workItem, seq int, text string) []*workItem {
	if seq != 0 {
		out := open[:0]
		for _, w := range open {
			if w.Seq != seq {
				out = append(out, w)
			}
		}
		return out
	}
	if text == "" {
		return open
	}
	out := open[:0]
	closed := false
	for _, w := range open {
		if !closed && w.Text == text {
			closed = true
			continue
		}
		out = append(out, w)
	}
	return out
}

// exemplar returns the most recently installed script, skipping the
// capabilities currently being asked about so a re-author is never anchored to
// its own broken past. A cold mind otherwise spends its first minutes
// rediscovering this instance's idiom from disk.
func (st *state) exemplar(skip map[string]bool) (string, string) {
	var name, script string
	best := 0
	for _, c := range st.Caps {
		if c.Receipt == nil || skip[c.key()] || c.RcptSeq < best {
			continue
		}
		best, name, script = c.RcptSeq, c.key(), c.Receipt.Script
	}
	const cap = 4096
	if len(script) > cap {
		script = trunc(script, cap) + "\n… (truncated)"
	}
	return name, script
}

// ──────────────────────────────── names ─────────────────────────────────────

// validCapability gates every name that reaches the filesystem. `run` is
// reserved as a trailing segment because that is the file a name's directory
// holds: a capability called notes/run would collide with notes' own script.
func validCapability(typ, name string) bool {
	if typ != kindCommand && typ != kindView {
		return false
	}
	if name == "" || strings.Contains(name, `\`) {
		return false
	}
	// Bounded so a receipt can never name something the filesystem cannot hold:
	// install would accept it and every later materialization — rehydrate
	// included — would fail on a name nothing can fix.
	if len(name) > 200 {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if len(seg) > 64 {
			return false
		}
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, ".") {
			return false
		}
		// `run` is the file a capability's own directory holds, so it collides
		// at every position, not only the last: a name like x/run/y needs
		// cap/command/x/run to be a directory, which is where x's script lives.
		if seg == "run" {
			return false
		}
	}
	return true
}

// ──────────────────────────── materialization ───────────────────────────────
//
// Installed bytes are content-addressed: cap/blob/<sha256> holds the script and
// cap/<type>/<name>/run is a symlink to it. Two consequences that a
// rewrite-in-place scheme cannot have: a running script's bytes can never
// change under it (different bytes are a different path), and verifying an
// install is comparing a file to its own name.

func capDir(home string) string  { return filepath.Join(home, "cap") }
func blobDir(home string) string { return filepath.Join(home, "cap", "blob") }

func blobPath(home, sum string) string { return filepath.Join(blobDir(home), sum) }

func linkPath(home, typ, name string) string {
	return filepath.Join(capDir(home), typ, name, "run")
}

// materialize resolves a capability to executable bytes on disk, healing
// whatever it finds. It fails closed and distinguishes the three ways a
// capability can have no trusted script, because "not found" sends an agent
// looking in the wrong place.
func materialize(home string, st *state, typ, name string) (string, error) {
	c := st.cap(typ, name)
	switch {
	case c == nil:
		other := kindView
		if typ == kindView {
			other = kindCommand
		}
		if st.cap(other, name) != nil {
			return "", fmt.Errorf("no %s %q in this log — but there is a %s by that name: try `self %s %s`",
				typ, name, other, map[string]string{kindCommand: "run", kindView: "view"}[other], name)
		}
		return "", fmt.Errorf("no %s %q in this log — `self %s` lists what exists",
			typ, name, map[string]string{kindCommand: "run", kindView: "view"}[typ])
	case c.Receipt == nil && st.Key == nil && len(st.Events) > 0:
		return "", fmt.Errorf("%s %q: no receipt verifies under this instance's key — is .secret missing next to events.jsonl?", typ, name)
	case c.Receipt == nil:
		return "", fmt.Errorf("%s %q is declared but pending: no script has been authored for it yet", typ, name)
	}

	script := c.Receipt.Script
	sum := sha256.Sum256([]byte(script))
	hexsum := hex.EncodeToString(sum[:])
	blob := blobPath(home, hexsum)

	if have, err := os.ReadFile(blob); err != nil || string(have) != script {
		if err == nil {
			fmt.Fprintf(os.Stderr, "self: %s/%s: blob %s did not match its own hash — restored from the receipt at seq %d\n", typ, name, hexsum[:12], c.RcptSeq)
		}
		if err := writeFileAtomic(blob, []byte(script), 0755); err != nil {
			return "", err
		}
	}
	if err := linkBlob(home, typ, name, hexsum); err != nil {
		return "", err
	}
	return blob, nil
}

// writeFileAtomic writes through a temp file and a rename, so a reader — or an
// exec — never sees a partial script, and a running process keeps the inode it
// started with.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// linkBlob points the readable path at a blob. The symlink is for humans and
// agents (`cat cap/command/entry/run`); execution always uses the blob.
func linkBlob(home, typ, name, sum string) error {
	link := linkPath(home, typ, name)
	rel, err := filepath.Rel(filepath.Dir(link), blobPath(home, sum))
	if err != nil {
		return err
	}
	if have, err := os.Readlink(link); err == nil && have == rel {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return err
	}
	// A unique temp name: two selves materializing the same capability at once
	// must not collide on it.
	tmp, err := os.MkdirTemp(filepath.Dir(link), ".link-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	staged := filepath.Join(tmp, "run")
	if err := os.Symlink(rel, staged); err != nil {
		return err
	}
	return os.Rename(staged, link)
}

// unlink removes a retired capability's readable path. Its blob stays until
// rehydrate collects it — blobs are shared and cheap; a dangling one is inert.
func unlink(home, typ, name string) {
	link := linkPath(home, typ, name)
	os.Remove(link)
	dir := filepath.Dir(link)
	for dir != capDir(home) && dir != "/" && dir != "." {
		if os.Remove(dir) != nil { // succeeds only when empty
			break
		}
		dir = filepath.Dir(dir)
	}
}

// rehydrate makes cap/ match the log exactly: materialize what live verified
// receipts require, remove every readable path they do not, and collect
// unreferenced blobs. It is the only thing allowed to delete, and it needs
// nothing but events.jsonl and .secret — no model, no network.
func rehydrate(home string) error {
	// Under the log lock: rehydrate decides what to DELETE from a snapshot of
	// the log, so a concurrent install would otherwise have its blob and link
	// collected out from under it — leaving cap/ not matching the log, which is
	// the one thing this verb exists to guarantee.
	unlock, err := lockLog(home)
	if err != nil {
		return err
	}
	defer unlock()
	st, err := loadState(home)
	if err != nil {
		return err
	}
	keepLinks := map[string]bool{}
	keepBlobs := map[string]bool{}
	installed, failed := 0, 0
	for _, c := range st.Caps {
		if c.Receipt == nil {
			continue
		}
		if _, err := materialize(home, st, c.Type, c.Name); err != nil {
			// One capability the filesystem cannot hold must not brick the
			// audit path: say so and keep reconciling the rest.
			fmt.Fprintf(os.Stderr, "self: %s could not be materialized: %s\n", c.key(), err)
			failed++
			continue
		}
		sum := sha256.Sum256([]byte(c.Receipt.Script))
		keepLinks[linkPath(home, c.Type, c.Name)] = true
		keepBlobs[hex.EncodeToString(sum[:])] = true
		installed++
	}

	removed := 0
	for _, typ := range []string{kindCommand, kindView} {
		root := filepath.Join(capDir(home), typ)
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			// "Exactly" means exactly: a file under cap/<type>/ that no live
			// receipt puts there is stale, whatever it is called.
			if !keepLinks[path] {
				os.Remove(path)
				removed++
			}
			return nil
		})
		pruneEmpty(root)
	}
	blobs, _ := os.ReadDir(blobDir(home))
	for _, b := range blobs {
		if !keepBlobs[b.Name()] {
			os.Remove(filepath.Join(blobDir(home), b.Name()))
			removed++
		}
	}
	fmt.Fprintf(os.Stderr, "self: %d capabilit(ies) materialized from the log, %d stale file(s) removed\n", installed, removed)
	if failed > 0 {
		return fmt.Errorf("%d capabilit(ies) in the log could not be materialized (see above)", failed)
	}
	return nil
}

func pruneEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			pruneEmpty(filepath.Join(dir, e.Name()))
		}
	}
	os.Remove(dir) // succeeds only when empty
}

// ───────────────────────────── running scripts ──────────────────────────────

// scriptEnv is deliberately scrubbed. Thesis: a view is a pure function of its
// events, and a rebuild is byte-identical. Neither survives inheriting $TZ,
// $LC_ALL or a caller's $PATH, so nothing is inherited except SELF_* variables,
// which are the documented way to hand a capability configuration on purpose.
// This is determinism, not containment — see the limits in `self help`.
func scriptEnv(selfHome, work string) []string {
	env := []string{
		"HOME=" + work,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"TZ=UTC",
		"LC_ALL=C",
		// Per-process hash randomization is enough to make a script that
		// iterates a set render different bytes every run. Determinism is a
		// claim this kernel makes, so it pins the seed rather than hoping.
		"PYTHONHASHSEED=0",
	}
	// A command is an effect on one instance and is told which. A view is a
	// pure function of its events and is told nothing.
	if selfHome != "" {
		env = append(env, "SELF_HOME="+selfHome)
	}
	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(k, "SELF_") && k != "SELF_HOME" {
			env = append(env, kv)
		}
	}
	return env
}

func feed(w io.WriteCloser, events []Event) {
	go func() {
		enc := json.NewEncoder(w)
		for i := range events {
			enc.Encode(events[i])
		}
		w.Close()
	}()
}

// runCommand executes a command capability and appends what it emits. via and
// by are the invocation's provenance, stamped onto every emitted event: a
// script's own output can never set them.
func runCommand(home string, st *state, name string, args []string, via, by string) ([]Event, error) {
	bin, err := materialize(home, st, kindCommand, name)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Env, cmd.Dir, cmd.Stderr = scriptEnv(home, home), home, os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	feed(stdin, st.Events)

	var out []Event
	var parseErr error
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), lineLimit)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Only name and payload are read: everything else about an event is
		// the kernel's to say.
		var p struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &p); err != nil {
			parseErr = fmt.Errorf("command %q printed a line that is not an event: %s", name, trunc(line, 120))
			continue
		}
		if !validEventName(p.Name) {
			parseErr = fmt.Errorf("command %q emitted the event name %q, which is not lowercase dotted", name, p.Name)
			continue
		}
		e := newEvent(p.Name, p.Payload)
		e.Via, e.By = via, by
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		// Stop the producer as well as the read: waiting on a process still
		// writing into a pipe nobody drains blocks forever.
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
		return nil, fmt.Errorf("reading command %q output: %w (nothing appended)", name, err)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("command %q exited: %w (nothing appended)", name, err)
	}
	if parseErr != nil {
		return nil, fmt.Errorf("%w (nothing appended)", parseErr)
	}
	if err := appendEvents(home, out); err != nil {
		return nil, err
	}
	return out, nil
}

// runView replays a view. Its input is exactly the events its RECEIPT names —
// not its latest declaration: re-declaring a view with a wider consumes list
// leaves it pending, and until it is re-authored the old script must keep
// seeing the stream it was signed against.
func runView(home string, st *state, name string) ([]byte, error) {
	if name == "log" && st.cap(kindView, "log") == nil {
		return builtinLogView(st), nil
	}
	bin, err := materialize(home, st, kindView, name)
	if err != nil {
		return nil, err
	}
	// A view is a pure function of its events, so it is given no path to the
	// instance: no SELF_HOME, and an empty scratch directory to run in. Its
	// whole input arrives on stdin. This is not a sandbox — a script can still
	// guess a path — but the kernel no longer hands a view the log to read, or
	// to write.
	scratch, err := os.MkdirTemp("", "self-view-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)
	cmd := exec.Command(bin)
	cmd.Env, cmd.Dir, cmd.Stderr = scriptEnv("", scratch), scratch, os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	feed(stdin, consumed(st.Events, st.cap(kindView, name).Receipt.Consumes))
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("view %q exited: %w", name, err)
	}
	return []byte(out.String()), nil
}

// consumed filters the log to a view's declared inputs. An empty list — or
// "*" — means the whole log: the view asked for everything.
func consumed(events []Event, consumes []string) []Event {
	if len(consumes) == 0 {
		return events
	}
	want := map[string]bool{}
	for _, c := range consumes {
		if c == "*" {
			return events
		}
		want[c] = true
	}
	var out []Event
	for _, e := range events {
		if want[e.Name] {
			out = append(out, e)
		}
	}
	return out
}

// builtinLogView answers the cheapest question at every cold start — what
// happened here lately, and who says so — on an instance that has not yet
// grown a single view. A declared view named "log" shadows it.
func builtinLogView(st *state) []byte {
	var b strings.Builder
	for _, e := range st.Events {
		by := e.By
		if by == "" {
			by = "-"
		}
		// RFC3339, not a literal Z: a deposited event keeps its own moment, and
		// that moment may carry an offset this log did not choose.
		fmt.Fprintf(&b, "%d\t%s\t%s\tvia=%s\tby=%s\t%s\n",
			e.Seq, e.OccurredAt.Format(time.RFC3339), e.Name, e.Via, by,
			trunc(compact(e.Payload), 200))
	}
	return []byte(b.String())
}
