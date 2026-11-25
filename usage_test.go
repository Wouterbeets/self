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

func TestRunningACommandLeavesNoSidecarFiles(t *testing.T) {
	h := home(t)
	made(t, h, "#!/bin/sh\ncat >/dev/null\necho '{\"name\":\"greet.said\",\"payload\":{}}'\n", "command", "greet", "say hi")
	if err := dispatch(h, "run", []string{"greet"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h, "used")); !os.IsNotExist(err) {
		t.Fatalf("running a command created a used/ sidecar directory")
	}
	st := replayed(t, h)
	if got := lately(st, lastUses(st), 5); len(got) != 1 || got[0] != "greet" {
		t.Fatalf("lately = %v, want [greet]", got)
	}
}

func TestArrivalNamesWhatHasGoneUntouched(t *testing.T) {
	h := home(t)
	for i := range listedInSession + 1 {
		made(t, h, "#!/bin/sh\ncat >/dev/null\n", "command", fmt.Sprintf("c%d", i), "filler")
	}
	st := replayed(t, h)
	if u := untouched(st, lastUses(st)); u != nil {
		t.Fatalf("a fresh instance named untouched capabilities: %v", u)
	}
	old := time.Now().Add(-2 * untouchedAfter)
	st.Events[0].OccurredAt = old
	for _, c := range st.Caps {
		c.RcptAt = old
	}
	last := lastUses(st)
	line := untouchedLine(st, last)
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
	st.Events[0].OccurredAt = old
	for _, c := range st.Caps {
		c.RcptAt = old
	}
	if u := untouched(st, lastUses(st)); len(u) != 0 {
		t.Fatalf("a capability kept on purpose was named untouched: %v", u)
	}
	st.Events[len(st.Events)-1].OccurredAt = old
	if u := untouched(st, lastUses(st)); len(u) != 1 {
		t.Fatalf("a keep older than 30 days still shielded it: %v", u)
	}
}

func TestAKeptIntentIsNotStale(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "intent.declared", intentItem{Name: "x/long", Description: "Something long."}))
	heard(t, h, line(t, "intent.kept", map[string]string{"name": "x/long", "reason": "waiting on a real deploy"}))
	st := replayed(t, h)
	st.Events[len(st.Events)-2].OccurredAt = time.Now().Add(-2 * untouchedAfter)
	if w := wantedLine(st); strings.Contains(w, "over 30 days") {
		t.Fatalf("a kept intent read as stale: %q", w)
	}
	st.Events[len(st.Events)-1].OccurredAt = time.Now().Add(-2 * untouchedAfter)
	if w := wantedLine(st); !strings.Contains(w, "1 open over 30 days") {
		t.Fatalf("a keep older than 30 days still shielded it: %q", w)
	}
}
