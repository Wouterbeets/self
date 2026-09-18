package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func requireSandbox(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("guarded loop is Linux-only")
	}
	for _, name := range []string{"bwrap", "prlimit", "git"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("guarded loop requires %s", name)
		}
	}
}

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func testRepository(t *testing.T) (repo, remote string) {
	t.Helper()
	root := t.TempDir()
	remote, repo = filepath.Join(root, "remote.git"), filepath.Join(root, "repo")
	if out, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("init bare: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "init", "-b", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("init repo: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "add", "README.md")
	testGit(t, repo, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "base")
	testGit(t, repo, "remote", "add", "origin", remote)
	testGit(t, repo, "push", "-u", "origin", "main")
	return repo, remote
}

func TestSandboxMechanicallySeparatesPlannerAndWorker(t *testing.T) {
	requireSandbox(t)
	root := t.TempDir()
	repo, worktree := filepath.Join(root, "repo"), filepath.Join(root, "worktree")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(worktree, 0755); err != nil {
		t.Fatal(err)
	}
	plannerTarget := filepath.Join(repo, "denied")
	err := runSandbox(context.Background(), "", repo, []string{"sh", "-c", "printf bad > denied"}, nil, io.Discard, io.Discard, 512, 30)
	if err == nil {
		t.Fatal("planner wrote to read-only repository")
	}
	if _, err := os.Stat(plannerTarget); !os.IsNotExist(err) {
		t.Fatalf("planner target exists: %v", err)
	}
	outside := filepath.Join(repo, "escape")
	err = runSandbox(context.Background(), worktree, worktree, []string{"sh", "-c", "printf good > allowed; printf bad > " + outside}, nil, io.Discard, io.Discard, 512, 30)
	if err == nil {
		t.Fatal("worker escaped its writable worktree")
	}
	if got, err := os.ReadFile(filepath.Join(worktree, "allowed")); err != nil || string(got) != "good" {
		t.Fatalf("allowed worker write=%q, %v", got, err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("worker escape exists: %v", err)
	}
}

func TestSandboxProtectsLinkedWorktreeGitPointer(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	worktree := filepath.Join(t.TempDir(), "worktree")
	if err := createGoalWorktree(repo, worktree, "goal/protected"); err != nil {
		t.Fatal(err)
	}
	before, err := gitPointer(worktree)
	if err != nil {
		t.Fatal(err)
	}
	err = runSandbox(context.Background(), worktree, worktree, []string{"sh", "-c", "printf 'gitdir: /tmp/attacker' > .git"}, nil, io.Discard, io.Discard, 512, 30)
	if err == nil {
		t.Fatal("worker replaced linked-worktree Git pointer")
	}
	if err := verifyGitPointer(worktree, before); err != nil {
		t.Fatal(err)
	}
}

func TestGuardedLoopCompletesInIsolatedWorktreeAndVerifiesRemote(t *testing.T) {
	requireSandbox(t)
	repo, remote := testRepository(t)
	home, worktrees := t.TempDir(), filepath.Join(t.TempDir(), "worktrees")
	if err := os.WriteFile(filepath.Join(repo, "unrelated.tmp"), []byte("leave me\n"), 0644); err != nil {
		t.Fatal(err)
	}
	before := testGit(t, repo, "status", "--porcelain=v1", "--untracked-files=all")
	plan := guardedPlan{
		Goal: "g", Project: repo, Summary: "write result", CommitMessage: "add result",
		Actions: []plannedAction{
			{Kind: "worker", Goal: "g", Project: repo, Command: []string{"sh", "-c", "printf result > result.txt"}, Files: []string{"result.txt"}},
			{Kind: "push", Goal: "g", Project: repo},
		},
		Checks: []plannedCheck{{Command: []string{"sh", "-c", "test \"$(cat result.txt)\" = result"}, Heavy: true}},
	}
	raw, _ := json.Marshal(plan)
	planner := []string{"sh", "-c", "printf '%s' " + shellQuote(string(raw))}
	args := []string{"--guarded", "--goal", "g", "--repo", repo, "--branch", "goal/g", "--worktree-root", worktrees, "--min-free-mb=1", "--memory-mb=512", "--cpu-seconds=30", "--timeout=10s", "--"}
	args = append(args, planner...)
	var diag bytes.Buffer
	if err := cmdGuardedLoop(home, args, io.Discard, &diag); err != nil {
		t.Fatalf("guarded loop: %v\n%s", err, diag.String())
	}
	after := testGit(t, repo, "status", "--porcelain=v1", "--untracked-files=all")
	if after != before {
		t.Fatalf("invocation checkout changed:\nbefore %q\nafter  %q", before, after)
	}
	if _, err := os.Stat(filepath.Join(repo, "result.txt")); !os.IsNotExist(err) {
		t.Fatalf("result leaked into invocation checkout: %v", err)
	}
	remoteCommit := testGit(t, repo, "ls-remote", remote, "refs/heads/goal/g")
	if remoteCommit == "" {
		t.Fatal("goal branch was not pushed")
	}
	st, err := loadState(home)
	if err != nil {
		t.Fatal(err)
	}
	completed, released := false, false
	for _, e := range st.Events {
		completed = completed || e.Name == "loop.pass.completed"
		released = released || e.Name == leaseReleased
	}
	if !completed || !released || len(st.Passes) != 1 {
		t.Fatalf("completed=%v released=%v passes=%d\n%s", completed, released, len(st.Passes), diag.String())
	}
	for _, audit := range st.Passes {
		if audit.Status != "completed" {
			t.Fatalf("pass status=%q", audit.Status)
		}
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func TestGuardedDispatchFailureIsDurableAndNeverFallsBack(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	testGit(t, repo, "branch", "goal/existing")
	home := t.TempDir()
	before := testGit(t, repo, "status", "--porcelain=v1", "--untracked-files=all")
	plan := guardedPlan{
		Goal: "g", Project: repo, CommitMessage: "test",
		Actions: []plannedAction{{Kind: "worker", Goal: "g", Project: repo, Command: []string{"true"}, Files: []string{"x"}}, {Kind: "push", Goal: "g", Project: repo}},
		Checks:  []plannedCheck{{Command: []string{"true"}}},
	}
	raw, _ := json.Marshal(plan)
	err := cmdGuardedLoop(home, []string{"--guarded", "--goal", "g", "--repo", repo, "--branch", "goal/existing", "--worktree-root", filepath.Join(t.TempDir(), "worktrees"), "--min-free-mb=1", "--", "sh", "-c", "printf '%s' " + shellQuote(string(raw))}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no direct-edit fallback") {
		t.Fatalf("error=%v", err)
	}
	if after := testGit(t, repo, "status", "--porcelain=v1", "--untracked-files=all"); after != before {
		t.Fatalf("invocation checkout changed: before=%q after=%q", before, after)
	}
	st, _ := loadState(home)
	if len(st.Passes) != 1 {
		t.Fatalf("passes=%d", len(st.Passes))
	}
	for _, pass := range st.Passes {
		if pass.Status != "failed" || !strings.Contains(strings.Join(pass.Failures, " "), "after one retry") {
			t.Fatalf("pass=%+v", pass)
		}
	}
}

func TestCheckpointApprovalRerunIsConsumedExactlyOnce(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	home, worktrees := t.TempDir(), filepath.Join(t.TempDir(), "worktrees")
	plan := guardedPlan{
		Goal: "g", Project: repo, CommitMessage: "approved work",
		Actions: []plannedAction{
			{Kind: "worker", Goal: "adjacent", Project: repo, Command: []string{"sh", "-c", "printf approved > approved.txt"}, Files: []string{"approved.txt"}},
			{Kind: "push", Goal: "g", Project: repo},
		},
		Checks: []plannedCheck{{Command: []string{"test", "-f", "approved.txt"}}},
	}
	raw, _ := json.Marshal(plan)
	args := []string{"--guarded", "--goal", "g", "--repo", repo, "--branch", "goal/approved", "--worktree-root", worktrees, "--min-free-mb=1", "--timeout=10s", "--", "sh", "-c", "printf '%s' " + shellQuote(string(raw))}
	err := cmdGuardedLoop(home, args, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "requires human approval") {
		t.Fatalf("first pass error=%v", err)
	}
	st, _ := loadState(home)
	var id string
	for candidate, c := range st.Checkpoints {
		if c.pending() {
			id = candidate
		}
	}
	if id == "" {
		t.Fatal("no pending checkpoint")
	}
	if _, err := checkpointChange(home, "approve", "", id, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := cmdGuardedLoop(home, args, io.Discard, io.Discard); err != nil {
		t.Fatalf("approved rerun: %v", err)
	}
	st, _ = loadState(home)
	if c := st.Checkpoints[id]; c.Consumed == 0 || c.usable() {
		t.Fatalf("checkpoint after rerun=%+v", c)
	}
}

func TestTimedOutWorkerCanResumeAfterLeaseExpiry(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	home, worktrees := t.TempDir(), filepath.Join(t.TempDir(), "worktrees")
	makePlan := func(command string) string {
		plan := guardedPlan{
			Goal: "g", Project: repo, CommitMessage: "resume work",
			Actions: []plannedAction{
				{Kind: "worker", Goal: "g", Project: repo, Command: []string{"sh", "-c", command}, Files: []string{"result.txt"}},
				{Kind: "push", Goal: "g", Project: repo},
			},
			Checks: []plannedCheck{{Command: []string{"test", "-s", "result.txt"}}},
		}
		raw, _ := json.Marshal(plan)
		return "printf '%s' " + shellQuote(string(raw))
	}
	base := []string{"--guarded", "--goal", "g", "--repo", repo, "--branch", "goal/resume", "--worktree-root", worktrees, "--min-free-mb=1", "--lease-ttl=400ms", "--timeout=100ms", "--", "sh", "-c"}
	first := append(append([]string{}, base...), makePlan("printf partial > result.txt; sleep 5"))
	err := cmdGuardedLoop(home, first, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "worker timed out") {
		t.Fatalf("timeout error=%v", err)
	}
	st, _ := loadState(home)
	var pass string
	for id := range st.Passes {
		pass = id
	}
	if pass == "" || st.Passes[pass].Status != "resumable" {
		t.Fatalf("passes=%+v", st.Passes)
	}
	time.Sleep(450 * time.Millisecond)
	secondBase := []string{"--guarded", "--resume", pass, "--goal", "g", "--repo", repo, "--branch", "goal/resume", "--worktree-root", worktrees, "--min-free-mb=1", "--lease-ttl=2s", "--timeout=1s", "--", "sh", "-c"}
	second := append(secondBase, makePlan("printf resumed > result.txt"))
	if err := cmdGuardedLoop(home, second, io.Discard, io.Discard); err != nil {
		t.Fatalf("resume: %v", err)
	}
	st, _ = loadState(home)
	if st.Passes[pass].Status != "completed" || !st.Leases["g"].Released {
		t.Fatalf("pass=%+v lease=%+v", st.Passes[pass], st.Leases["g"])
	}
}

func TestPostBoundaryGuardedFlagStaysLegacyMindArgument(t *testing.T) {
	home := t.TempDir()
	var out, diag bytes.Buffer
	if err := cmdLoop(home, []string{"--max-passes=1", "--settle=1", "--", "sh", "-c", "test \"$1\" = --guarded", "mind", "--guarded"}, &out, &diag); err != nil {
		t.Fatalf("legacy loop reinterpreted mind argument: %v\n%s", err, diag.String())
	}
	opts, err := parseGuardedOptions([]string{"--guarded", "--goal", "g", "--repo", ".", "--branch", "b", "--", "mind", "--guarded"})
	if err != nil {
		t.Fatal(err)
	}
	if got := opts.Planner[len(opts.Planner)-1]; got != "--guarded" {
		t.Fatalf("planner tail=%q", got)
	}
}

func TestGuardedPolicyForbidsDangerousActionsAndNeedsChecks(t *testing.T) {
	opts := guardedOptions{Goal: "g", Repo: "/repo", MaxFiles: 20}
	base := guardedPlan{Goal: "g", Project: "/repo", CommitMessage: "x", Actions: []plannedAction{{Kind: "worker", Goal: "g", Project: "/repo", Command: []string{"true"}, Files: []string{"x"}}}}
	for action := range forbiddenActions {
		t.Run(action, func(t *testing.T) {
			plan := base
			plan.Actions = append(append([]plannedAction{}, base.Actions...), plannedAction{Kind: action, Goal: "g", Project: "/repo"})
			if _, err := validatePlan(opts, plan); err == nil || !strings.Contains(err.Error(), "forbidden") {
				t.Fatalf("error=%v", err)
			}
		})
	}
	if _, err := validatePlan(opts, base); err == nil || !strings.Contains(err.Error(), "verification check") {
		t.Fatalf("missing checks error=%v", err)
	}
}

func TestGuardedBudgetsAreConfigurableAndPlanOutputIsBounded(t *testing.T) {
	opts, err := parseGuardedOptions([]string{"--guarded", "--goal", "g", "--repo", ".", "--branch", "b", "--max-files=3", "--budgets", `{"changed_files":3,"pushes":0}`, "--", "mind"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Budget.Limits["changed_files"] != 3 || opts.Budget.Limits["pushes"] != 0 {
		t.Fatalf("budget=%+v", opts.Budget)
	}
	b := boundedBuffer{Limit: 3}
	if _, err := b.Write([]byte("four")); err == nil {
		t.Fatal("oversized planner output was accepted")
	}
}

func TestGuardedRejectsSelfHomeSymlinkIntoCheckout(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	target := filepath.Join(repo, ".self")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(target, home); err != nil {
		t.Fatal(err)
	}
	err := cmdGuardedLoop(home, []string{"--guarded", "--goal", "g", "--repo", repo, "--branch", "b", "--min-free-mb=1", "--", "true"}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "SELF_HOME must be outside") {
		t.Fatalf("error=%v", err)
	}
}
