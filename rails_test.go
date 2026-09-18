package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLeaseLifecycleAndReplay(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	first := goalLease{Goal: "g", Owner: "one", Invocation: "i1", Repository: "/repo", Branch: "goal/g", Worktree: "/wt/one"}
	events, err := leaseChange(home, "acquire", first, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(events[0].Payload, &first)
	if _, err := leaseChange(home, "renew", first, 2*time.Minute, now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	st, err := loadState(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Leases["g"]; got == nil || got.Owner != "one" || !got.ExpiresAt.Equal(now.Add(150*time.Second)) {
		t.Fatalf("renewed lease = %+v", got)
	}
	if _, err := leaseChange(home, "steal", goalLease{Goal: "g", Owner: "two", Invocation: "i2"}, time.Minute, now.Add(time.Minute)); err == nil {
		t.Fatal("active lease was stolen")
	}
	second := goalLease{Goal: "g", Owner: "two", Invocation: "i2", Repository: "/repo", Branch: "goal/g/two", Worktree: "/wt/two"}
	events, err = leaseChange(home, "steal", second, time.Minute, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(events[0].Payload, &second)
	if _, err := leaseChange(home, "release", first, 0, now); err == nil {
		t.Fatal("old owner released stolen lease")
	}
	if _, err := leaseChange(home, "release", second, 0, now); err != nil {
		t.Fatal(err)
	}
	st, _ = loadState(home)
	if !st.Leases["g"].Released {
		t.Fatal("release did not survive replay")
	}
}

func TestConcurrentLeaseAcquisitionHasOneWinner(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	const contenders = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			lease := goalLease{Goal: "same", Owner: fmt.Sprintf("owner-%d", i), Invocation: fmt.Sprintf("invocation-%d", i), Repository: "/repo", Branch: fmt.Sprintf("goal/%d", i), Worktree: fmt.Sprintf("/wt/%d", i)}
			_, err := leaseChange(home, "acquire", lease, time.Minute, now)
			if err == nil {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if winners != 1 {
		t.Fatalf("winners=%d, want 1", winners)
	}
	st, err := loadState(home)
	if err != nil {
		t.Fatal(err)
	}
	refused := 0
	for _, e := range st.Events {
		if e.Name == leaseRefused {
			refused++
		}
	}
	if refused != contenders-1 {
		t.Fatalf("durable refusals=%d, want %d", refused, contenders-1)
	}
}

func TestLeaseRejectsConflictingBranchAcrossGoals(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	first := goalLease{Goal: "one", Owner: "a", Invocation: "i1", Repository: "/repo", Branch: "goal/shared", Worktree: "/wt/one"}
	second := goalLease{Goal: "two", Owner: "b", Invocation: "i2", Repository: "/repo", Branch: "goal/shared", Worktree: "/wt/two"}
	if _, err := leaseChange(home, "acquire", first, time.Minute, now); err != nil {
		t.Fatal(err)
	}
	if _, err := leaseChange(home, "acquire", second, time.Minute, now); err == nil || !strings.Contains(err.Error(), "conflicts with goal") {
		t.Fatalf("conflict error=%v", err)
	}
}

func TestLeaseBranchConflictsAreRepositoryScoped(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for _, lease := range []goalLease{
		{Goal: "one", Owner: "a", Invocation: "i1", Repository: "/repo/one", Branch: "goal/shared", Worktree: "/wt/shared"},
		{Goal: "two", Owner: "b", Invocation: "i2", Repository: "/repo/two", Branch: "goal/shared", Worktree: "/wt/two"},
	} {
		if _, err := leaseChange(home, "acquire", lease, time.Minute, now); err != nil {
			t.Fatalf("cross-repository lease conflict: %v", err)
		}
	}
}

func TestLeaseWorktreeConflictIsGlobal(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	first := goalLease{Goal: "one", Owner: "a", Invocation: "i1", Repository: "/repo/one", Branch: "one", Worktree: "/wt/shared"}
	second := goalLease{Goal: "two", Owner: "b", Invocation: "i2", Repository: "/repo/two", Branch: "two", Worktree: "/wt/shared"}
	if _, err := leaseChange(home, "acquire", first, time.Minute, now); err != nil {
		t.Fatal(err)
	}
	if _, err := leaseChange(home, "acquire", second, time.Minute, now); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("shared worktree error=%v", err)
	}
}

func TestForgedRailEventsAreInert(t *testing.T) {
	payload, _ := json.Marshal(goalLease{Goal: "g", Owner: "attacker", Invocation: "fake", ExpiresAt: time.Now().Add(time.Hour)})
	forged := newEvent(leaseAcquired, payload)
	forged.Via = doorHear
	st := replay([]Event{forged}, nil)
	if st.Leases["g"] != nil {
		t.Fatal("a non-kernel rail event acquired a lease")
	}
}

func TestCompletionRequiresCurrentLeaseAndReleasesAtomically(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	old := goalLease{Goal: "g", Owner: "one", Invocation: "i1", Repository: "/r", Branch: "b", Worktree: "/w"}
	events, err := leaseChange(home, "acquire", old, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(events[0].Payload, &old)
	newOwner := goalLease{Goal: "g", Owner: "one", Invocation: "i1", Repository: "/r", Branch: "b", Worktree: "/w"}
	events, err = leaseChange(home, "steal", newOwner, time.Minute, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(events[0].Payload, &newOwner)
	if err := finishGuardedPass(home, old, map[string]any{"pass": "p", "goal": "g"}, now.Add(2*time.Minute)); err == nil {
		t.Fatal("stale owner completed work")
	}
	if err := finishGuardedPass(home, newOwner, map[string]any{"pass": "p", "goal": "g"}, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	st, _ := loadState(home)
	if !st.Leases["g"].Released || st.Passes["p"].Status != "completed" {
		t.Fatalf("lease=%+v pass=%+v", st.Leases["g"], st.Passes["p"])
	}
	completedSeq := st.Passes["p"].Events[len(st.Passes["p"].Events)-1].Seq
	if st.Leases["g"].Seq != completedSeq+1 {
		t.Fatalf("completion seq=%d release seq=%d", completedSeq, st.Leases["g"].Seq)
	}
}

func TestCheckpointIsExactDurableAndOneShot(t *testing.T) {
	home := t.TempDir()
	actions := `[{"kind":"push","branch":"goal/g"}]`
	events, err := checkpointChange(home, "request", "pass-1", "", actions, "")
	if err != nil {
		t.Fatal(err)
	}
	var requested checkpoint
	if err := json.Unmarshal(events[0].Payload, &requested); err != nil {
		t.Fatal(err)
	}
	canonical, _ := canonicalActions(actions)
	if requested.ID != checkpointID("pass-1", canonical) {
		t.Fatalf("unstable id %q", requested.ID)
	}
	if _, err := checkpointChange(home, "approve", "", requested.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	st, _ := loadState(home)
	if !st.Checkpoints[requested.ID].usable() {
		t.Fatal("approved checkpoint not usable after replay")
	}
	if _, err := checkpointChange(home, "consume", "", requested.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := checkpointChange(home, "consume", "", requested.ID, "", ""); err == nil {
		t.Fatal("checkpoint consumed twice")
	}
	changed, _ := canonicalActions(`[{"kind":"push","branch":"other"}]`)
	if checkpointID("pass-1", changed) == requested.ID {
		t.Fatal("altered action set retained approval id")
	}
}

func TestCheckpointRejectionPreservesReason(t *testing.T) {
	home := t.TempDir()
	events, err := checkpointChange(home, "request", "p", "", `[{"kind":"open-pr"}]`, "")
	if err != nil {
		t.Fatal(err)
	}
	var c checkpoint
	json.Unmarshal(events[0].Payload, &c)
	if _, err := checkpointChange(home, "reject", "", c.ID, "", "not this release"); err != nil {
		t.Fatal(err)
	}
	st, _ := loadState(home)
	got := st.Checkpoints[c.ID]
	if got.Reason != "not this release" || got.Rejected == 0 || got.usable() {
		t.Fatalf("rejected checkpoint = %+v", got)
	}
}

func TestGuardedBudgetEveryLimit(t *testing.T) {
	for category, limit := range guardedBudget().Limits {
		t.Run(category, func(t *testing.T) {
			b := guardedBudget()
			if err := b.consume(category, limit); err != nil {
				t.Fatal(err)
			}
			if err := b.consume(category, 1); err == nil || !strings.Contains(err.Error(), "exhausted") {
				t.Fatalf("over limit error=%v", err)
			}
			if b.Used[category] != limit {
				t.Fatalf("failed consumption changed use to %d", b.Used[category])
			}
		})
	}
}

func TestActionScopeClassification(t *testing.T) {
	cases := []struct {
		name string
		a    map[string]string
		want actionScope
	}{
		{"inside", map[string]string{"goal": "g", "project": "p"}, scopeInside},
		{"decomposition", map[string]string{"kind": "create-goal", "parent": "g", "project": "p"}, scopeDecomposition},
		{"adjacent", map[string]string{"goal": "other", "project": "p"}, scopeAdjacent},
		{"expanding", map[string]string{"goal": "other", "project": "q"}, scopeExpanding},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyAction("g", "p", tc.a); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestLoopViewReplaysPassLeaseAndCheckpoint(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	leaseChange(home, "acquire", goalLease{Goal: "g", Owner: "o", Invocation: "i", Repository: "/r", Branch: "b", Worktree: "/w"}, time.Minute, now)
	cp, _ := checkpointChange(home, "request", "pass-1", "", `[{"kind":"push"}]`, "")
	_ = cp
	plan, _ := json.Marshal(map[string]any{"pass": "pass-1", "goal": "g", "plan": map[string]any{"actions": []string{"worker"}}})
	if err := appendEvents(home, []Event{railEvent("loop.pass.planned", json.RawMessage(plan))}); err != nil {
		t.Fatal(err)
	}
	st, _ := loadState(home)
	page, err := builtinLoopView(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pass-1", "goal=g", "unreleased leases", "owner=o", "checkpoints", "pending"} {
		if !strings.Contains(string(page), want) {
			t.Fatalf("view missing %q:\n%s", want, page)
		}
	}
}
