package main

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	Herdr        string
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
runs declared checks, commits declared files, performs one additive
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
  --herdr PATH          Herdr binary for live-agent reconciliation (or SELF_HERDR_BIN/PATH)

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
	flags.StringVar(&opts.Herdr, "herdr", os.Getenv("SELF_HERDR_BIN"), "")
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

var gitCommandTimeout = 30 * time.Second
var guardedFault func(string) error

func commandWithGroup(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

func bwrapDirs(path string) []string {
	if !pathWithin("/tmp", path) {
		return nil
	}
	var dirs []string
	for current := "/tmp"; current != path; {
		rel, _ := filepath.Rel(current, path)
		segment := strings.Split(filepath.ToSlash(rel), "/")[0]
		current = filepath.Join(current, segment)
		dirs = append(dirs, current)
	}
	return dirs
}

func runSandbox(ctx context.Context, writable, dir string, argv []string, stdin []byte, out, diag io.Writer, memoryMB, cpuSeconds uint64) error {
	return runSandboxStarted(ctx, writable, dir, argv, stdin, out, diag, memoryMB, cpuSeconds, nil)
}

func runSandboxStarted(ctx context.Context, writable, dir string, argv []string, stdin []byte, out, diag io.Writer, memoryMB, cpuSeconds uint64, started func() error) error {
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
	if pathWithin("/tmp", dir) {
		for _, path := range bwrapDirs(dir) {
			args = append(args, "--dir", path)
		}
		if writable == "" {
			args = append(args, "--ro-bind", dir, dir)
		}
	}
	if writable != "" {
		if pathWithin("/tmp", writable) && writable != dir {
			for _, path := range bwrapDirs(writable) {
				args = append(args, "--dir", path)
			}
		}
		args = append(args, "--bind", writable, writable)
		gitFile := filepath.Join(writable, ".git")
		if _, err := os.Stat(gitFile); err == nil {
			args = append(args, "--ro-bind", gitFile, gitFile)
		}
	}
	var readyR, readyW *os.File
	if started != nil {
		var err error
		readyR, readyW, err = os.Pipe()
		if err != nil {
			return err
		}
		defer readyR.Close()
		argv = append([]string{"sh", "-c", "printf 1 >&3; exec \"$@\"", "self-worker"}, argv...)
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
	cmd := commandWithGroup(ctx, program, programArgs...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C", "LC_ALL=C", "TZ=UTC"}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(stdin), out, diag
	if readyW != nil {
		cmd.ExtraFiles = []*os.File{readyW}
	}
	if err := cmd.Start(); err != nil {
		if readyW != nil {
			readyW.Close()
		}
		return err
	}
	if started != nil {
		readyW.Close()
		var ready [1]byte
		if _, err := io.ReadFull(readyR, ready[:]); err != nil {
			waitErr := cmd.Wait()
			if waitErr != nil {
				return waitErr
			}
			return fmt.Errorf("worker exited before sandbox readiness: %w", err)
		}
		if err := started(); err != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
			return err
		}
	}
	return cmd.Wait()
}

func repoOutput(repo string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()
	return repoOutputContext(ctx, repo, args...)
}

func repoOutputContext(ctx context.Context, repo string, args ...string) ([]byte, error) {
	safe := []string{"-C", repo, "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false"}
	cmd := commandWithGroup(ctx, "git", append(safe, args...)...)
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
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()
	return repoRunContext(ctx, repo, args...)
}

func repoRunContext(ctx context.Context, repo string, args ...string) error {
	safe := []string{"-C", repo, "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false"}
	cmd := commandWithGroup(ctx, "git", append(safe, args...)...)
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
	remoteOut, err := repoOutput(repo, "ls-remote", "origin", "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("discovering remote branch %s: %w", branch, err)
	}
	remoteExists := len(strings.Fields(string(remoteOut))) > 0
	if remoteExists {
		if err := repoRun(repo, "fetch", "--no-tags", "origin", "refs/heads/"+branch+":refs/remotes/origin/"+branch); err != nil {
			return fmt.Errorf("fetching remote branch %s: %w", branch, err)
		}
	} else if _, err := repoOutput(repo, "show-ref", "--verify", "refs/remotes/origin/"+branch); err == nil {
		return fmt.Errorf("remote branch %s is absent but a stale tracking ref remains; prune or reconcile it explicitly", branch)
	}
	base := []string{"worktree", "add"}
	if _, err := repoOutput(repo, "show-ref", "--verify", "refs/heads/"+branch); err == nil {
		if remoteExists {
			localRaw, _ := repoOutput(repo, "rev-parse", "refs/heads/"+branch)
			remoteRaw, _ := repoOutput(repo, "rev-parse", "refs/remotes/origin/"+branch)
			local, remote := strings.TrimSpace(string(localRaw)), strings.TrimSpace(string(remoteRaw))
			if local != remote {
				if repoRun(repo, "merge-base", "--is-ancestor", local, remote) == nil {
					if err := repoRun(repo, "branch", "-f", branch, remote); err != nil {
						return err
					}
				} else if repoRun(repo, "merge-base", "--is-ancestor", remote, local) != nil {
					return fmt.Errorf("local and remote branch %s diverged; refusing non-additive attachment", branch)
				}
			}
		}
		base = append(base, worktree, branch)
	} else if remoteExists {
		base = append(base, "-b", branch, worktree, "refs/remotes/origin/"+branch)
	} else {
		base = append(base, "-b", branch, worktree, "HEAD")
	}
	var reasons []string
	for attempt := 1; attempt <= 2; attempt++ {
		err := repoRun(repo, base...)
		if err == nil {
			return nil
		}
		reasons = append(reasons, fmt.Sprintf("attempt %d: %v", attempt, err))
	}
	return fmt.Errorf("isolated worktree creation failed after one retry; no direct-edit fallback: %s", strings.Join(reasons, "; "))
}

type registeredWorktree struct {
	Path   string
	Branch string
}

func registeredWorktrees(repo string) ([]registeredWorktree, error) {
	out, err := repoOutput(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var rows []registeredWorktree
	var row registeredWorktree
	flush := func() {
		if row.Path != "" {
			rows = append(rows, row)
		}
		row = registeredWorktree{}
	}
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			row.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			row.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "":
			flush()
		}
	}
	flush()
	return rows, nil
}

func herdrBinary(configured string) (string, error) {
	if configured != "" {
		path, err := exec.LookPath(configured)
		if err != nil {
			return "", fmt.Errorf("configured Herdr binary %q is unavailable: %w", configured, err)
		}
		return path, nil
	}
	path, err := exec.LookPath("herdr")
	if err != nil {
		return "", fmt.Errorf("live agent records require Herdr reconciliation; set --herdr or SELF_HERDR_BIN: %w", err)
	}
	return path, nil
}

type herdrAgent struct {
	Name string
	Cwd  string
}

func liveHerdrAgents(ctx context.Context, binary string) ([]herdrAgent, error) {
	cmd := commandWithGroup(ctx, binary, "agent", "list")
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("herdr agent list failed: %w", err)
	}
	var response struct {
		Result struct {
			Agents *[]struct {
				Name   string `json:"name"`
				Agent  string `json:"agent"`
				Status string `json:"agent_status"`
				Cwd    string `json:"cwd"`
			} `json:"agents"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return nil, fmt.Errorf("herdr agent list returned invalid JSON: %w", err)
	}
	if response.Result.Agents == nil {
		return nil, fmt.Errorf("herdr agent list response has no agents array")
	}
	var live []herdrAgent
	for _, agent := range *response.Result.Agents {
		status := strings.ToLower(agent.Status)
		name := agent.Name
		if name == "" {
			name = agent.Agent
		}
		if (name != "" || agent.Cwd != "") && status != "done" && status != "failed" {
			if name == "" {
				name = "(unnamed)"
			}
			live = append(live, herdrAgent{Name: name, Cwd: agent.Cwd})
		}
	}
	return live, nil
}

func reconcileExternalWriters(home string, opts guardedOptions, worktree string, resuming bool) error {
	rows, err := registeredWorktrees(opts.Repo)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if resuming && row.Path == worktree {
			continue
		}
		if row.Path == worktree || row.Branch == opts.Branch {
			return fmt.Errorf("guarded acquisition conflicts with registered worktree %s on branch %s", row.Path, row.Branch)
		}
	}
	st, err := loadState(home)
	if err != nil {
		return err
	}
	type dispatch struct{ agent, goal, repo, branch, worktree string }
	dispatches := map[string]dispatch{}
	for _, event := range st.Events {
		var payload struct {
			Agent, Goal, Repo, Repository, Branch, Worktree string
		}
		if json.Unmarshal(event.Payload, &payload) != nil || payload.Agent == "" {
			continue
		}
		switch event.Name {
		case "agent.started":
			repo := payload.Repository
			if repo == "" {
				repo = payload.Repo
			}
			dispatches[payload.Agent] = dispatch{payload.Agent, payload.Goal, repo, payload.Branch, payload.Worktree}
		case "agent.failed", "agent.reclaimed":
			delete(dispatches, payload.Agent)
		}
	}
	var candidates []dispatch
	for _, d := range dispatches {
		matchesRepository := false
		for _, path := range []string{d.repo, d.worktree} {
			if path == "" {
				continue
			}
			_, identity, identityErr := repositoryIdentity(path)
			if identityErr == nil {
				_, wanted, _ := repositoryIdentity(opts.Repo)
				if identity == wanted {
					matchesRepository = true
					break
				}
			}
		}
		if d.goal == opts.Goal || matchesRepository {
			candidates = append(candidates, d)
		}
	}
	binary := ""
	if len(candidates) > 0 || opts.Herdr != "" {
		binary, err = herdrBinary(opts.Herdr)
		if err != nil {
			return err
		}
	} else if discovered, lookupErr := exec.LookPath("herdr"); lookupErr == nil {
		binary = discovered
	}
	if binary == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	live, err := liveHerdrAgents(ctx, binary)
	if err != nil {
		return err
	}
	for _, d := range candidates {
		for _, agent := range live {
			if agent.Name == d.agent {
				return fmt.Errorf("guarded acquisition conflicts with live Herdr agent %s for goal %s at %s", d.agent, d.goal, d.worktree)
			}
		}
	}
	_, wantedIdentity, err := repositoryIdentity(opts.Repo)
	if err != nil {
		return err
	}
	for _, agent := range live {
		if agent.Cwd == "" {
			continue
		}
		_, identity, err := repositoryIdentity(agent.Cwd)
		if err == nil && identity == wantedIdentity {
			return fmt.Errorf("guarded acquisition conflicts with observed raw Herdr agent %s at %s", agent.Name, agent.Cwd)
		}
	}
	return nil
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
	project, err := canonicalPath(plan.Project)
	if err != nil {
		return d, fmt.Errorf("canonical plan project: %w", err)
	}
	if plan.Goal != opts.Goal || project != opts.Repo {
		return d, fmt.Errorf("plan goal/project must exactly match the guarded lease")
	}
	plan.Project = project
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
		if a.Project != "" {
			canonical, err := canonicalPath(a.Project)
			if err != nil {
				return d, err
			}
			a.Project = canonical
		}
		scope := classifyAction(opts.Goal, opts.Repo, map[string]string{"kind": a.Kind, "goal": a.Goal, "project": a.Project, "parent": a.Parent})
		if forbiddenActions[a.Kind] {
			return d, fmt.Errorf("action %q is forbidden in guarded mode", a.Kind)
		}
		if scope == scopeExpanding {
			return d, fmt.Errorf("scope-expanding action %q has no guarded executor", a.Kind)
		}
		if scope == scopeAdjacent || a.Kind == "open-pr" || a.Kind == "infrastructure" {
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
	if !d.Push {
		return d, fmt.Errorf("guarded completion requires exactly one push action")
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

func checkDidNotMutate(before, after string, index int) error {
	if after != before {
		return fmt.Errorf("check %d mutated the worktree", index)
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

func matchingCheckpoint(home string, actions json.RawMessage) (string, bool, error) {
	canonical, err := canonicalActions(string(actions))
	if err != nil {
		return "", false, err
	}
	st, err := loadState(home)
	if err != nil {
		return "", false, err
	}
	for id, c := range st.Checkpoints {
		if c.usable() && bytes.Equal(c.Actions, canonical) {
			return id, true, nil
		}
	}
	return "", false, nil
}

func beginWorker(home string, lease goalLease, pass, checkpointID string, now time.Time) error {
	_, err := withRailLock(home, func(st *state) ([]Event, error) {
		current := st.Leases[lease.Goal]
		if current == nil || !current.active(now) || current.Token != lease.Token {
			return nil, fmt.Errorf("guarded ownership was lost before worker dispatch")
		}
		batch := []Event{}
		if checkpointID != "" {
			c := st.Checkpoints[checkpointID]
			if c == nil || !c.usable() {
				return nil, fmt.Errorf("checkpoint %q has no unused approval", checkpointID)
			}
			batch = append(batch,
				railEvent(checkpointUsed, map[string]string{"id": checkpointID}),
				railEvent("loop.pass.checkpoint.consumed", map[string]any{"pass": pass, "goal": lease.Goal, "checkpoint": checkpointID}),
			)
		}
		batch = append(batch, railEvent("loop.pass.worker.started", map[string]any{"pass": pass, "goal": lease.Goal, "worktree": lease.Worktree}))
		return batch, nil
	})
	return err
}

func preflightWorkerSandbox(opts guardedOptions, worktree string, command []string, diag io.Writer) error {
	if len(command) == 0 {
		return fmt.Errorf("worker command is empty")
	}
	path := command[0]
	if strings.Contains(path, string(filepath.Separator)) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(worktree, path)
		}
	} else {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return fmt.Errorf("worker executable %q is unavailable: %w", command[0], err)
		}
		path = resolved
	}
	if err := validateExecutable(path); err != nil {
		return fmt.Errorf("worker executable %q is unavailable: %w", command[0], err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	if err := runSandbox(ctx, "", worktree, []string{"true"}, nil, io.Discard, diag, opts.MemoryMB, opts.CPUSeconds); err != nil {
		return fmt.Errorf("worker sandbox preflight failed: %w", err)
	}
	return nil
}

func validateExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return fmt.Errorf("not an executable regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 256)
	n, err := f.Read(header)
	if err != nil && err != io.EOF {
		return err
	}
	header = header[:n]
	if len(header) >= 4 && bytes.Equal(header[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		binary, err := elf.Open(path)
		if err != nil {
			return fmt.Errorf("invalid ELF executable: %w", err)
		}
		defer binary.Close()
		if binary.Type != elf.ET_EXEC && binary.Type != elf.ET_DYN {
			return fmt.Errorf("ELF type %s is not executable", binary.Type)
		}
		machines := map[string]elf.Machine{
			"amd64": elf.EM_X86_64, "386": elf.EM_386, "arm64": elf.EM_AARCH64,
			"arm": elf.EM_ARM, "ppc64": elf.EM_PPC64, "riscv64": elf.EM_RISCV,
		}
		if want, ok := machines[runtime.GOARCH]; ok && binary.Machine != want {
			return fmt.Errorf("ELF machine %s does not match %s", binary.Machine, runtime.GOARCH)
		}
		for _, program := range binary.Progs {
			if program.Type != elf.PT_INTERP {
				continue
			}
			data, err := io.ReadAll(program.Open())
			if err != nil {
				return err
			}
			interpreter := strings.TrimRight(string(data), "\x00")
			if info, err := os.Stat(interpreter); err != nil || info.Mode()&0111 == 0 {
				return fmt.Errorf("ELF interpreter %q is unavailable", interpreter)
			}
		}
		return nil
	}
	if !bytes.HasPrefix(header, []byte("#!")) {
		return fmt.Errorf("missing ELF header or script shebang")
	}
	line := strings.TrimSpace(strings.SplitN(string(header[2:]), "\n", 2)[0])
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return fmt.Errorf("empty script shebang")
	}
	interpreter := fields[0]
	if filepath.Base(interpreter) == "env" && len(fields) > 1 {
		interpreter = fields[1]
	}
	resolved, err := exec.LookPath(interpreter)
	if err != nil {
		return fmt.Errorf("shebang interpreter %q is unavailable", interpreter)
	}
	interpreterInfo, err := os.Stat(resolved)
	if err != nil || interpreterInfo.Mode()&0111 == 0 {
		return fmt.Errorf("shebang interpreter %q is not executable", interpreter)
	}
	return nil
}

func verifyRemoteContext(ctx context.Context, worktree, branch, commit string) error {
	out, err := repoOutputContext(ctx, worktree, "ls-remote", "--exit-code", "origin", "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("expected remote branch is absent: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 || fields[0] != commit {
		return fmt.Errorf("remote branch points to %q, expected %q", strings.Join(fields, " "), commit)
	}
	return nil
}

type guardedRecovery struct {
	CommitBegan bool
	Commit      string
	Base        string
	PushBegan   bool
	Pushed      bool
	Completed   bool
}

func recoverPassState(a *passAudit) guardedRecovery {
	var state guardedRecovery
	if a == nil {
		return state
	}
	for _, event := range a.Events {
		var payload struct {
			Commit string `json:"commit"`
			Base   string `json:"base"`
		}
		_ = json.Unmarshal(event.Payload, &payload)
		switch event.Name {
		case "loop.pass.commit.started":
			state.CommitBegan, state.Base = true, payload.Base
		case "loop.pass.commit.completed":
			state.Commit, state.Base = payload.Commit, payload.Base
		case "loop.pass.push.started":
			state.PushBegan = true
		case "loop.pass.push.completed":
			state.Pushed = true
		case "loop.pass.completed":
			state.Completed = true
		}
	}
	return state
}

func completeInterruptedCommit(home string, opts guardedOptions, lease goalLease, pass, worktree string, audit *passAudit, recovery guardedRecovery) (guardedRecovery, error) {
	if !recovery.CommitBegan || recovery.Commit != "" {
		return recovery, nil
	}
	if _, err := os.Stat(worktree); err != nil {
		return recovery, fmt.Errorf("commit began but recovery worktree is unavailable: %w", err)
	}
	var plan guardedPlan
	if len(audit.Plan) == 0 || json.Unmarshal(audit.Plan, &plan) != nil || strings.TrimSpace(plan.CommitMessage) == "" {
		return recovery, fmt.Errorf("commit began but its plan or commit message is unavailable")
	}
	headRaw, err := repoOutput(worktree, "rev-parse", "HEAD")
	if err != nil {
		return recovery, err
	}
	head := strings.TrimSpace(string(headRaw))
	if head == recovery.Base {
		if _, err := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); err != nil {
			return recovery, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		err = repoRunContext(ctx, worktree, "-c", "user.name=self guarded loop", "-c", "user.email=self@localhost", "commit", "-m", plan.CommitMessage)
		cancel()
		if err != nil {
			return recovery, fmt.Errorf("finishing staged commit: %w", err)
		}
		headRaw, err = repoOutput(worktree, "rev-parse", "HEAD")
		if err != nil {
			return recovery, err
		}
		head = strings.TrimSpace(string(headRaw))
	} else {
		parent, err := repoOutput(worktree, "rev-parse", head+"^")
		if err != nil || strings.TrimSpace(string(parent)) != recovery.Base {
			return recovery, fmt.Errorf("post-commit recovery found unexpected HEAD %s from base %s", head, recovery.Base)
		}
	}
	recovery.Commit = head
	if err := appendRailEvents(home, railEvent("loop.pass.commit.completed", map[string]any{"pass": pass, "goal": opts.Goal, "base": recovery.Base, "commit": head, "branch": opts.Branch, "reconciled": true})); err != nil {
		return recovery, err
	}
	return recovery, nil
}

func cleanupSuccessfulWorktree(ctx context.Context, repo, worktree string) error {
	if _, err := os.Stat(worktree); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return repoRunContext(ctx, repo, "worktree", "remove", worktree)
}

func reconcileCommittedPass(home string, opts guardedOptions, lease goalLease, pass, worktree string, audit *passAudit, recovery guardedRecovery, initialStatus string) error {
	if recovery.Commit == "" {
		return fmt.Errorf("pass %q has no committed state to reconcile", pass)
	}
	var plan guardedPlan
	if len(audit.Plan) == 0 || json.Unmarshal(audit.Plan, &plan) != nil {
		return fmt.Errorf("pass %q has no replayable guarded plan", pass)
	}
	decision, err := validatePlan(opts, plan)
	if err != nil {
		return err
	}
	if len(decision.Checkpoint) > 0 {
		// The approval was consumed with worker.started before this commit.
		decision.Checkpoint = nil
	}
	if _, err := repoOutput(opts.Repo, "cat-file", "-e", recovery.Commit+"^{commit}"); err != nil {
		return fmt.Errorf("recorded commit %s is unavailable: %w", recovery.Commit, err)
	}
	if err := reconcileExternalWriters(home, opts, worktree, true); err != nil {
		return fmt.Errorf("writer appeared during reconciliation: %w", err)
	}
	if decision.Push {
		ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		err = verifyRemoteContext(ctx, opts.Repo, opts.Branch, recovery.Commit)
		cancel()
		if err != nil {
			if _, renewErr := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); renewErr != nil {
				return renewErr
			}
			if appendErr := appendRailEvents(home, railEvent("loop.pass.push.reconciling", map[string]any{"pass": pass, "goal": opts.Goal, "branch": opts.Branch, "commit": recovery.Commit})); appendErr != nil {
				return appendErr
			}
			ctx, cancel = context.WithTimeout(context.Background(), opts.Timeout)
			err = repoRunContext(ctx, opts.Repo, "push", "origin", recovery.Commit+":refs/heads/"+opts.Branch)
			cancel()
			if err != nil {
				return err
			}
		}
		ctx, cancel = context.WithTimeout(context.Background(), opts.Timeout)
		err = verifyRemoteContext(ctx, opts.Repo, opts.Branch, recovery.Commit)
		cancel()
		if err != nil {
			return err
		}
		if err := appendRailEvents(home, railEvent("loop.pass.push.reconciled", map[string]any{"pass": pass, "goal": opts.Goal, "branch": opts.Branch, "commit": recovery.Commit})); err != nil {
			return err
		}
	}
	if finalStatus, err := repoStatus(opts.Repo); err != nil || finalStatus != initialStatus {
		return fmt.Errorf("invocation checkout changed during reconciliation: before=%q after=%q: %w", initialStatus, finalStatus, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	err = cleanupSuccessfulWorktree(ctx, opts.Repo, worktree)
	cancel()
	if err != nil {
		return err
	}
	if err := appendRailEvents(home, railEvent("loop.pass.worktree.cleaned", map[string]any{"pass": pass, "goal": opts.Goal, "worktree": worktree})); err != nil {
		return err
	}
	evidence := map[string]any{
		"pass": pass, "goal": opts.Goal, "commit": recovery.Commit, "branch": opts.Branch,
		"remote_verified": decision.Push, "worktree_clean": true, "worktree_removed": true,
		"invocation_checkout_unchanged": true, "declared_files": decision.Files,
		"checks": len(plan.Checks), "force_push": false, "reconciled": true,
	}
	return finishGuardedPass(home, lease, evidence, time.Now().UTC())
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
	opts.Repo = canonicalRepo
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
	opts.WorktreeRoot = canonicalRoot
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
	if opts.Resume != "" {
		pass = opts.Resume
	}
	if owner == "" {
		owner = "guarded:" + pass
	}
	reservation, err := acquireRepositoryReservation(opts.Repo, "guarded-pass "+pass)
	if err != nil {
		_ = appendRailEvents(home, railEvent("loop.pass.failed", map[string]any{"pass": pass, "goal": opts.Goal, "reason": err.Error(), "stage": "repository-reservation"}))
		return err
	}
	defer reservation.Release()
	worktree := filepath.Join(opts.WorktreeRoot, pass)
	resuming := opts.Resume != ""
	var resumeAudit *passAudit
	var recovery guardedRecovery
	if resuming {
		st, err := loadState(home)
		if err != nil {
			return err
		}
		prior := st.Leases[opts.Goal]
		if prior == nil || prior.Invocation != pass || prior.Repository != opts.Repo || prior.Branch != opts.Branch {
			return fmt.Errorf("pass %q has no matching lease to recover", pass)
		}
		resumeAudit = st.Passes[pass]
		recovery = recoverPassState(resumeAudit)
		if recovery.Completed {
			return fmt.Errorf("pass %q is already completed and cannot rerun a worker", pass)
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
	if err := reconcileExternalWriters(home, opts, worktree, resuming); err != nil {
		_ = appendRailEvents(home, railEvent("loop.pass.failed", map[string]any{"pass": pass, "goal": opts.Goal, "reason": err.Error(), "worktree": worktree, "stage": "writer-reconciliation"}))
		return err
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
	if resuming && recovery.CommitBegan {
		recovery, err = completeInterruptedCommit(home, opts, lease, pass, worktree, resumeAudit, recovery)
		if err != nil {
			return fail("resumable", fmt.Sprintf("post-commit reconciliation failed: %v", err))
		}
	}
	if resuming && recovery.Commit != "" {
		if err := reconcileCommittedPass(home, opts, lease, pass, worktree, resumeAudit, recovery, initialStatus); err != nil {
			return fail("resumable", fmt.Sprintf("post-commit reconciliation failed: %v", err))
		}
		settled = true
		fmt.Fprintf(diag, "self loop: reconciled guarded pass %s at %s without rerunning its worker\n", pass, recovery.Commit)
		return nil
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
	checkpointID := ""
	if len(decision.Checkpoint) > 0 {
		id, approved, consumeErr := matchingCheckpoint(home, decision.Checkpoint)
		if consumeErr != nil {
			return fail("failed", consumeErr.Error())
		}
		if approved {
			checkpointID = id
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
	if err := reconcileExternalWriters(home, opts, worktree, true); err != nil {
		return fail("failed", fmt.Sprintf("writer appeared before dispatch: %v", err))
	}
	if err := preflightWorkerSandbox(opts, worktree, decision.Worker.Command, diag); err != nil {
		return fail("failed", err.Error())
	}

	if _, err := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); err != nil {
		return fail("resumable", err.Error())
	}
	ctx, cancel = context.WithTimeout(context.Background(), opts.Timeout)
	err = runSandboxStarted(ctx, worktree, worktree, decision.Worker.Command, nil, out, diag, opts.MemoryMB, opts.CPUSeconds, func() error {
		return beginWorker(home, lease, pass, checkpointID, time.Now().UTC())
	})
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
	if err := appendRailEvents(home, railEvent("loop.pass.worker.completed", map[string]any{"pass": pass, "goal": opts.Goal, "files": actual})); err != nil {
		return err
	}
	for i, check := range plan.Checks {
		beforeCheck, err := repoStatus(worktree)
		if err != nil {
			return fail("failed", err.Error())
		}
		run := func() error {
			if _, err := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
			defer cancel()
			return runSandbox(ctx, "", worktree, check.Command, nil, out, diag, opts.MemoryMB, opts.CPUSeconds)
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
		afterCheck, err := repoStatus(worktree)
		if err != nil {
			return fail("failed", err.Error())
		}
		if err := checkDidNotMutate(beforeCheck, afterCheck, i+1); err != nil {
			return fail("failed", err.Error())
		}
		_ = appendRailEvents(home, railEvent("loop.pass.check.passed", map[string]any{"pass": pass, "goal": opts.Goal, "command": check.Command, "heavy": check.Heavy}))
	}
	if err := appendRailEvents(home, railEvent("loop.pass.checks.completed", map[string]any{"pass": pass, "goal": opts.Goal, "checks": len(plan.Checks)})); err != nil {
		return err
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
	if err := reconcileExternalWriters(home, opts, worktree, true); err != nil {
		return fail("failed", fmt.Sprintf("writer appeared before commit: %v", err))
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
	baseRaw, err := repoOutput(worktree, "rev-parse", "HEAD")
	if err != nil {
		return fail("failed", err.Error())
	}
	baseCommit := strings.TrimSpace(string(baseRaw))
	if err := appendRailEvents(home, railEvent("loop.pass.commit.started", map[string]any{"pass": pass, "goal": opts.Goal, "base": baseCommit, "branch": opts.Branch})); err != nil {
		return err
	}
	if guardedFault != nil {
		if err := guardedFault("after-commit-started"); err != nil {
			return fail("resumable", err.Error())
		}
	}
	gitCtx, cancel = context.WithTimeout(context.Background(), opts.Timeout)
	err = repoRunContext(gitCtx, worktree, "-c", "user.name=self guarded loop", "-c", "user.email=self@localhost", "commit", "-m", plan.CommitMessage)
	cancel()
	if err != nil {
		return fail("failed", err.Error())
	}
	if guardedFault != nil {
		if err := guardedFault("after-commit-before-event"); err != nil {
			return fail("resumable", err.Error())
		}
	}
	commitRaw, err := repoOutput(worktree, "rev-parse", "HEAD")
	if err != nil {
		return fail("failed", err.Error())
	}
	commit := strings.TrimSpace(string(commitRaw))
	if err := appendRailEvents(home, railEvent("loop.pass.commit.completed", map[string]any{"pass": pass, "goal": opts.Goal, "base": baseCommit, "commit": commit, "branch": opts.Branch})); err != nil {
		return err
	}
	if guardedFault != nil {
		if err := guardedFault("after-commit"); err != nil {
			return fail("resumable", err.Error())
		}
	}
	if decision.Push {
		if err := reconcileExternalWriters(home, opts, worktree, true); err != nil {
			return fail("failed", fmt.Sprintf("writer appeared before push: %v", err))
		}
		if _, err := leaseChange(home, "renew", lease, opts.LeaseTTL, time.Now().UTC()); err != nil {
			return fail("resumable", err.Error())
		}
		if err := appendRailEvents(home, railEvent("loop.pass.push.started", map[string]any{"pass": pass, "goal": opts.Goal, "branch": opts.Branch, "commit": commit})); err != nil {
			return err
		}
		pushCtx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		err := repoRunContext(pushCtx, worktree, "push", "origin", "HEAD:refs/heads/"+opts.Branch)
		cancel()
		if err != nil {
			return fail("resumable", err.Error())
		}
		if guardedFault != nil {
			if err := guardedFault("after-push"); err != nil {
				return fail("resumable", err.Error())
			}
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
	verifyCtx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	err = verifyRemoteContext(verifyCtx, worktree, opts.Branch, commit)
	cancel()
	if err != nil {
		return fail("resumable", err.Error())
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	err = cleanupSuccessfulWorktree(cleanupCtx, opts.Repo, worktree)
	cancel()
	if err != nil {
		return fail("resumable", fmt.Sprintf("successful worktree cleanup failed: %v", err))
	}
	if err := appendRailEvents(home, railEvent("loop.pass.worktree.cleaned", map[string]any{"pass": pass, "goal": opts.Goal, "worktree": worktree})); err != nil {
		return err
	}
	evidence := map[string]any{
		"pass": pass, "goal": opts.Goal, "commit": commit, "branch": opts.Branch,
		"remote_verified": true, "worktree_clean": true, "worktree_removed": true, "invocation_checkout_unchanged": true,
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
