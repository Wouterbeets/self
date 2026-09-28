package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestConditionalHearConcurrentWriters(t *testing.T) {
	h := home(t)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		wg.Go(func() {
			results <- cmdHear(h, []byte(`{"name":"resource.claimed","payload":{}}`), io.Discard, "empty")
		})
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !strings.Contains(err.Error(), "log changed") {
			t.Fatal(err)
		}
	}
	if winners != 1 || len(replayed(t, h).Events) != 1 {
		t.Fatalf("expected one committed claim, got %d", winners)
	}
	before := head(replayed(t, h).Events)
	if err := cmdHear(h, []byte(`{"name":"resource.released","payload":{}}`), io.Discard, before); err != nil {
		t.Fatal(err)
	}
	if err := cmdHear(h, []byte(`{"name":"view.declared","payload":{"name":"stale"}}`), io.Discard, before); err == nil {
		t.Fatal("stale declaration committed")
	}
}

func TestLearnIdempotenceOriginAndGrouping(t *testing.T) {
	h, dir := home(t), t.TempDir()
	os.WriteFile(filepath.Join(dir, "intent.md"), []byte("Interpret these observations."), 0600)
	e := newEvent("sample.observed", json.RawMessage(`{"large":9007199254740993,"a":1}`))
	write := func(e Event) {
		raw, _ := json.Marshal(e)
		os.WriteFile(filepath.Join(dir, "record.jsonl"), raw, 0600)
	}
	write(e)
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for range 8 {
		wg.Go(func() { errors <- cmdLearn(h, dir, io.Discard, "learn/samples") })
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	st := replayed(t, h)
	if len(st.Events) != 3 || st.Events[1].Origin != e.ID || st.Events[1].ID == e.ID {
		t.Fatalf("delivery duplicated or origin lost: %+v", st.Events)
	}
	// A second delivery with reordered JSON retains the original observation.
	e.Payload = json.RawMessage(`{"a":1,"large":9007199254740993}`)
	write(e)
	if err := cmdLearn(h, dir, io.Discard, "learn/samples"); err != nil {
		t.Fatal(err)
	}
	if st = replayed(t, h); len(st.Events) != 5 || len(st.Intents) != 1 {
		t.Fatal("grouped import duplicated testimony or intents")
	}
	// Identity reuse with different testimony refuses the entire delivery.
	e.Payload = json.RawMessage(`{"large":9007199254740992,"a":1}`)
	write(e)
	if err := cmdLearn(h, dir, io.Discard); err == nil || len(replayed(t, h).Events) != 5 {
		t.Fatal("conflicting origin changed the log")
	}
	// Exporting and learning again keeps the original identity through another hop.
	gift := filepath.Join(t.TempDir(), "gift")
	if err := cmdGive(h, "sample.", gift); err != nil {
		t.Fatal(err)
	}
	other := home(t)
	if err := cmdLearn(other, gift, io.Discard); err != nil {
		t.Fatal(err)
	}
	if replayed(t, other).Events[1].Origin != e.ID {
		t.Fatal("second hop lost the origin")
	}
}

func TestWatchReadOnlyFilteringAndMissingCursor(t *testing.T) {
	h := home(t)
	heard(t, h, `{"name":"sample.one","payload":{}}`)
	cursor := head(replayed(t, h).Events)
	heard(t, h, `{"name":"noise.one","payload":{}}
{"name":"sample.two","payload":{}}`)
	before, _ := os.ReadFile(logPath(h))
	var out bytes.Buffer
	if err := cmdWatch(h, []string{"--after", cursor, "sample."}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sample.two") || strings.Contains(out.String(), "noise.") || strings.Contains(out.String(), "sample.one") {
		t.Fatal(out.String())
	}
	if err := cmdWatch(h, []string{"--timeout", "1ms"}, io.Discard); err == nil {
		t.Fatal("watch failed to time out")
	}
	if err := cmdWatch(h, []string{"--after", "gone"}, io.Discard); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatal("missing cursor was silently accepted")
	}
	after, _ := os.ReadFile(logPath(h))
	if !bytes.Equal(before, after) {
		t.Fatal("waiting changed the log")
	}
}

func TestLoopExplicitSettlementIsScopedToSuccessfulStdout(t *testing.T) {
	for _, tc := range []struct {
		wire, suffix string
		settled      bool
	}{
		{`{"name":"note.saved","payload":{}}
{"name":"loop.settled","payload":{"reason":"Waiting for input"}}`, "", true},
		{`{"name":"loop.settled","payload":{"reason":""}}`, "", false},
		{`{"name":"loop.settled","payload":{"reason":"Wait"}}`, "; exit 1", false},
		{`{"name":"loop.settled","payload":{"reason":"Wait"}}
{"name":"note.saved","payload":{}}`, "", false},
	} {
		h := home(t)
		var diag bytes.Buffer
		// A previous writer's settlement cannot stop this pass.
		heard(t, h, `{"name":"loop.settled","payload":{"reason":"Earlier pass"}}`)
		cmd := "cat >/dev/null; printf '%s\\n' '" + tc.wire + "'" + tc.suffix
		err := cmdLoop(h, []string{"--max-passes", "1", "--", "/bin/sh", "-c", cmd}, io.Discard, &diag)
		if (err == nil) != tc.settled {
			t.Fatalf("settlement=%v: %v\n%s", tc.settled, err, diag.String())
		}
	}
}

func TestAtomicCommandDecidesFromTheLogItCommits(t *testing.T) {
	// claim: succeeds only while no claim is in the log; count: numbers ticks.
	claim := "#!/bin/sh\nif grep -q '\"name\":\"slot.claimed\"'; then echo held >&2; exit 3; fi\n" +
		"echo '{\"name\":\"slot.claimed\",\"payload\":{}}'\n"
	count := "#!/bin/sh\nn=$(grep -c '\"name\":\"tick.counted\"')\n" +
		"echo \"{\\\"name\\\":\\\"tick.counted\\\",\\\"payload\\\":{\\\"n\\\":$((n+1))}}\"\n"
	for _, atomic := range []bool{true, false} {
		h := home(t)
		heard(t, h, line(t, "command.declared", decl{Name: "claim", Atomic: atomic})+
			line(t, "script.authored", authored{Type: kindCommand, Name: "claim", Script: claim})+
			line(t, "command.declared", decl{Name: "count", Atomic: atomic})+
			line(t, "script.authored", authored{Type: kindCommand, Name: "count", Script: count}))
		const n = 6
		var wg sync.WaitGroup
		claims := make(chan error, n)
		for range n {
			wg.Go(func() {
				_, err := runCommand(h, replayed(t, h), "claim", nil, doorCLI, "")
				claims <- err
			})
			wg.Go(func() { runCommand(h, replayed(t, h), "count", nil, doorCLI, "") })
		}
		wg.Wait()
		close(claims)
		winners, ticks := 0, map[string]bool{}
		for err := range claims {
			if err == nil {
				winners++
			}
		}
		for _, e := range replayed(t, h).Events {
			if e.Name == "tick.counted" {
				ticks[string(e.Payload)] = true
			}
		}
		if atomic && (winners != 1 || len(ticks) != n) {
			t.Fatalf("atomic: %d claims won, %d distinct ticks of %d", winners, len(ticks), n)
		}
		if !atomic {
			t.Logf("unconditional: %d claims won, %d distinct ticks of %d", winners, len(ticks), n)
		}
	}
}
