package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	seedEnabled = false
	os.Exit(m.Run())
}

func seeded(t *testing.T) string {
	t.Helper()
	seedEnabled = true
	t.Cleanup(func() { seedEnabled = false })
	return home(t)
}

func ran(t *testing.T, h, verb string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := dispatch(h, verb, args, &out); err != nil {
		t.Fatalf("self %s %v: %v", verb, args, err)
	}
	return out.String()
}

const noteScripts = `{"name":"script.authored","payload":{"type":"command","name":"note","script":"#!/bin/sh\nprintf '{\"name\":\"note.added\",\"payload\":{\"text\":\"%s\"}}\\n' \"$*\"\n"}}
{"name":"script.authored","payload":{"type":"view","name":"notes","script":"#!/bin/sh\ngrep -c note.added\n"}}`

func TestAnEmptyLogShowsTheSeedPendingWithoutWriting(t *testing.T) {
	h := seeded(t)
	brief := ran(t, h, "")
	for _, want := range []string{"note (pending)", "notes (pending)", "log: 0 events", "command/note (the first write to this log declares it)", protocolLayer("first")} {
		if !strings.Contains(brief, want) {
			t.Fatalf("the empty brief lacks %q:\n%s", want, brief)
		}
	}
	if one := ran(t, h, "brief", "note"); !strings.Contains(one, "note.added") {
		t.Fatalf("brief note does not say what to build:\n%s", one)
	}
	if p := situated(t, h, "an ask"); !strings.Contains(p, `command "note" declared`) {
		t.Fatalf("an execution prompt on an empty log does not carry the pending seed:\n%s", p)
	}
	var out bytes.Buffer
	if err := dispatch(h, "run", []string{"note", "hello"}, &out); err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("running an unscripted seed command: %v", err)
	}
	if entries, err := os.ReadDir(h); err != nil || len(entries) != 0 {
		t.Fatalf("reading an empty instance left files behind: %v %v", entries, err)
	}
}

func TestTheFirstWriteRecordsTheDeclarationsAheadOfIt(t *testing.T) {
	h := seeded(t)
	if report := heard(t, h, `{"name":"thing.happened","payload":{"text":"hello"}}`); !strings.Contains(report, "seq 3-3") || !strings.Contains(report, "pending: command/note, view/notes") {
		t.Fatalf("the first heard event did not land after the seed: %s", report)
	}
	events, err := readEvents(h)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range events {
		names = append(names, e.Name+"/"+e.Via)
	}
	if got, want := strings.Join(names, " "), "command.declared/seed view.declared/seed thing.happened/hear"; got != want {
		t.Fatalf("log = %s\nwant  %s", got, want)
	}
	heard(t, h, `{"name":"thing.happened","payload":{}}`)
	if again, _ := readEvents(h); len(again) != 4 {
		t.Fatalf("a later write seeded again: %d events", len(again))
	}
}

func TestAMindAuthorsTheSeedAsItsFirstWrite(t *testing.T) {
	h := seeded(t)
	report := heard(t, h, noteScripts)
	for _, want := range []string{"installed command/note", "installed view/notes"} {
		if !strings.Contains(report, want) {
			t.Fatalf("authoring on an empty log: %s", report)
		}
	}
	st, err := loadState(h)
	if err != nil {
		t.Fatal(err)
	}
	if st.Preview || !st.capabilitiesReady() {
		t.Fatalf("the seed is not installed from the log: preview=%v pending=%v", st.Preview, st.pending())
	}
	if out := ran(t, h, "run", "note", "staging", "resets"); !strings.Contains(out, "5\tnote.added") {
		t.Fatalf("note after authoring: %s", out)
	}
	if page := strings.TrimSpace(ran(t, h, "view", "notes")); page != "1" {
		t.Fatalf("notes after one note: %q", page)
	}
}

func TestAConditionalFirstWriteStillMatchesAnEmptyHead(t *testing.T) {
	h := seeded(t)
	var out bytes.Buffer
	if err := cmdHear(h, []byte(`{"name":"thing.happened","payload":{}}`), &out, "empty"); err != nil {
		t.Fatalf("hear --after empty on an empty log: %v", err)
	}
	if err := cmdHear(h, []byte(`{"name":"thing.happened","payload":{}}`), &out, "empty"); err == nil {
		t.Fatal("hear --after empty succeeded on a log that holds events")
	}
}

func TestALogThatHoldsEventsIsNeverSeeded(t *testing.T) {
	h := home(t)
	heard(t, h, `{"name":"thing.happened","payload":{}}`)
	seedEnabled = true
	t.Cleanup(func() { seedEnabled = false })
	heard(t, h, `{"name":"thing.happened","payload":{}}`)
	st, err := loadState(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Events) != 2 || st.Preview || st.cap(kindCommand, "note") != nil {
		t.Fatalf("an existing log was seeded: %d events, preview=%v", len(st.Events), st.Preview)
	}
}
