package kernel

import (
	"os"
	"path/filepath"
	"testing"
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
