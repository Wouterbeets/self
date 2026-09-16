package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkLifecycleAndReadOnlyDiscovery(t *testing.T) {
	h := home(t)
	declare := func(description string) {
		heard(t, h, line(t, "work.declared", workItem{Name: "fit", Summary: "Check fit", Description: description}))
	}
	declare("Compare measured clearance with the model.")
	before, _ := os.ReadFile(filepath.Join(h, "events.jsonl"))
	for _, tc := range []struct {
		verb string
		args []string
	}{
		{"", nil}, {"prompt", nil}, {"brief", nil},
		{"brief", []string{"work/"}}, {"brief", []string{"work/fit"}},
		{"__complete", []string{"brief", "work/"}},
	} {
		var out bytes.Buffer
		if err := dispatch(h, tc.verb, tc.args, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "work/fit") {
			t.Fatalf("%s %v lost work: %s", tc.verb, tc.args, &out)
		}
	}
	after, _ := os.ReadFile(filepath.Join(h, "events.jsonl"))
	if !bytes.Equal(before, after) {
		t.Fatal("discovery appended")
	}
	heard(t, h, line(t, "work.closed", workClosure{Name: "fit", Outcome: "completed", Reason: "Measured 4 mm clearance; model requires 3 mm."}))
	st := replayed(t, h)
	if len(st.openWork()) != 0 {
		t.Fatal("closed work remains open")
	}
	detail, err := briefOne(st, "work/fit")
	if err != nil || !strings.Contains(detail, "4 mm") {
		t.Fatalf("lost closure evidence: %s %v", detail, err)
	}
	declare("Recheck the revised model.")
	if w := replayed(t, h).work("fit"); w.Closure != nil || w.Description != "Recheck the revised model." {
		t.Fatal("redeclare did not replace and reopen")
	}
	heard(t, h, line(t, "work.closed", workClosure{Name: "fit", Outcome: "dropped", Reason: "Replacement no longer needed."}))
	if len(replayed(t, h).openWork()) != 0 {
		t.Fatal("drop did not close")
	}
}

func TestWorkInvalidEventsAndImportedClosuresAreInert(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "work.declared", workItem{Name: "fit", Description: "Check clearance."}))
	for _, p := range []any{workClosure{Name: "fit", Outcome: "completed"}, workClosure{Name: "fit", Outcome: "unknown", Reason: "x"}, workClosure{Name: "absent", Outcome: "completed", Reason: "x"}, nil} {
		heard(t, h, line(t, "work.closed", p))
	}
	for _, p := range []any{workItem{Name: "fit"}, workItem{Name: "bad name", Description: "x"}, nil} {
		heard(t, h, line(t, "work.declared", p))
	}
	st := replayed(t, h)
	if len(st.openWork()) != 1 || st.work("fit").Description != "Check clearance." {
		t.Fatal("invalid work altered state")
	}
	e := newEvent("work.closed", []byte(`{"name":"fit","outcome":"completed","reason":"foreign claim"}`))
	e.Via = "learn:peer"
	st.apply([]Event{e})
	if len(st.openWork()) != 1 {
		t.Fatal("learned closure closed local work")
	}
}

func TestWorkBriefIsBoundedAndOverflowDiscoverable(t *testing.T) {
	h := home(t)
	for i := 0; i < 20; i++ {
		heard(t, h, line(t, "work.declared", workItem{Name: fmt.Sprintf("item-%02d", i), Description: strings.Repeat("context ", 1000)}))
	}
	st := replayed(t, h)
	brief := workBrief(st)
	if len(brief) > 4500 || !strings.Contains(brief, "8 more") || strings.Contains(brief, "work/item-19") {
		t.Fatalf("unbounded brief: %s", brief)
	}
	index, err := workDetail(st, "")
	if err != nil || !strings.Contains(index, "work/item-19") {
		t.Fatal("overflow inaccessible")
	}
}

func TestWorkAccountsRequireLocalAdoption(t *testing.T) {
	source, destination, account := home(t), t.TempDir(), t.TempDir()
	heard(t, source, line(t, "work.declared", workItem{Name: "fit", Description: "Check fit."})+line(t, "work.closed", workClosure{Name: "fit", Outcome: "completed", Reason: "Source measurements fit."}))
	if err := cmdGive(source, "work.", account); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(account, "record.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("lineage.work.declared")) || !bytes.Contains(raw, []byte("lineage.work.closed")) {
		t.Fatal("work crossed as live vocabulary")
	}
	fresh := t.TempDir()
	var imported bytes.Buffer
	if err := cmdLearn(fresh, account, &imported); err != nil {
		t.Fatal(err)
	}
	if len(replayed(t, fresh).Work) != 0 {
		t.Fatal("learning adopted work without local declaration")
	}
	heard(t, destination, line(t, "work.declared", workItem{Name: "fit", Description: "Local measurements still needed."}))
	var prompt bytes.Buffer
	if err := cmdLearn(destination, account, &prompt); err != nil {
		t.Fatal(err)
	}
	st := replayed(t, destination)
	if len(st.Work) != 1 || len(st.openWork()) != 1 || st.work("fit").Description != "Local measurements still needed." {
		t.Fatal("account changed local work")
	}
	if !strings.Contains(prompt.String(), "Receiving work does not commit") {
		t.Fatal("learning omitted adoption boundary")
	}
	// Raw kernel work in a hand-written account is rejected before any deposit.
	for _, name := range []string{"work.declared", "work.closed"} {
		if err := os.WriteFile(filepath.Join(account, "record.jsonl"), []byte(line(t, name, workItem{Name: "foreign", Description: "x"})), 0600); err != nil {
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

func TestLoopCanSettleWithUnresolvedWork(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "work.declared", workItem{Name: "weigh-spool", Description: "Waiting for a physical measurement."}))
	var out, diag bytes.Buffer
	if err := cmdLoop(h, []string{"--max-passes", "3", "--settle", "2", "--", "sh", "-c", "cat >/dev/null"}, &out, &diag); err != nil {
		t.Fatal(err)
	}
	if len(replayed(t, h).openWork()) != 1 || !strings.Contains(diag.String(), "converged after 2") {
		t.Fatal("quiet work did not survive convergence")
	}
}

func TestWorkFixturesRunAcrossPersonas(t *testing.T) {
	for _, tc := range []struct{ fixture, name, result, evidence string }{
		{"printer", "enclosure-fit", "print.assessed", "143.75"},
		{"household", "thursday-dinner", "shopping.prepared", "tomatoes"},
		{"author", "sum-filament", "artifact.written", "negative input rejected"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			h := home(t)
			fixture, err := os.ReadFile(filepath.Join("examples", "work", tc.fixture+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			heard(t, h, string(fixture))
			mind, err := filepath.Abs(filepath.Join("examples", "work", "mind.py"))
			if err != nil {
				t.Fatal(err)
			}
			var out, diag bytes.Buffer
			if err := cmdLoop(h, []string{"--max-passes", "4", "--timeout", "5s", "--", "python3", mind}, &out, &diag); err != nil {
				t.Fatal(err)
			}
			st := replayed(t, h)
			if len(st.openWork()) != 0 || len(st.Caps) != 0 {
				t.Fatal("work required capabilities or failed to close")
			}
			detail, err := workDetail(st, tc.name)
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
	fixture, err := os.ReadFile(filepath.Join("examples", "work", "printer.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	_, withoutMeasurement, ok := strings.Cut(string(fixture), "\n")
	if !ok {
		t.Fatal("missing fixture lines")
	}
	heard(t, h, withoutMeasurement)
	mind, err := filepath.Abs(filepath.Join("examples", "work", "mind.py"))
	if err != nil {
		t.Fatal(err)
	}
	before := len(replayed(t, h).Events)
	var out, diag bytes.Buffer
	if err := cmdLoop(h, []string{"--max-passes", "3", "--", "python3", mind}, &out, &diag); err != nil {
		t.Fatal(err)
	}
	st := replayed(t, h)
	if len(st.Events) != before || len(st.openWork()) != 1 {
		t.Fatal("missing evidence produced a result or closed work")
	}
}
