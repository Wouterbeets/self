package kernel

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCap(t *testing.T, home, dir, name, script string) {
	t.Helper()
	d := filepath.Join(home, "capabilities", dir)
	os.MkdirAll(d, 0755)
	if err := os.WriteFile(filepath.Join(d, name), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestSelfTestPassesAndReportsCoverage(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "site"), 0755)
	// greet: declared WITH an example, installed and correct (echoes argv)
	writeCap(t, home, "commands", "greet", "#!/bin/sh\nprintf '%s' \"$1\"\n")
	// quiet: declared with NO examples, installed — should count as untested
	writeCap(t, home, "commands", "quiet", "#!/bin/sh\n:\n")

	log := `{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{}}
{"id":"b","seq":2,"name":"command.declared","occurred_at":"2026-01-01T00:00:00Z","payload":{"name":"greet","description":"d","event":{"name":"greeted"},"examples":[{"args":["hi-there"],"expect_contains":["hi-there"]}]}}
{"id":"c","seq":3,"name":"command.declared","occurred_at":"2026-01-01T00:00:00Z","payload":{"name":"quiet","description":"d","event":{"name":"q"}}}
`
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(log), 0644)

	results, err := SelfTest(home)
	if err != nil {
		t.Fatalf("SelfTest: %v", err)
	}
	byName := map[string]CapTestResult{}
	for _, r := range results {
		byName[r.Name] = r
	}
	if g := byName["greet"]; !g.HasExamples || !g.OK() {
		t.Errorf("greet should pass with examples, got %+v", g)
	}
	if q := byName["quiet"]; q.HasExamples || !q.OK() {
		t.Errorf("quiet should be untested-but-OK, got %+v", q)
	}
}

func TestSelfTestCatchesARegression(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "site"), 0755)
	// installed binary that does NOT satisfy the declared example (echoes nothing)
	writeCap(t, home, "commands", "greet", "#!/bin/sh\n:\n")
	log := `{"id":"a","seq":1,"name":"kernel.initialized","occurred_at":"2026-01-01T00:00:00Z","payload":{}}
{"id":"b","seq":2,"name":"command.declared","occurred_at":"2026-01-01T00:00:00Z","payload":{"name":"greet","description":"d","event":{"name":"greeted"},"examples":[{"args":["hi-there"],"expect_contains":["hi-there"]}]}}
`
	os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(log), 0644)

	results, err := SelfTest(home)
	if err != nil {
		t.Fatalf("SelfTest: %v", err)
	}
	if len(results) != 1 || results[0].OK() {
		t.Fatalf("a drifted binary must fail selftest, got %+v", results)
	}
}
