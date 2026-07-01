package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// TestStrangeLoop drives the whole kernel loop offline: a declaration arrives
// as an event, the (stub) compiler turns it into an installed script with a
// signed receipt, running the command appends its event, and the projection
// re-renders to site/ showing it. This is the spirit in one test.
func TestStrangeLoop(t *testing.T) {
	t.Setenv("SELF_LLM_STUB", "1")
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
		filepath.Join(home, "capabilities", "commands", "note"),
		filepath.Join(home, "capabilities", "projectors", "board"),
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
	if _, err := runCommand(home, "note", []string{"water", "the", "garden"}); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(home, "site", "board.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "water the garden") {
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
	payload, _ := json.Marshal(receipt{"command", "evil", "#!/bin/sh\necho pwned", "deadbeef"})
	e := newEvent("script.compiled", payload)
	if err := appendEvent(home, &e); err != nil {
		t.Fatal(err)
	}
	if err := rehydrate(home); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(home, "capabilities", "commands", "evil")) {
		t.Fatal("a forged receipt installed")
	}
}

// TestGardenRehydrates resurrects the committed body in garden/ — a real
// organism stored as just events.jsonl + .secret — and checks that every organ
// it grew comes back from the log alone. If this passes, the minimal kernel
// still carries the spirit.
func TestGardenRehydrates(t *testing.T) {
	home := t.TempDir()
	for _, f := range []string{"events.jsonl", ".secret"} {
		data, err := os.ReadFile(filepath.Join("garden", f))
		if err != nil {
			t.Fatalf("the garden body is missing: %s", err)
		}
		if err := os.WriteFile(filepath.Join(home, f), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := rehydrate(home); err != nil {
		t.Fatal(err)
	}
	organs := map[string][]string{
		"commands":   {"note", "claim", "verify", "bequeath", "awaken", "wonder", "resolve", "weigh"},
		"projectors": {"chronicle", "pulse", "notes", "ledger", "inheritance", "lineage", "questions", "toll"},
	}
	for kind, names := range organs {
		for _, name := range names {
			if !fileExists(filepath.Join(home, "capabilities", kind, name)) {
				t.Errorf("garden %s %q did not rehydrate", strings.TrimSuffix(kind, "s"), name)
			}
		}
	}
	// The projections replayed too — the previous mind's letter is readable.
	page, err := os.ReadFile(filepath.Join(home, "site", "inheritance.html"))
	if err != nil {
		t.Fatalf("inheritance did not render: %s", err)
	}
	if len(page) == 0 {
		t.Fatal("inheritance rendered empty")
	}
}
