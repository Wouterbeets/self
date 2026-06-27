package kernel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"self/internal/event"
)

func TestRenderAndReadWiring(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "site"), 0755)

	// Write a minimal events.jsonl with command.declared, projector.declared,
	// and seed.planted events.
	events := `{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{"version":"self/v0"}}
{"id":"b","seq":2,"name":"command.declared","occurred_at":"2026-01-01T00:00:00Z","payload":{"name":"cal-add","description":"Add event","params":{"date":"string"},"event":{"name":"calendar.event.added","fields":{"event_id":"string"}}}}
{"id":"c","seq":3,"name":"command.declared","occurred_at":"2026-01-01T00:00:00Z","payload":{"name":"cal-del","description":"Delete event","params":{"event_id":"string"},"event":{"name":"calendar.event.deleted","fields":{"event_id":"string"}}}}
{"id":"d","seq":4,"name":"projector.declared","occurred_at":"2026-01-01T00:00:00Z","payload":{"name":"calendar","description":"Month view","consumes":["calendar.event.added","calendar.event.edited","calendar.event.deleted"]}}
{"id":"e","seq":5,"name":"seed.planted","occurred_at":"2026-01-01T00:00:00Z","payload":{"seed":"calendar","commands":["cal-add","cal-del"],"projectors":["calendar"]}}
`
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(events), 0644)

	if err := RenderHTML(home); err != nil {
		t.Fatal(err)
	}

	// kernel.html should exist
	path := filepath.Join(home, "site", "kernel.html")
	if _, err := os.Stat(path); err != nil {
		t.Fatal("kernel.html not written")
	}

	// Read wiring back
	w, err := ReadWiring(home)
	if err != nil {
		t.Fatal(err)
	}

	// calendar projector consumes 3 events
	addProjectors := w.ProjectorsForEvent("calendar.event.added")
	if len(addProjectors) != 1 || addProjectors[0] != "calendar" {
		t.Errorf("calendar.event.added -> %v, want [calendar]", addProjectors)
	}
	delProjectors := w.ProjectorsForEvent("calendar.event.deleted")
	if len(delProjectors) != 1 || delProjectors[0] != "calendar" {
		t.Errorf("calendar.event.deleted -> %v, want [calendar]", delProjectors)
	}

	// Unknown event returns empty
	unknown := w.ProjectorsForEvent("unknown.event")
	if len(unknown) != 0 {
		t.Errorf("unknown.event -> %v, want []", unknown)
	}
}

func TestReadWiringMissingFile(t *testing.T) {
	home := t.TempDir()
	w, err := ReadWiring(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.ProjectorsByEvent) != 0 {
		t.Error("missing kernel.html should return empty wiring")
	}
}

func TestRenderHTMLEmptyKernel(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "site"), 0755)
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(`{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{"version":"self/v0"}}
`), 0644)

	if err := RenderHTML(home); err != nil {
		t.Fatal(err)
	}

	w, err := ReadWiring(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.ProjectorsByEvent) != 0 {
		t.Error("empty kernel should have no wiring")
	}
}

func TestCompileDeclarationsStub(t *testing.T) {
	// Force stub mode so we don't need an LLM API key.
	os.Setenv("SELF_LLM_STUB", "1")
	t.Cleanup(func() { os.Unsetenv("SELF_LLM_STUB") })

	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "capabilities", "commands"), 0755)
	os.MkdirAll(filepath.Join(home, "capabilities", "projectors"), 0755)
	os.MkdirAll(filepath.Join(home, "site"), 0755)

	// Minimal events.jsonl so RenderHTML has something to read.
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(
		`{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{"version":"self/v0"}}
`), 0644)

	// Simulate events a command might emit — including a declaration.
	cmdPayload, _ := json.Marshal(map[string]any{
		"name":        "summarize",
		"description": "summarize a note",
		"params":      map[string]string{"text": "string"},
		"event": map[string]any{
			"name":   "summary.generated",
			"fields": map[string]string{"text": "string"},
		},
	})
	projPayload, _ := json.Marshal(map[string]any{
		"name":        "summaries",
		"description": "render summaries",
		"consumes":    []string{"summary.generated"},
	})

	events := []event.Event{
		event.New(event.CommandDeclared, cmdPayload),
		event.New(event.ProjectorDeclared, projPayload),
	}

	// In the real flow, cmdInvoke appends events to the store before
	// calling CompileDeclarations. Simulate that here so RenderHTML
	// sees the declarations when it re-reads events.jsonl.
	storeData, _ := os.ReadFile(filepath.Join(home, "events.jsonl"))
	for _, e := range events {
		line, _ := json.Marshal(e)
		storeData = append(storeData, append(line, '\n')...)
	}
	os.WriteFile(filepath.Join(home, "events.jsonl"), storeData, 0644)

	cmds, projs, err := CompileDeclarations(home, events)
	if err != nil {
		t.Fatalf("CompileDeclarations: %v", err)
	}
	if len(cmds) != 1 || cmds[0] != "summarize" {
		t.Errorf("commands = %v, want [summarize]", cmds)
	}
	if len(projs) != 1 || projs[0] != "summaries" {
		t.Errorf("projectors = %v, want [summaries]", projs)
	}

	// Scripts should exist in the registry.
	if _, err := os.Stat(filepath.Join(home, "capabilities", "commands", "summarize")); err != nil {
		t.Error("summarize command script not written")
	}
	if _, err := os.Stat(filepath.Join(home, "capabilities", "projectors", "summaries")); err != nil {
		t.Error("summaries projector script not written")
	}

	// kernel.html should be re-rendered with the new wiring.
	w, err := ReadWiring(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := w.ProjectorsForEvent("summary.generated"); len(got) != 1 || got[0] != "summaries" {
		t.Errorf("wiring after compile: summary.generated -> %v, want [summaries]", got)
	}
}

func TestCompileDeclarationsIgnoresScriptCompiled(t *testing.T) {
	os.Setenv("SELF_LLM_STUB", "1")
	t.Cleanup(func() { os.Unsetenv("SELF_LLM_STUB") })

	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "capabilities", "commands"), 0755)
	os.MkdirAll(filepath.Join(home, "site"), 0755)
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(
		`{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{"version":"self/v0"}}
`), 0644)

	// script.compiled is kernel-only. CompileDeclarations must NOT install code
	// from it — that path was removed precisely because it ran foreign bytes with
	// no compile and no adaptation. The kernel only compiles declarations.
	exact := "#!/usr/bin/env python3\nprint('pwned')\n"
	payload, _ := json.Marshal(map[string]any{"type": "command", "name": "ping", "script": exact})
	events := []event.Event{event.New(event.ScriptCompiled, payload)}

	cmds, projs, err := CompileDeclarations(home, events)
	if err != nil {
		t.Fatalf("CompileDeclarations: %v", err)
	}
	if len(cmds) != 0 || len(projs) != 0 {
		t.Fatalf("cmds=%v projs=%v, want none — script.compiled must not install code", cmds, projs)
	}
	if _, err := os.Stat(filepath.Join(home, "capabilities", "commands", "ping")); err == nil {
		t.Error("a script.compiled event installed code — the reserve was breached")
	}
}

func TestRestoreRollsBackAndPinsBySeq(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "capabilities", "commands"), 0755)
	os.MkdirAll(filepath.Join(home, "site"), 0755)
	v1 := "#!/bin/sh\necho v1\n"
	v2 := "#!/bin/sh\necho v2\n"
	r1 := mustReceipt(t, home, "command", "greet", v1)
	r2 := mustReceipt(t, home, "command", "greet", v2)
	// A log with two kernel-signed receipts for greet (v1 at seq 2, v2 at seq 3);
	// the registry currently holds v2.
	log := `{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{}}
{"id":"b","seq":2,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + r1 + `}
{"id":"c","seq":3,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + r2 + `}
`
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(log), 0644)
	os.WriteFile(filepath.Join(home, "capabilities", "commands", "greet"), []byte(v2), 0755)
	path := filepath.Join(home, "capabilities", "commands", "greet")

	// rollback one → v1
	if seq, kind, err := Restore(home, "greet", 0); err != nil || seq != 2 || kind != "command" {
		t.Fatalf("Restore rollback: seq=%d kind=%q err=%v, want 2 command nil", seq, kind, err)
	}
	if b, _ := os.ReadFile(path); string(b) != v1 {
		t.Errorf("after rollback, greet = %q, want v1", b)
	}

	// pin by seq → v2 again (the restore appended a receipt, so seq 3 still exists)
	if seq, _, err := Restore(home, "greet", 3); err != nil || seq != 3 {
		t.Fatalf("Restore by seq: seq=%d err=%v, want 3 nil", seq, err)
	}
	if b, _ := os.ReadFile(path); string(b) != v2 {
		t.Errorf("after restore-by-seq, greet = %q, want v2", b)
	}
}

func TestApplyRestoresFromEvent(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "capabilities", "commands"), 0755)
	os.MkdirAll(filepath.Join(home, "site"), 0755)
	v1 := "#!/bin/sh\necho v1\n"
	v2 := "#!/bin/sh\necho v2\n"
	r1 := mustReceipt(t, home, "command", "greet", v1)
	r2 := mustReceipt(t, home, "command", "greet", v2)
	log := `{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{}}
{"id":"b","seq":2,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + r1 + `}
{"id":"c","seq":3,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + r2 + `}
`
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(log), 0644)
	os.WriteFile(filepath.Join(home, "capabilities", "commands", "greet"), []byte(v2), 0755)

	// A data-only restore.requested intent drives the install through the hook.
	payload, _ := json.Marshal(map[string]any{"name": "greet", "seq": 0})
	restored, err := ApplyRestores(home, []event.Event{event.New(event.RestoreRequested, payload)})
	if err != nil {
		t.Fatalf("ApplyRestores: %v", err)
	}
	if len(restored) != 1 || restored[0] != "greet" {
		t.Fatalf("restored = %v, want [greet]", restored)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "capabilities", "commands", "greet")); string(b) != v1 {
		t.Errorf("after restore.requested, greet = %q, want v1", b)
	}
}

func TestRestoreErrors(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "capabilities", "commands"), 0755)
	os.MkdirAll(filepath.Join(home, "site"), 0755)
	r1 := mustReceipt(t, home, "command", "solo", "#!/bin/sh\n:\n")
	log := `{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{}}
{"id":"b","seq":2,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + r1 + `}
`
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(log), 0644)

	if _, _, err := Restore(home, "ghost", 0); err == nil {
		t.Error("restoring an unknown name should error")
	}
	if _, _, err := Restore(home, "solo", 0); err == nil {
		t.Error("rolling back a single-version capability should error")
	}
}

func TestRehydrateRebuildsFromLog(t *testing.T) {
	home := t.TempDir()
	// Receipts only — no capabilities/ on disk, as in a home that is just the log.
	greetV1 := "#!/bin/sh\necho v1\n"
	greetV2 := "#!/bin/sh\necho v2\n"
	board := "#!/bin/sh\necho board\n"
	rg1 := mustReceipt(t, home, "command", "greet", greetV1)
	rg2 := mustReceipt(t, home, "command", "greet", greetV2)
	rb := mustReceipt(t, home, "projector", "board", board)
	log := `{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{}}
{"id":"b","seq":2,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + rg1 + `}
{"id":"c","seq":3,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + rb + `}
{"id":"d","seq":4,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + rg2 + `}
`
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(log), 0644)

	commands, projectors, err := Rehydrate(home)
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	if len(commands) != 1 || commands[0] != "greet" {
		t.Errorf("commands = %v, want [greet]", commands)
	}
	if len(projectors) != 1 || projectors[0] != "board" {
		t.Errorf("projectors = %v, want [board]", projectors)
	}
	// The latest receipt for a name wins (greet v2, not v1).
	if b, _ := os.ReadFile(filepath.Join(home, "capabilities", "commands", "greet")); string(b) != greetV2 {
		t.Errorf("greet = %q, want v2 (latest receipt)", b)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "capabilities", "projectors", "board")); string(b) != board {
		t.Errorf("board = %q, want %q", b, board)
	}
}

func TestRehydrateForeignKeyInstallsNothing(t *testing.T) {
	// Receipts signed by one home must not install into another (a different
	// .secret): you inherit a node's declarations, not its key. This is the same
	// signature gate Restore uses, so Rehydrate opens no arbitrary-code path.
	signer := t.TempDir()
	r := mustReceipt(t, signer, "command", "greet", "#!/bin/sh\necho hi\n")
	other := t.TempDir() // a fresh home → a fresh, non-matching key
	log := `{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{}}
{"id":"b","seq":2,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + r + `}
`
	os.WriteFile(filepath.Join(other, "events.jsonl"), []byte(log), 0644)

	commands, projectors, err := Rehydrate(other)
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	if len(commands) != 0 || len(projectors) != 0 {
		t.Errorf("foreign-key rehydrate installed %v / %v, want nothing", commands, projectors)
	}
	if _, statErr := os.Stat(filepath.Join(other, "capabilities", "commands", "greet")); statErr == nil {
		t.Error("greet was installed under a non-signing key — the signature gate failed")
	}
}

// mustReceipt builds a kernel-signed script.compiled payload for home.
func mustReceipt(t *testing.T, home, typ, name, script string) string {
	t.Helper()
	p, err := SignedReceipt(home, typ, name, script)
	if err != nil {
		t.Fatal(err)
	}
	return string(p)
}

func TestSignVerifyRoundTrip(t *testing.T) {
	home := t.TempDir()
	secret, err := loadOrCreateSecret(home)
	if err != nil {
		t.Fatal(err)
	}
	good, _ := SignedReceipt(home, "command", "greet", "echo hi")
	if !verifyReceipt(secret, good) {
		t.Error("a freshly signed receipt should verify")
	}
	// Tampered bytes — same name, different script — must not verify.
	var r compiledReceipt
	json.Unmarshal(good, &r)
	r.Script = "echo PWNED"
	bad, _ := json.Marshal(r)
	if verifyReceipt(secret, bad) {
		t.Error("tampering with the script must invalidate the signature")
	}
	// A different home's key must not verify our receipt.
	other, _ := loadOrCreateSecret(t.TempDir())
	if verifyReceipt(other, good) {
		t.Error("another home's key must not verify this home's receipt")
	}
}

func TestRestoreIgnoresForgedReceipt(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "capabilities", "commands"), 0755)
	os.MkdirAll(filepath.Join(home, "site"), 0755)
	v1 := "#!/bin/sh\necho v1\n"
	v2 := "#!/bin/sh\necho v2\n"
	r1 := mustReceipt(t, home, "command", "greet", v1) // seq 2, signed
	r2 := mustReceipt(t, home, "command", "greet", v2) // seq 3, signed
	// A forged receipt at the HIGHEST seq, with a bogus signature — a command
	// could append exactly this. It must be invisible to restore.
	forged := `{"type":"command","name":"greet","script":"#!/bin/sh\necho PWNED\n","sig":"deadbeef"}`
	log := `{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{}}
{"id":"b","seq":2,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + r1 + `}
{"id":"c","seq":3,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + r2 + `}
{"id":"d","seq":4,"name":"script.compiled","occurred_at":"2026-01-01T00:00:00Z","payload":` + forged + `}
`
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(log), 0644)
	os.WriteFile(filepath.Join(home, "capabilities", "commands", "greet"), []byte(v2), 0755)

	// rollback one: the forged seq-4 is ignored, so "previous" is v1 (seq 2).
	seq, _, err := Restore(home, "greet", 0)
	if err != nil || seq != 2 {
		t.Fatalf("rollback: seq=%d err=%v, want 2 nil (forged receipt must be ignored)", seq, err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "capabilities", "commands", "greet")); string(b) != v1 {
		t.Errorf("greet = %q, want v1 — forged bytes must never install", b)
	}
	// And pinning the forged seq directly must be refused.
	if _, _, err := Restore(home, "greet", 4); err == nil {
		t.Error("restoring a forged (unsigned) receipt by seq must error")
	}
}

func TestInstallScriptRejectsUnsafeName(t *testing.T) {
	dir := t.TempDir()
	evil, _ := json.Marshal(map[string]any{"type": "command", "name": "../escape", "script": "x"})
	if _, _, err := installScript(dir, evil); err == nil {
		t.Error("unsafe name should be rejected even on the kernel-internal install path")
	}
}

func TestCompileDeclarationsNoDeclarations(t *testing.T) {
	os.Setenv("SELF_LLM_STUB", "1")
	t.Cleanup(func() { os.Unsetenv("SELF_LLM_STUB") })

	home := t.TempDir()

	// Events with no declarations — should be a no-op.
	events := []event.Event{
		event.New("chat.message", json.RawMessage(`{"role":"user","content":"hi"}`)),
	}
	cmds, projs, err := CompileDeclarations(home, events)
	if err != nil {
		t.Fatalf("CompileDeclarations: %v", err)
	}
	if len(cmds) != 0 || len(projs) != 0 {
		t.Errorf("expected no compilations, got cmds=%v projs=%v", cmds, projs)
	}
}
