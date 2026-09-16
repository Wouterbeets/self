package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntentLifecycleAndReadOnlyDiscovery(t *testing.T) {
	h := home(t)
	declare := func(description string) {
		heard(t, h, line(t, "intent.declared", intentItem{Name: "fit", Summary: "Check fit", Description: description}))
	}
	declare("Compare measured clearance with the model.")
	before, _ := os.ReadFile(filepath.Join(h, "events.jsonl"))
	for _, tc := range []struct {
		verb string
		args []string
	}{
		{"", nil}, {"prompt", nil}, {"brief", nil},
		{"brief", []string{"intent/"}}, {"brief", []string{"intent/fit"}},
		{"__complete", []string{"brief", "intent/"}},
	} {
		var out bytes.Buffer
		if err := dispatch(h, tc.verb, tc.args, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "intent/fit") {
			t.Fatalf("%s %v lost work: %s", tc.verb, tc.args, &out)
		}
	}
	after, _ := os.ReadFile(filepath.Join(h, "events.jsonl"))
	if !bytes.Equal(before, after) {
		t.Fatal("discovery appended")
	}
	heard(t, h, line(t, "intent.closed", intentClosure{Name: "fit", Outcome: "completed", Reason: "Measured 4 mm clearance; model requires 3 mm."}))
	st := replayed(t, h)
	if len(st.openIntents()) != 0 {
		t.Fatal("closed work remains open")
	}
	detail, err := briefOne(st, "intent/fit")
	if err != nil || !strings.Contains(detail, "4 mm") {
		t.Fatalf("lost closure evidence: %s %v", detail, err)
	}
	declare("Recheck the revised model.")
	if w := replayed(t, h).intent("fit"); w.Closure != nil || w.Description != "Recheck the revised model." {
		t.Fatal("redeclare did not replace and reopen")
	}
	heard(t, h, line(t, "intent.closed", intentClosure{Name: "fit", Outcome: "dropped", Reason: "Replacement no longer needed."}))
	if len(replayed(t, h).openIntents()) != 0 {
		t.Fatal("drop did not close")
	}
}

func TestIntentInvalidEventsAndImportedClosuresAreInert(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "intent.declared", intentItem{Name: "fit", Description: "Check clearance."}))
	for _, p := range []any{intentClosure{Name: "fit", Outcome: "completed"}, intentClosure{Name: "fit", Outcome: "unknown", Reason: "x"}, intentClosure{Name: "absent", Outcome: "completed", Reason: "x"}, nil} {
		heard(t, h, line(t, "intent.closed", p))
	}
	for _, p := range []any{intentItem{Name: "fit"}, intentItem{Name: "bad name", Description: "x"}, nil} {
		heard(t, h, line(t, "intent.declared", p))
	}
	st := replayed(t, h)
	if len(st.openIntents()) != 1 || st.intent("fit").Description != "Check clearance." {
		t.Fatal("invalid work altered state")
	}
	e := newEvent("intent.closed", []byte(`{"name":"fit","outcome":"completed","reason":"foreign claim"}`))
	e.Via = "learn:peer"
	st.apply([]Event{e})
	if len(st.openIntents()) != 1 {
		t.Fatal("learned closure closed local work")
	}
}

func TestIntentBriefIsBoundedAndOverflowDiscoverable(t *testing.T) {
	h := home(t)
	for i := 0; i < 20; i++ {
		heard(t, h, line(t, "intent.declared", intentItem{Name: fmt.Sprintf("item-%02d", i), Description: strings.Repeat("context ", 1000)}))
	}
	st := replayed(t, h)
	brief := intentBrief(st)
	if len(brief) > 4500 || !strings.Contains(brief, "8 more") || strings.Contains(brief, "intent/item-19") {
		t.Fatalf("unbounded brief: %s", brief)
	}
	index, err := intentDetail(st, "")
	if err != nil || !strings.Contains(index, "intent/item-19") {
		t.Fatal("overflow inaccessible")
	}
}

func TestIntentAccountsRequireLocalAdoption(t *testing.T) {
	source, destination, account := home(t), t.TempDir(), t.TempDir()
	heard(t, source, line(t, "intent.declared", intentItem{Name: "fit", Description: "Check fit."})+line(t, "intent.closed", intentClosure{Name: "fit", Outcome: "completed", Reason: "Source measurements fit."}))
	if err := cmdGive(source, "intent.", account); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(account, "record.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("lineage.intent.declared")) || !bytes.Contains(raw, []byte("lineage.intent.closed")) {
		t.Fatal("work crossed as live vocabulary")
	}
	fresh := t.TempDir()
	var imported bytes.Buffer
	if err := cmdLearn(fresh, account, &imported); err != nil {
		t.Fatal(err)
	}
	if freshState := replayed(t, fresh); len(freshState.Intents) != 1 || freshState.intent("fit") != nil || !strings.HasPrefix(freshState.Intents[0].Name, "learn/") {
		t.Fatal("learning must declare interpretation, not adopt the source's intent")
	}
	heard(t, destination, line(t, "intent.declared", intentItem{Name: "fit", Description: "Local measurements still needed."}))
	var prompt bytes.Buffer
	if err := cmdLearn(destination, account, &prompt); err != nil {
		t.Fatal(err)
	}
	st := replayed(t, destination)
	if len(st.Intents) != 2 || len(st.openIntents()) != 2 || st.intent("fit").Description != "Local measurements still needed." {
		t.Fatal("account changed local work")
	}
	if !strings.Contains(prompt.String(), "Receiving an account does not commit") {
		t.Fatal("learning omitted adoption boundary")
	}
	// Raw kernel work in a hand-written account is rejected before any deposit.
	for _, name := range []string{"intent.declared", "intent.closed"} {
		if err := os.WriteFile(filepath.Join(account, "record.jsonl"), []byte(line(t, name, intentItem{Name: "foreign", Description: "x"})), 0600); err != nil {
			t.Fatal(err)
		}
		before := len(replayed(t, destination).Events)
		if err := cmdLearn(destination, account, &prompt); err == nil {
			t.Fatal("raw work accepted")
		}
		if len(replayed(t, destination).Events) != before {
			t.Fatal("rejected account partly deposited")
		}
	}
}

func TestLoopCanSettleWithUnresolvedIntent(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "intent.declared", intentItem{Name: "weigh-spool", Description: "Waiting for a physical measurement."}))
	var out, diag bytes.Buffer
	if err := cmdLoop(h, []string{"--max-passes", "3", "--settle", "2", "--", "sh", "-c", "cat >/dev/null"}, &out, &diag); err != nil {
		t.Fatal(err)
	}
	if len(replayed(t, h).openIntents()) != 1 || !strings.Contains(diag.String(), "converged after 2") {
		t.Fatal("quiet work did not survive convergence")
	}
}

func TestIntentFixturesRunAcrossPersonas(t *testing.T) {
	for _, tc := range []struct{ fixture, name, result, evidence string }{
		{"printer", "enclosure-fit", "print.assessed", "143.75"},
		{"household", "thursday-dinner", "shopping.prepared", "tomatoes"},
		{"author", "sum-filament", "artifact.written", "negative input rejected"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			h := home(t)
			fixture, err := os.ReadFile(filepath.Join("examples", "intent", tc.fixture+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			heard(t, h, string(fixture))
			mind, err := filepath.Abs(filepath.Join("examples", "intent", "mind.py"))
			if err != nil {
				t.Fatal(err)
			}
			var out, diag bytes.Buffer
			if err := cmdLoop(h, []string{"--max-passes", "4", "--timeout", "5s", "--", "python3", mind}, &out, &diag); err != nil {
				t.Fatal(err)
			}
			st := replayed(t, h)
			if len(st.openIntents()) != 0 || len(st.Caps) != 0 {
				t.Fatal("work required capabilities or failed to close")
			}
			detail, err := intentDetail(st, tc.name)
			if err != nil || !strings.Contains(detail, tc.evidence) {
				t.Fatalf("missing evidence: %s %v", detail, err)
			}
			found := false
			for _, e := range st.Events {
				if e.Name == tc.result {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing domain result %s", tc.result)
			}
			if !strings.Contains(diag.String(), "converged after 3") {
				t.Fatalf("did not stop after useful work: %s", &diag)
			}
		})
	}
}

func TestPrinterMindWaitsForMissingMeasurement(t *testing.T) {
	h := home(t)
	fixture, err := os.ReadFile(filepath.Join("examples", "intent", "printer.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	_, withoutMeasurement, ok := strings.Cut(string(fixture), "\n")
	if !ok {
		t.Fatal("missing fixture lines")
	}
	heard(t, h, withoutMeasurement)
	mind, err := filepath.Abs(filepath.Join("examples", "intent", "mind.py"))
	if err != nil {
		t.Fatal(err)
	}
	before := len(replayed(t, h).Events)
	var out, diag bytes.Buffer
	if err := cmdLoop(h, []string{"--max-passes", "3", "--", "python3", mind}, &out, &diag); err != nil {
		t.Fatal(err)
	}
	st := replayed(t, h)
	if len(st.Events) != before || len(st.openIntents()) != 1 {
		t.Fatal("missing evidence produced a result or closed work")
	}
}

func TestLegacyIntentAndWorkCompatibility(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "intent.declared", map[string]string{"account": "old", "intent": "Historical account purpose"})+
		line(t, "work.declared", intentItem{Name: "old-work", Description: "An outcome declared before the rename."}))
	st := replayed(t, h)
	if len(st.openIntents()) != 1 || st.intent("old-work") == nil {
		t.Fatal("legacy account opened, or legacy work disappeared")
	}
	detail, err := briefOne(st, "work/old-work")
	if err != nil || !strings.Contains(detail, "intent/old-work") {
		t.Fatal("legacy selector no longer resolves")
	}
	heard(t, h, line(t, "intent.closed", intentClosure{Name: "old-work", Outcome: "completed", Reason: "Closed through the new vocabulary."}))
	if len(replayed(t, h).openIntents()) != 0 {
		t.Fatal("new closure did not close legacy declaration")
	}
}

func TestLearningIntentSurvivesWithoutAccountDirectory(t *testing.T) {
	h, account := home(t), t.TempDir()
	if err := os.WriteFile(filepath.Join(account, "intent.md"), []byte("Keep a useful printer calibration method."), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := cmdLearn(h, account, &out); err != nil {
		t.Fatal(err)
	}
	st := replayed(t, h)
	if len(st.openIntents()) != 1 {
		t.Fatal("learning left no discoverable intent")
	}
	name := st.openIntents()[0].Name
	if err := os.Remove(filepath.Join(account, "intent.md")); err != nil {
		t.Fatal(err)
	}
	detail, err := intentDetail(replayed(t, h), name)
	if err != nil || !strings.Contains(detail, "Keep a useful printer calibration method.") || !strings.Contains(detail, "intent.closed") {
		t.Fatalf("learning context lost: %s %v", detail, err)
	}
	if !strings.Contains(situated(t, h, ""), "intent/"+name) {
		t.Fatal("later pass cannot discover unfinished learning")
	}
	heard(t, h, line(t, "intent.closed", intentClosure{Name: name, Outcome: "completed", Reason: "Reviewed; no applicable printer here, so nothing adopted."}))
	if len(replayed(t, h).openIntents()) != 0 {
		t.Fatal("learning cannot finish by declining")
	}
}
