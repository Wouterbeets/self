package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunningACommandStampsItsUseOutsideTheLog(t *testing.T) {
	h := home(t)
	made(t, h, "#!/bin/sh\ncat >/dev/null\necho hi\n", "command", "greet", "say hi")
	before := len(replayed(t, h).Events)
	if err := dispatch(h, "run", []string{"greet"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(usedDir(h), "command.greet")); err != nil {
		t.Fatalf("running a command left no use stamp: %v", err)
	}
	st := replayed(t, h)
	if len(st.Events) != before {
		t.Fatal("a use stamp went into the log")
	}
	if got := lately(st, lastUses(h, st), 5); len(got) != 1 || got[0] != "greet" {
		t.Fatalf("lately = %v, want [greet]", got)
	}
}

func TestArrivalNamesWhatHasGoneUntouched(t *testing.T) {
	h := home(t)
	for i := range listedInSession + 1 {
		made(t, h, "#!/bin/sh\ncat >/dev/null\n", "command", fmt.Sprintf("c%d", i), "filler")
	}
	if err := dispatch(h, "run", []string{"c0"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	st := replayed(t, h)
	if u := untouched(h, st, lastUses(h, st)); u != nil {
		t.Fatalf("a fresh stamp record named untouched capabilities: %v", u)
	}
	old := time.Now().Add(-2 * untouchedAfter)
	os.Chtimes(filepath.Join(usedDir(h), ".since"), old, old)
	os.Chtimes(filepath.Join(usedDir(h), "command.c0"), old, old)
	for _, c := range st.Caps {
		c.RcptAt = old
	}
	last := lastUses(h, st)
	for k := range last {
		last[k] = old
	}
	line := untouchedLine(h, st, last)
	if !strings.Contains(line, "c0") || !strings.Contains(line, "Rework, retire") || !strings.Contains(line, "and 5 more") {
		t.Fatalf("arrival did not name what went untouched: %q", line)
	}
}

func TestArrivalPointsAtStaleIntents(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "intent.declared", intentItem{Name: "x/fresh", Description: "Something."}))
	st := replayed(t, h)
	if w := wantedLine(st); !strings.HasPrefix(w, "1 wanted") || strings.Contains(w, "over 30 days") {
		t.Fatalf("a fresh intent read as stale: %q", w)
	}
	st.Events[len(st.Events)-1].OccurredAt = time.Now().Add(-2 * untouchedAfter)
	if w := wantedLine(st); !strings.Contains(w, "1 open over 30 days, oldest intent/x/fresh") {
		t.Fatalf("a stale intent was not named: %q", w)
	}
}

func TestKeptOnPurposeIsNotUntouched(t *testing.T) {
	h := home(t)
	made(t, h, "#!/bin/sh\ncat >/dev/null\n", "command", "rare", "run twice a year")
	heard(t, h, line(t, "capability.kept", map[string]string{"type": "command", "name": "rare", "reason": "yearly tax export"}))
	st := replayed(t, h)
	old := time.Now().Add(-2 * untouchedAfter)
	if err := os.MkdirAll(usedDir(h), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(usedDir(h), ".since"), nil, 0o644)
	os.Chtimes(filepath.Join(usedDir(h), ".since"), old, old)
	for _, c := range st.Caps {
		c.RcptAt = old
	}
	if u := untouched(h, st, lastUses(h, st)); len(u) != 0 {
		t.Fatalf("a capability kept on purpose was named untouched: %v", u)
	}
	st.Events[len(st.Events)-1].OccurredAt = old
	if u := untouched(h, st, lastUses(h, st)); len(u) != 1 {
		t.Fatalf("a keep older than 30 days still shielded it: %v", u)
	}
}
