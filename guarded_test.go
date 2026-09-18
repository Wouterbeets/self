package main

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
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

func TestSandboxSupportsRepositoriesUnderTmp(t *testing.T) {
	requireSandbox(t)
	root, err := os.MkdirTemp("/tmp", "self-guarded-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "readable"), []byte("yes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runSandbox(context.Background(), "", repo, []string{"test", "-f", "readable"}, nil, io.Discard, io.Discard, 512, 30); err != nil {
		t.Fatalf("read-only /tmp repository unavailable: %v", err)
	}
	if err := runSandbox(context.Background(), repo, repo, []string{"sh", "-c", "printf yes > writable"}, nil, io.Discard, io.Discard, 512, 30); err != nil {
		t.Fatalf("writable /tmp worktree unavailable: %v", err)
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
	external := filepath.Join(t.TempDir(), "external")
	testGit(t, repo, "worktree", "add", "-b", "goal/existing", external, "HEAD")
	home := t.TempDir()
	before := testGit(t, repo, "status", "--porcelain=v1", "--untracked-files=all")
	plan := guardedPlan{
		Goal: "g", Project: repo, CommitMessage: "test",
		Actions: []plannedAction{{Kind: "worker", Goal: "g", Project: repo, Command: []string{"true"}, Files: []string{"x"}}, {Kind: "push", Goal: "g", Project: repo}},
		Checks:  []plannedCheck{{Command: []string{"true"}}},
	}
	raw, _ := json.Marshal(plan)
	err := cmdGuardedLoop(home, []string{"--guarded", "--goal", "g", "--repo", repo, "--branch", "goal/existing", "--worktree-root", filepath.Join(t.TempDir(), "worktrees"), "--min-free-mb=1", "--", "sh", "-c", "printf '%s' " + shellQuote(string(raw))}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "registered worktree") {
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
		if pass.Status != "failed" || !strings.Contains(strings.Join(pass.Failures, " "), "registered worktree") {
			t.Fatalf("pass=%+v", pass)
		}
	}
}

func guardedArgs(repo, worktrees, goal, branch string, plan guardedPlan) []string {
	raw, _ := json.Marshal(plan)
	return []string{"--guarded", "--goal", goal, "--repo", repo, "--branch", branch, "--worktree-root", worktrees, "--min-free-mb=1", "--timeout=2s", "--", "sh", "-c", "printf '%s' " + shellQuote(string(raw))}
}

func TestChecksCannotMutateWorkerOutput(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	plan := guardedPlan{
		Goal: "g", Project: repo, CommitMessage: "must not commit",
		Actions: []plannedAction{{Kind: "worker", Goal: "g", Project: repo, Command: []string{"sh", "-c", "printf worker > result.txt"}, Files: []string{"result.txt"}}, {Kind: "push", Goal: "g", Project: repo}},
		Checks:  []plannedCheck{{Command: []string{"sh", "-c", "printf adversary > result.txt"}}},
	}
	home := t.TempDir()
	err := cmdGuardedLoop(home, guardedArgs(repo, filepath.Join(t.TempDir(), "worktrees"), "g", "goal/check-ro", plan), io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "check 1 failed") {
		t.Fatalf("mutating check error=%v", err)
	}
	if out := testGit(t, repo, "ls-remote", "origin", "refs/heads/goal/check-ro"); out != "" {
		t.Fatalf("mutating check produced remote branch: %s", out)
	}
}

func TestCheckMutationDetectorRejectsAnyStatusChange(t *testing.T) {
	if err := checkDidNotMutate(" M declared\n", " M declared\n?? surprise\n", 3); err == nil || !strings.Contains(err.Error(), "check 3 mutated") {
		t.Fatalf("detector error=%v", err)
	}
}

func TestRepeatedPassesAttachBranchAndCleanWorktrees(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	home, root := t.TempDir(), filepath.Join(t.TempDir(), "worktrees")
	run := func(file string) {
		plan := guardedPlan{
			Goal: "g", Project: repo, CommitMessage: "add " + file,
			Actions: []plannedAction{{Kind: "worker", Goal: "g", Project: repo, Command: []string{"sh", "-c", "printf pass > " + file}, Files: []string{file}}, {Kind: "push", Goal: "g", Project: repo}},
			Checks:  []plannedCheck{{Command: []string{"test", "-f", file}}},
		}
		if err := cmdGuardedLoop(home, guardedArgs(repo, root, "g", "goal/repeated", plan), io.Discard, io.Discard); err != nil {
			t.Fatalf("pass for %s: %v", file, err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("successful worktree leaked: entries=%v err=%v", entries, err)
		}
	}
	run("one.txt")
	first := strings.Fields(testGit(t, repo, "ls-remote", "origin", "refs/heads/goal/repeated"))[0]
	run("two.txt")
	second := strings.Fields(testGit(t, repo, "ls-remote", "origin", "refs/heads/goal/repeated"))[0]
	if first == second {
		t.Fatal("second pass did not advance remote branch")
	}
	if parent := testGit(t, repo, "rev-parse", second+"^"); parent != first {
		t.Fatalf("second pass is not additive: parent=%s first=%s", parent, first)
	}
}

func TestCreateGoalWorktreeAttachesRemoteOnlyBranch(t *testing.T) {
	repo, _ := testRepository(t)
	head := testGit(t, repo, "rev-parse", "HEAD")
	testGit(t, repo, "push", "origin", "HEAD:refs/heads/goal/remote-only")
	testGit(t, repo, "update-ref", "-d", "refs/remotes/origin/goal/remote-only")
	if cmd := exec.Command("git", "-C", repo, "show-ref", "--verify", "refs/remotes/origin/goal/remote-only"); cmd.Run() == nil {
		t.Fatal("test setup unexpectedly has a remote-tracking ref")
	}
	worktree := filepath.Join(t.TempDir(), "worktree")
	if err := createGoalWorktree(repo, worktree, "goal/remote-only"); err != nil {
		t.Fatal(err)
	}
	if branch := testGit(t, worktree, "branch", "--show-current"); branch != "goal/remote-only" {
		t.Fatalf("branch=%q", branch)
	}
	if got := testGit(t, worktree, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD=%s, want remote %s", got, head)
	}
}

func TestCreateGoalWorktreeRejectsDeletedRemoteWithStaleTrackingRef(t *testing.T) {
	repo, remote := testRepository(t)
	testGit(t, repo, "push", "origin", "HEAD:refs/heads/goal/deleted")
	if _, err := repoOutput(repo, "show-ref", "--verify", "refs/remotes/origin/goal/deleted"); err != nil {
		t.Fatal("test setup has no tracking ref")
	}
	testGit(t, remote, "update-ref", "-d", "refs/heads/goal/deleted")
	err := createGoalWorktree(repo, filepath.Join(t.TempDir(), "worktree"), "goal/deleted")
	if err == nil || !strings.Contains(err.Error(), "stale tracking ref") {
		t.Fatalf("stale tracking error=%v", err)
	}
}

func TestApprovedCheckpointSurvivesPreDispatchConflict(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	home, root := t.TempDir(), filepath.Join(t.TempDir(), "worktrees")
	plan := guardedPlan{
		Goal: "g", Project: repo, CommitMessage: "approved",
		Actions: []plannedAction{{Kind: "worker", Goal: "adjacent", Project: repo, Command: []string{"sh", "-c", "printf x > x"}, Files: []string{"x"}}, {Kind: "push", Goal: "g", Project: repo}},
		Checks:  []plannedCheck{{Command: []string{"test", "-f", "x"}}},
	}
	args := guardedArgs(repo, root, "g", "goal/approval-conflict", plan)
	if err := cmdGuardedLoop(home, args, io.Discard, io.Discard); err == nil {
		t.Fatal("checkpoint was not requested")
	}
	st, _ := loadState(home)
	var id string
	for candidate := range st.Checkpoints {
		id = candidate
	}
	if _, err := checkpointChange(home, "approve", "", id, "", ""); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external")
	testGit(t, repo, "worktree", "add", "-b", "goal/approval-conflict", external, "HEAD")
	if err := cmdGuardedLoop(home, args, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "registered worktree") {
		t.Fatalf("pre-dispatch error=%v", err)
	}
	st, _ = loadState(home)
	if !st.Checkpoints[id].usable() {
		t.Fatalf("approval was consumed before dispatch: %+v", st.Checkpoints[id])
	}
}

func TestApprovedCheckpointSurvivesWorkerPreflightFailure(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	if err := os.WriteFile(filepath.Join(repo, "bad-worker"), []byte("\x7fELFtruncated"), 0755); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "add", "bad-worker")
	testGit(t, repo, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "add invalid worker")
	home, root := t.TempDir(), filepath.Join(t.TempDir(), "worktrees")
	plan := guardedPlan{
		Goal: "g", Project: repo, CommitMessage: "approved",
		Actions: []plannedAction{{Kind: "worker", Goal: "adjacent", Project: repo, Command: []string{"./bad-worker"}, Files: []string{"x"}}, {Kind: "push", Goal: "g", Project: repo}},
		Checks:  []plannedCheck{{Command: []string{"true"}}},
	}
	args := guardedArgs(repo, root, "g", "goal/approval-preflight", plan)
	if err := cmdGuardedLoop(home, args, io.Discard, io.Discard); err == nil {
		t.Fatal("checkpoint was not requested")
	}
	st, _ := loadState(home)
	var id string
	for candidate := range st.Checkpoints {
		id = candidate
	}
	if _, err := checkpointChange(home, "approve", "", id, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := cmdGuardedLoop(home, args, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "worker executable") {
		t.Fatalf("preflight error=%v", err)
	}
	st, _ = loadState(home)
	if !st.Checkpoints[id].usable() {
		t.Fatalf("approval was consumed during worker preflight: %+v", st.Checkpoints[id])
	}
}

func TestExecutableValidationRejectsNonExecutableAndWrongArchitectureELF(t *testing.T) {
	source, err := os.ReadFile("/bin/true")
	if err != nil || len(source) < 20 {
		t.Skip("no ELF /bin/true available")
	}
	var order binary.ByteOrder = binary.LittleEndian
	if source[5] == 2 {
		order = binary.BigEndian
	}
	for name, mutate := range map[string]func([]byte){
		"relocatable":        func(data []byte) { order.PutUint16(data[16:18], uint16(elf.ET_REL)) },
		"wrong architecture": func(data []byte) { order.PutUint16(data[18:20], uint16(elf.EM_MIPS)) },
	} {
		t.Run(name, func(t *testing.T) {
			data := append([]byte(nil), source...)
			mutate(data)
			path := filepath.Join(t.TempDir(), "candidate")
			if err := os.WriteFile(path, data, 0755); err != nil {
				t.Fatal(err)
			}
			if err := validateExecutable(path); err == nil {
				t.Fatal("invalid ELF accepted as executable")
			}
		})
	}
}

func TestLiveHerdrAgentBlocksGuardedAcquisition(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	home := t.TempDir()
	payload, _ := json.Marshal(map[string]string{"agent": "writer-one", "goal": "g", "repo": repo, "branch": "goal/live", "worktree": filepath.Join(t.TempDir(), "agent-worktree")})
	event := newEvent("agent.started", payload)
	event.Via = doorHear
	if err := appendEvents(home, []Event{event}); err != nil {
		t.Fatal(err)
	}
	herdr := filepath.Join(t.TempDir(), "herdr")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"result\":{\"agents\":[{\"name\":\"writer-one\",\"agent_status\":\"working\"}]}}'\n"
	if err := os.WriteFile(herdr, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	plan := guardedPlan{Goal: "g", Project: repo, CommitMessage: "x", Actions: []plannedAction{{Kind: "worker", Goal: "g", Project: repo, Command: []string{"true"}, Files: []string{"x"}}, {Kind: "push", Goal: "g", Project: repo}}, Checks: []plannedCheck{{Command: []string{"true"}}}}
	args := guardedArgs(repo, filepath.Join(t.TempDir(), "worktrees"), "g", "goal/live", plan)
	args = append(args[:1], append([]string{"--herdr", herdr}, args[1:]...)...)
	err := cmdGuardedLoop(home, args, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "live Herdr agent writer-one") {
		t.Fatalf("live agent error=%v", err)
	}
}

func TestHerdrUnexpectedSchemaFailsClosed(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	home := t.TempDir()
	payload, _ := json.Marshal(map[string]string{"agent": "writer-one", "goal": "g", "repo": repo})
	event := newEvent("agent.started", payload)
	event.Via = doorHear
	if err := appendEvents(home, []Event{event}); err != nil {
		t.Fatal(err)
	}
	herdr := filepath.Join(t.TempDir(), "herdr")
	if err := os.WriteFile(herdr, []byte("#!/bin/sh\nprintf '{}\\n'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	plan := guardedPlan{Goal: "g", Project: repo, CommitMessage: "x", Actions: []plannedAction{{Kind: "worker", Goal: "g", Project: repo, Command: []string{"true"}, Files: []string{"x"}}, {Kind: "push", Goal: "g", Project: repo}}, Checks: []plannedCheck{{Command: []string{"true"}}}}
	args := guardedArgs(repo, filepath.Join(t.TempDir(), "worktrees"), "g", "goal/schema", plan)
	args = append(args[:1], append([]string{"--herdr", herdr}, args[1:]...)...)
	if err := cmdGuardedLoop(home, args, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "no agents array") {
		t.Fatalf("schema error=%v", err)
	}
}

func TestGitNetworkTimeoutKillsHelperProcessGroup(t *testing.T) {
	requireSandbox(t)
	repo, _ := testRepository(t)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	wrapper := filepath.Join(dir, "git")
	script := "#!/bin/sh\ncase \" $* \" in\n  *\" ls-remote \"*) sleep 30 & echo $! > " + shellQuote(pidFile) + "; wait;;\n  *) exec " + shellQuote(realGit) + " \"$@\";;\nesac\n"
	if err := os.WriteFile(wrapper, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	start := time.Now()
	err = verifyRemoteContext(ctx, repo, "never", strings.Repeat("0", 40))
	cancel()
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("timeout error=%v elapsed=%s", err, time.Since(start))
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	deadline := time.Now().Add(time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		t.Fatalf("git helper descendant %d survived timeout", pid)
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

func TestPostCommitAndPushCrashesReconcileWithoutWorkerRerun(t *testing.T) {
	requireSandbox(t)
	for _, phase := range []string{"after-commit-started", "after-commit-before-event", "after-commit", "after-push"} {
		t.Run(phase, func(t *testing.T) {
			repo, _ := testRepository(t)
			home, root := t.TempDir(), filepath.Join(t.TempDir(), "worktrees")
			branch := "goal/" + phase
			plan := guardedPlan{
				Goal: "g", Project: repo, CommitMessage: phase,
				Actions: []plannedAction{{Kind: "worker", Goal: "g", Project: repo, Command: []string{"sh", "-c", "test ! -e once && printf once > once"}, Files: []string{"once"}}, {Kind: "push", Goal: "g", Project: repo}},
				Checks:  []plannedCheck{{Command: []string{"test", "-f", "once"}}},
			}
			args := guardedArgs(repo, root, "g", branch, plan)
			args = append([]string{}, args...)
			for i, arg := range args {
				if arg == "--timeout=2s" {
					args[i] = "--timeout=500ms"
				}
			}
			for i, arg := range args {
				if arg == "--" {
					args = append(args[:i], append([]string{"--lease-ttl=1s"}, args[i:]...)...)
					break
				}
			}
			guardedFault = func(at string) error {
				if at == phase {
					return errors.New("simulated crash " + phase)
				}
				return nil
			}
			err := cmdGuardedLoop(home, args, io.Discard, io.Discard)
			guardedFault = nil
			if err == nil || !strings.Contains(err.Error(), "simulated crash") {
				t.Fatalf("fault error=%v", err)
			}
			st, _ := loadState(home)
			var pass string
			for id := range st.Passes {
				pass = id
			}
			recovery := recoverPassState(st.Passes[pass])
			if !recovery.CommitBegan || recovery.Completed {
				t.Fatalf("recovery state=%+v", recovery)
			}
			time.Sleep(1100 * time.Millisecond)
			resume := append([]string{"--guarded", "--resume", pass}, args[1:]...)
			if err := cmdGuardedLoop(home, resume, io.Discard, io.Discard); err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			st, _ = loadState(home)
			if !recoverPassState(st.Passes[pass]).Completed {
				t.Fatal("reconciled pass is not complete")
			}
			if err := cmdGuardedLoop(home, resume, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "already completed") {
				t.Fatalf("completed resume error=%v", err)
			}
		})
	}
}

func TestPushRaceFailsReconciliationWithoutWorkerRerun(t *testing.T) {
	requireSandbox(t)
	repo, remote := testRepository(t)
	home, root := t.TempDir(), filepath.Join(t.TempDir(), "worktrees")
	plan := guardedPlan{
		Goal: "g", Project: repo, CommitMessage: "guarded commit",
		Actions: []plannedAction{{Kind: "worker", Goal: "g", Project: repo, Command: []string{"sh", "-c", "test ! -e once && printf once > once"}, Files: []string{"once"}}, {Kind: "push", Goal: "g", Project: repo}},
		Checks:  []plannedCheck{{Command: []string{"test", "-f", "once"}}},
	}
	args := guardedArgs(repo, root, "g", "goal/push-race", plan)
	for i, arg := range args {
		if arg == "--timeout=2s" {
			args[i] = "--timeout=500ms"
		}
	}
	for i, arg := range args {
		if arg == "--" {
			args = append(args[:i], append([]string{"--lease-ttl=1s"}, args[i:]...)...)
			break
		}
	}
	guardedFault = func(at string) error {
		if at != "after-push" {
			return nil
		}
		clone := filepath.Join(t.TempDir(), "racer")
		if out, err := exec.Command("git", "clone", "-b", "goal/push-race", remote, clone).CombinedOutput(); err != nil {
			return fmt.Errorf("clone racer: %v: %s", err, out)
		}
		if err := os.WriteFile(filepath.Join(clone, "racer"), []byte("ahead\n"), 0644); err != nil {
			return err
		}
		testGit(t, clone, "add", "racer")
		testGit(t, clone, "-c", "user.name=racer", "-c", "user.email=racer@example.com", "commit", "-m", "race ahead")
		testGit(t, clone, "push", "origin", "HEAD:refs/heads/goal/push-race")
		return errors.New("crash after raced push")
	}
	err := cmdGuardedLoop(home, args, io.Discard, io.Discard)
	guardedFault = nil
	if err == nil || !strings.Contains(err.Error(), "crash after raced push") {
		t.Fatalf("fault error=%v", err)
	}
	st, _ := loadState(home)
	var pass string
	for id := range st.Passes {
		pass = id
	}
	time.Sleep(1100 * time.Millisecond)
	resume := append([]string{"--guarded", "--resume", pass}, args[1:]...)
	err = cmdGuardedLoop(home, resume, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "post-commit reconciliation failed") {
		t.Fatalf("race reconciliation error=%v", err)
	}
	st, _ = loadState(home)
	if recoverPassState(st.Passes[pass]).Completed {
		t.Fatal("diverged remote was marked completed")
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
	withoutPush := base
	withoutPush.Checks = []plannedCheck{{Command: []string{"true"}}}
	if _, err := validatePlan(opts, withoutPush); err == nil || !strings.Contains(err.Error(), "push action") {
		t.Fatalf("missing push error=%v", err)
	}
	expanding := base
	expanding.Actions = []plannedAction{{Kind: "worker", Goal: "other", Project: "/elsewhere", Command: []string{"true"}, Files: []string{"x"}}}
	if _, err := validatePlan(opts, expanding); err == nil || !strings.Contains(err.Error(), "scope-expanding") {
		t.Fatalf("scope expansion error=%v", err)
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
