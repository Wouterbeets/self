package kernel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ks/internal/event"
)

func TestRenderAndReadWiring(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "site"), 0755)

	// Write a minimal events.jsonl with command.declared, projector.declared,
	// and seed.planted events.
	events := `{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{"version":"ks/v0"}}
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
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(`{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{"version":"ks/v0"}}
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
	os.Setenv("KS_LLM_STUB", "1")
	t.Cleanup(func() { os.Unsetenv("KS_LLM_STUB") })

	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "registry", "commands"), 0755)
	os.MkdirAll(filepath.Join(home, "registry", "projectors"), 0755)
	os.MkdirAll(filepath.Join(home, "site"), 0755)

	// Minimal events.jsonl so RenderHTML has something to read.
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(
		`{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{"version":"ks/v0"}}
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
	if _, err := os.Stat(filepath.Join(home, "registry", "commands", "summarize")); err != nil {
		t.Error("summarize command script not written")
	}
	if _, err := os.Stat(filepath.Join(home, "registry", "projectors", "summaries")); err != nil {
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

func TestCompileDeclarationsNoDeclarations(t *testing.T) {
	os.Setenv("KS_LLM_STUB", "1")
	t.Cleanup(func() { os.Unsetenv("KS_LLM_STUB") })

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
