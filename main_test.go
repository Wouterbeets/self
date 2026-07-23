package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogAppendRead(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		e := newEvent(name, json.RawMessage(`{"x":1}`))
		if err := appendEvent(home, &e); err != nil {
			t.Fatal(err)
		}
	}
	events, err := readEvents(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	for i, e := range events {
		if e.Seq != i+1 {
			t.Errorf("event %d has seq %d", i, e.Seq)
		}
		if e.ID == "" || e.OccurredAt.IsZero() {
			t.Errorf("event %d missing id or timestamp", i)
		}
	}
}

// TestConcurrentAppendsDoNotCollide pins the single-writer property under
// contention: many writers appending at once must still yield unique,
// contiguous sequence numbers — the advisory log lock is what guarantees it.
func TestConcurrentAppendsDoNotCollide(t *testing.T) {
	home := t.TempDir()
	const writers = 24
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := newEvent("tick", json.RawMessage(`{}`))
			if err := appendEvent(home, &e); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	events, err := readEvents(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != writers {
		t.Fatalf("got %d events, want %d — an append was lost to a race", len(events), writers)
	}
	seen := map[int]bool{}
	for _, e := range events {
		if seen[e.Seq] {
			t.Fatalf("duplicate seq %d — two writers collided", e.Seq)
		}
		seen[e.Seq] = true
	}
	for i := 1; i <= writers; i++ {
		if !seen[i] {
			t.Fatalf("seq %d missing — sequence is not contiguous", i)
		}
	}
}

// TestStrangeLoop drives the whole kernel loop offline: a declaration arrives
// as an event, the (stub) compiler turns it into an installed script with a
// signed receipt, running the command appends its event, and the projection
// re-renders to site/ showing it. This is the core loop in one test.
func TestStrangeLoop(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	home := t.TempDir()

	decls := []Event{
		newEvent("command.declared", json.RawMessage(
			`{"name":"note","description":"take a note","params":{"text":"string"},"event":{"name":"note.taken","fields":{"title":"string"}}}`)),
		newEvent("projector.declared", json.RawMessage(
			`{"name":"board","description":"all notes","consumes":["note.taken"]}`)),
	}
	if err := ingest(home, decls); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		filepath.Join(home, "capabilities", "commands", "note", "run"),
		filepath.Join(home, "capabilities", "projectors", "board", "run"),
	} {
		if !fileExists(p) {
			t.Fatalf("strange loop did not install %s", p)
		}
	}

	// Each compile logged a receipt this home's kernel signed.
	secret, err := loadSecret(home)
	if err != nil {
		t.Fatal(err)
	}
	events, _ := readEvents(home)
	receipts := 0
	for _, e := range events {
		if e.Name != "script.compiled" {
			continue
		}
		if _, ok := verifiedReceipt(secret, e.Payload); !ok {
			t.Errorf("seq %d: receipt does not verify", e.Seq)
		}
		receipts++
	}
	if receipts != 2 {
		t.Fatalf("got %d signed receipts, want 2", receipts)
	}

	// Run the grown command; its event must land on the log and in the view.
	if _, err := runCommand(home, "note", []string{"water", "the", "plants"}, "cli", ""); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(home, "site", "board.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "water the plants") {
		t.Fatalf("board.html does not show the note:\n%s", page)
	}
}

// TestForgedReceiptIsInert pins the trust model: anything may append a
// script.compiled, but only a kernel-signed receipt ever installs.
func TestForgedReceiptIsInert(t *testing.T) {
	home := t.TempDir()
	if _, err := loadSecret(home); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(receipt{"command", "evil", "#!/bin/sh\necho pwned", "a liar about who wrote this", "deadbeef"})
	e := newEvent("script.compiled", payload)
	if err := appendEvent(home, &e); err != nil {
		t.Fatal(err)
	}
	if err := rehydrate(home); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(home, "capabilities", "commands", "evil", "run")) {
		t.Fatal("a forged receipt installed")
	}
}

// TestRehydrateRoundTrip pins deterministic reconstruction: an instance
// rebuilt from events.jsonl + .secret alone reproduces its installed scripts
// and rendered projections byte-for-byte.
func TestRehydrateRoundTrip(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	src := t.TempDir()
	decls := []Event{
		newEvent("command.declared", json.RawMessage(
			`{"name":"entry","description":"record an entry","params":{"text":"string"},"event":{"name":"journal.entry","fields":{"title":"string"}}}`)),
		newEvent("projector.declared", json.RawMessage(
			`{"name":"journal","description":"all entries","consumes":["journal.entry"]}`)),
	}
	if err := ingest(src, decls); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(src, "entry", []string{"first", "entry"}, "cli", ""); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	for _, f := range []string{"events.jsonl", ".secret"} {
		data, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, f), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := rehydrate(dst); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join("capabilities", "commands", "entry", "run"),
		filepath.Join("capabilities", "projectors", "journal", "run"),
		filepath.Join("site", "journal.html"),
	} {
		a, err := os.ReadFile(filepath.Join(src, p))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dst, p))
		if err != nil {
			t.Fatalf("%s did not reconstruct: %s", p, err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("%s differs after reconstruction", p)
		}
	}
}

func TestRehydrateEmptyLogClearsDerivedState(t *testing.T) {
	home := t.TempDir()
	stale := filepath.Join(home, "capabilities", "commands", "stale", "run")
	if err := os.MkdirAll(filepath.Dir(stale), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := rehydrate(home); err != nil {
		t.Fatal(err)
	}
	if fileExists(stale) {
		t.Fatal("empty-log rehydrate preserved stale executable state")
	}
}

func TestRehydrateFailurePreservesWorkingDerivedState(t *testing.T) {
	home := t.TempDir()
	page := filepath.Join(home, "site", "working.html")
	if err := os.MkdirAll(filepath.Dir(page), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte("working before rebuild"), 0644); err != nil {
		t.Fatal(err)
	}
	decl := newEvent("projector.declared", json.RawMessage(`{"name":"broken","description":"fails","consumes":[]}`))
	if err := appendEvent(home, &decl); err != nil {
		t.Fatal(err)
	}
	if err := installTrustedScript(home, "projector", "broken", "#!/bin/sh\nexit 1\n", "test"); err != nil {
		t.Fatal(err)
	}
	if err := rehydrate(home); err == nil {
		t.Fatal("rehydrate succeeded despite a failing staged projector")
	}
	got, err := os.ReadFile(page)
	if err != nil || string(got) != "working before rebuild" {
		t.Fatalf("failed rehydrate damaged working derived state: %q, %v", got, err)
	}
}

// TestRehydrateTypeCollision pins that a command and a projector sharing a
// name both reconstruct: receipts are keyed by (type, name), not name. The
// chat lesson (a chat command and a chat projector) is the natural collision.
func TestRehydrateTypeCollision(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	home := t.TempDir()
	decls := []Event{
		newEvent("command.declared", json.RawMessage(
			`{"name":"chat","description":"say something","event":{"name":"chat.message","fields":{"content":"string"}}}`)),
		newEvent("projector.declared", json.RawMessage(
			`{"name":"chat","description":"the conversation","consumes":["chat.message"]}`)),
	}
	if err := ingest(home, decls); err != nil {
		t.Fatal(err)
	}
	cmd := filepath.Join(home, "capabilities", "commands", "chat", "run")
	proj := filepath.Join(home, "capabilities", "projectors", "chat", "run")
	os.Remove(cmd)
	os.Remove(proj)
	if err := rehydrate(home); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{cmd, proj} {
		if !fileExists(p) {
			t.Fatalf("%s did not survive rehydration — receipts collided across types", p)
		}
	}
}

// TestRetireRemovesDerivedStateAndSurvivesRehydrate pins the deletion story:
// events are forever, derived state is a fold. Retiring a projector removes
// its script and page, delists it from kernel.html, holds through a rehydrate
// (the tombstone outranks earlier receipts), and a later re-declaration
// revives it — deletion is a fold rule, not an erasure.
func TestRetireRemovesDerivedStateAndSurvivesRehydrate(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	home := t.TempDir()
	decls := []Event{
		newEvent("command.declared", json.RawMessage(
			`{"name":"note","description":"take a note","params":{"text":"string"},"event":{"name":"note.taken","fields":{"title":"string"}}}`)),
		newEvent("projector.declared", json.RawMessage(
			`{"name":"board","description":"all notes","consumes":["note.taken"]}`)),
	}
	if err := ingest(home, decls); err != nil {
		t.Fatal(err)
	}

	proj := filepath.Join(home, "capabilities", "projectors", "board", "run")
	page := filepath.Join(home, "site", "board.html")
	cmd := filepath.Join(home, "capabilities", "commands", "note", "run")
	for _, p := range []string{proj, page, cmd} {
		if !fileExists(p) {
			t.Fatalf("setup: %s missing", p)
		}
	}

	if err := cmdRetire(home, "projector/board"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{proj, page} {
		if fileExists(p) {
			t.Fatalf("retire left %s behind", p)
		}
	}
	// Only the named (type, name) retires; the command is untouched.
	if !fileExists(cmd) {
		t.Fatal("retiring the projector removed the command")
	}
	kernel, err := os.ReadFile(filepath.Join(home, "site", "kernel.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(kernel), `href="/board"`) {
		t.Fatal("kernel.html still lists the retired projection")
	}

	if err := rehydrate(home); err != nil {
		t.Fatal(err)
	}
	if fileExists(proj) || fileExists(page) {
		t.Fatal("rehydrate reinstalled a retired capability")
	}
	if !fileExists(cmd) {
		t.Fatal("rehydrate dropped a live capability")
	}

	// Revival: a declaration after the tombstone re-enters the fold, and the
	// fresh receipt outranks the tombstone on the next rehydrate too.
	if err := ingest(home, []Event{newEvent("projector.declared", json.RawMessage(
		`{"name":"board","description":"all notes, back again","consumes":["note.taken"]}`))}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(proj) || !fileExists(page) {
		t.Fatal("re-declaration did not revive the retired projector")
	}
	if err := rehydrate(home); err != nil {
		t.Fatal(err)
	}
	if !fileExists(proj) {
		t.Fatal("revival did not survive rehydration")
	}
}

// TestRetireRefusesUnknownTargets pins the guardrails: retiring something
// never declared (or a malformed target) is an error, not a silent tombstone.
func TestRetireRefusesUnknownTargets(t *testing.T) {
	home := t.TempDir()
	if err := cmdRetire(home, "projector/ghost"); err == nil {
		t.Fatal("retiring an undeclared capability should error")
	}
	if err := cmdRetire(home, "gizmo/board"); err == nil {
		t.Fatal("an unknown capability type should error")
	}
	if err := cmdRetire(home, "projector/../escape"); err == nil {
		t.Fatal("a traversal name should error")
	}
}

func TestReviseCompilesWithCurrentScriptAndRequest(t *testing.T) {
	mind := filepath.Join(t.TempDir(), "mind")
	if err := os.WriteFile(mind, []byte(`#!/usr/bin/env python3
import os, sys, json
prompt = sys.argv[-1]
sys.stdin.read()
if os.environ.get("SELF_ASK") != "compile":
    raise SystemExit("unexpected ask")
if "old sentinel" not in prompt:
    raise SystemExit("previous script was not provided")
if "make it revised" not in prompt:
    raise SystemExit("revision request was not provided")
script = "#!/bin/sh\necho '{\"name\":\"note.added\",\"payload\":{\"text\":\"revised\"}}'\n"
print(json.dumps({"name":"script.authored","payload":{"script":script}}))
`), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELF_MIND", mind)
	t.Setenv("SELF_MIND_ID", "revision mind")

	home := t.TempDir()
	decl := newEvent("command.declared", json.RawMessage(`{"name":"note","description":"take a note","event":{"name":"note.added","fields":{"text":"string"}}}`))
	if err := appendEvent(home, &decl); err != nil {
		t.Fatal(err)
	}
	oldScript := "#!/bin/sh\n# old sentinel\necho '{\"name\":\"note.added\",\"payload\":{\"text\":\"old\"}}'\n"
	if err := installTrustedScript(home, "command", "note", oldScript, "old mind"); err != nil {
		t.Fatal(err)
	}

	if err := cmdRevise(home, "command/note", []string{"make", "it", "revised"}); err != nil {
		t.Fatal(err)
	}
	events, err := readEvents(home)
	if err != nil {
		t.Fatal(err)
	}
	revisions := 0
	decls := 0
	for _, e := range events {
		switch e.Name {
		case "capability.revision.requested":
			revisions++
			var p struct {
				Type        string `json:"type"`
				Name        string `json:"name"`
				Request     string `json:"request"`
				FromReceipt string `json:"from_receipt"`
			}
			if json.Unmarshal(e.Payload, &p) != nil || p.Type != "command" || p.Name != "note" || p.Request != "make it revised" || p.FromReceipt == "" {
				t.Fatalf("bad revision event: %s", e.Payload)
			}
		case "command.declared":
			decls++
		}
	}
	if revisions != 1 {
		t.Fatalf("recorded %d revision requests, want 1", revisions)
	}
	if decls != 2 {
		t.Fatalf("recorded %d declarations, want original + revised", decls)
	}
	got, err := os.ReadFile(filepath.Join(home, "capabilities", "commands", "note", "run"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "revised") {
		t.Fatalf("revised script was not installed:\n%s", got)
	}
}

// TestPluggableMind pins the README's oldest promise, now true everywhere:
// the mind is just a process behind one contract, and the kernel can't tell
// the difference. A fake external mind — a few lines of python, no HTTP, no
// stub — answers a reflection with prose plus a declaration, then answers the
// compile ask the strange loop fires, and the capability it authored installs with
// a receipt signed by this home carrying the external mind's name.
func TestPluggableMind(t *testing.T) {
	mind := filepath.Join(t.TempDir(), "mind")
	if err := os.WriteFile(mind, []byte(`#!/usr/bin/env python3
import os, sys, json
sys.stdin.read()  # the log — an external mind may read it or not
ask = os.environ.get("SELF_ASK", "")
if ask == "compile":
    script = "#!/usr/bin/env python3\nimport sys, json\nprint(json.dumps({\"name\": \"pinged\", \"payload\": {\"title\": \" \".join(sys.argv[1:]) or \"pong\"}}))\n"
    print(json.dumps({"name": "script.authored", "payload": {"script": script}}))
elif ask == "reflect":
    print("I looked around; this instance cannot ping. Declaring that.")  # prose — tolerated
    print(json.dumps({"name": "command.declared", "payload": {
        "name": "ping", "description": "answer with a pong",
        "event": {"name": "pinged", "fields": {"title": "string"}}}}))
else:
    print("thought about: " + sys.argv[-1])
`), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELF_MIND", mind)
	t.Setenv("SELF_MIND_ID", "an external mind, plugged in whole")

	home := t.TempDir()
	if err := cmdReflect(home); err != nil {
		t.Fatal(err)
	}

	// the declaration compiled through the external mind, not HTTP, not stubs
	installed := filepath.Join(home, "capabilities", "commands", "ping", "run")
	data, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("the external mind's capability did not install: %s", err)
	}
	if !strings.Contains(string(data), "pinged") {
		t.Fatalf("installed script is not the mind's: %s", data)
	}

	// the receipt is home-signed and carries the external mind's name
	secret, _ := loadSecret(home)
	events, _ := readEvents(home)
	found := false
	for _, e := range events {
		if e.Name != "script.compiled" {
			continue
		}
		r, ok := verifiedReceipt(secret, e.Payload)
		if !ok {
			t.Fatalf("seq %d: receipt does not verify", e.Seq)
		}
		if r.By != "an external mind, plugged in whole" {
			t.Fatalf("receipt authored by %q", r.By)
		}
		found = true
	}
	if !found {
		t.Fatal("no receipt for the external mind's compile")
	}

	// and the capability runs
	evs, err := runCommand(home, "ping", []string{"hello"}, "cli", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Name != "pinged" {
		t.Fatalf("ping emitted %v", evs)
	}

	// think flows through the same seam, prose and all
	res, err := pipeMind(home, "think", "are you there?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Response, "thought about: are you there?") {
		t.Fatalf("think response = %q", res.Response)
	}
}

// A chat-shaped mind (claude -p and its kin) answers in Markdown: it wraps the
// event JSON in backticks or a ```json fence and narrates around it. The pipe
// must still find the events, or the headline SELF_MIND="claude -p" is a broken
// promise. This pins that a Markdown-speaking mind plugs in unchanged.
func TestMindMarkdownFencedJSON(t *testing.T) {
	if _, fence := unfence("```json"); !fence {
		t.Fatal("```json should be a fence marker")
	}
	if _, fence := unfence("```"); !fence {
		t.Fatal("bare ``` should be a fence marker")
	}
	if c, _ := unfence("`{\"name\":\"x\"}`"); c != `{"name":"x"}` {
		t.Fatalf("inline-backticked JSON not unwrapped: %q", c)
	}
	if c, _ := unfence(`{"name":"x"}`); c != `{"name":"x"}` {
		t.Fatalf("plain JSON should pass through untouched: %q", c)
	}
	if c, _ := unfence("use `self run entry`"); c != "use `self run entry`" {
		t.Fatalf("prose with inline code must not be stripped: %q", c)
	}

	mind := filepath.Join(t.TempDir(), "mind")
	// Mimics claude -p: prose, then a backtick-wrapped declaration, then a
	// fenced compile answer.
	if err := os.WriteFile(mind, []byte("#!/usr/bin/env python3\n"+
		`import os, sys, json
sys.stdin.read()
ask = os.environ.get("SELF_ASK", "")
if ask == "compile":
    script = "#!/usr/bin/env python3\nimport sys, json\nprint(json.dumps({\"name\": \"noted\", \"payload\": {\"text\": \" \".join(sys.argv[1:]) or \"()\"}}))\n"
    print("Here is the script:")
    print("`+"```"+`json")
    print(json.dumps({"name": "script.authored", "payload": {"script": script}}))
    print("`+"```"+`")
else:
    print("I'll declare the note command per the contract.")
    print("`+"`"+`" + json.dumps({"name": "command.declared", "payload": {
        "name": "note", "description": "record a note",
        "params": {"text": "string"},
        "event": {"name": "noted", "fields": {"text": "string"}}}}) + "`+"`"+`")
    print("Declared the `+"`"+`note`+"`"+`command.")
`), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELF_MIND", mind)
	t.Setenv("SELF_MIND_ID", "a markdown-speaking mind")

	home := t.TempDir()
	res, err := pipeMind(home, "learn", "learn a note capability")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 1 || res.Events[0]["name"] != "command.declared" {
		t.Fatalf("backtick-wrapped event not parsed: %+v", res.Events)
	}
	// prose survives, fence markers do not leak into it
	if !strings.Contains(res.Response, "declare the note command") {
		t.Fatalf("prose lost: %q", res.Response)
	}
	if strings.Contains(res.Response, "```") {
		t.Fatalf("fence markers leaked into prose: %q", res.Response)
	}
	// the fenced compile answer is found and drives a real install
	if err := ingest(home, mustEvents(t, res.Events)); err != nil {
		t.Fatal(err)
	}
	if p := filepath.Join(home, "capabilities", "commands", "note", "run"); !fileExists(p) {
		t.Fatal("the note capability compiled via the fenced mind did not install")
	}
}

func mustEvents(t *testing.T, decls []map[string]any) []Event {
	t.Helper()
	var evs []Event
	for _, d := range decls {
		name, _ := d["name"].(string)
		payload, _ := json.Marshal(d["payload"])
		evs = append(evs, newEvent(name, payload))
	}
	return evs
}

// stubMind returns the absolute path of examples/mind-stub — the
// deterministic offline mind the tests plug in through the one seam every
// real mind uses. There is no in-kernel stub: a mind is a process.
func stubMind(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "examples", "mind-stub")
}

// installTrustedScript simulates an earlier legitimate install: a script on
// disk plus the kernel-signed receipt that put it there. Test scaffolding —
// the kernel's only install path is a compile.
func installTrustedScript(home, typ, name, script, by string) error {
	if err := installScript(home, typ, name, script); err != nil {
		return err
	}
	return appendReceipt(home, typ, name, script, by)
}

// A capable mind (claude -p) will otherwise try to persist its own work —
// write events.jsonl, run the CLI, install a script — and emit Markdown. Every
// event-expecting ask must tell it the answer channel is stdout only, plain
// JSON, one line each. This pins that guidance into the prompts the mind sees.
func TestEventAsksGuideTheMindToStdout(t *testing.T) {
	must := func(where, prompt string, needles ...string) {
		low := strings.ToLower(prompt)
		for _, n := range needles {
			if !strings.Contains(low, n) {
				t.Errorf("%s prompt is missing guidance %q", where, n)
			}
		}
	}
	// learn and reflect expect declarations: answer on stdout, plain JSON.
	must("learn", learnPrompt(".", "some intent", nil), "stdout", "events.jsonl", "no markdown", "one line")
	// think is report-only, but the mind must still be told stdout is the
	// only channel — a tool-capable mind otherwise tries to persist its work.
	must("think", thinkPrompt("what is missing here?"), "stdout", "cannot write the log", "no code fences", "report-only")
	must("answer contract", mindAnswerContract, "stdout", "cannot write the log", "no code fences", "reply is final", "never re-invoked")
	must("think contract", mindThinkContract, "report-only", "does not append")
	// compile: the mind may test with its tools, but must not install or persist.
	must("compile", compilePrompt("", "", "", "", "command", "note", `{"name":"note"}`),
		"do not install", "events.jsonl", "no code fence")
	// the intent-woven variant keeps the same guidance.
	must("compile+intent", compilePrompt("a product", "", "", "", "command", "note", `{"name":"note"}`),
		"do not install", "no code fence")
	// during a learn the orchestrator's reasoning rides in-band in the prompt.
	must("compile+reasoning", compilePrompt("a product", "declared note because the intent asks for one", "", "", "command", "note", `{"name":"note"}`),
		"orchestrator", "declared note because the intent asks for one", "do not install")
}

// The orchestrator's stated reasoning is provenance. cmdLearn appends it to the
// log as learn.orchestrated and weaves it into every compile of that learn — the
// in-band alternative to remembering through a session store outside the log:
// rehydrate replays it, audit can read it.
func TestLearnLogsOrchestratorReasoning(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	home := t.TempDir()

	seed := filepath.Join(t.TempDir(), "notes")
	if err := os.Mkdir(seed, 0755); err != nil {
		t.Fatal(err)
	}
	intent := "`self run note <text>` appends one `note.added` event. `/notes` renders notes."
	if err := os.WriteFile(filepath.Join(seed, "intent.md"), []byte(intent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cmdLearn(home, seed); err != nil {
		t.Fatal(err)
	}

	events, err := readEvents(home)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Lesson    string `json:"lesson"`
		Reasoning string `json:"reasoning"`
	}
	found := false
	for _, e := range events {
		if e.Name == "learn.orchestrated" {
			if err := json.Unmarshal(e.Payload, &got); err != nil {
				t.Fatal(err)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("learn did not append a learn.orchestrated event")
	}
	if got.Lesson != "notes" || strings.TrimSpace(got.Reasoning) == "" {
		t.Fatalf("learn.orchestrated payload = %+v, want lesson \"notes\" and non-empty reasoning", got)
	}
}

func TestStubMindCoversThinkAndLearn(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	home := t.TempDir()

	res, err := pipeMind(home, "think", "are you there?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Response, "stub thought about: are you there?") {
		t.Fatalf("stub think response = %q", res.Response)
	}

	seed := filepath.Join(t.TempDir(), "journal")
	if err := os.Mkdir(seed, 0755); err != nil {
		t.Fatal(err)
	}
	intent := "`self run entry <text>` appends one `journal.entry` event. `/journal` renders entries."
	if err := os.WriteFile(filepath.Join(seed, "intent.md"), []byte(intent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cmdLearn(home, seed); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(home, "capabilities", "commands", "entry", "run")) {
		t.Fatal("stub learn did not install the declared command")
	}
	if !fileExists(filepath.Join(home, "capabilities", "projectors", "journal", "run")) {
		t.Fatal("stub learn did not install the declared projector")
	}
	if _, err := runCommand(home, "entry", []string{"hello", "offline", "world"}, "cli", ""); err != nil {
		t.Fatal(err)
	}
	page, err := runProjection(home, "journal")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "hello offline world") {
		t.Fatalf("stub-learned projection did not show entry:\n%s", page)
	}
}

func TestLearnFailsWhenCompilationFails(t *testing.T) {
	mind := filepath.Join(t.TempDir(), "mind")
	if err := os.WriteFile(mind, []byte("#!/bin/sh\nif [ \"$SELF_ASK\" = learn ]; then printf '%s\\n' '{\"name\":\"command.declared\",\"payload\":{\"name\":\"broken\",\"description\":\"broken\",\"event\":{\"name\":\"broken.ran\"}}}'; else exit 1; fi\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELF_MIND", mind)
	seed := filepath.Join(t.TempDir(), "broken")
	if err := os.Mkdir(seed, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "intent.md"), []byte("declare broken"), 0644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := cmdLearn(home, seed); err == nil {
		t.Fatal("learn reported success after compile failure")
	}
	events, _ := readEvents(home)
	for _, e := range events {
		if e.Name == "lesson.learned" {
			t.Fatal("failed learn wrote lesson.learned")
		}
	}
	if err := rehydrate(home); err != nil {
		t.Fatalf("failed declaration made the instance unreconstructable: %v", err)
	}
}

func TestLiveExecutionRejectsTamperedScripts(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	home := t.TempDir()
	if err := ingest(home, []Event{
		newEvent("command.declared", json.RawMessage(`{"name":"note","description":"note","event":{"name":"note.added","fields":{"text":"string"}}}`)),
		newEvent("projector.declared", json.RawMessage(`{"name":"notes","description":"notes","consumes":["note.added"]}`)),
	}); err != nil {
		t.Fatal(err)
	}
	command, _ := scriptPath(home, "command", "note")
	if err := os.WriteFile(command, []byte("#!/bin/sh\necho tampered\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(home, "note", []string{"x"}, "cli", ""); err == nil || !strings.Contains(err.Error(), "verified receipt") {
		t.Fatalf("tampered command execution error = %v", err)
	}
	projector, _ := scriptPath(home, "projector", "notes")
	if err := os.WriteFile(projector, []byte("#!/bin/sh\necho tampered\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := runProjection(home, "notes"); err == nil || !strings.Contains(err.Error(), "verified receipt") {
		t.Fatalf("tampered projector execution error = %v", err)
	}
}

func TestStubCommandHonorsDeclaredField(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	home := t.TempDir()
	decl := newEvent("command.declared", json.RawMessage(
		`{"name":"memo","description":"record a memo","event":{"name":"memo.added","fields":{"text":"string"}}}`))
	if err := ingest(home, []Event{decl}); err != nil {
		t.Fatal(err)
	}
	events, err := runCommand(home, "memo", []string{"uses", "text"}, "cli", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Name != "memo.added" {
		t.Fatalf("stub command emitted %v", events)
	}
	var payload map[string]string
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["text"] != "uses text" {
		t.Fatalf("stub command ignored declared field: %s", events[0].Payload)
	}
}

// TestReceiptProvenance pins the by-line: authorship is inside the signature,
// so it can no more be forged, stripped, or moved than the script itself.
func TestReceiptProvenance(t *testing.T) {
	home := t.TempDir()
	secret, err := loadSecret(home)
	if err != nil {
		t.Fatal(err)
	}

	mint := func(r receipt) json.RawMessage {
		p, _ := json.Marshal(r)
		return p
	}

	// a signed by-line verifies, and survives the round trip
	good := receipt{"command", "graze", "#!/bin/sh\necho hi", "agent A at endpoint B", ""}
	good.Sig = sign(secret, good.Type, good.Name, good.Script, good.By)
	if r, ok := verifiedReceipt(secret, mint(good)); !ok || r.By != good.By {
		t.Fatal("signed provenance did not verify")
	}

	// authorship cannot be relabeled
	relabeled := good
	relabeled.By = "some other agent"
	if _, ok := verifiedReceipt(secret, mint(relabeled)); ok {
		t.Fatal("relabeled authorship verified — provenance is forgeable")
	}

	// authorship cannot be stripped by folding it into the script (the
	// concatenation attack the v2 domain separation exists to kill)
	folded := receipt{good.Type, good.Name, good.Script + "\x00" + good.By, "", good.Sig}
	if _, ok := verifiedReceipt(secret, mint(folded)); ok {
		t.Fatal("by-line folded into script verified — field boundaries are ambiguous")
	}

	// and a receipt the kernel mints carries the mind's identity
	c := newLLM(home)
	t.Setenv("SELF_MIND_ID", "")
	t.Setenv("SELF_MIND", "some-mind")
	if got := c.identity(); got != "some-mind" {
		t.Fatalf("mind identity = %q, want the executable", got)
	}
	t.Setenv("SELF_MIND_ID", "an agent-chosen identity")
	if got := c.identity(); got != "an agent-chosen identity" {
		t.Fatalf("SELF_MIND_ID override = %q", got)
	}
}

func TestProtocolHelpIsVisibleFromCLI(t *testing.T) {
	protocol := protocolText()
	for _, want := range []string{
		"SELF_ASK     request kind: think | reflect | learn | compile",
		"command.declared",
		"projector.declared",
		"script.authored",
		"command script",
		"projector script",
	} {
		if !strings.Contains(protocol, want) {
			t.Fatalf("protocol help missing %q:\n%s", want, protocol)
		}
	}

	usage := usageText()
	if !strings.Contains(usage, "self protocol") {
		t.Fatalf("usage does not advertise protocol help:\n%s", usage)
	}
	if got, ok := commandHelp("protocol"); !ok || got != protocol {
		t.Fatalf("help protocol did not return protocol text")
	}
}

func TestCommandHelpTreatsFlagsAsHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		if !wantsHelp(args) {
			t.Fatalf("wantsHelp(%v) = false", args)
		}
	}

	runHelp, ok := commandHelp("run")
	if !ok {
		t.Fatal("run help missing")
	}
	if !strings.Contains(runHelp, "usage: self run <command> [args...]") {
		t.Fatalf("run help is not command usage:\n%s", runHelp)
	}
}

func TestHomeDefaultsToWorkingDirectory(t *testing.T) {
	t.Setenv("SELF_HOME", "")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := homeDir(); got != wd {
		t.Fatalf("homeDir default = %q, want cwd %q", got, wd)
	}
}

// TestInjectShellShape checks the shell is layered onto a page without
// disturbing it: the stylesheet goes inside <head> and the nav right after
// <body>, leaving the page's own content intact.
func TestInjectShellShape(t *testing.T) {
	page := []byte("<!DOCTYPE html><html><head><title>t</title></head><body><h1>hi</h1></body></html>")
	sampleNav := `<nav class="self-nav"><a href="/">self</a></nav>`

	out := string(injectShell(page, sampleNav))
	if !strings.Contains(out, shellCSS) {
		t.Fatal("stylesheet not injected")
	}
	head := strings.Index(out, "</head>")
	if i := strings.Index(out, "<style>"); i < 0 || i > head {
		t.Fatal("stylesheet not inside <head>")
	}
	if i := strings.Index(out, sampleNav); i < 0 || i < strings.Index(out, "<body>") {
		t.Fatal("site nav not placed right after <body>")
	}
	if !strings.Contains(out, "<h1>hi</h1>") {
		t.Fatal("injectShell dropped the page's own content")
	}
}

// TestNestedProjectionsUnfold pins progressive unfolding: a projector may
// declare a nested name (finances/bills); it compiles, renders to a nested
// page under site/, survives rehydrate, and stays OFF the top nav — depth is
// reached from the parent page, so the surface unfolds instead of flooding.
func TestNestedProjectionsUnfold(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	home := t.TempDir()

	for _, n := range []string{"finances", "finances/bills"} {
		decl := newEvent("projector.declared", json.RawMessage(`{"name":"`+n+`","description":"d","consumes":["bill.paid"]}`))
		if err := ingest(home, []Event{decl}); err != nil {
			t.Fatal(err)
		}
	}
	if !fileExists(filepath.Join(home, "site", "finances", "bills.html")) {
		t.Fatal("nested projection did not render to a nested page")
	}

	// the whole thing rebuilds from the log alone
	os.RemoveAll(filepath.Join(home, "capabilities"))
	os.RemoveAll(filepath.Join(home, "site"))
	if err := rehydrate(home); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(home, "capabilities", "projectors", "finances", "bills")) {
		t.Fatal("rehydrate did not reinstall the nested projector")
	}

	// the nav unfolds: top level only, and a nested page marks its parent
	nav := siteNav(home, "finances/bills")
	if strings.Contains(nav, `href="/finances/bills"`) {
		t.Fatalf("nested page leaked into the top nav:\n%s", nav)
	}
	if !strings.Contains(nav, `href="/finances" aria-current="true"`) {
		t.Fatalf("nested page did not mark its top-level parent:\n%s", nav)
	}

	// traversal never installs, whatever declares it
	if err := installScript(home, "projector", "../escape", "#!/bin/sh\n"); err == nil {
		t.Fatal("traversal name was installed")
	}
	if err := installScript(home, "projector", "a/.hidden", "#!/bin/sh\n"); err == nil {
		t.Fatal("hidden segment was installed")
	}
}

// TestSiteNavListsProjections pins the human way around an instance: the
// injected nav is a replay of the log — every declared projection, in
// declaration order, plus the kernel surfaces — with the current page marked.
func TestSiteNavListsProjections(t *testing.T) {
	home := t.TempDir()
	for _, n := range []string{"notes", "memory"} {
		e := newEvent("projector.declared", json.RawMessage(`{"name":"`+n+`","description":"d","consumes":["x"]}`))
		if err := appendEvent(home, &e); err != nil {
			t.Fatal(err)
		}
	}
	nav := siteNav(home, "memory")
	for _, want := range []string{`href="/notes"`, `href="/memory" aria-current="true"`, `href="/brief"`, `href="/events"`} {
		if !strings.Contains(nav, want) {
			t.Fatalf("nav missing %s:\n%s", want, nav)
		}
	}
	if strings.Index(nav, "/notes") > strings.Index(nav, "/memory") {
		t.Fatal("nav does not preserve declaration order")
	}
}

// TestMindReceivesStateBriefNotRawLog pins the renovation: the mind no longer
// gets the whole event log dumped on stdin. It gets an orientation brief —
// the same current-state unfolding the projections draw — and is pointed at
// SELF_HOME for depth (the raw log and rendered pages live on disk). A mind
// reads state, not a firehose; an instance's mind prompt stays O(state), not
// O(history), so a long-lived instance doesn't grow an unbounded ask. The stub
// mind (examples/mind-stub) ignores stdin too, so this pins the seam itself.
func TestMindReceivesStateBriefNotRawLog(t *testing.T) {
	// A mind that records its stdin to a file so we can inspect what the kernel
	// actually fed it. It answers a think ask with one prose line.
	seen := filepath.Join(t.TempDir(), "stdin.txt")
	mind := filepath.Join(t.TempDir(), "mind")
	if err := os.WriteFile(mind, []byte(`#!/usr/bin/env python3
import os, sys
data = sys.stdin.read()
with open(os.environ["SEEN"], "w") as f:
    f.write(data)
print("read " + str(len(data)) + " bytes on stdin")
`), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELF_MIND", mind)
	t.Setenv("SELF_MIND_ID", "the recorder mind")
	home := t.TempDir()
	// lay down a small, recognizable log: a declaration + a couple of events
	decl := newEvent("command.declared", json.RawMessage(`{"name":"note","description":"take a note","event":{"name":"note.taken","fields":{"title":"string"}}}`))
	if err := appendEvent(home, &decl); err != nil {
		t.Fatal(err)
	}
	msg := newEvent("note.taken", json.RawMessage(`{"title":"water the plants"}`))
	if err := appendEvent(home, &msg); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SEEN", seen)
	res, err := pipeMind(home, "think", "what do you see?")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Response, "bytes on stdin") {
		t.Fatalf("mind did not run / record: %q", res.Response)
	}

	fed, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	brief := string(fed)

	// the brief names the instance, teaches mechanism, and lists the catalog
	if !strings.Contains(brief, "# self — orientation brief") {
		t.Fatalf("brief missing instance header:\n%s", brief)
	}
	if !strings.Contains(brief, "How you act") {
		t.Fatalf("brief missing mechanism section:\n%s", brief)
	}
	if !strings.Contains(brief, "stdout") {
		t.Fatalf("brief missing process-receive (stdout) guidance:\n%s", brief)
	}
	if !strings.Contains(brief, "site/kernel.html") {
		t.Fatalf("brief does not mention kernel.html as optional depth:\n%s", brief)
	}
	if !strings.Contains(brief, "note.taken") {
		t.Fatalf("brief missing the note command's event:\n%s", brief)
	}
	if !strings.Contains(brief, "events.jsonl") {
		t.Fatalf("brief does not point the mind at the raw log:\n%s", brief)
	}
	if !strings.Contains(brief, "self run note") {
		t.Fatalf("brief missing run contract for note:\n%s", brief)
	}

	// and it is NOT the raw JSONL log: no event-object line with a `"seq":` key
	if strings.Contains(brief, `"seq":`) {
		t.Fatalf("mind was fed the raw log, not a brief:\n%s", brief)
	}
	// bounded: O(state) — a small catalog stays far under a few KiB
	if len(brief) > 8192 {
		t.Fatalf("brief is %d bytes — not bounded for a tiny catalog", len(brief))
	}
}

// TestStateBriefIsEmptyAndBounded pins the brief's shape at the two extremes:
// an empty home yields an "empty log" line, and a home with many events still
// produces a brief far smaller than the raw log — O(state), not O(history),
// and crucially contains NO event-log digest, because the brief is pure
// orientation: where the mind is, what exists, where to look for the rest.
func TestStateBriefIsEmptyAndBounded(t *testing.T) {
	empty := t.TempDir()
	if b := stateBrief(empty); !strings.Contains(b, "Empty log") {
		t.Fatalf("empty home brief = %q", b)
	}

	home := t.TempDir()
	decl := newEvent("command.declared", json.RawMessage(`{"name":"note","description":"take a note","event":{"name":"note.taken","fields":{"title":"string"}}}`))
	if err := appendEvent(home, &decl); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 500; i++ {
		e := newEvent("note.taken", json.RawMessage(`{"title":"a note with a reasonably long title to make the log meaty"}`))
		if err := appendEvent(home, &e); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(home, "events.jsonl"))
	brief := stateBrief(home)
	if len(brief) >= len(raw) {
		t.Fatalf("brief (%d) not smaller than the raw log (%d) — not O(state)", len(brief), len(raw))
	}
	// the orientation brief has NO event-log digest — no `seq` lines at all.
	// the mind is pointed at events.jsonl if it needs the raw material.
	if strings.Contains(brief, "seq ") {
		t.Fatalf("brief contains a seq digest — not pure orientation:\n%s", brief)
	}
}

// ────────────────────── files: bytes in the store, hashes in the log ─────────

// TestLastSeqScansOnlyTheTail pins O(1) append: the next sequence number comes
// from the log's last line alone — including when that line is bigger than one
// backward-scan chunk — with no sidecar state to drift.
func TestLastSeqScansOnlyTheTail(t *testing.T) {
	home := t.TempDir()
	for i := 0; i < 3; i++ {
		e := newEvent("tick", json.RawMessage(`{}`))
		if err := appendEvent(home, &e); err != nil {
			t.Fatal(err)
		}
	}
	big := newEvent("blob.of.text", json.RawMessage(`{"note":"`+strings.Repeat("x", 100_000)+`"}`))
	if err := appendEvent(home, &big); err != nil {
		t.Fatal(err)
	}
	after := newEvent("tick", json.RawMessage(`{}`))
	if err := appendEvent(home, &after); err != nil {
		t.Fatal(err)
	}
	if after.Seq != 5 {
		t.Fatalf("seq after oversized line = %d, want 5", after.Seq)
	}
	if n, err := lastSeq(home); n != 5 || err != nil {
		t.Fatalf("lastSeq = %d, %v", n, err)
	}
}

// TestProjectorStdinIsFilteredByConsumes pins the operative half of a
// projector declaration: the kernel feeds a projector ONLY the events its
// consumes list names, so the script never filters. Empty consumes still
// means the whole log.
func TestProjectorStdinIsFilteredByConsumes(t *testing.T) {
	home := t.TempDir()
	decl := newEvent("projector.declared", json.RawMessage(`{"name":"picky","description":"d","consumes":["a.happened"]}`))
	if err := appendEvent(home, &decl); err != nil {
		t.Fatal(err)
	}
	echo := "#!/bin/sh\necho '<pre>'\ncat\necho '</pre>'\n"
	if err := installTrustedScript(home, "projector", "picky", echo, "test"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"name":"a.happened","payload":{"t":"AAA"}}`, `{"name":"b.happened","payload":{"t":"BBB"}}`} {
		var e Event
		json.Unmarshal([]byte(raw), &e)
		fresh := newEvent(e.Name, e.Payload)
		if err := appendEvent(home, &fresh); err != nil {
			t.Fatal(err)
		}
	}
	page, err := runProjection(home, "picky")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "AAA") || strings.Contains(string(page), "BBB") {
		t.Fatalf("filtered stdin is wrong:\n%s", page)
	}

	wide := newEvent("projector.declared", json.RawMessage(`{"name":"wide","description":"d","consumes":[]}`))
	if err := appendEvent(home, &wide); err != nil {
		t.Fatal(err)
	}
	if err := installTrustedScript(home, "projector", "wide", echo, "test"); err != nil {
		t.Fatal(err)
	}
	page, err = runProjection(home, "wide")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "AAA") || !strings.Contains(string(page), "BBB") {
		t.Fatalf("empty consumes must still feed everything:\n%s", page)
	}
}

// TestSelectiveRefreshSkipsUnconsumingProjections pins the write-path win: an
// ingest re-runs only the projections consuming what just landed, and marks
// the skipped ones verified-fresh so the server keeps serving their
// materialized pages. The projector subprocess is the cost; not paying it for
// unrelated events is what keeps a many-page instance fast at 100k events.
func TestSelectiveRefreshSkipsUnconsumingProjections(t *testing.T) {
	home := t.TempDir()
	for name, consumes := range map[string]string{"xview": `["x.happened"]`, "yview": `["y.happened"]`} {
		decl := newEvent("projector.declared", json.RawMessage(`{"name":"`+name+`","description":"d","consumes":`+consumes+`}`))
		if err := appendEvent(home, &decl); err != nil {
			t.Fatal(err)
		}
		// A run-counter script: impure on purpose, so the test can SEE re-runs.
		script := "#!/bin/sh\necho run >> \"$SELF_HOME/.runs-" + name + "\"\necho '<p>ok</p>'\n"
		if err := installTrustedScript(home, "projector", name, script, "test"); err != nil {
			t.Fatal(err)
		}
	}
	runs := func(name string) int {
		data, _ := os.ReadFile(filepath.Join(home, ".runs-"+name))
		return strings.Count(string(data), "run")
	}
	refreshSite(home) // materialize both once
	if runs("xview") != 1 || runs("yview") != 1 {
		t.Fatalf("full refresh runs = %d/%d, want 1/1", runs("xview"), runs("yview"))
	}
	if err := ingest(home, []Event{newEvent("x.happened", json.RawMessage(`{}`))}); err != nil {
		t.Fatal(err)
	}
	if runs("xview") != 2 {
		t.Fatalf("xview runs = %d, want 2 — its event arrived", runs("xview"))
	}
	if runs("yview") != 1 {
		t.Fatalf("yview runs = %d, want 1 — nothing it consumes arrived", runs("yview"))
	}
	// the skipped page was verified fresh: the server may serve it as-is.
	if freshSitePage(home, "yview") == nil {
		t.Fatal("skipped projection is not marked fresh — every GET would replay it")
	}
	// a capability event refreshes everything: the projector set changed.
	if err := ingest(home, []Event{newEvent("capability.retired", json.RawMessage(`{"type":"command","name":"nonexistent"}`))}); err != nil {
		t.Fatal(err)
	}
	if runs("yview") != 2 {
		t.Fatalf("yview runs = %d, want 2 — capability lifecycle refreshes all", runs("yview"))
	}
}

// TestFreshSitePageTracksTheLog pins the freshness rule: a materialized page
// serves only when its mtime postdates the log's last append; anything else
// falls back to a live replay. Pure filesystem, no cursor files — a forgotten
// refresh degrades to a slower page, never a stale one.
func TestFreshSitePageTracksTheLog(t *testing.T) {
	home := t.TempDir()
	decl := newEvent("projector.declared", json.RawMessage(`{"name":"board","description":"d","consumes":["note.taken"]}`))
	if err := appendEvent(home, &decl); err != nil {
		t.Fatal(err)
	}
	if err := installTrustedScript(home, "projector", "board", "#!/bin/sh\necho '<p>ok</p>'\n", "test"); err != nil {
		t.Fatal(err)
	}
	refreshSite(home)
	if freshSitePage(home, "board") == nil {
		t.Fatal("just-rendered page must be fresh")
	}
	// an append the renderer never saw — e.g. a reflection outside ingest
	hb := newEvent("self.reflected", json.RawMessage(`{}`))
	if err := appendEvent(home, &hb); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(logPath(home), future, future) // make the ordering unambiguous on any filesystem
	if freshSitePage(home, "board") != nil {
		t.Fatal("page older than the log must not serve as fresh")
	}
}

// TestGiveLearnRoundTrip pins the account round trip: give writes the
// selected events verbatim with a manifest attesting to them; learn deposits
// them in another instance with their own moments intact, and its
// lesson.learned receipt attests to the same digest the manifest claimed.
func TestGiveLearnRoundTrip(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	giver := t.TempDir()
	past := time.Date(2024, 3, 9, 12, 30, 0, 0, time.UTC)
	for i, text := range []string{"low tide at dawn", "nest three hatched"} {
		e := newEvent("note.taken", json.RawMessage(`{"title":"`+text+`"}`))
		e.OccurredAt = past.Add(time.Duration(i) * time.Hour)
		if err := appendEvent(giver, &e); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "notes")
	if err := cmdGive(giver, "note.", dir); err != nil {
		t.Fatal(err)
	}

	// the account is complete: record, manifest with a true digest, intent stub
	if !fileExists(filepath.Join(dir, "record.jsonl")) {
		t.Fatal("give wrote no record")
	}
	var m manifest
	mb, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err := json.Unmarshal(mb, &m); err != nil {
		t.Fatal(err)
	}
	if m.Events != 2 || m.Prefix != "note." {
		t.Fatalf("manifest = %+v, want 2 events with prefix note.", m)
	}
	if !fileExists(filepath.Join(dir, "intent.md")) {
		t.Fatal("give wrote no intent stub")
	}
	// the giving is remembered
	events, _ := readEvents(giver)
	gave := false
	for _, e := range events {
		if e.Name == "account.given" {
			gave = true
		}
	}
	if !gave {
		t.Fatal("give left no account.given event in the giver's log")
	}

	receiver := t.TempDir()
	if err := cmdLearn(receiver, dir); err != nil {
		t.Fatal(err)
	}
	events, _ = readEvents(receiver)
	deposited := 0
	var learned struct {
		Events         int    `json:"events"`
		RecordSha256   string `json:"record_sha256"`
		ManifestSha256 string `json:"manifest_sha256"`
	}
	for _, e := range events {
		if e.Name == "note.taken" {
			if !e.OccurredAt.Equal(past) && !e.OccurredAt.Equal(past.Add(time.Hour)) {
				t.Fatalf("deposited event lost its moment: %s", e.OccurredAt)
			}
			deposited++
		}
		if e.Name == "lesson.learned" {
			if err := json.Unmarshal(e.Payload, &learned); err != nil {
				t.Fatal(err)
			}
		}
	}
	if deposited != 2 {
		t.Fatalf("deposited %d events, want 2", deposited)
	}
	if learned.Events != 2 {
		t.Fatalf("lesson.learned events = %d, want 2", learned.Events)
	}
	if learned.RecordSha256 == "" || learned.RecordSha256 != m.RecordSha256 || learned.ManifestSha256 != m.RecordSha256 {
		t.Fatalf("digests do not agree: learned %q/%q vs manifest %q", learned.RecordSha256, learned.ManifestSha256, m.RecordSha256)
	}
}

// TestLearnRefusesKernelVocabulary pins the receiver's gate: the kernel's own
// vocabulary never travels raw, so a hostile record that tries to speak it —
// here, depositing a script.compiled — is refused before anything is appended.
func TestLearnRefusesKernelVocabulary(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	dir := filepath.Join(t.TempDir(), "hostile")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "intent.md"), []byte("a friendly account"), 0644); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(receipt{"command", "evil", "#!/bin/sh\necho pwned", "attacker", "deadbeef"})
	if err := os.WriteFile(filepath.Join(dir, "record.jsonl"),
		[]byte(`{"name":"script.compiled","payload":`+string(payload)+"}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := cmdLearn(home, dir); err == nil {
		t.Fatal("learn accepted a record speaking the kernel's vocabulary")
	}
	if events, _ := readEvents(home); len(events) != 0 {
		t.Fatalf("refused learn still appended %d event(s)", len(events))
	}
}

// TestGiveCapabilityAsLineage pins the capability flavor: give renames the
// declarations and receipts to lineage.*, learn deposits them as inert
// evidence, and the only thing that installs is what the receiver's own
// mind declared, under the receiver's own key. Foreign bytes never install.
func TestGiveCapabilityAsLineage(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	giver := t.TempDir()
	decl := newEvent("command.declared", json.RawMessage(
		`{"name":"note","description":"take a note","params":{"text":"string"},"event":{"name":"note.taken","fields":{"title":"string"}}}`))
	if err := ingest(giver, []Event{decl}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "gift")
	if err := cmdGive(giver, "command/note", dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "record.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"lineage.command.declared"`) ||
		!strings.Contains(string(raw), `"lineage.script.compiled"`) {
		t.Fatalf("capability account does not carry its history as lineage:\n%s", raw)
	}
	if strings.Contains(string(raw), `"name":"script.compiled"`) {
		t.Fatal("a raw script.compiled left the giver")
	}

	receiver := t.TempDir()
	if err := cmdLearn(receiver, dir); err != nil {
		t.Fatal(err)
	}
	// the giver's capability name never installed by itself; whatever the
	// receiver's mind declared is signed by the receiver's key alone
	if p, _ := scriptPath(receiver, "command", "note"); fileExists(p) {
		t.Fatal("the foreign declaration installed without the receiver's mind declaring it")
	}
	secret, err := loadSecret(receiver)
	if err != nil {
		t.Fatal(err)
	}
	events, _ := readEvents(receiver)
	receipts := 0
	for _, e := range events {
		if e.Name != "script.compiled" {
			continue
		}
		if _, ok := verifiedReceipt(secret, e.Payload); !ok {
			t.Fatalf("seq %d: a receipt in the receiver's log does not verify under its key", e.Seq)
		}
		receipts++
	}
	if receipts == 0 {
		t.Fatal("the receiver's mind declared nothing — the lesson did not take")
	}
}

// TestLearnRecordsInterventionDigest pins intervention visibility: editing an
// account between giving and learning is not forbidden — it is the receiver's
// (or a curator's) move — but the lesson.learned receipt carries both the
// manifest's claim and the digest of what was actually deposited, so the edit
// is visible forever.
func TestLearnRecordsInterventionDigest(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	giver := t.TempDir()
	for _, text := range []string{"keep this", "redact this"} {
		e := newEvent("note.taken", json.RawMessage(`{"title":"`+text+`"}`))
		if err := appendEvent(giver, &e); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "curated")
	if err := cmdGive(giver, "note.", dir); err != nil {
		t.Fatal(err)
	}
	// the intervention: one line of the record is removed before learning
	raw, _ := os.ReadFile(filepath.Join(dir, "record.jsonl"))
	lines := strings.SplitN(strings.TrimSpace(string(raw)), "\n", 2)
	if err := os.WriteFile(filepath.Join(dir, "record.jsonl"), []byte(lines[0]+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	receiver := t.TempDir()
	if err := cmdLearn(receiver, dir); err != nil {
		t.Fatal(err)
	}
	events, _ := readEvents(receiver)
	var learned struct {
		RecordSha256   string `json:"record_sha256"`
		ManifestSha256 string `json:"manifest_sha256"`
	}
	for _, e := range events {
		if e.Name == "lesson.learned" {
			if err := json.Unmarshal(e.Payload, &learned); err != nil {
				t.Fatal(err)
			}
		}
	}
	if learned.RecordSha256 == "" || learned.ManifestSha256 == "" {
		t.Fatal("lesson.learned does not carry both digests")
	}
	if learned.RecordSha256 == learned.ManifestSha256 {
		t.Fatal("an edited record still matches the manifest — the intervention is invisible")
	}
}

// TestProvenanceDoorStamped pins the door rule: via records the channel the
// kernel itself witnessed, stamped at append time — a script that emits its
// own via/by is claiming a door, and doors are not claimable. by carries the
// caller's claim verbatim.
func TestProvenanceDoorStamped(t *testing.T) {
	home := t.TempDir()
	script := "#!/bin/sh\necho '{\"name\":\"fact.stated\",\"via\":\"kernel\",\"by\":\"forged\",\"payload\":{\"text\":\"hello\"}}'\n"
	if err := installTrustedScript(home, "command", "state", script, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCommand(home, "state", nil, "cli", "alice"); err != nil {
		t.Fatal(err)
	}
	events, _ := readEvents(home)
	found := false
	for _, e := range events {
		if e.Name != "fact.stated" {
			continue
		}
		found = true
		if e.Via != "cli" {
			t.Fatalf("via = %q, want %q — a script set its own door", e.Via, "cli")
		}
		if e.By != "alice" {
			t.Fatalf("by = %q, want the caller's claim %q", e.By, "alice")
		}
	}
	if !found {
		t.Fatal("the command's event did not land")
	}
}

// TestProvenanceHTTPDoor pins the second door: a form POST lands with
// via http:<remote-addr> and the X-Self-Caller header recorded verbatim as
// the claimed speaker.
func TestProvenanceHTTPDoor(t *testing.T) {
	home := t.TempDir()
	script := "#!/bin/sh\nprintf '{\"name\":\"fact.stated\",\"payload\":{\"text\":\"%s\"}}\\n' \"$1\"\n"
	if err := installTrustedScript(home, "command", "state", script, "tester"); err != nil {
		t.Fatal(err)
	}
	mux := serveMux(home)
	req := httptest.NewRequest("POST", "/run/state", strings.NewReader("text=from+the+web"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Self-Caller", "claude-main")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("POST /run/state = %d: %s", w.Code, w.Body.String())
	}
	events, _ := readEvents(home)
	found := false
	for _, e := range events {
		if e.Name != "fact.stated" {
			continue
		}
		found = true
		if !strings.HasPrefix(e.Via, "http:") {
			t.Fatalf("via = %q, want an http:<addr> door", e.Via)
		}
		if e.By != "claude-main" {
			t.Fatalf("by = %q, want the header's claim %q", e.By, "claude-main")
		}
	}
	if !found {
		t.Fatal("the form's event did not land")
	}
}

// TestDepositProvenance pins the travel rule: by is portable like
// occurred_at — testimony keeps its speaker across bodies — while via is
// local like seq, so whatever door a record claims, the deposit here is
// stamped learn:<account>. The learn's own receipts carry their doors too:
// the mind's declarations enter mind:*, the attestation is the kernel's.
func TestDepositProvenance(t *testing.T) {
	t.Setenv("SELF_MIND", stubMind(t))
	giver := t.TempDir()
	e := newEvent("note.taken", json.RawMessage(`{"title":"low tide at dawn"}`))
	e.Via, e.By = "http:10.0.0.7:9999", "giver-mind"
	if err := appendEvent(giver, &e); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "notes")
	if err := cmdGive(giver, "note.", dir); err != nil {
		t.Fatal(err)
	}
	receiver := t.TempDir()
	if err := cmdLearn(receiver, dir); err != nil {
		t.Fatal(err)
	}
	events, _ := readEvents(receiver)
	deposited, attested, declared := false, false, false
	for _, ev := range events {
		switch ev.Name {
		case "note.taken":
			deposited = true
			if ev.By != "giver-mind" {
				t.Fatalf("deposited by = %q — the speaker did not travel", ev.By)
			}
			if ev.Via != "learn:notes" {
				t.Fatalf("deposited via = %q, want %q — a foreign door was inherited", ev.Via, "learn:notes")
			}
		case "lesson.learned":
			attested = true
			if ev.Via != "kernel" {
				t.Fatalf("lesson.learned via = %q, want kernel", ev.Via)
			}
		case "command.declared", "projector.declared":
			declared = true
			if !strings.HasPrefix(ev.Via, "mind:") {
				t.Fatalf("declaration via = %q, want a mind:* door", ev.Via)
			}
		}
	}
	if !deposited || !attested || !declared {
		t.Fatalf("missing events: deposited=%v attested=%v declared=%v", deposited, attested, declared)
	}
}

// TestVocabularySpeaksMind pins the nomenclature: the process plugged through
// SELF_MIND is a MIND, everywhere — code, docs, prompts, examples. The old
// word was renamed away more than once and kept creeping back through
// generated code and fresh docs, so the invariant lives here with the other
// pinned properties. The forbidden word is spelled in halves so this test
// does not trip itself.
func TestVocabularySpeaksMind(t *testing.T) {
	old := "br" + "ain"
	skipDirs := map[string]bool{".git": true, ".self": true, "site": true,
		"capabilities": true, "__pycache__": true}
	skipFiles := map[string]bool{"events.jsonl": true, ".secret": true, "self": true}
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if skipFiles[d.Name()] {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > 1<<20 {
			return nil // oversized or unreadable: not vocabulary
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return nil // binary, not vocabulary
		}
		for i, line := range strings.Split(strings.ToLower(string(data)), "\n") {
			if strings.Contains(line, old) {
				t.Errorf("%s:%d speaks %q — the word is mind", path, i+1, old)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
