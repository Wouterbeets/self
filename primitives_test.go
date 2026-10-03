package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

// A capability's own exit status is the caller's answer, not just "failed".
func TestCapabilityExitStatusPassesThrough(t *testing.T) {
	h := home(t)
	refuse := "#!/bin/sh\ncat >/dev/null\necho held >&2\nexit 3\n"
	heard(t, h, line(t, "command.declared", decl{Name: "claim"})+
		line(t, "script.authored", authored{Type: kindCommand, Name: "claim", Script: refuse})+
		line(t, "view.declared", decl{Name: "gate"})+
		line(t, "script.authored", authored{Type: kindView, Name: "gate", Script: refuse}))
	for verb, name := range map[string]string{"run": "claim", "view": "gate"} {
		var exit *exec.ExitError
		if err := dispatch(h, verb, []string{name}, io.Discard); !errors.As(err, &exit) || exit.ExitCode() != 3 {
			t.Fatalf("self %s lost the exit status: %v", verb, err)
		}
	}
}

// A stdin command reads the caller's bytes on stdin and its consumed log on fd 3;
// only a consumed append reruns it, and the rerun gets the same bytes.
func TestStdinCommandWithConsumesRerunsOnlyOnItsOwnEvents(t *testing.T) {
	script := "#!/bin/sh\nn=$(cat attempts 2>/dev/null || echo 0)\necho $((n+1)) >attempts\n" +
		"[ \"$n\" = 0 ] && echo \"$SELF_INJECT\" >>\"$SELF_HOME/events.jsonl\"\n" +
		"printf '{\"name\":\"slot.claimed\",\"payload\":{\"stdin\":\"%s\",\"fed\":%s}}\\n' \"$(cat)\" \"$(grep -c . <&3)\"\n"
	for inject, want := range map[string]string{"noise.made": `{"stdin":"piped","fed":0}`, "slot.claimed": `{"stdin":"piped","fed":1}`} {
		h := home(t)
		heard(t, h, line(t, "command.declared", decl{Name: "claim", Atomic: true, Stdin: true, Consumes: []string{"slot.claimed"}})+
			line(t, "script.authored", authored{Type: kindCommand, Name: "claim", Script: script}))
		raw, _ := json.Marshal(newEvent(inject, nil))
		t.Setenv("SELF_INJECT", string(raw))
		in := filepath.Join(t.TempDir(), "in")
		os.WriteFile(in, []byte("piped"), 0600)
		f, _ := os.Open(in)
		old := os.Stdin
		os.Stdin = f
		evs, err := runCommand(h, replayed(t, h), "claim", nil, doorCLI, "")
		os.Stdin = old
		f.Close()
		attempts, _ := os.ReadFile(filepath.Join(h, "attempts"))
		if err != nil || len(evs) != 1 || string(evs[0].Payload) != want {
			t.Fatalf("%s: %v %v", inject, err, evs)
		}
		if reran := strings.TrimSpace(string(attempts)) == "2"; reran != (inject == "slot.claimed") {
			t.Fatalf("%s: %s attempt(s)", inject, attempts)
		}
	}
}

func TestWatchFollowStreamsUntilTheTimeout(t *testing.T) {
	h := home(t)
	cursor := head(replayed(t, h).Events)
	var out bytes.Buffer
	done := make(chan error)
	go func() {
		done <- cmdWatch(h, []string{"--after", cursor, "--timeout", "1s", "--follow", "sample."}, &out)
	}()
	heard(t, h, `{"name":"sample.one","payload":{}}`)
	time.Sleep(500 * time.Millisecond)
	heard(t, h, `{"name":"noise.one","payload":{}}
{"name":"sample.two","payload":{}}`)
	if err := <-done; err != nil || strings.Count(out.String(), "\n") != 2 || !strings.Contains(out.String(), "sample.two") {
		t.Fatalf("follow: %v\n%s", err, out.String())
	}
}

// Commands now sign consumes too; a receipt without it signs exactly as before.
func TestCommandConsumesIsSignedAndOldReceiptsVerify(t *testing.T) {
	h := home(t)
	os.WriteFile(secretPath(h), []byte(hex.EncodeToString([]byte("an old key"))), 0600)
	heard(t, h, line(t, "command.declared", decl{Name: "lease"})+
		line(t, "script.authored", authored{Type: kindCommand, Name: "lease", Script: "#!/bin/sh\ntrue\n"})+
		line(t, "command.declared", decl{Name: "count", Consumes: []string{"tick.counted"}})+
		line(t, "script.authored", authored{Type: kindCommand, Name: "count", Script: "#!/bin/sh\ntrue\n"}))
	st := replayed(t, h)
	// Signed by the kernel before commands could consume.
	if sig := st.cap(kindCommand, "lease").Receipt.Sig; sig != "34b42417db5e42ab8dd7d03c15dc4dc07371f00ee5e209bf5b36aae6fbbe33e8" {
		t.Fatalf("a receipt without consumes changed signature: %s", sig)
	}
	page, _ := briefOne(st, "count")
	if r := st.cap(kindCommand, "count").Receipt; len(r.Consumes) != 1 || !strings.Contains(page, "consumes: tick.counted") {
		t.Fatalf("command consumes not signed or shown: %+v\n%s", r, page)
	}
}
