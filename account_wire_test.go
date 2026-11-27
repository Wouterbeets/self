package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGiveLearnPipeRoundTrip(t *testing.T) {
	giver := home(t)
	growJournal(t, giver)
	runCommand(giver, replayed(t, giver), "entry", []string{"first note"}, doorCLI, "wouter")
	runCommand(giver, replayed(t, giver), "entry", []string{"second note"}, doorCLI, "wouter")

	var wire bytes.Buffer
	if err := cmdGiveOut(giver, "journal.", "-", &wire); err != nil {
		t.Fatalf("give to stdout: %v", err)
	}
	var parsed wireAccount
	if err := json.Unmarshal(wire.Bytes(), &parsed); err != nil {
		t.Fatalf("give stdout is not valid json account: %v", err)
	}
	if parsed.Name != "journal" || parsed.Intent == "" || len(parsed.Record) != 2 {
		t.Fatalf("unexpected wire account: name=%q intent=%d record=%d", parsed.Name, len(parsed.Intent), len(parsed.Record))
	}

	receiver := home(t)
	var out1 bytes.Buffer
	if err := cmdLearnFrom(receiver, "-", bytes.NewReader(wire.Bytes()), &out1); err != nil {
		t.Fatalf("learn from stdin: %v", err)
	}
	if strings.Contains(out1.String(), "-/record.jsonl") {
		t.Fatalf("stdin learn prompt leaked bogus path: %s", out1.String())
	}
	events, _ := readEvents(receiver)
	if len(events) != 4 {
		t.Fatalf("want intent + 2 deposits + attestation, got %d", len(events))
	}
	if events[1].Via != doorLearn+"journal" {
		t.Fatalf("want via=%s, got %q", doorLearn+"journal", events[1].Via)
	}

	// Repeating the identical piped delivery is idempotent.
	var out2 bytes.Buffer
	if err := cmdLearnFrom(receiver, "-", bytes.NewReader(wire.Bytes()), &out2); err != nil {
		t.Fatalf("repeat learn from stdin: %v", err)
	}
	events2, _ := readEvents(receiver)
	if len(events2) != len(events) {
		t.Fatalf("repeat stdin learn appended %d extra events", len(events2)-len(events))
	}
}

func TestGiveCapabilityPipePreservesLineage(t *testing.T) {
	giver := home(t)
	growJournal(t, giver)

	var wire bytes.Buffer
	if err := cmdGiveOut(giver, "command/entry", "-", &wire); err != nil {
		t.Fatalf("give capability to stdout: %v", err)
	}
	receiver := home(t)
	var out bytes.Buffer
	if err := cmdLearnFrom(receiver, "-", bytes.NewReader(wire.Bytes()), &out); err != nil {
		t.Fatalf("learn piped capability: %v", err)
	}
	if !hasEvent(t, receiver, "lineage.command.declared") || !hasEvent(t, receiver, "lineage.script.installed") {
		t.Fatalf("piped capability did not deposit inert lineage.* events")
	}
	if hasEvent(t, receiver, "command.declared") || hasEvent(t, receiver, "script.installed") {
		t.Fatalf("piped capability installed foreign code directly")
	}
}

func TestLearnJSONFileAndRefusesKernelVocabulary(t *testing.T) {
	p := filepath.Join(t.TempDir(), "goals.json")
	payload := `{"id":"goals","title":"goals","intent":"# goals\nkeep outcomes","commit":"abc123","record":[{"name":"lineage.command.declared","payload":{"name":"goal"}},{"name":"goal.created","payload":{"goal":"g1"}}],"evidence":{"extra":"ignored"}}`
	if err := os.WriteFile(p, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	receiver := home(t)
	var out bytes.Buffer
	if err := cmdLearnFrom(receiver, p, strings.NewReader(""), &out); err != nil {
		t.Fatalf("learn json file: %v", err)
	}
	events, _ := readEvents(receiver)
	if len(events) != 4 || events[1].Name != "lineage.command.declared" || events[2].Name != "goal.created" {
		t.Fatalf("unexpected deposited events: %+v", events)
	}

	// Raw kernel vocabulary in JSON record is still refused wholesale.
	bad := `{"name":"bad","intent":"x","record":[{"name":"command.declared","payload":{"name":"evil"}}]}`
	receiver2 := home(t)
	if err := cmdLearnFrom(receiver2, "-", strings.NewReader(bad), &bytes.Buffer{}); err == nil {
		t.Fatalf("expected raw kernel vocabulary in json record to be refused")
	}
	if evs, _ := readEvents(receiver2); len(evs) != 0 {
		t.Fatalf("refused json learn deposited %d events", len(evs))
	}
}
