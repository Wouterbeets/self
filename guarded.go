package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type guardedOptions struct {
	Goal         string
	Repo         string
	Branch       string
	WorktreeRoot string
	LeaseTTL     time.Duration
	Timeout      time.Duration
	MaxFiles     int
	MinFreeMB    uint64
	MemoryMB     uint64
	CPUSeconds   uint64
	Budget       passBudget
	Resume       string
	Planner      []string
}

type guardedPlan struct {
	Goal          string          `json:"goal"`
	Project       string          `json:"project"`
	Summary       string          `json:"summary"`
	CommitMessage string          `json:"commit_message"`
	Actions       []plannedAction `json:"actions"`
	Checks        []plannedCheck  `json:"checks"`
}

type plannedAction struct {
	Kind    string            `json:"kind"`
	Goal    string            `json:"goal"`
	Project string            `json:"project"`
	Parent  string            `json:"parent,omitempty"`
	Command []string          `json:"command,omitempty"`
	Files   []string          `json:"files,omitempty"`
	Params  map[string]string `json:"params,omitempty"`
}

type plannedCheck struct {
	Command []string `json:"command"`
	Heavy   bool     `json:"heavy,omitempty"`
}

const maxPlanBytes = 1024 * 1024

type boundedBuffer struct {
	bytes.Buffer
	Limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.Limit {
		return 0, fmt.Errorf("planner output exceeds %d bytes", b.Limit)
	}
	return b.Buffer.Write(p)
}

const guardedUsage = `Guarded mode:
  self loop --guarded --goal ID --repo PATH --branch NAME [options] -- <planner> [args...]

The planner runs in a bubblewrap mount/network/PID namespace with the repository
read-only and emits one JSON plan. The controller creates a goal worktree and
runs one worker action with only that worktree writable and no network. It then
runs declared checks, commits declared files, optionally performs one additive
push, verifies the remote branch, records a pass summary, and releases the lease.

Options:
  --worktree-root PATH  parent for isolated goal worktrees (default: sibling .self-worktrees)
  --lease-ttl D         expiring ownership period (default 1h)
  --timeout D           planner, worker, and check timeout (default 30m)
  --max-files N         changed-file budget, at most 20 (default 20)
  --min-free-mb N       required free temporary space (default 512)
  --memory-mb N         process address-space limit (default 2048)
  --cpu-seconds N       process CPU-time limit (default 1800)
  --budgets JSON        override guarded category limits
  --resume PASS         explicitly steal an expired pass lease and reuse its worktree

Guarded mode requires git and bubblewrap. It does not fall back to the invocation
checkout when dispatch fails. Arbitrary legacy loop minds remain unrestricted.`

func parseGuardedOptions(args []string) (guardedOptions, error) {
	var opts guardedOptions
	flags := flag.NewFlagSet("guarded loop", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.Goal, "goal", "", "")
	flags.StringVar(&opts.Repo, "repo", "", "")
	flags.StringVar(&opts.Branch, "branch", "", "")
	flags.StringVar(&opts.WorktreeRoot, "worktree-root", "", "")
	flags.StringVar(&opts.Resume, "resume", "", "")
	leaseTTL := flags.String("lease-ttl", "1h", "")
	timeout := flags.String("timeout", "30m", "")
	maxFiles := flags.String("max-files", "20", "")
	minFree := flags.String("min-free-mb", "512", "")
	memory := flags.String("memory-mb", "2048", "")
	cpu := flags.String("cpu-seconds", "1800", "")
	budgets := flags.String("budgets", "", "")
	filtered := args
	if len(filtered) > 0 && filtered[0] == "--guarded" {
		filtered = filtered[1:]
	}
	if err := flags.Parse(filtered); err != nil {
		return opts, fmt.Errorf("guarded loop options: %w — %s", err, guardedUsage)
	}
	opts.Planner = flags.Args()
	var err error
	if opts.LeaseTTL, err = positiveDuration(*leaseTTL, "--lease-ttl"); err != nil {
		return opts, err
	}
	if opts.Timeout, err = positiveDuration(*timeout, "--timeout"); err != nil {
		return opts, err
	}
	if opts.LeaseTTL <= opts.Timeout {
		return opts, fmt.Errorf("--lease-ttl must be longer than --timeout so ownership cannot expire during one bounded action")
	}
	if opts.MaxFiles, err = positiveInt(*maxFiles, "--max-files"); err != nil {
		return opts, err
	}
	opts.Budget = guardedBudget()
	if *budgets != "" {
		var overrides map[string]int
		if err := json.Unmarshal([]byte(*budgets), &overrides); err != nil {
			return opts, fmt.Errorf("--budgets needs a JSON object of category limits: %w", err)
		}
		for category, limit := range overrides {
			if _, known := opts.Budget.Limits[category]; !known || limit < 0 {
				return opts, fmt.Errorf("--budgets has unknown category or negative limit %q", category)
			}
			opts.Budget.Limits[category] = limit
		}
	}
	if opts.MaxFiles > opts.Budget.Limits["changed_files"] {
		return opts, fmt.Errorf("--max-files cannot exceed guarded changed_files limit %d", opts.Budget.Limits["changed_files"])
	}
	parsedFree, err := strconv.ParseUint(*minFree, 10, 64)
	if err != nil || parsedFree < 1 {
		return opts, fmt.Errorf("--min-free-mb needs a positive integer")
	}
	opts.MinFreeMB = parsedFree
	parsedMemory, err := strconv.ParseUint(*memory, 10, 64)
	if err != nil || parsedMemory < 1 {
		return opts, fmt.Errorf("--memory-mb needs a positive integer")
	}
	opts.MemoryMB = parsedMemory
	parsedCPU, err := strconv.ParseUint(*cpu, 10, 64)
	if err != nil || parsedCPU < 1 {
		return opts, fmt.Errorf("--cpu-seconds needs a positive integer")
	}
	opts.CPUSeconds = parsedCPU
	if opts.Goal == "" || opts.Repo == "" || opts.Branch == "" || len(opts.Planner) == 0 {
		return opts, fmt.Errorf("guarded mode requires --goal, --repo, --branch, and a planner after --")
	}
	opts.Repo, err = filepath.Abs(opts.Repo)
	if err != nil {
		return opts, err
	}
	if opts.WorktreeRoot == "" {
		opts.WorktreeRoot = filepath.Join(filepath.Dir(opts.Repo), ".self-worktrees")
	} else if opts.WorktreeRoot, err = filepath.Abs(opts.WorktreeRoot); err != nil {
		return opts, err
	}
	return opts, nil
}

func appendRailEvents(home string, events ...Event) error {
	return appendEvents(home, events)
}

func guardedPassID() string { return "pass-" + newEvent("loop.pass.started", nil).ID[:16] }

func runSandbox(ctx context.Context, writable, dir string, argv []string, stdin []byte, out, diag io.Writer, memoryMB, cpuSeconds uint64) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty sandbox command")
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return fmt.Errorf("guarded mode requires bubblewrap: %w", err)
	}
	args := []string{
		"--die-with-parent", "--new-session", "--unshare-net", "--unshare-pid",
		"--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc",
		"--tmpfs", "/tmp", "--tmpfs", "/run", "--dir", "/tmp/home",
		"--setenv", "HOME", "/tmp/home", "--setenv", "SELF_GUARDED", "1",
	}
	if writable != "" {
		args = append(args, "--bind", writable, writable)
		gitFile := filepath.Join(writable, ".git")
		if _, err := os.Stat(gitFile); err == nil {
			args = append(args, "--ro-bind", gitFile, gitFile)
		}
	}
	args = append(args, "--chdir", dir, "--")
	args = append(args, argv...)
	program, programArgs := bwrap, args
	if memoryMB > 0 || cpuSeconds > 0 {
		prlimit, err := exec.LookPath("prlimit")
		if err != nil {
			return fmt.Errorf("guarded resource limits require prlimit: %w", err)
		}
		programArgs = []string{fmt.Sprintf("--as=%d", memoryMB*1024*1024), fmt.Sprintf("--cpu=%d", cpuSeconds), "--", bwrap}
		programArgs = append(programArgs, args...)
		program = prlimit
	}
	cmd := exec.CommandContext(ctx, program, programArgs...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C", "LC_ALL=C", "TZ=UTC"}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(stdin), out, diag
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	return cmd.Run()
}

func repoOutput(repo string, args ...string) ([]byte, error) {
	safe := []string{"-C", repo, "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false"}
	cmd := exec.Command("git", append(safe, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exit.Stderr)))
		}
	}
	return out, err
}

func repoRun(repo string, args ...string) error {
	return repoRunContext(context.Background(), repo, args...)
}

func repoRunContext(ctx context.Context, repo string, args ...string) error {
	safe := []string{"-C", repo, "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false"}
	cmd := exec.CommandContext(ctx, "git", append(safe, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return nil
}

func repoStatus(repo string) (string, error) {
	out, err := repoOutput(repo, "status", "--porcelain=v1", "--untracked-files=all")
	return string(out), err
}

func gitPointer(worktree string) ([]byte, error) {
	return os.ReadFile(filepath.Join(worktree, ".git"))
}

func verifyGitPointer(worktree string, expected []byte) error {
	actual, err := gitPointer(worktree)
	if err != nil {
		return fmt.Errorf("worktree Git pointer is unreadable: %w", err)
	}
	if !bytes.Equal(actual, expected) {
		return fmt.Errorf("worker changed the protected worktree Git pointer")
	}
	return nil
}

func verifyRepository(repo string) error {
	inside, err := repoOutput(repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("--repo is not a Git worktree: %w", err)
	}
	top, err := filepath.EvalSymlinks(strings.TrimSpace(string(inside)))
	if err != nil {
		return err
	}
	requested, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return err
	}
	if top != requested {
		return fmt.Errorf("--repo must name the worktree root %s, not %s", top, repo)
	}
	return nil
}

func verifyRecoveredWorktree(worktree, repository, branch string) error {
	common, err := repoOutput(worktree, "rev-parse", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("cannot recover worktree: %w", err)
	}
	wantCommon, err := repoOutput(repository, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	resolve := func(base string, raw []byte) (string, error) {
		path := strings.TrimSpace(string(raw))
		if !filepath.IsAbs(path) {
			path = filepath.Join(base, path)
		}
		return filepath.EvalSymlinks(path)
	}
	actualCommon, err := resolve(worktree, common)
	if err != nil {
		return err
	}
	expectedCommon, err := resolve(repository, wantCommon)
	if err != nil {
		return err
	}
	if actualCommon != expectedCommon {
		return fmt.Errorf("recovery worktree belongs to another repository")
	}
	actualBranch, err := repoOutput(worktree, "branch", "--show-current")
	if err != nil || strings.TrimSpace(string(actualBranch)) != branch {
		return fmt.Errorf("recovery worktree branch is %q, expected %q: %w", strings.TrimSpace(string(actualBranch)), branch, err)
	}
	return nil
}

func pathWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	probe := abs
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", err
		}
		missing = append(missing, filepath.Base(probe))
		probe = parent
	}
}

func freeMegabytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize) / (1024 * 1024), nil
}

func createGoalWorktree(repo, worktree, branch string) error {
	var reasons []string
	for attempt := 1; attempt <= 2; attempt++ {
		err := repoRun(repo, "worktree", "add", "-b", branch, worktree, "HEAD")
		if err == nil {
			return nil
		}
		reasons = append(reasons, fmt.Sprintf("attempt %d: %v", attempt, err))
	}
	return fmt.Errorf("isolated worktree creation failed after one retry; no direct-edit fallback: %s", strings.Join(reasons, "; "))
}

func parsePlan(raw []byte) (guardedPlan, error) {
	var plan guardedPlan
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&plan); err != nil {
		return plan, fmt.Errorf("planner output is not one guarded plan: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return plan, fmt.Errorf("planner output must contain exactly one JSON object")
	}
	return plan, nil
}

func cleanRelativeFiles(files []string) ([]string, error) {
	seen := map[string]bool{}
	cleaned := make([]string, 0, len(files))
	for _, name := range files {
		clean := filepath.ToSlash(filepath.Clean(name))
		if clean == "." || filepath.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, ".git/") || clean == ".git" {
			return nil, fmt.Errorf("declared file %q escapes the worktree or names Git metadata", name)
		}
		if !seen[clean] {
			seen[clean] = true
			cleaned = append(cleaned, clean)
		}
	}
	return cleaned, nil
}

type planDecision struct {
	Worker      *plannedAction
	Push        bool
	Files       []string
	Checkpoint  json.RawMessage
	Budget      passBudget
	HeavyChecks int
	Unsupported []string
}

var forbiddenActions = map[string]bool{
	"force-push": true, "merge": true, "production-write": true, "rollout": true,
	"external-comment": true, "external-message": true,
}

func validatePlan(opts guardedOptions, plan guardedPlan) (planDecision, error) {
	budget := opts.Budget
	if budget.Limits == nil {
		budget = guardedBudget()
	}
	budget.Used = map[string]int{}
	d := planDecision{Budget: budget}
	if plan.Goal != opts.Goal || plan.Project != opts.Repo {
		return d, fmt.Errorf("plan goal/project must exactly match the guarded lease")
	}
	if len(plan.Actions) == 0 {
		return d, fmt.Errorf("plan has no actions")
	}
	if len(plan.Actions) > 4 {
		return d, fmt.Errorf("guarded pass action limit is 4")
	}
	if len(plan.Checks) > 8 {
		return d, fmt.Errorf("guarded pass check limit is 8")
	}
	if err := d.Budget.consume("repositories", 1); err != nil {
		return d, err
	}
	var gated []plannedAction
	for i := range plan.Actions {
		a := &plan.Actions[i]
		scope := classifyAction(opts.Goal, opts.Repo, map[string]string{"kind": a.Kind, "goal": a.Goal, "project": a.Project, "parent": a.Parent})
		if forbiddenActions[a.Kind] {
			return d, fmt.Errorf("action %q is forbidden in guarded mode", a.Kind)
		}
		if scope == scopeAdjacent || scope == scopeExpanding || a.Kind == "open-pr" || a.Kind == "infrastructure" {
			gated = append(gated, *a)
		}
		switch a.Kind {
		case "worker":
			if d.Worker != nil || len(a.Command) == 0 {
				return d, fmt.Errorf("a pass needs exactly one nonempty worker action")
			}
			if err := d.Budget.consume("dispatches", 1); err != nil {
				return d, err
			}
			files, err := cleanRelativeFiles(a.Files)
			if err != nil {
				return d, err
			}
			if len(files) == 0 || len(files) > opts.MaxFiles {
				return d, fmt.Errorf("worker must declare 1-%d changed files", opts.MaxFiles)
			}
			if err := d.Budget.consume("changed_files", len(files)); err != nil {
				return d, err
			}
			d.Worker, d.Files = a, files
		case "push":
			if d.Push {
				return d, fmt.Errorf("guarded pass permits exactly one push action")
			}
			if err := d.Budget.consume("pushes", 1); err != nil {
				return d, err
			}
			d.Push = true
		case "open-pr", "infrastructure":
			if err := d.Budget.consume("pr_mutations", 1); err != nil {
				return d, err
			}
			d.Unsupported = append(d.Unsupported, a.Kind)
		case "create-goal":
			if err := d.Budget.consume("new_goals", 1); err != nil {
				return d, err
			}
			d.Unsupported = append(d.Unsupported, a.Kind)
		default:
			return d, fmt.Errorf("unsupported guarded action %q", a.Kind)
		}
	}
	if d.Worker == nil {
		return d, fmt.Errorf("plan has no worker action")
	}
	for _, check := range plan.Checks {
		if len(check.Command) == 0 {
			return d, fmt.Errorf("check command is empty")
		}
		if check.Heavy {
			d.HeavyChecks++
			if err := d.Budget.consume("heavy_checks", 1); err != nil {
				return d, err
			}
		}
	}
	if len(plan.Checks) == 0 {
		return d, fmt.Errorf("guarded completion requires at least one declared verification check")
	}
	if len(d.Unsupported) > 0 {
		return d, fmt.Errorf("this controller has no executor for %s", strings.Join(d.Unsupported, ", "))
	}
	if len(gated) > 0 {
		d.Checkpoint, _ = json.Marshal(gated)
	}
	return d, nil
}

func changedFiles(worktree string) ([]string, error) {
	out, err := repoOutput(worktree, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(out, []byte{0})
	var files []string
	for i := 0; i < len(parts)-1; i++ {
		entry := string(parts[i])
		if len(entry) < 4 {
			continue
		}
		name := entry[3:]
		if strings.HasPrefix(entry, "R") || strings.HasPrefix(entry[1:], "R") || strings.HasPrefix(entry, "C") || strings.HasPrefix(entry[1:], "C") {
			return nil, fmt.Errorf("renames and copies are not supported in a guarded pass; declare delete/add separately")
		}
		files = append(files, filepath.ToSlash(name))
	}
	return files, nil
}

func sameFiles(actual, declared []string) error {
	allowed := map[string]bool{}
	for _, f := range declared {
		allowed[f] = true
	}
	for _, f := range actual {
		if !allowed[f] {
			return fmt.Errorf("worker changed undeclared file %q", f)
		}
	}
	return nil
}

func withHeavyLock(ctx context.Context, home string, fn func() error) error {
	f, err := os.OpenFile(filepath.Join(home, ".loop-heavy.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("heavyweight verification lock: %w", ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

func consumeMatchingCheckpoint(home string, actions json.RawMessage) (string, bool, error) {
	canonical, err := canonicalActions(string(actions))
	if err != nil {
		return "", false, err
	}
	events, err := withRailLock(home, func(st *state) ([]Event, error) {
		for id, c := range st.Checkpoints {
			if c.usable() && bytes.Equal(c.Actions, canonical) {
				return []Event{railEvent(checkpointUsed, map[string]string{"id": id})}, nil
			}
		}
		return nil, nil
	})
	if err != nil || len(events) == 0 {
		return "", false, err
	}
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(events[0].Payload, &p); err != nil {
		return "", false, err
	}
	return p.ID, true, nil
}

func verifyRemote(worktree, branch, commit string) error {
	out, err := repoOutput(worktree, "ls-remote", "--exit-code", "origin", "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("expected remote branch is absent: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 || fields[0] != commit {
		return fmt.Errorf("remote branch points to %q, expected %q", strings.Join(fields, " "), commit)
	}
	return nil
}

func assertLease(home string, expected goalLease, now time.Time) error {
	_, err := withRailLock(home, func(st *state) ([]Event, error) {
		current := st.Leases[expected.Goal]
		if current == nil || !current.active(now) || current.Owner != expected.Owner || current.Invocation != expected.Invocation || current.Token != expected.Token || current.Repository != expected.Repository || current.Branch != expected.Branch || current.Worktree != expected.Worktree {
			return nil, fmt.Errorf("guarded ownership was lost for goal %q", expected.Goal)
		}
		return nil, nil
	})
	return err
}

func finishGuardedPass(home string, lease goalLease, evidence map[string]any, now time.Time) error {
	_, err := withRailLock(home, func(st *state) ([]Event, error) {
		current := st.Leases[lease.Goal]
		if current == nil || !current.active(now) || current.Owner != lease.Owner || current.Invocation != lease.Invocation || current.Token != lease.Token || current.Repository != lease.Repository || current.Branch != lease.Branch || current.Worktree != lease.Worktree {
			return nil, fmt.Errorf("guarded ownership was lost before completion")
		}
		return []Event{
			railEvent("loop.pass.completed", evidence),
			railEvent(leaseReleased, map[string]string{"goal": lease.Goal, "owner": lease.Owner, "invocation": lease.Invocation, "token": lease.Token}),
		}, nil
	})
	return err
}

func cmdGuardedLoop(home string, args []string, out, diag io.Writer) (retErr error) {
	opts, err := parseGuardedOptions(args)
	if err != nil {
		return err
	}
	if err := verifyRepository(opts.Repo); err != nil {
		return err
	}
	canonicalRepo, err := filepath.EvalSymlinks(opts.Repo)
	if err != nil {
		return err
	}
	canonicalHome, err := canonicalPath(home)
	if err != nil {
		return err
	}
	if pathWithin(canonicalRepo, canonicalHome) {
		return fmt.Errorf("SELF_HOME must be outside the guarded invocation checkout")
	}
	if err := os.MkdirAll(opts.WorktreeRoot, 0755); err != nil {
		return err
	}
	canonicalRoot, err := filepath.EvalSymlinks(opts.WorktreeRoot)
	if err != nil {
		return err
	}
	if pathWithin(canonicalRepo, canonicalRoot) {
		return fmt.Errorf("--worktree-root must be outside the guarded invocation checkout")
	}
	free, err := freeMegabytes(opts.WorktreeRoot)
	if err != nil || free < opts.MinFreeMB {
		return fmt.Errorf("guarded loop needs %dMB free under %s; found %dMB: %w", opts.MinFreeMB, opts.WorktreeRoot, free, err)
	}
	initialStatus, err := repoStatus(opts.Repo)
	if err != nil {
		return err
	}
	pass, owner := guardedPassID(), callerClaim()
	if owner == "" {
		owner = "guarded:" + pass
	}
	worktree := filepath.Join(opts.WorktreeRoot, pass)
	resuming := opts.Resume != ""
	if resuming {
		pass = opts.Resume
		st, err := loadState(home)
		if err != nil {
			return err
		}
		prior := st.Leases[opts.Goal]
		if prior == nil || prior.Invocation != pass || prior.Repository != opts.Repo || prior.Branch != opts.Branch {
			return fmt.Errorf("pass %q has no matching lease to recover", pass)
		}
		worktree = prior.Worktree
		if _, err := os.Stat(worktree); err == nil {
			if err := verifyRecoveredWorktree(worktree, opts.Repo, opts.Branch); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	lease := goalLease{Goal: opts.Goal, Owner: owner, Invocation: pass, Repository: opts.Repo, Branch: opts.Branch, Worktree: worktree}
	leaseOp := "acquire"
	if resuming {
		leaseOp = "steal"
	}
	leaseEvents, err := leaseChange(home, leaseOp, lease, opts.LeaseTTL, time.Now().UTC())
	if err != nil {
		return err
	}
	if len(leaseEvents) != 1 || json.Unmarshal(leaseEvents[0].Payload, &lease) != nil || lease.Token == "" {
		return fmt.Errorf("lease operation returned no fencing generation")
	}
	passEvent := "loop.pass.started"
	if resuming {
		passEvent = "loop.pass.resumed"
	}
	started := railEvent(passEvent, map[string]any{"pass": pass, "goal": opts.Goal, "repository": opts.Repo, "branch": opts.Branch, "worktree": worktree})
	if err := appendRailEvents(home, started); err != nil {
		return err
	}
	settled := false
	defer func() {
		if settled {
			return
		}
		// A failed pass remains owned until expiry so a preserved dirty worktree
		// cannot race a fresh writer. Recovery is an explicit expired-lease steal.
	}()
	fail := func(status, reason string) error {
		_ = appendRailEvents(home, railEvent("loop.pass."+status, map[string]any{"pass": pass, "goal": opts.Goal, "reason": reason, "worktree": worktree}))
		return errors.New(reason)
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	if _, err := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); err != nil {
		return fail("resumable", err.Error())
	}
	plannerOut := boundedBuffer{Limit: maxPlanBytes}
	plannerCommand := append([]string{"env", "SELF_HOME=" + home}, opts.Planner...)
	err = runSandbox(ctx, "", opts.Repo, plannerCommand, nil, &plannerOut, diag, opts.MemoryMB, opts.CPUSeconds)
	timedOut := ctx.Err() == context.DeadlineExceeded
	cancel()
	if timedOut {
		return fail("resumable", fmt.Sprintf("planner timed out after %s; process group killed", opts.Timeout))
	}
	if err != nil {
		return fail("resumable", fmt.Sprintf("planner failed: %v", err))
	}
	plan, err := parsePlan(plannerOut.Bytes())
	if err != nil {
		return fail("failed", err.Error())
	}
	decision, err := validatePlan(opts, plan)
	if err != nil {
		var exhausted *budgetExhausted
		if errors.As(err, &exhausted) {
			_ = appendRailEvents(home, railEvent("loop.budget.exhausted", map[string]any{
				"pass": pass, "goal": opts.Goal, "reason": err.Error(), "category": exhausted.Category,
				"used": exhausted.Used, "requested": exhausted.Requested, "limit": exhausted.Limit, "budget": decision.Budget,
			}))
		}
		return fail("failed", err.Error())
	}
	planRaw, _ := json.Marshal(plan)
	budgetRaw := map[string]any{"limits": decision.Budget.Limits, "used": decision.Budget.Used}
	if err := appendRailEvents(home, railEvent("loop.pass.planned", map[string]any{"pass": pass, "goal": opts.Goal, "plan": json.RawMessage(planRaw), "budget": budgetRaw})); err != nil {
		return err
	}
	if len(decision.Checkpoint) > 0 {
		id, approved, consumeErr := consumeMatchingCheckpoint(home, decision.Checkpoint)
		if consumeErr != nil {
			return fail("failed", consumeErr.Error())
		}
		if approved {
			_ = appendRailEvents(home, railEvent("loop.pass.checkpoint.consumed", map[string]any{"pass": pass, "goal": opts.Goal, "checkpoint": id}))
			if len(decision.Unsupported) > 0 {
				return fail("failed", fmt.Sprintf("checkpoint authorized %s, but this controller has no executor for it", strings.Join(decision.Unsupported, ", ")))
			}
		} else {
			events, requestErr := checkpointChange(home, "request", pass, "", string(decision.Checkpoint), "")
			if len(events) > 0 {
				var c checkpoint
				json.Unmarshal(events[0].Payload, &c)
				id = c.ID
			}
			if requestErr != nil {
				return fail("failed", requestErr.Error())
			}
			if _, err := leaseChange(home, "release", lease, 0, time.Now().UTC()); err != nil {
				return fail("resumable", fmt.Sprintf("checkpoint %s pending and lease release failed: %v", id, err))
			}
			settled = true
			return fail("resumable", fmt.Sprintf("checkpoint %s requires human approval; rerun after approval", id))
		}
	}
	if _, err := os.Stat(worktree); os.IsNotExist(err) {
		if err := createGoalWorktree(opts.Repo, worktree, opts.Branch); err != nil {
			return fail("failed", err.Error())
		}
	} else if err != nil {
		return fail("failed", err.Error())
	}
	pointer, err := gitPointer(worktree)
	if err != nil {
		return fail("failed", err.Error())
	}

	if _, err := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); err != nil {
		return fail("resumable", err.Error())
	}
	ctx, cancel = context.WithTimeout(context.Background(), opts.Timeout)
	err = runSandbox(ctx, worktree, worktree, decision.Worker.Command, nil, out, diag, opts.MemoryMB, opts.CPUSeconds)
	timedOut = ctx.Err() == context.DeadlineExceeded
	cancel()
	if timedOut {
		return fail("resumable", fmt.Sprintf("worker timed out after %s; process group killed", opts.Timeout))
	}
	if err != nil {
		return fail("resumable", fmt.Sprintf("worker failed: %v", err))
	}
	if err := verifyGitPointer(worktree, pointer); err != nil {
		return fail("failed", err.Error())
	}
	actual, err := changedFiles(worktree)
	if err != nil {
		return fail("failed", err.Error())
	}
	if err := sameFiles(actual, decision.Files); err != nil {
		return fail("failed", err.Error())
	}
	for i, check := range plan.Checks {
		run := func() error {
			if _, err := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
			defer cancel()
			return runSandbox(ctx, worktree, worktree, check.Command, nil, out, diag, opts.MemoryMB, opts.CPUSeconds)
		}
		if check.Heavy {
			lockCtx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
			err = withHeavyLock(lockCtx, home, run)
			cancel()
		} else {
			err = run()
		}
		if err != nil {
			return fail("resumable", fmt.Sprintf("check %d failed: %v", i+1, err))
		}
		if err := verifyGitPointer(worktree, pointer); err != nil {
			return fail("failed", err.Error())
		}
		_ = appendRailEvents(home, railEvent("loop.pass.check.passed", map[string]any{"pass": pass, "goal": opts.Goal, "command": check.Command, "heavy": check.Heavy}))
	}
	actual, err = changedFiles(worktree)
	if err != nil {
		return fail("failed", err.Error())
	}
	if err := sameFiles(actual, decision.Files); err != nil {
		return fail("failed", err.Error())
	}
	if strings.TrimSpace(plan.CommitMessage) == "" {
		return fail("failed", "plan needs a nonempty commit_message")
	}
	if _, err := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); err != nil {
		return fail("resumable", err.Error())
	}
	addArgs := append([]string{"add", "--"}, decision.Files...)
	gitCtx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	err = repoRunContext(gitCtx, worktree, addArgs...)
	cancel()
	if err != nil {
		return fail("failed", err.Error())
	}
	if _, err := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); err != nil {
		return fail("resumable", err.Error())
	}
	gitCtx, cancel = context.WithTimeout(context.Background(), opts.Timeout)
	err = repoRunContext(gitCtx, worktree, "-c", "user.name=self guarded loop", "-c", "user.email=self@localhost", "commit", "-m", plan.CommitMessage)
	cancel()
	if err != nil {
		return fail("failed", err.Error())
	}
	commitRaw, err := repoOutput(worktree, "rev-parse", "HEAD")
	if err != nil {
		return fail("failed", err.Error())
	}
	commit := strings.TrimSpace(string(commitRaw))
	if decision.Push {
		if _, err := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); err != nil {
			return fail("resumable", err.Error())
		}
		pushCtx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		err := repoRunContext(pushCtx, worktree, "push", "origin", "HEAD:refs/heads/"+opts.Branch)
		cancel()
		if err != nil {
			return fail("resumable", err.Error())
		}
		_ = appendRailEvents(home, railEvent("loop.pass.push.completed", map[string]any{"pass": pass, "goal": opts.Goal, "branch": opts.Branch, "commit": commit, "force": false}))
	}
	worktreeStatus, err := repoStatus(worktree)
	if err != nil || worktreeStatus != "" {
		return fail("failed", fmt.Sprintf("worktree is not clean after commit: %q: %v", worktreeStatus, err))
	}
	finalStatus, err := repoStatus(opts.Repo)
	if err != nil || finalStatus != initialStatus {
		return fail("failed", fmt.Sprintf("invocation checkout changed: before=%q after=%q: %v", initialStatus, finalStatus, err))
	}
	if !decision.Push {
		return fail("resumable", "completion verification requires the guarded push action so the expected remote branch can be proven")
	}
	if err := verifyRemote(worktree, opts.Branch, commit); err != nil {
		return fail("resumable", err.Error())
	}
	evidence := map[string]any{
		"pass": pass, "goal": opts.Goal, "commit": commit, "branch": opts.Branch,
		"remote_verified": true, "worktree_clean": true, "invocation_checkout_unchanged": true,
		"declared_files": decision.Files, "checks": len(plan.Checks), "force_push": false,
		"budget": budgetRaw,
	}
	if err := finishGuardedPass(home, lease, evidence, time.Now().UTC()); err != nil {
		return err
	}
	settled = true
	fmt.Fprintf(diag, "self loop: guarded pass %s completed at %s on %s\n", pass, commit, opts.Branch)
	return nil
}
