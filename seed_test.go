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

func TestAnEmptyLogPreviewsTheSeedWithoutWriting(t *testing.T) {
	h := seeded(t)
	brief := ran(t, h, "")
	for _, want := range []string{"entry", "journal", "log: 0 events", protocolLayer("first")} {
		if !strings.Contains(brief, want) {
			t.Fatalf("the empty brief lacks %q:\n%s", want, brief)
		}
	}
	if one := ran(t, h, "brief", "entry"); !strings.Contains(one, "journal.entry") {
		t.Fatalf("brief entry does not describe the seed command:\n%s", one)
	}
	if page := ran(t, h, "view", "journal"); page != "" {
		t.Fatalf("an empty journal printed %q", page)
	}
	if entries, err := os.ReadDir(h); err != nil || len(entries) != 0 {
		t.Fatalf("reading an empty instance left files behind: %v %v", entries, err)
	}
}

func TestTheFirstWriteRecordsTheSeedAheadOfIt(t *testing.T) {
	h := seeded(t)
	if report := heard(t, h, `{"name":"note.added","payload":{"text":"hello"}}`); !strings.Contains(report, "seq 5-5") {
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
	want := "command.declared/seed view.declared/seed script.installed/kernel script.installed/kernel note.added/hear"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("log = %s\nwant  %s", got, want)
	}
	st, err := loadState(h)
	if err != nil {
		t.Fatal(err)
	}
	if st.Preview || !st.capabilitiesReady() || st.cap(kindCommand, "entry") == nil || st.cap(kindView, "journal") == nil {
		t.Fatalf("the seed is not installed from the log: preview=%v caps=%v", st.Preview, st.Caps)
	}
	heard(t, h, `{"name":"note.added","payload":{"text":"again"}}`)
	if again, _ := readEvents(h); len(again) != 6 {
		t.Fatalf("a later write seeded again: %d events", len(again))
	}
}

func TestTheSeedCommandRunsOnAnEmptyLog(t *testing.T) {
	h := seeded(t)
	if out := ran(t, h, "run", "entry", "staging", "resets", "on", "Monday"); !strings.Contains(out, "5\tjournal.entry") {
		t.Fatalf("entry on an empty log: %s", out)
	}
	ran(t, h, "run", "entry", "second")
	lines := strings.Split(strings.TrimSpace(ran(t, h, "view", "journal")), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "\tsecond") || !strings.HasSuffix(lines[1], "\tstaging resets on Monday") {
		t.Fatalf("journal is not newest first: %q", lines)
	}
}

func TestAConditionalFirstWriteStillMatchesAnEmptyHead(t *testing.T) {
	h := seeded(t)
	var out bytes.Buffer
	if err := cmdHear(h, []byte(`{"name":"note.added","payload":{}}`), &out, "empty"); err != nil {
		t.Fatalf("hear --after empty on an empty log: %v", err)
	}
	if err := cmdHear(h, []byte(`{"name":"note.added","payload":{}}`), &out, "empty"); err == nil {
		t.Fatal("hear --after empty succeeded on a log that holds events")
	}
}

func TestALogThatHoldsEventsIsNeverSeeded(t *testing.T) {
	h := home(t)
	heard(t, h, `{"name":"note.added","payload":{}}`)
	seedEnabled = true
	t.Cleanup(func() { seedEnabled = false })
	heard(t, h, `{"name":"note.added","payload":{}}`)
	st, err := loadState(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Events) != 2 || st.Preview || st.cap(kindCommand, "entry") != nil {
		t.Fatalf("an existing log was seeded: %d events, preview=%v", len(st.Events), st.Preview)
	}
}
