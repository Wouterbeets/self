package main

// The pinned invariants. Each test names a property the thesis or the protocol
// depends on, so a future change that breaks one has to break a test that says
// what it was for.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// ─────────────────────────────── helpers ────────────────────────────────────

func home(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SELF_HOME", dir)
	t.Setenv("SELF_CALLER", "")
	return dir
}

// heard pipes a body through the write door and returns its report.
func heard(t *testing.T, h, body string) string {
	t.Helper()
	var out bytes.Buffer
	if err := cmdHear(h, []byte(body), &out); err != nil && !strings.Contains(err.Error(), "refused") {
		t.Fatalf("hear %q: %v", trunc(body, 60), err)
	}
	return out.String()
}

func situated(t *testing.T, h, ask string) string {
	t.Helper()
	var out bytes.Buffer
	err := cmdSituate(h, ask, &out)
	if err != nil {
		t.Fatalf("situate: %v", err)
	}
	return out.String()
}

func line(t *testing.T, name string, payload any) string {
	t.Helper()
	p, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"name": name, "payload": json.RawMessage(p)})
	return string(b) + "\n"
}

func replayed(t *testing.T, h string) *state {
	t.Helper()
	st, err := loadState(h)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// growJournal builds the canonical instance: one command, one view, both
// installed through the real wire.
func growJournal(t *testing.T, h string) {
	t.Helper()
	body := line(t, "command.declared", decl{Name: "entry", Description: "append an entry"}) +
		line(t, "script.authored", authored{Type: "command", Name: "entry",
			Script: "#!/bin/sh\ncat >/dev/null\nprintf '{\"name\":\"journal.entry\",\"payload\":{\"text\":\"%s\"}}\\n' \"$*\"\n"}) +
		line(t, "view.declared", decl{Name: "journal", Description: "entries", Consumes: []string{"journal.entry"}}) +
		line(t, "script.authored", authored{Type: "view", Name: "journal",
			Script: "#!/bin/sh\nsed -n 's/.*\"text\":\"\\([^\"]*\\)\".*/- \\1/p'\n"})
	report := heard(t, h, body)
	if !strings.Contains(report, "installed command/entry") || !strings.Contains(report, "installed view/journal") {
		t.Fatalf("growJournal did not install: %s", report)
	}
}

// ─────────────────────────── the law: reads project ─────────────────────────

// Orientation must not write. The previous kernel appended self.asked before
// printing a prompt and minted .secret from every read path, so looking at an
// instance changed it and a stray `self` in any directory left a key behind.
func TestReadsNeverWrite(t *testing.T) {
	h := home(t)
	before, _ := os.ReadDir(h)
	if len(before) != 0 {
		t.Fatal("fresh home is not empty")
	}
	situated(t, h, "what is going on?")
	situated(t, h, "")
	var out bytes.Buffer
	if err := dispatch(h, "brief", nil, &out); err != nil {
		t.Fatal(err)
	}
	if err := dispatch(h, "view", []string{"log"}, &out); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadDir(h)
	if len(after) != 0 {
		names := []string{}
		for _, e := range after {
			names = append(names, e.Name())
		}
		t.Fatalf("reads created files: %v", names)
	}
}

// The read face must never touch stdin: it would block at the head of a
// pipeline, which is why direction is structural (argv asks, stdin hears)
// rather than sniffed from a terminal.
func TestSituateIgnoresStdin(t *testing.T) {
	h := home(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	old := os.Stdin
	os.Stdin = r // never written to, never closed: a read would hang
	defer func() { os.Stdin = old }()

	done := make(chan string, 1)
	go func() { done <- situated(t, h, "an ask") }()
	select {
	case got := <-done:
		if !strings.Contains(got, "an ask") {
			t.Fatalf("ask missing from prompt")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the read face read stdin and blocked")
	}
}

// ────────────────────────────── the log ─────────────────────────────────────

func TestLogAppendAndReplay(t *testing.T) {
	h := home(t)
	if _, err := ensureSecret(h); err != nil {
		t.Fatal(err)
	}
	evs := []Event{newEvent("a.one", nil), newEvent("a.two", nil)}
	if err := appendEvents(h, evs); err != nil {
		t.Fatal(err)
	}
	if evs[0].Seq != 1 || evs[1].Seq != 2 {
		t.Fatalf("seqs not assigned in the batch: %d %d", evs[0].Seq, evs[1].Seq)
	}
	got, err := readEvents(h)
	if err != nil || len(got) != 2 || got[1].Name != "a.two" {
		t.Fatalf("read back %v %v", got, err)
	}
}

func TestEventNamesMustBeDotted(t *testing.T) {
	h := home(t)
	ensureSecret(h)
	for _, bad := range []string{"Note", "note", "note..x", "note.X", "9note.x", ""} {
		if err := appendEvents(h, []Event{newEvent(bad, nil)}); err == nil {
			t.Fatalf("accepted the event name %q", bad)
		}
	}
	if err := appendEvents(h, []Event{newEvent("note.added", nil)}); err != nil {
		t.Fatalf("rejected a valid name: %v", err)
	}
}

// A payload key that carries null is the same as no payload: normalized, so no
// view has to defend against None. And raw bytes never reach the log, because a
// RawMessage is written through verbatim and would break every view after it.
func TestPayloadIsAlwaysValidJSONObject(t *testing.T) {
	h := home(t)
	heard(t, h, `{"name":"null.one","payload":null}`+"\n")
	events, _ := readEvents(h)
	if len(events) != 1 || string(events[0].Payload) != "{}" {
		t.Fatalf("a null payload was not normalized: %s", events[0].Payload)
	}
	ensureSecret(h)
	bad := newEvent("bad.one", json.RawMessage("{\"t\":\"\xff\xfe\"}"))
	if err := appendEvents(h, []Event{bad}); err == nil {
		t.Fatal("invalid UTF-8 reached the log")
	}
	if events2, err := readEvents(h); err != nil || len(events2) != 1 {
		t.Fatalf("the log did not survive the refusal: %d %v", len(events2), err)
	}
}

// Duplicate sequence numbers are the one thing replay cannot recover from, so
// the next batch continues past the HIGHEST sequence in the log's tail, not the
// last one written. The kernel never writes them out of order; a hand-appended
// line can.
func TestNextSeqClearsTheHighestNotTheLast(t *testing.T) {
	h := home(t)
	ensureSecret(h)
	heard(t, h, line(t, "one.happened", map[string]string{})+
		line(t, "two.happened", map[string]string{})+
		line(t, "three.happened", map[string]string{}))

	// Someone hand-appends an out-of-order record.
	f, _ := os.OpenFile(logPath(h), os.O_WRONLY|os.O_APPEND, 0644)
	f.WriteString(`{"id":"x","seq":1,"name":"out.of.order","occurred_at":"2026-01-01T00:00:00Z","payload":{}}` + "\n")
	f.Close()

	heard(t, h, line(t, "next.one", map[string]string{}))
	events, err := readEvents(h)
	if err != nil {
		t.Fatal(err)
	}
	// The hand-appended line already collides with seq 1; that damage is the
	// editor's. What must hold is that the kernel's next append clears
	// everything already in the log rather than continuing from the last line.
	highestBefore, next := 0, 0
	for _, e := range events {
		if e.Name == "next.one" {
			next = e.Seq
			continue
		}
		if e.Seq > highestBefore {
			highestBefore = e.Seq
		}
	}
	if next <= highestBefore {
		t.Fatalf("the append took seq %d, at or below the %d already in the log", next, highestBefore)
	}
}

func TestConcurrentAppendsDoNotCollide(t *testing.T) {
	h := home(t)
	ensureSecret(h)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			appendEvents(h, []Event{newEvent("race.one", json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)))})
		}(i)
	}
	wg.Wait()
	events, err := readEvents(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 20 {
		t.Fatalf("want 20 events, got %d", len(events))
	}
	seen := map[int]bool{}
	for _, e := range events {
		if seen[e.Seq] {
			t.Fatalf("duplicate seq %d", e.Seq)
		}
		seen[e.Seq] = true
	}
}

// A crash, a full disk, or a kill mid-write leaves a line with no newline. That
// is not a record, so it must not brick the instance — which it did: every read
// and every write failed permanently, rehydrate included.
func TestTornTailNeverBricksTheInstance(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	before, err := readEvents(h)
	if err != nil {
		t.Fatal(err)
	}

	f, _ := os.OpenFile(logPath(h), os.O_WRONLY|os.O_APPEND, 0644)
	f.WriteString(`{"name":"torn.wri`)
	f.Close()

	// Reads still work, and see exactly the committed records.
	after, err := readEvents(h)
	if err != nil {
		t.Fatalf("a torn tail broke every read: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("the torn tail was counted as a record: %d vs %d", len(after), len(before))
	}
	// The next append lands cleanly, and the log stays wholly readable — the
	// committed record above the tear must survive.
	heard(t, h, line(t, "after.crash", map[string]string{}))
	final, err := readEvents(h)
	if err != nil {
		t.Fatalf("the log did not survive the append after a tear: %v", err)
	}
	if len(final) != len(before)+1 {
		t.Fatalf("want %d records, got %d", len(before)+1, len(final))
	}
	seen := map[int]bool{}
	for _, e := range final {
		if seen[e.Seq] {
			t.Fatalf("duplicate seq %d after recovering from a tear", e.Seq)
		}
		seen[e.Seq] = true
	}
	if err := rehydrate(h); err != nil {
		t.Fatalf("rehydrate — the documented repair path — failed after a tear: %v", err)
	}
}

// An unterminated final line that PARSES is a whole record whose terminator went
// missing — which is what an editor that strips trailing newlines leaves behind.
// Dropping it destroys a committed event, and it did.
func TestCompleteFinalLineWithoutNewlineIsARecord(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "one.happened", map[string]string{"t": "first"}))

	f, _ := os.OpenFile(logPath(h), os.O_WRONLY|os.O_APPEND, 0644)
	f.WriteString(`{"id":"x","seq":2,"name":"two.happened","occurred_at":"2026-01-01T00:00:00Z","payload":{"t":"second"}}`)
	f.Close()

	events, err := readEvents(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Name != "two.happened" {
		t.Fatalf("a complete final event without a newline was dropped: %d events", len(events))
	}
	// The next append must not overwrite it, glue onto it, or reuse its seq.
	heard(t, h, line(t, "three.happened", map[string]string{}))
	events, err = readEvents(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("want 3 records after the append, got %d", len(events))
	}
	if events[1].Name != "two.happened" {
		t.Fatal("the committed record was destroyed by the append")
	}
	if events[2].Seq != 3 {
		t.Fatalf("the append reused a sequence number: %d", events[2].Seq)
	}
}

// Real corruption in the middle is not the same thing and must not be silently
// skipped: that would change the instance's state without saying so.
func TestCorruptMiddleLineIsAnErrorThatNamesIt(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	raw, _ := os.ReadFile(logPath(h))
	lines := strings.SplitAfter(string(raw), "\n")
	lines[1] = "{ this is not json }\n"
	os.WriteFile(logPath(h), []byte(strings.Join(lines, "")), 0644)
	_, err := readEvents(h)
	if err == nil {
		t.Fatal("a corrupt middle line was silently skipped")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("the error does not name the line: %v", err)
	}
}

func TestLastSeqScansOnlyTheTail(t *testing.T) {
	h := home(t)
	ensureSecret(h)
	var evs []Event
	for i := 0; i < 500; i++ {
		evs = append(evs, newEvent("bulk.one", json.RawMessage(`{"pad":"`+strings.Repeat("x", 400)+`"}`)))
	}
	if err := appendEvents(h, evs); err != nil {
		t.Fatal(err)
	}
	n, err := lastSeq(h)
	if err != nil || n != 500 {
		t.Fatalf("lastSeq = %d, %v", n, err)
	}
}

// ──────────────────────────── the strange loop ──────────────────────────────

// The whole thesis in one test: a declaration becomes pending work, a mind
// authors it through the wire, the kernel signs and installs it, and the new
// capability is immediately usable — with nothing left pending.
func TestStrangeLoop(t *testing.T) {
	h := home(t)
	st := replayed(t, h)
	if !st.capabilitiesReady() {
		t.Fatal("a fresh instance is not capability-ready")
	}

	heard(t, h, line(t, "command.declared", decl{Name: "entry", Description: "append an entry"}))
	st = replayed(t, h)
	if len(st.pending()) != 1 || st.capabilitiesReady() {
		t.Fatalf("a declaration did not become pending work: %d pending", len(st.pending()))
	}
	// The pending ask must carry the declaration, so a cold mind can act on it.
	if p := situated(t, h, ""); !strings.Contains(p, `command "entry"`) {
		t.Fatalf("pending work missing from the prompt:\n%s", p)
	}

	heard(t, h, line(t, "script.authored", authored{Type: "command", Name: "entry",
		Script: "#!/bin/sh\ncat >/dev/null\nprintf '{\"name\":\"journal.entry\",\"payload\":{\"text\":\"%s\"}}\\n' \"$*\"\n"}))
	st = replayed(t, h)
	if len(st.pending()) != 0 || !st.capabilitiesReady() {
		t.Fatal("authoring did not converge the loop")
	}

	evs, err := runCommand(h, st, "entry", []string{"watered", "the", "plants"}, doorCLI, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Name != "journal.entry" {
		t.Fatalf("the installed command emitted %v", evs)
	}
	if evs[0].Via != doorCLI || evs[0].By != "tester" {
		t.Fatalf("provenance not stamped: via=%q by=%q", evs[0].Via, evs[0].By)
	}
	if !strings.Contains(string(evs[0].Payload), "watered the plants") {
		t.Fatalf("payload lost the argv: %s", evs[0].Payload)
	}
}

// The loop's sharpest form: a capability that declares capabilities. A command's
// stdout is event JSONL and a declaration is an event, so the INSTANCE can ask
// for its own growth — and the kernel deliberately does not distinguish that
// from growth a mind asked for.
func TestACapabilityCanDeclareCapabilities(t *testing.T) {
	h := home(t)
	body := line(t, "command.declared", decl{Name: "propose", Description: "declare a view"}) +
		line(t, "script.authored", authored{Type: "command", Name: "propose",
			Script: "#!/bin/sh\ncat >/dev/null\nprintf '{\"name\":\"view.declared\",\"payload\":{\"name\":\"%s\",\"description\":\"proposed from inside\",\"consumes\":[\"*\"]}}\\n' \"$1\"\n"})
	heard(t, h, body)
	st := replayed(t, h)
	if !st.capabilitiesReady() {
		t.Fatal("the instance should be capability-ready before it proposes anything")
	}

	if _, err := runCommand(h, st, "propose", []string{"census"}, doorCLI, ""); err != nil {
		t.Fatal(err)
	}
	st = replayed(t, h)
	pending := st.pending()
	if len(pending) != 1 || pending[0].key() != "view/census" {
		t.Fatalf("the instance's own declaration is not pending work: %v", pending)
	}
	if st.capabilitiesReady() {
		t.Fatal("work the instance asked for did not wake the loop")
	}
	// It rides the prompt like any other pending declaration.
	if p := situated(t, h, ""); !strings.Contains(p, `view "census"`) {
		t.Fatal("the instance's own declaration is missing from the prompt")
	}
	// And it closes the same way.
	heard(t, h, line(t, "script.authored", authored{Type: "view", Name: "census",
		Script: "#!/bin/sh\nwc -l\n"}))
	if !replayed(t, h).capabilitiesReady() {
		t.Fatal("authoring the instance's own proposal did not converge the loop")
	}
	page, err := runView(h, replayed(t, h), "census")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(page)) == "" {
		t.Fatal("the proposed view rendered nothing")
	}
}

// A re-declaration is what revision looks like on an append-only log: pending
// again, while the older script keeps running until it is re-authored.
func TestRedeclarationReopensPendingWork(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	heard(t, h, line(t, "command.declared", decl{Name: "entry", Description: "now with a mood"}))
	st := replayed(t, h)
	if len(st.pending()) != 1 {
		t.Fatal("re-declaring did not reopen pending work")
	}
	if _, err := runCommand(h, st, "entry", []string{"still", "works"}, doorCLI, ""); err != nil {
		t.Fatalf("the older script stopped running while pending: %v", err)
	}
}

// ───────────────────────────── the trust gate ───────────────────────────────

// A mind can only propose. Nothing installs without a declaration in this log.
func TestUndeclaredScriptIsRefused(t *testing.T) {
	h := home(t)
	report := heard(t, h, line(t, "script.authored", authored{Type: "command", Name: "sneak", Script: "#!/bin/sh\necho hi\n"}))
	if !strings.Contains(report, "REFUSED") {
		t.Fatalf("an undeclared script was not refused: %s", report)
	}
	if _, err := os.Stat(linkPath(h, kindCommand, "sneak")); err == nil {
		t.Fatal("a refused script reached the filesystem")
	}
	// The refusal is memory, not a terminal incident: it stands in the brief.
	st := replayed(t, h)
	if len(st.Reject) != 1 || !strings.Contains(st.Reject[0].Reason, "not declared") {
		t.Fatalf("the refusal was not recorded: %+v", st.Reject)
	}
	if st.capabilitiesReady() {
		t.Fatal("a standing refusal must keep the loop awake")
	}
}

// A receipt that does not verify under this instance's key is inert data, no
// matter who wrote it. This is the whole safety story of the account protocol.
func TestForgedReceiptIsInert(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	forged := receipt{Type: kindCommand, Name: "entry", Script: "#!/bin/sh\necho pwned\n", Sig: strings.Repeat("0", 64)}
	payload, _ := json.Marshal(forged)
	heard(t, h, string(mustLine("script.installed", payload)))

	st := replayed(t, h)
	if strings.Contains(st.cap(kindCommand, "entry").Receipt.Script, "pwned") {
		t.Fatal("a forged receipt was trusted")
	}
	bin, err := materialize(h, st, kindCommand, "entry")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(bin); strings.Contains(string(b), "pwned") {
		t.Fatal("a forged script was materialized")
	}
}

// The receipt preimage must be injective. consumes was joined into one
// length-prefixed field, so the PARTITION of the list went unsigned: a
// two-element list and a single element holding the same bytes with a separator
// inside hashed identically — and that single element matches no event name, so
// the view is fed nothing under a signature that still verifies.
func TestSignatureCoversTheConsumesPartition(t *testing.T) {
	key := []byte("a key for signing")
	sep := string([]byte{0})
	two := receipt{Type: kindView, Name: "v", Script: "#!/bin/sh\ntrue\n", Consumes: []string{"a.one", "c.two"}}
	one := two
	one.Consumes = []string{"a.one" + sep + "c.two"}
	if sign(key, two) == sign(key, one) {
		t.Fatal("two different consumes lists share a signature")
	}
	empty := two
	empty.Consumes = nil
	blank := two
	blank.Consumes = []string{""}
	if sign(key, empty) == sign(key, blank) {
		t.Fatal("no consumes and one empty element share a signature — one means the whole log")
	}
	// And nothing else about a receipt may collide either.
	base := receipt{Type: kindCommand, Name: "ab", Script: "s", By: "x"}
	shifted := receipt{Type: kindCommand, Name: "a", Script: "bs", By: "x"}
	if sign(key, base) == sign(key, shifted) {
		t.Fatal("a field boundary is not covered")
	}
}

// Editing an installed script has no effect: execution resolves the receipt,
// checks the blob against its own hash, and restores it.
func TestTamperedScriptIsHealed(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	st := replayed(t, h)
	bin, err := materialize(h, st, kindCommand, "entry")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho pwned\n"), 0755); err != nil {
		t.Fatal(err)
	}
	evs, err := runCommand(h, st, "entry", []string{"hello"}, doorCLI, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Name != "journal.entry" {
		t.Fatalf("a hand-edited script ran instead of the signed one: %v", evs)
	}
}

// A refusal names which of the three ways a capability can have no script,
// because "not found" sends a reader looking in the wrong place.
func TestMaterializeFailsClosedAndSaysWhy(t *testing.T) {
	h := home(t)
	st := replayed(t, h)
	if _, err := materialize(h, st, kindCommand, "nope"); err == nil || !strings.Contains(err.Error(), "no command") {
		t.Fatalf("undeclared: %v", err)
	}
	heard(t, h, line(t, "command.declared", decl{Name: "later", Description: "x"}))
	st = replayed(t, h)
	if _, err := materialize(h, st, kindCommand, "later"); err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("declared but unauthored: %v", err)
	}

	growJournal(t, h)
	os.Remove(secretPath(h))
	st = replayed(t, h)
	if _, err := materialize(h, st, kindCommand, "entry"); err == nil || !strings.Contains(err.Error(), ".secret") {
		t.Fatalf("missing key: %v", err)
	}
}

// A trailing "run" segment would collide with the file a capability's own
// directory holds, so it is refused at receipt time rather than at exec time.
func TestReservedNamesAreRefused(t *testing.T) {
	h := home(t)
	for _, bad := range []string{"notes/run", "x/run/y", "run", "../escape", ".hidden", "a//b",
		strings.Repeat("n", 201), strings.Repeat("s", 65) + "/x"} {
		body := line(t, "command.declared", decl{Name: bad, Description: "x"}) +
			line(t, "script.authored", authored{Type: "command", Name: bad, Script: "#!/bin/sh\ntrue\n"})
		heard(t, h, body)
		if replayed(t, h).cap(kindCommand, bad) != nil {
			t.Fatalf("the name %q was accepted", bad)
		}
	}
}

// ───────────────────────────────── the wire ─────────────────────────────────

// A line needs BOTH a dotted name and a payload key. On the name test alone, a
// mind reporting {"name":"notes","status":"ok"} would land an event in the log.
func TestWireDiscriminator(t *testing.T) {
	evs, _, prose, err := wire(`{"name":"notes","status":"installed"}` + "\n" +
		`{"name":"Note.Added","payload":{}}` + "\n" +
		`{"text":"no name here"}` + "\n" +
		`{"name":"note.added","payload":{"text":"real"}}` + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Name != "note.added" {
		t.Fatalf("wire took %d events: %v", len(evs), evs)
	}
	if len(prose) != 3 {
		t.Fatalf("want 3 lines ignored, got %d", len(prose))
	}
}

// The lesson of driving the real loop: a chat-trained mind opens with a line of
// preamble and then emits perfect events. Strictness threw the events away.
func TestPreambleDoesNotCostThePass(t *testing.T) {
	h := home(t)
	body := "All good — the JSON encodes cleanly. Printing the lines now.\n" +
		"```json\n" +
		line(t, "command.declared", decl{Name: "entry", Description: "append an entry"}) +
		"```\n" +
		"That should do it.\n"
	report := heard(t, h, body)
	if !strings.Contains(report, "heard 1 event") {
		t.Fatalf("a preamble cost the pass: %s", report)
	}
	if replayed(t, h).cap(kindCommand, "entry") == nil {
		t.Fatal("the declaration did not land")
	}
	// Only the report reaches stdout: a driver script parses it, so chatter
	// must not be interleaved with it. The ignored lines go to stderr, named
	// and counted, which is the whole price of choosing leniency.
	if strings.Contains(report, "That should do it.") {
		t.Fatalf("ignored prose was interleaved with the report on stdout:\n%s", report)
	}
}

// Order within one body must not matter: hear appends everything first, then
// resolves declared-ness, then installs. A mind that prints its script before
// its declaration is not wrong, just unordered.
func TestScriptBeforeItsDeclarationInOneBody(t *testing.T) {
	h := home(t)
	body := line(t, "script.authored", authored{Type: "view", Name: "n", Script: "#!/bin/sh\ncat >/dev/null\necho ok\n"}) +
		line(t, "view.declared", decl{Name: "n", Description: "x", Consumes: []string{"a.one"}})
	report := heard(t, h, body)
	if !strings.Contains(report, "installed view/n") {
		t.Fatalf("order within one body mattered: %s", report)
	}
	c := replayed(t, h).cap(kindView, "n")
	if c.Receipt == nil || len(c.Receipt.Consumes) != 1 || c.Receipt.Consumes[0] != "a.one" {
		t.Fatalf("the receipt did not pick up the declaration's consumes: %+v", c.Receipt)
	}
}

// An over-long line used to discard itself and every line after it, silently,
// and still report success.
func TestOverlongLineIsAnErrorNotSilentTruncation(t *testing.T) {
	h := home(t)
	body := line(t, "first.one", map[string]string{"t": "kept?"}) +
		`{"name":"huge.one","payload":{"t":"` + strings.Repeat("x", lineLimit+16) + `"}}` + "\n" +
		line(t, "third.one", map[string]string{"t": "kept?"})
	var out bytes.Buffer
	if err := cmdHear(h, []byte(body), &out); err == nil {
		t.Fatal("an over-long line was silently swallowed")
	}
	if events, _ := readEvents(h); len(events) != 0 {
		t.Fatalf("a body that could not be read whole appended %d event(s)", len(events))
	}
}

// A body with no events writes nothing and passes through unchanged: a
// tool-capable mind does its work with its own tools and then says what it did.
func TestProseBodyWritesNothingAndPassesThrough(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	before, _ := readEvents(h)
	answer := "I logged two entries and confirmed the view renders them.\n"
	var out bytes.Buffer
	if err := cmdHear(h, []byte(answer), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != answer {
		t.Fatalf("prose was not passed through verbatim: %q", out.String())
	}
	after, _ := readEvents(h)
	if len(after) != len(before) {
		t.Fatalf("a prose body appended %d event(s)", len(after)-len(before))
	}
}

// ─────────────────────────── views are pure replays ─────────────────────────

func TestViewSeesOnlyWhatItConsumes(t *testing.T) {
	h := home(t)
	body := line(t, "view.declared", decl{Name: "narrow", Description: "only mine", Consumes: []string{"mine.one"}}) +
		line(t, "script.authored", authored{Type: "view", Name: "narrow", Script: "#!/bin/sh\nwc -l\n"})
	heard(t, h, body)
	heard(t, h, line(t, "mine.one", map[string]string{"a": "1"})+
		line(t, "other.one", map[string]string{"a": "2"})+
		line(t, "mine.one", map[string]string{"a": "3"}))

	page, err := runView(h, replayed(t, h), "narrow")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.TrimSpace(string(page)); n != "2" {
		t.Fatalf("the view was fed %s lines, want 2", n)
	}
}

func TestViewReceivesArgumentsWithoutAppending(t *testing.T) {
	h := home(t)
	body := line(t, "view.declared", decl{Name: "lookup", Description: "look up one key", Consumes: []string{"note.added"}}) +
		line(t, "script.authored", authored{Type: "view", Name: "lookup", Script: "#!/bin/sh\nprintf '%s\\n' \"$1\"\n"})
	heard(t, h, body)
	before := len(replayed(t, h).Events)

	var out bytes.Buffer
	if err := dispatch(h, "view", []string{"lookup", "chosen-key"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "chosen-key\n" {
		t.Fatalf("view did not receive argv: %q", out.String())
	}
	if after := len(replayed(t, h).Events); after != before {
		t.Fatalf("reading a parameterized view appended: before=%d after=%d", before, after)
	}
}

// A view is fed the list its RECEIPT names, not its latest declaration.
// Re-declaring a view with a wider consumes list leaves it pending, and until
// it is re-authored the old script must keep seeing the stream it was signed
// against — otherwise `self view` renders bytes no signed unit corresponds to.
func TestViewInputFollowsTheReceiptNotTheDeclaration(t *testing.T) {
	h := home(t)
	body := line(t, "view.declared", decl{Name: "narrow", Description: "x", Consumes: []string{"mine.one"}}) +
		line(t, "script.authored", authored{Type: "view", Name: "narrow", Script: "#!/bin/sh\nwc -l\n"})
	heard(t, h, body)
	heard(t, h, line(t, "mine.one", map[string]string{"a": "1"})+line(t, "other.one", map[string]string{"a": "2"}))
	heard(t, h, line(t, "view.declared", decl{Name: "narrow", Description: "x", Consumes: []string{"mine.one", "other.one"}}))

	page, err := runView(h, replayed(t, h), "narrow")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.TrimSpace(string(page)); n != "1" {
		t.Fatalf("the pending view saw %s events; its receipt names one", n)
	}
}

// Same log, same key, same bytes — twice, and in a second body.
func TestViewsAreDeterministic(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	st := replayed(t, h)
	runCommand(h, st, "entry", []string{"one"}, doorCLI, "")
	runCommand(h, replayed(t, h), "entry", []string{"two"}, doorCLI, "")

	first, err := runView(h, replayed(t, h), "journal")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := runView(h, replayed(t, h), "journal")
	if !bytes.Equal(first, second) {
		t.Fatal("two replays of one log produced different bytes")
	}

	mirror := t.TempDir()
	for _, f := range []string{"events.jsonl", ".secret"} {
		data, err := os.ReadFile(filepath.Join(h, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(mirror, f), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := rehydrate(mirror); err != nil {
		t.Fatal(err)
	}
	third, err := runView(mirror, replayed(t, mirror), "journal")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, third) {
		t.Fatalf("a rebuild from the log alone rendered different bytes:\n%s\n---\n%s", first, third)
	}
	if len(bytes.TrimSpace(first)) == 0 {
		t.Fatal("the view rendered nothing, so this proved nothing")
	}
}

// A view must not be able to see the caller's environment: determinism is the
// claim, and $TZ or $LC_ALL would break it silently. Nor is a view given any
// path to the instance — its whole input is stdin, so SELF_HOME and the
// instance as a working directory were the log handed over for no reason.
func TestViewGetsNoPathToTheInstance(t *testing.T) {
	h := home(t)
	t.Setenv("SNEAKY", "leaked")
	t.Setenv("SELF_PASSED", "on purpose")
	body := line(t, "view.declared", decl{Name: "env", Description: "x", Consumes: []string{"*"}}) +
		line(t, "script.authored", authored{Type: "view", Name: "env",
			Script: "#!/bin/sh\ncat >/dev/null\nenv | sort\necho \"cwd=$(pwd)\"\nls events.jsonl 2>&1 | sed 's/^/ls: /'\n"})
	heard(t, h, body)
	page, err := runView(h, replayed(t, h), "env")
	if err != nil {
		t.Fatal(err)
	}
	got := string(page)
	if strings.Contains(got, "SNEAKY") {
		t.Fatalf("the caller's environment leaked into a view:\n%s", got)
	}
	if strings.Contains(got, "SELF_HOME=") {
		t.Fatalf("a view was handed a path to the instance:\n%s", got)
	}
	if strings.Contains(got, "cwd="+h) {
		t.Fatalf("a view ran inside the instance:\n%s", got)
	}
	if !strings.Contains(got, "ls: ") || !strings.Contains(got, "events.jsonl") {
		t.Fatalf("the probe did not run:\n%s", got)
	}
	if !strings.Contains(got, "No such file") && !strings.Contains(got, "cannot access") {
		t.Fatalf("a view could reach the log from its working directory:\n%s", got)
	}
	for _, want := range []string{"TZ=UTC", "LC_ALL=C", "PYTHONHASHSEED=0", "SELF_PASSED=on purpose"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q from the script environment:\n%s", want, got)
		}
	}
}

// A command, unlike a view, is an effect on this instance and is told which one.
func TestCommandGetsTheInstanceAndNothingElse(t *testing.T) {
	h := home(t)
	t.Setenv("SNEAKY", "leaked")
	body := line(t, "command.declared", decl{Name: "env", Description: "x"}) +
		line(t, "script.authored", authored{Type: "command", Name: "env",
			Script: "#!/bin/sh\ncat >/dev/null\nprintf '{\"name\":\"env.seen\",\"payload\":{\"home\":\"%s\",\"cwd\":\"%s\",\"sneaky\":\"%s\"}}\\n' \"$SELF_HOME\" \"$(pwd)\" \"${SNEAKY:-}\"\n"})
	heard(t, h, body)
	evs, err := runCommand(h, replayed(t, h), "env", nil, doorCLI, "")
	if err != nil {
		t.Fatal(err)
	}
	var p struct{ Home, Cwd, Sneaky string }
	json.Unmarshal(evs[0].Payload, &p)
	if p.Home != h || p.Cwd != h {
		t.Fatalf("a command was not told its instance: home=%q cwd=%q want %q", p.Home, p.Cwd, h)
	}
	if p.Sneaky != "" {
		t.Fatalf("the caller's environment leaked into a command: %q", p.Sneaky)
	}
}

// ─────────────────────────────── retirement ─────────────────────────────────

func TestRetirementLeavesTheSurfaceAndTheLogKeepsEverything(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	before, _ := readEvents(h)

	heard(t, h, line(t, "capability.retired", map[string]string{"type": "command", "name": "entry"}))
	if _, err := os.Lstat(linkPath(h, kindCommand, "entry")); err == nil {
		t.Fatal("a retired capability is still on the surface")
	}
	st := replayed(t, h)
	if st.cap(kindCommand, "entry") != nil {
		t.Fatal("a retired capability is still listed")
	}
	if _, err := runCommand(h, st, "entry", nil, doorCLI, ""); err == nil {
		t.Fatal("a retired capability still runs")
	}
	after, _ := readEvents(h)
	if len(after) <= len(before) {
		t.Fatal("retirement did not append a tombstone")
	}

	// Re-declaring brings the capability back as PENDING work — the retired
	// script does not silently return. The log kept every event, so nothing was
	// destroyed; the mind authors it fresh.
	heard(t, h, line(t, "command.declared", decl{Name: "entry", Description: "again"}))
	st = replayed(t, h)
	c := st.cap(kindCommand, "entry")
	if c == nil {
		t.Fatal("re-declaring did not bring the capability back")
	}
	if !c.Pending() || c.Receipt != nil {
		t.Fatal("a retired script came back without being re-authored")
	}
	if _, err := runCommand(h, st, "entry", nil, doorCLI, ""); err == nil {
		t.Fatal("a retired script ran again after a bare re-declaration")
	}
}

// One body can retire a capability and declare it again. Replay is the only
// thing that knows which came last, so unlinking on the tombstone alone would
// delete something that is live.
func TestRetireThenRedeclareInOneBody(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	body := line(t, "capability.retired", map[string]string{"type": "command", "name": "entry"}) +
		line(t, "command.declared", decl{Name: "entry", Description: "back again"}) +
		line(t, "script.authored", authored{Type: "command", Name: "entry",
			Script: "#!/bin/sh\ncat >/dev/null\nprintf '{\"name\":\"journal.entry\",\"payload\":{\"text\":\"%s\"}}\\n' \"$*\"\n"})
	heard(t, h, body)
	st := replayed(t, h)
	if st.cap(kindCommand, "entry") == nil {
		t.Fatal("the re-declaration did not survive the tombstone")
	}
	if _, err := runCommand(h, st, "entry", []string{"alive"}, doorCLI, ""); err != nil {
		t.Fatalf("a revived capability was unlinked: %v", err)
	}
}

// script.authored is a wire message. No door may land it in the log — including
// a command capability that emits one, which would leave a lie there.
func TestScriptAuthoredNeverLandsInTheLog(t *testing.T) {
	h := home(t)
	ensureSecret(h)
	if err := appendEvents(h, []Event{newEvent("script.authored", json.RawMessage(`{"type":"command"}`))}); err == nil {
		t.Fatal("script.authored was appended")
	}
	if events, _ := readEvents(h); len(events) != 0 {
		t.Fatal("it landed anyway")
	}
}

// A receipt is the kernel's own testimony, so it counts only through the
// kernel's door. Replaying a genuine receipt payload through the wire used to
// re-install it — undoing a retirement, or rolling a fixed script back — with no
// key and no declaration, because a real signature stays valid forever.
func TestReplayedReceiptCannotUndoARetirement(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	var genuine json.RawMessage
	for _, e := range replayed(t, h).Events {
		if e.Name == "script.installed" {
			genuine = e.Payload
		}
	}
	if genuine == nil {
		t.Fatal("no receipt to replay")
	}
	heard(t, h, line(t, "capability.retired", map[string]string{"type": "command", "name": "entry"}))
	heard(t, h, string(mustLine("script.installed", genuine)))

	st := replayed(t, h)
	if st.cap(kindCommand, "entry") != nil {
		t.Fatal("a replayed receipt resurrected a retired capability")
	}
	if _, err := runCommand(h, st, "entry", nil, doorCLI, ""); err == nil {
		t.Fatal("the resurrected capability ran")
	}
}

// A refusal that names nothing a declaration or retirement could ever match had
// no way to close, so the instance never reported capability-ready again and the documented
// loop spun forever.
func TestUnkeyedRefusalDoesNotWedgeTheLoop(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "script.authored", authored{Type: "banana", Name: "x", Script: "#!/bin/sh\ntrue\n"}))
	if replayed(t, h).capabilitiesReady() {
		t.Fatal("a refusal did not wake the loop")
	}
	growJournal(t, h)
	if !replayed(t, h).capabilitiesReady() {
		t.Fatal("a bogus refusal wedged the loop permanently")
	}
}

// Invalid UTF-8 must not reach the log through ANY door, including the wire.
func TestWireCannotSmuggleRawBytes(t *testing.T) {
	h := home(t)
	var out bytes.Buffer
	body := "{\"name\":\"bad.one\",\"payload\":{\"t\":\"\xff\xfe\"}}\n"
	if err := cmdHear(h, []byte(body), &out); err == nil {
		t.Fatal("the wire smuggled invalid UTF-8 into the log")
	}
	if events, err := readEvents(h); err != nil || len(events) != 0 {
		t.Fatalf("the log took it anyway: %d %v", len(events), err)
	}
}

// rehydrate is the only thing allowed to delete, and it makes disk match the
// log exactly — including collecting the blobs nothing references.
func TestRehydrateReconciles(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	heard(t, h, line(t, "capability.retired", map[string]string{"type": "view", "name": "journal"}))

	junkLink := linkPath(h, kindCommand, "ghost")
	os.MkdirAll(filepath.Dir(junkLink), 0755)
	os.WriteFile(junkLink, []byte("#!/bin/sh\ntrue\n"), 0755)
	junkBlob := blobPath(h, strings.Repeat("a", 64))
	os.MkdirAll(filepath.Dir(junkBlob), 0755)
	os.WriteFile(junkBlob, []byte("orphan"), 0755)

	if err := rehydrate(h); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(junkLink); err == nil {
		t.Fatal("rehydrate kept a capability the log does not declare")
	}
	if _, err := os.Stat(junkBlob); err == nil {
		t.Fatal("rehydrate kept an unreferenced blob")
	}
	if _, err := os.Lstat(linkPath(h, kindCommand, "entry")); err != nil {
		t.Fatal("rehydrate did not materialize a live capability")
	}
	if _, err := os.Lstat(linkPath(h, kindView, "journal")); err == nil {
		t.Fatal("rehydrate materialized a retired capability")
	}
}

// A home with a log but no key has no capabilities. That is the truth about it,
// and it must not be reported as "not found".
func TestNoKeyMeansNoCapabilities(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	os.Remove(secretPath(h))
	st := replayed(t, h)
	if st.Key != nil {
		t.Fatal("a key was read that does not exist")
	}
	if c := st.cap(kindCommand, "entry"); c == nil || c.Receipt != nil {
		t.Fatal("a receipt verified without a key")
	}
	if !strings.Contains(brief(h, st), "no .secret") {
		t.Fatal("the brief hides a missing key")
	}
}

// ──────────────────────────── the account protocol ──────────────────────────

// The round trip: give writes plain text, learn deposits it verbatim, and the
// receiving instance grows its OWN expression of the intent.
func TestGiveLearnRoundTrip(t *testing.T) {
	giver := home(t)
	growJournal(t, giver)
	st := replayed(t, giver)
	runCommand(giver, st, "entry", []string{"the first thing"}, doorCLI, "wouter")
	runCommand(giver, replayed(t, giver), "entry", []string{"the second"}, doorCLI, "wouter")

	dir := filepath.Join(t.TempDir(), "account")
	if err := cmdGive(giver, "journal.", dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"intent.md", "record.jsonl", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("give did not write %s", f)
		}
	}
	var m manifest
	raw, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	json.Unmarshal(raw, &m)
	if m.Events != 2 || m.RecordSha256 == "" {
		t.Fatalf("manifest: %+v", m)
	}
	// Giving is remembered.
	if !hasEvent(t, giver, "account.given") {
		t.Fatal("the giver does not remember giving")
	}

	receiver := home(t)
	var out bytes.Buffer
	if err := cmdLearn(receiver, dir, &out); err != nil {
		t.Fatal(err)
	}
	// The mechanical half: intent first, deposits, attestation last.
	events, _ := readEvents(receiver)
	if len(events) != 4 {
		t.Fatalf("want intent + 2 deposits + attestation, got %d", len(events))
	}
	if events[0].Name != "intent.declared" || events[3].Name != "lesson.learned" {
		t.Fatalf("order: %s … %s", events[0].Name, events[3].Name)
	}
	// Moments and speakers travel; the door is re-stamped.
	original, _ := readEvents(giver)
	var first Event
	for _, e := range original {
		if e.Name == "journal.entry" {
			first = e
			break
		}
	}
	if !events[1].OccurredAt.Equal(first.OccurredAt) {
		t.Fatal("a deposited event lost its own moment")
	}
	if events[1].By != "wouter" {
		t.Fatalf("a deposited event lost its speaker: %q", events[1].By)
	}
	if events[1].Via != doorLearn+"account" {
		t.Fatalf("the door was not re-stamped: %q", events[1].Via)
	}
	// The intelligent half rides the pipe.
	prompt := out.String()
	if !strings.Contains(prompt, "--- INTENT") || !strings.Contains(prompt, "--- END INTENT ---") {
		t.Fatal("learn did not print the learning prompt")
	}
	// The intent is another instance's prose riding a prompt a mind will act on,
	// so every line of it is quoted: it must not be able to close the block or
	// forge a section of the prompt's own structure.
	body := prompt[strings.Index(prompt, "--- INTENT"):]
	for _, l := range strings.Split(body, "\n")[1:] {
		if strings.HasPrefix(l, "--- END INTENT") {
			break
		}
		if l != "" && !strings.HasPrefix(l, "| ") {
			t.Fatalf("an intent line rode the prompt unquoted: %q", l)
		}
	}
	// Nothing installed: an account cannot install anything, ever.
	if len(replayed(t, receiver).Caps) != 0 {
		t.Fatal("learning an account installed a capability")
	}
}

// The frozen refused set is the SOLE gate between a foreign account and the
// strange loop. Without it a deposited command.declared becomes pending work
// and the next pass signs an attacker's script under the local key.
func TestLearnRefusesKernelVocabularyWholesale(t *testing.T) {
	for _, name := range []string{"command.declared", "view.declared", "script.installed",
		"script.compiled", "projector.declared", "self.asked", "kernel.initialized"} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "intent.md"), []byte("# hostile"), 0644)
		body, _ := json.Marshal(map[string]any{"name": name, "payload": map[string]string{"name": "pwn"}})
		os.WriteFile(filepath.Join(dir, "record.jsonl"),
			append([]byte(`{"name":"innocent.one","payload":{}}`+"\n"), append(body, '\n')...), 0644)

		h := home(t)
		err := cmdLearn(h, dir, &bytes.Buffer{})
		if err == nil {
			t.Fatalf("%s was accepted in a record", name)
		}
		events, _ := readEvents(h)
		if len(events) != 0 {
			t.Fatalf("%s: a refused account deposited %d event(s) — it must be all or nothing", name, len(events))
		}
	}
}

// The protocol lists the refused set. If the code and the list disagree, an
// account author learns the difference by hitting an error, which is exactly the
// friction one source of truth exists to prevent.
func TestProtocolListsEveryRefusedName(t *testing.T) {
	for name := range refused {
		if !strings.Contains(protocolDoc, "`"+name+"`") {
			t.Fatalf("%q is refused in a record but appears nowhere in PROTOCOL.md", name)
		}
	}
}

// A name may leave the kernel's vocabulary; it never leaves the refused set.
func TestRefusedSetCoversRetiredNames(t *testing.T) {
	live := []string{"command.declared", "view.declared", "script.authored", "script.installed",
		"script.rejected", "capability.retired", "intent.declared", "lesson.learned", "account.given"}
	for _, n := range live {
		if !refused[n] {
			t.Fatalf("live kernel name %q is not refused in a record", n)
		}
	}
	for _, n := range []string{"projector.declared", "script.compiled", "self.asked", "self.replied",
		"self.reflected", "kernel.initialized", "learn.orchestrated", "capability.revision.requested"} {
		if !refused[n] {
			t.Fatalf("retired kernel name %q became depositable", n)
		}
	}
}

// Lineage arrives as evidence and lands inert: give renames the vocabulary, and
// a lineage.* event is just data in the receiving log.
func TestCapabilityTravelsAsInertLineage(t *testing.T) {
	giver := home(t)
	growJournal(t, giver)
	dir := filepath.Join(t.TempDir(), "cap-account")
	if err := cmdGive(giver, "command/entry", dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "record.jsonl"))
	if !strings.Contains(string(raw), "lineage.command.declared") ||
		!strings.Contains(string(raw), "lineage.script.installed") {
		t.Fatalf("the vocabulary was not renamed:\n%s", trunc(string(raw), 300))
	}

	receiver := home(t)
	if err := cmdLearn(receiver, dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	st := replayed(t, receiver)
	if len(st.Caps) != 0 {
		t.Fatal("lineage installed a capability")
	}
	if len(st.pending()) != 0 {
		t.Fatal("lineage created pending work")
	}
}

// Curation is editing a directory, and it shows in both logs forever.
func TestInterventionIsVisible(t *testing.T) {
	giver := home(t)
	growJournal(t, giver)
	runCommand(giver, replayed(t, giver), "entry", []string{"one"}, doorCLI, "")
	runCommand(giver, replayed(t, giver), "entry", []string{"two"}, doorCLI, "")
	dir := filepath.Join(t.TempDir(), "account")
	cmdGive(giver, "journal.", dir)

	// Delete a line before learning: legitimate, and visible.
	path := filepath.Join(dir, "record.jsonl")
	raw, _ := os.ReadFile(path)
	kept := strings.SplitN(strings.TrimSpace(string(raw)), "\n", 2)[0] + "\n"
	os.WriteFile(path, []byte(kept), 0644)

	receiver := home(t)
	if err := cmdLearn(receiver, dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	events, _ := readEvents(receiver)
	att := events[len(events)-1]
	if att.Name != "lesson.learned" {
		t.Fatalf("last event is %s", att.Name)
	}
	var p struct {
		RecordSha256   string `json:"record_sha256"`
		ManifestSha256 string `json:"manifest_sha256"`
		Events         int    `json:"events"`
	}
	json.Unmarshal(att.Payload, &p)
	if p.RecordSha256 == "" || p.ManifestSha256 == "" {
		t.Fatalf("the attestation carries no digests: %s", att.Payload)
	}
	if p.RecordSha256 == p.ManifestSha256 {
		t.Fatal("an edited record hashed the same as the manifest claimed")
	}
	if p.Events != 1 {
		t.Fatalf("attested %d events, one was kept", p.Events)
	}
}

// A record is parsed for the four fields a deposit keeps. A wrong type in one it
// discards anyway must not cost the whole account — and an advisory manifest
// that will not parse is worth a word on stderr, not an abort.
func TestRecordSurvivesFieldsLearnDiscards(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "intent.md"), []byte("# from a different shape of kernel"), 0644)
	os.WriteFile(filepath.Join(dir, "record.jsonl"),
		[]byte(`{"name":"note.added","seq":"three","id":42,"via":["odd"],"payload":{"t":"1"}}`+"\n"), 0644)
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{ not json at all"), 0644)
	h := home(t)
	if err := cmdLearn(h, dir, &bytes.Buffer{}); err != nil {
		t.Fatalf("a field learn discards anyway killed the account: %v", err)
	}
	if !hasEvent(t, h, "note.added") {
		t.Fatal("the record was not deposited")
	}
}

// A record that is present but unreadable is not the same as no record: treating
// it as absent would silently learn half an account.
func TestUnreadableRecordIsAnError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "intent.md"), []byte("# x"), 0644)
	os.Mkdir(filepath.Join(dir, "record.jsonl"), 0755) // there, and not readable as a file
	h := home(t)
	if err := cmdLearn(h, dir, &bytes.Buffer{}); err == nil {
		t.Fatal("an unreadable record was treated as absent")
	}
	if events, _ := readEvents(h); len(events) != 0 {
		t.Fatal("it deposited anyway")
	}
}

// A manifest is advisory: an unknown capability value must not steer learn.
func TestManifestIsAdvisory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "intent.md"), []byte("# from an older kernel"), 0644)
	os.WriteFile(filepath.Join(dir, "record.jsonl"), []byte(`{"name":"note.added","payload":{"t":"1"}}`+"\n"), 0644)
	os.WriteFile(filepath.Join(dir, "manifest.json"),
		[]byte(`{"events":1,"record_sha256":"whatever","capability":"projector/old"}`), 0644)
	h := home(t)
	if err := cmdLearn(h, dir, &bytes.Buffer{}); err != nil {
		t.Fatalf("an unknown capability value blocked a learn: %v", err)
	}
	if !hasEvent(t, h, "note.added") {
		t.Fatal("the record was not deposited")
	}
}

// An account must say what it is for, and giving must say what it gives: an
// empty selector would write every event — every installed script included —
// out to a directory.
func TestAccountEdgesAreRefused(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "intent.md"), []byte("   \n"), 0644)
	h := home(t)
	if err := cmdLearn(h, dir, &bytes.Buffer{}); err == nil {
		t.Fatal("an account with an empty intent was learned")
	}
	if events, _ := readEvents(h); len(events) != 0 {
		t.Fatal("a refused account still wrote")
	}
	growJournal(t, h)
	if err := cmdGive(h, "", t.TempDir()); err == nil {
		t.Fatal("an empty selector gave the whole log away")
	}
}

// ───────────────────────────── prompts and briefs ───────────────────────────

// One description of the contract, spliced rather than restated. Six
// hand-synced copies is how the previous kernel came to print two
// contradictory instructions back to back inside one brief.
func TestPromptSplicesTheProtocol(t *testing.T) {
	if !strings.Contains(protocolDoc, "<!-- prompt:begin -->") {
		t.Fatal("PROTOCOL.md lost its splice marker")
	}
	w := wireContract()
	if !strings.Contains(w, "script.authored") || !strings.Contains(w, "## Capability scripts") {
		t.Fatalf("the spliced section is missing the contract:\n%s", trunc(w, 200))
	}
	if strings.Contains(w, "## Accounts") {
		t.Fatal("the splice ran past its end marker")
	}
	h := home(t)
	if p := situated(t, h, "an ask"); !strings.Contains(p, w) {
		t.Fatal("the prompt does not carry the spliced contract verbatim")
	}
}

// The refusal reason must ride the next prompt: it is the only thing that makes
// script.rejected teach anyone anything.
func TestRefusalReasonRidesTheNextPrompt(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "command.declared", decl{Name: "entry", Description: "x"}))
	heard(t, h, line(t, "script.authored", authored{Type: "command", Name: "entry", Script: "   "}))
	p := situated(t, h, "")
	if !strings.Contains(p, "REFUSED") || !strings.Contains(p, "carries no script") {
		t.Fatalf("the refusal reason is not in the prompt:\n%s", p)
	}
}

// A cold mind should not spend its first minutes rediscovering this instance's
// idiom, so one installed script rides along — and never the broken one it is
// being asked to replace.
func TestPromptCarriesAnExemplarButNotTheBrokenOne(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	heard(t, h, line(t, "view.declared", decl{Name: "second", Description: "x", Consumes: []string{"*"}}))
	p := situated(t, h, "")
	if !strings.Contains(p, "as idiom") {
		t.Fatal("no exemplar in the prompt")
	}
	if strings.Contains(p, "--- view/second ---") {
		t.Fatal("the exemplar is the capability being asked about")
	}
}

// The brief is bounded and honest on an empty instance: it must not claim
// capabilities, and it must say the caller is anonymous.
func TestBriefOnAnEmptyInstance(t *testing.T) {
	h := home(t)
	b := brief(h, replayed(t, h))
	if len(b) > 2500 {
		t.Fatalf("the empty brief is %d bytes", len(b))
	}
	for _, want := range []string{"log: 0 events", "none yet", "SELF_CALLER", "nothing pending"} {
		if !strings.Contains(b, want) {
			t.Fatalf("the empty brief is missing %q:\n%s", want, b)
		}
	}
}

// Every event in flight shows in the built-in log view, so a fresh instance is
// never opaque: it is the one read that works before anything is grown.
func TestBuiltinLogViewIsShadowable(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	page, err := runView(h, replayed(t, h), "log")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "command.declared") || !strings.Contains(string(page), "via=") {
		t.Fatalf("the built-in log view is not the log:\n%s", page)
	}
	body := line(t, "view.declared", decl{Name: "log", Description: "mine", Consumes: []string{"*"}}) +
		line(t, "script.authored", authored{Type: "view", Name: "log", Script: "#!/bin/sh\ncat >/dev/null\necho MINE\n"})
	heard(t, h, body)
	page, _ = runView(h, replayed(t, h), "log")
	if strings.TrimSpace(string(page)) != "MINE" {
		t.Fatalf("a declared view did not shadow the built-in: %s", page)
	}
}

// Bare orientation always presents the body. Whether anything warrants action
// belongs to the mind; convergence belongs to `self loop`, which can witness
// every append without pretending to understand domain state.
func TestBareSituateAlwaysOrients(t *testing.T) {
	h := home(t)
	var out bytes.Buffer
	if err := cmdSituate(h, "", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No specific ask") {
		t.Fatalf("bare orientation lost its ask:\n%s", out.String())
	}
	if err := cmdSituate(h, "an actual ask", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func TestLoopRunsOnceAndConvergesOnUnchangedState(t *testing.T) {
	h := home(t)
	var out, diag bytes.Buffer
	err := cmdLoop(h, []string{"--max-passes", "2", "--timeout", "5s", "--", "/bin/sh", "-c", "cat >/dev/null"}, &out, &diag)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diag.String(), "converged after 1 pass") {
		t.Fatalf("loop did not run one naked turn:\n%s", diag.String())
	}
	if len(replayed(t, h).Events) != 0 {
		t.Fatal("silent loop turn appended")
	}
}

func TestLoopRepeatsAfterAppendThenConverges(t *testing.T) {
	h := home(t)
	script := `prompt=$(cat); case "$prompt" in *"log: 0 events"*) printf '%s\n' '{"name":"note.added","payload":{"text":"one"}}';; esac`
	var out, diag bytes.Buffer
	err := cmdLoop(h, []string{"--max-passes", "3", "--timeout", "5s", "--", "/bin/sh", "-c", script}, &out, &diag)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diag.String(), "pass 1 changed authoritative state") || !strings.Contains(diag.String(), "converged after 2 pass") {
		t.Fatalf("loop did not reach the append fixed point:\n%s", diag.String())
	}
	events := replayed(t, h).Events
	if len(events) != 1 || events[0].Name != "note.added" {
		t.Fatalf("loop landed unexpected events: %+v", events)
	}
}

func TestLoopUsesEnvironmentDefaults(t *testing.T) {
	h := home(t)
	t.Setenv("SELF_LOOP_MIND", "cat >/dev/null")
	t.Setenv("SELF_LOOP_MAX_PASSES", "2")
	t.Setenv("SELF_LOOP_TIMEOUT", "5s")
	var out, diag bytes.Buffer
	if err := cmdLoop(h, nil, &out, &diag); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diag.String(), "pass 1/2") || !strings.Contains(diag.String(), "converged after 1 pass") {
		t.Fatalf("loop ignored environment defaults:\n%s", diag.String())
	}
}

func TestLoopCLIOverridesEnvironmentDefaults(t *testing.T) {
	t.Setenv("SELF_LOOP_MIND", "exit 9")
	t.Setenv("SELF_LOOP_MAX_PASSES", "9")
	t.Setenv("SELF_LOOP_TIMEOUT", "9m")
	opts, err := parseLoopOptions([]string{"--max-passes", "2", "--timeout", "5s", "--", "/bin/true"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.MaxPasses != 2 || opts.Timeout != 5*time.Second || len(opts.Mind) != 1 || opts.Mind[0] != "/bin/true" {
		t.Fatalf("CLI did not override loop environment: %+v", opts)
	}
}

// ─────────────────────────────── the CLI shape ──────────────────────────────

func TestUnknownVerbIsNotSilentlyAnAsk(t *testing.T) {
	h := home(t)
	if err := dispatch(h, "brif", nil, &bytes.Buffer{}); err == nil {
		t.Fatal("a mistyped verb was answered as a question")
	}
	var out bytes.Buffer
	if err := dispatch(h, "what", []string{"is", "going", "on"}, &out); err != nil {
		t.Fatalf("a multi-word ask was not an ask: %v", err)
	}
	if !strings.Contains(out.String(), "what is going on") {
		t.Fatal("the ask was lost")
	}
}

func TestCLIHelpNamesTheLoopAndProtocol(t *testing.T) {
	var out bytes.Buffer
	if err := dispatch(home(t), "--help", nil, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"self loop --help", "SELF_LOOP_MIND", "self help", "self view <name> [args...]"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("CLI help is missing %q:\n%s", want, out.String())
		}
	}
}

func TestLoopHelpDocumentsDefaultsAndExecution(t *testing.T) {
	var out bytes.Buffer
	if err := cmdLoop(home(t), []string{"--help"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"default 12", "default 30m", "SELF_LOOP_MIND", "executed directly", "sh -c"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("loop help is missing %q:\n%s", want, out.String())
		}
	}
}

// No file in this repository may show a pipeline whose last stage is a bare
// `self`. That pipeline used to be the headline idiom; now the read face takes
// its ask from argv and never reads stdin, so it silently discards whatever came
// down the pipe and situates the default prompt instead.
//
// This is not hypothetical tidiness. Five copies of the old loop survived the
// rewrite — in main.go's package doc, pipe.go's header, cmdLearn's doc comment,
// lessons/chat, and worst of all a runtime stderr line that told every user of
// `self learn` to run the broken pipeline — in a change whose own commit message
// boasted about eliminating six hand-synced copies of one contract. A comment
// cannot be tested by reading it, so it is tested here.
func TestNoFileShowsThePipelineThatDiscardsTheAsk(t *testing.T) {
	// Built from pieces so this file does not trip its own check.
	needle := "|" + " self"
	var offences []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "cap" {
				return filepath.SkipDir
			}
			return nil
		}
		// This file builds the needle from pieces so it does not trip itself.
		// CHANGELOG.md is exempt for a real reason: its job is to show what
		// changed, and the old loop is what changed.
		if path == "self_test.go" || path == "CHANGELOG.md" {
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".md", ".sh", ".yml", "":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || !utf8.Valid(data) {
			return nil
		}
		for n, ln := range strings.Split(string(data), "\n") {
			rest := ln
			for {
				i := strings.Index(rest, needle)
				if i < 0 {
					break
				}
				after := strings.TrimLeft(rest[i+len(needle):], " \t")
				// A pipeline into `self hear` is the write door and correct.
				// Anything else — end of line, a comment, a redirect, another
				// pipe — is the loop that throws the ask away.
				if !strings.HasPrefix(after, "hear") {
					offences = append(offences,
						fmt.Sprintf("%s:%d: %s", path, n+1, strings.TrimSpace(ln)))
				}
				rest = rest[i+len(needle):]
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offences) > 0 {
		t.Fatalf("a pipeline ending in a bare `self` discards its ask — the write door is `self hear`:\n  %s",
			strings.Join(offences, "\n  "))
	}
}

func TestHelpIsTheProtocol(t *testing.T) {
	var out bytes.Buffer
	if err := dispatch(home(t), "help", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != protocolDoc {
		t.Fatal("self help is not PROTOCOL.md verbatim")
	}
}

// ─────────────────────────────── small helpers ──────────────────────────────

func hasEvent(t *testing.T, h, name string) bool {
	t.Helper()
	events, _ := readEvents(h)
	for _, e := range events {
		if e.Name == name {
			return true
		}
	}
	return false
}

func mustLine(name string, payload json.RawMessage) []byte {
	b, _ := json.Marshal(map[string]any{"name": name, "payload": payload})
	return append(b, '\n')
}
