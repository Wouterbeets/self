package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func buildSelfForReservationTest(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "self")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build self: %v\n%s", err, out)
	}
	return binary
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func reservationCommand(binary, home, reservations string, args ...string) *exec.Cmd {
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home, "SELF_CALLER=reservation-test", "SELF_RESERVATION_DIR="+reservations)
	return cmd
}

func installReservationFixture(t *testing.T, home, name, filename string) {
	t.Helper()
	script, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatal(err)
	}
	declaration, _ := json.Marshal(map[string]any{"name": "command.declared", "payload": map[string]string{"name": name, "description": "fixture reserved dispatch"}})
	authored, _ := json.Marshal(map[string]any{"name": "script.authored", "payload": map[string]string{"type": "command", "name": name, "script": string(script)}})
	body := append(append(declaration, '\n'), authored...)
	body = append(body, '\n')
	if err := cmdHear(home, body, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func fixtureHerdr(t *testing.T) (string, string) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "herdr-used")
	binary := filepath.Join(t.TempDir(), "herdr")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\ntouch \"$SELF_FIXTURE_HERDR_MARKER\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return binary, marker
}

func TestInstalledDispatchCapabilityRecursesThroughReservation(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	home, reservations := t.TempDir(), t.TempDir()
	installReservationFixture(t, home, "dispatch", "dispatch-reserved.py")
	marker := filepath.Join(t.TempDir(), "started")
	herdr, herdrMarker := fixtureHerdr(t)
	dispatch := reservationCommand(binary, home, reservations, "run", "dispatch", "g", "fixture-agent", "opencode", repo)
	dispatch.Env = append(dispatch.Env, "SELF_FIXTURE_DISPATCH_MARKER="+marker, "SELF_FIXTURE_DISPATCH_DELAY=0.4", "SELF_HERDR_BIN="+herdr, "SELF_FIXTURE_REQUIRE_HERDR=1", "SELF_FIXTURE_HERDR_MARKER="+herdrMarker)
	var output bytes.Buffer
	dispatch.Stdout, dispatch.Stderr = &output, &output
	if err := dispatch.Start(); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, marker)
	if out, err := reservationCommand(binary, home, reservations, "reserve", "check", repo).CombinedOutput(); err == nil || !strings.Contains(string(out), "reserved") {
		t.Fatalf("dispatch capability did not serialize repository: %v\n%s", err, out)
	}
	if err := dispatch.Wait(); err != nil {
		t.Fatalf("self run dispatch: %v\n%s", err, output.String())
	}
	st, err := loadState(home)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range st.Events {
		if event.Name != "agent.started" {
			continue
		}
		var payload struct {
			ReservationValidated bool `json:"reservation_validated"`
		}
		_ = json.Unmarshal(event.Payload, &payload)
		found = payload.ReservationValidated
	}
	if !found {
		t.Fatal("recursive reserved dispatch did not commit validated terminal evidence")
	}
	if _, err := os.Stat(herdrMarker); err != nil {
		t.Fatalf("configured Herdr binary was not executed: %v", err)
	}
}

func TestInstalledDispatchPaneCapabilityPublishesProvenCleanup(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	home, reservations := t.TempDir(), t.TempDir()
	installReservationFixture(t, home, "dispatch.pane", "dispatch-pane-reserved.py")
	resources := filepath.Join(t.TempDir(), "partial-resources")
	herdr, herdrMarker := fixtureHerdr(t)
	cmd := reservationCommand(binary, home, reservations, "run", "dispatch.pane", "g", "pane-agent", "custom", repo)
	cmd.Env = append(cmd.Env, "SELF_FIXTURE_RESOURCE_ROOT="+resources, "SELF_HERDR_BIN="+herdr, "SELF_FIXTURE_REQUIRE_HERDR=1", "SELF_FIXTURE_HERDR_MARKER="+herdrMarker)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("self run dispatch.pane: %v\n%s", err, out)
	}
	st, err := loadState(home)
	if err != nil {
		t.Fatal(err)
	}
	foundFailure, foundReceipt := false, false
	for _, event := range st.Events {
		foundFailure = foundFailure || event.Name == "agent.failed"
		foundReceipt = foundReceipt || event.Name == "loop.reservation.dispatch.published"
	}
	if !foundFailure || !foundReceipt {
		t.Fatalf("failed=%v receipt=%v", foundFailure, foundReceipt)
	}
	if _, err := os.Stat(resources); !os.IsNotExist(err) {
		t.Fatalf("partial pane/workspace/worktree resources survived cleanup: %v", err)
	}
	if _, err := os.Stat(herdrMarker); err != nil {
		t.Fatalf("dispatch.pane did not execute configured Herdr: %v", err)
	}
}

func TestInstalledDispatchCapabilityPublishesProvenPartialCleanup(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	home, reservations := t.TempDir(), t.TempDir()
	installReservationFixture(t, home, "dispatch", "dispatch-reserved.py")
	resources := filepath.Join(t.TempDir(), "partial-resources")
	cmd := reservationCommand(binary, home, reservations, "run", "dispatch", "g", "partial-agent", "opencode", repo)
	cmd.Env = append(cmd.Env, "SELF_FIXTURE_DISPATCH_MODE=failed-clean", "SELF_FIXTURE_RESOURCE_ROOT="+resources)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("self run dispatch failed cleanup: %v\n%s", err, out)
	}
	if _, err := os.Stat(resources); !os.IsNotExist(err) {
		t.Fatalf("partial child/worktree resources survived cleanup: %v", err)
	}
	st, _ := loadState(home)
	foundFailure, foundReceipt := false, false
	for _, event := range st.Events {
		foundFailure = foundFailure || event.Name == "agent.failed"
		foundReceipt = foundReceipt || event.Name == "loop.reservation.dispatch.published"
	}
	if !foundFailure || !foundReceipt {
		t.Fatalf("failed=%v receipt=%v", foundFailure, foundReceipt)
	}
}

func TestDispatchFixturesAuditReservationAndHerdrDiscovery(t *testing.T) {
	for _, filename := range []string{"dispatch-reserved.py", "dispatch-pane-reserved.py"} {
		data, err := os.ReadFile(filepath.Join("testdata", filename))
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		for _, required := range []string{"SELF_HERDR_BIN", `shutil.which("herdr")`, `"reserve", "held"`, `"reserve", "dispatch"`, `"reserve", "publish"`} {
			if !strings.Contains(source, required) {
				t.Errorf("%s missing %q", filename, required)
			}
		}
		for _, forbidden := range []string{"/opt/homebrew", "/usr/local/bin/herdr", "/usr/bin/herdr"} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s contains hardcoded Herdr path %q", filename, forbidden)
			}
		}
	}
}

func TestProductionDefaultReservationMatchesScrubbedCapabilityEnvironment(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	home := t.TempDir()
	t.Setenv("SELF_RESERVATION_DIR", "")
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "xdg-outside"))
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "tmp-outside"))
	directPath, _, directIdentity, err := reservationPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Join("/tmp", "self-"+strconv.Itoa(os.Getuid()), "reservations")
	if filepath.Dir(directPath) != wantRoot {
		t.Fatalf("direct default root=%s, want %s", filepath.Dir(directPath), wantRoot)
	}
	if info, err := os.Stat(wantRoot); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0700 {
		t.Fatalf("default root mode=%v", info.Mode().Perm())
	}
	installReservationFixture(t, home, "reservation.probe", "reservation-probe.py")
	held, err := acquireRepositoryReservation(repo, "direct-production-default")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	cmd := exec.Command(binary, "run", "reservation.probe", repo)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home, "SELF_CALLER=reservation-default-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("reservation probe capability: %v\n%s", err, out)
	}
	st, err := loadState(home)
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Identity         string `json:"identity"`
		Path             string `json:"path"`
		Available        bool   `json:"available"`
		SawXDGRuntimeDir bool   `json:"saw_xdg_runtime_dir"`
		SawTMPDIR        bool   `json:"saw_tmpdir"`
	}
	for _, event := range st.Events {
		if event.Name == "reservation.probed" {
			_ = json.Unmarshal(event.Payload, &probe)
		}
	}
	if probe.Path != directPath || probe.Identity != directIdentity {
		t.Fatalf("capability lock=%s/%s, direct=%s/%s", probe.Identity, probe.Path, directIdentity, directPath)
	}
	if probe.Available {
		t.Fatal("scrubbed capability acquired a different reservation while direct lock was held")
	}
	if probe.SawXDGRuntimeDir {
		t.Fatal("capability unexpectedly inherited XDG_RUNTIME_DIR")
	}
	if probe.SawTMPDIR {
		t.Fatal("capability unexpectedly inherited TMPDIR")
	}
}

func TestRepositoryReservationRaceOrderingsAcrossProcesses(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	reservations := t.TempDir()
	t.Setenv("SELF_RESERVATION_DIR", reservations)

	t.Run("dispatch wins before guarded", func(t *testing.T) {
		home := t.TempDir()
		ready := filepath.Join(t.TempDir(), "dispatch-ready")
		payload, _ := json.Marshal(map[string]string{"agent": "dispatch-winner", "goal": "g", "repo": repo})
		wire := `{"name":"agent.started","payload":` + string(payload) + `}`
		dispatch := reservationCommand(binary, home, reservations, "reserve", "dispatch", repo, "--", "sh", "-c", "touch \"$1\"; sleep 0.4; printf '%s\\n' \"$2\" | \"$SELF_BINARY\" reserve publish \"$3\"", "dispatch", ready, wire, repo)
		if err := dispatch.Start(); err != nil {
			t.Fatal(err)
		}
		waitForFile(t, ready)
		guarded := reservationCommand(binary, home, reservations, "reserve", "check", repo)
		out, err := guarded.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "reserved by another self writer") {
			t.Fatalf("guarded contender error=%v\n%s", err, out)
		}
		if err := dispatch.Wait(); err != nil {
			t.Fatal(err)
		}
		st, err := loadState(home)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, event := range st.Events {
			found = found || event.Name == "agent.started"
		}
		if !found {
			t.Fatal("dispatch released reservation before agent.started committed")
		}
		herdr := filepath.Join(t.TempDir(), "herdr")
		if err := os.WriteFile(herdr, []byte("#!/bin/sh\nprintf '%s\\n' '{\"result\":{\"agents\":[{\"name\":\"dispatch-winner\",\"agent_status\":\"working\"}]}}'\n"), 0755); err != nil {
			t.Fatal(err)
		}
		plan := guardedPlan{Goal: "g", Project: repo, CommitMessage: "x", Actions: []plannedAction{{Kind: "worker", Goal: "g", Project: repo, Command: []string{"true"}, Files: []string{"x"}}, {Kind: "push", Goal: "g", Project: repo}}, Checks: []plannedCheck{{Command: []string{"true"}}}}
		args := guardedArgs(repo, filepath.Join(t.TempDir(), "worktrees"), "g", "goal/race", plan)
		args = append(args[:1], append([]string{"--herdr", herdr}, args[1:]...)...)
		if err := cmdGuardedLoop(home, args, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "live Herdr agent dispatch-winner") {
			t.Fatalf("guarded pass did not refuse committed active writer: %v", err)
		}
	})

	t.Run("guarded wins before dispatch", func(t *testing.T) {
		home := t.TempDir()
		created := filepath.Join(t.TempDir(), "dispatch-created")
		worktrees := filepath.Join(t.TempDir(), "worktrees")
		plan := guardedPlan{Goal: "guarded", Project: repo, CommitMessage: "guarded wins", Actions: []plannedAction{{Kind: "worker", Goal: "guarded", Project: repo, Command: []string{"sh", "-c", "sleep 0.5; printf guarded > guarded.txt"}, Files: []string{"guarded.txt"}}, {Kind: "push", Goal: "guarded", Project: repo}}, Checks: []plannedCheck{{Command: []string{"test", "-f", "guarded.txt"}}}}
		raw, _ := json.Marshal(plan)
		guarded := reservationCommand(binary, home, reservations, "loop", "--guarded", "--goal", "guarded", "--repo", repo, "--branch", "goal/reservation-race", "--worktree-root", worktrees, "--min-free-mb=1", "--timeout=2s", "--", "sh", "-c", "printf '%s' "+shellQuote(string(raw)))
		if err := guarded.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			rows, _ := registeredWorktrees(repo)
			seen := false
			for _, row := range rows {
				seen = seen || row.Branch == "goal/reservation-race"
			}
			if seen {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		dispatch := reservationCommand(binary, home, reservations, "reserve", "dispatch", repo, "--", "sh", "-c", "touch \"$1\"", "dispatch", created)
		out, err := dispatch.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "reserved by another self writer") {
			t.Fatalf("dispatch contender error=%v\n%s", err, out)
		}
		if _, err := os.Stat(created); !os.IsNotExist(err) {
			t.Fatalf("losing dispatch ran before reservation: %v", err)
		}
		if err := guarded.Wait(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDispatchReservationRequiresAuthoritativeEvidence(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	home, reservations := t.TempDir(), t.TempDir()
	unsafe := reservationCommand(binary, home, reservations, "reserve", "dispatch", repo, "--", "sh", "-c", "printf prose")
	if err := unsafe.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if out, err := reservationCommand(binary, home, reservations, "reserve", "check", repo).CombinedOutput(); err == nil || !strings.Contains(string(out), "reserved") {
		t.Fatalf("unsafe evidence released reservation: %v\n%s", err, out)
	}
	if err := unsafe.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = unsafe.Wait()
}

func TestPartialStartFailureRequiresStructuredCleanupEvidence(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	home, reservations := t.TempDir(), t.TempDir()
	unsafePayload, _ := json.Marshal(map[string]any{"agent": "partial", "repo": repo, "writer": map[string]any{"state": "stopped"}})
	unsafeWire := `{"name":"agent.failed","payload":` + string(unsafePayload) + `}`
	unsafe := reservationCommand(binary, home, reservations, "reserve", "dispatch", repo, "--", "sh", "-c", "printf '%s\\n' \"$1\" | \"$SELF_BINARY\" reserve publish \"$2\"", "helper", unsafeWire, repo)
	if err := unsafe.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if out, err := reservationCommand(binary, home, reservations, "reserve", "check", repo).CombinedOutput(); err == nil || !strings.Contains(string(out), "reserved") {
		t.Fatalf("unproved cleanup released reservation: %v\n%s", err, out)
	}
	_ = unsafe.Process.Signal(syscall.SIGTERM)
	_ = unsafe.Wait()

	safePayload, _ := json.Marshal(map[string]any{"agent": "partial", "repo": repo, "writer": map[string]any{"state": "stopped", "cleanup_confirmed": true, "evidence": "child process absent; worktree removed"}})
	safeWire := `{"name":"agent.failed","payload":` + string(safePayload) + `}`
	safe := reservationCommand(binary, home, reservations, "reserve", "dispatch", repo, "--", "sh", "-c", "printf '%s\\n' \"$1\" | \"$SELF_BINARY\" reserve publish \"$2\"", "helper", safeWire, repo)
	if out, err := safe.CombinedOutput(); err != nil {
		t.Fatalf("proved cleanup did not release reservation: %v\n%s", err, out)
	}
}

func TestInheritedReservationValidationRejectsForgedEnvironment(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	home, reservations := t.TempDir(), t.TempDir()
	_, _, identity, err := reservationPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	forgedFile, err := os.CreateTemp(t.TempDir(), "unrelated")
	if err != nil {
		t.Fatal(err)
	}
	defer forgedFile.Close()
	cmd := reservationCommand(binary, home, reservations, "reserve", "held", repo)
	cmd.Env = append(cmd.Env, "SELF_REPOSITORY_RESERVATION="+identity, "SELF_REPOSITORY_RESERVATION_FD=3")
	cmd.ExtraFiles = []*os.File{forgedFile}
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "does not name canonical lock") {
		t.Fatalf("forged reservation accepted: %v\n%s", err, out)
	}
}

func TestInheritedReservationValidationRejectsCanonicalUnlockedFD(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	home, reservations := t.TempDir(), t.TempDir()
	t.Setenv("SELF_RESERVATION_DIR", reservations)
	path, _, identity, err := reservationPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cmd := reservationCommand(binary, home, reservations, "reserve", "held", repo)
	cmd.Env = append(cmd.Env, "SELF_REPOSITORY_RESERVATION="+identity, "SELF_REPOSITORY_RESERVATION_FD=3")
	cmd.ExtraFiles = []*os.File{file}
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "does not own") {
		t.Fatalf("canonical unlocked descriptor accepted: %v\n%s", err, out)
	}
}

func TestContradictoryStartedThenNotStartedFailureIsUnsafe(t *testing.T) {
	repo, _ := testRepository(t)
	_, identity, err := repositoryIdentity(repo)
	if err != nil {
		t.Fatal(err)
	}
	reservation := &repositoryReservation{Identity: identity, Repository: repo}
	started, _ := json.Marshal(map[string]any{"agent": "a", "repo": repo})
	failed, _ := json.Marshal(map[string]any{"agent": "a", "repo": repo, "writer": map[string]any{"state": "not_started"}})
	wire := []byte(`{"name":"agent.started","payload":` + string(started) + `}` + "\n" + `{"name":"agent.failed","payload":` + string(failed) + `}` + "\n")
	if _, err := validateDispatchEvidence(wire, reservation, nil); err == nil || !strings.Contains(err.Error(), "unknown or cleanup is unproved") {
		t.Fatalf("contradictory evidence accepted: %v", err)
	}
	onlyFailed := []byte(`{"name":"agent.failed","payload":` + string(failed) + `}` + "\n")
	if _, err := validateDispatchEvidence(onlyFailed, reservation, map[string]bool{"a": true}); err == nil {
		t.Fatal("not_started failure erased an existing live writer")
	}
}

func TestDispatchPublicationRejectsAnyUnmatchedWriterEvent(t *testing.T) {
	repo, _ := testRepository(t)
	other, _ := testRepository(t)
	_, identity, err := repositoryIdentity(repo)
	if err != nil {
		t.Fatal(err)
	}
	reservation := &repositoryReservation{Identity: identity, Repository: repo}
	matching, _ := json.Marshal(map[string]string{"agent": "a", "repo": repo})
	unmatched, _ := json.Marshal(map[string]string{"agent": "b", "repo": other})
	wire := []byte(`{"name":"agent.started","payload":` + string(matching) + `}` + "\n" + `{"name":"agent.started","payload":` + string(unmatched) + `}` + "\n")
	if _, err := validateDispatchEvidence(wire, reservation, nil); err == nil || !strings.Contains(err.Error(), "does not match held") {
		t.Fatalf("mixed-repository publication accepted: %v", err)
	}
}

func TestDispatchPublicationRejectsUnmatchedReclaimedEvent(t *testing.T) {
	repo, _ := testRepository(t)
	other, _ := testRepository(t)
	_, identity, _ := repositoryIdentity(repo)
	reservation := &repositoryReservation{Identity: identity, Repository: repo}
	matching, _ := json.Marshal(map[string]string{"agent": "a", "repo": repo})
	unmatched, _ := json.Marshal(map[string]any{"agent": "b", "repo": other, "writer": map[string]any{"state": "stopped", "cleanup_confirmed": true, "evidence": "removed"}})
	wire := []byte(`{"name":"agent.started","payload":` + string(matching) + `}` + "\n" + `{"name":"agent.reclaimed","payload":` + string(unmatched) + `}` + "\n")
	if _, err := validateDispatchEvidence(wire, reservation, nil); err == nil || !strings.Contains(err.Error(), "does not match held") {
		t.Fatalf("unmatched reclaimed event accepted: %v", err)
	}
}

func TestDispatchHelperInheritsReservationAcrossWrapperDeath(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	home, reservations := t.TempDir(), t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready")
	payload, _ := json.Marshal(map[string]string{"agent": "survivor", "repo": repo})
	wire := `{"name":"agent.started","payload":` + string(payload) + `}`
	wrapper := reservationCommand(binary, home, reservations, "reserve", "dispatch", repo, "--", "sh", "-c", "touch \"$1\"; sleep 0.5; printf '%s\\n' \"$2\" | \"$SELF_BINARY\" reserve publish \"$3\"", "helper", ready, wire, repo)
	if err := wrapper.Start(); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, ready)
	if err := wrapper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = wrapper.Wait()
	contender := reservationCommand(binary, home, reservations, "reserve", "check", repo)
	if out, err := contender.CombinedOutput(); err == nil || !strings.Contains(string(out), "reserved") {
		t.Fatalf("helper lost inherited reservation after wrapper death: %v\n%s", err, out)
	}
	time.Sleep(600 * time.Millisecond)
	if out, err := reservationCommand(binary, home, reservations, "reserve", "check", repo).CombinedOutput(); err != nil {
		t.Fatalf("reservation did not release after helper exit: %v\n%s", err, out)
	}
	st, err := loadState(home)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range st.Events {
		found = found || event.Name == "agent.started"
	}
	if !found {
		t.Fatal("helper survived wrapper death but did not publish authoritative evidence")
	}
}

func TestReservationRootCannotBeInsideSiblingWorktree(t *testing.T) {
	repo, _ := testRepository(t)
	sibling := filepath.Join(t.TempDir(), "sibling")
	testGit(t, repo, "worktree", "add", "-b", "sibling", sibling, "HEAD")
	root := filepath.Join(sibling, ".locks")
	t.Setenv("SELF_RESERVATION_DIR", root)
	if _, err := acquireRepositoryReservation(repo, "bad-root"); err == nil || !strings.Contains(err.Error(), "outside every worktree") {
		t.Fatalf("sibling worktree reservation root error=%v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("rejected reservation root mutated sibling worktree: %v", err)
	}
}

func TestRepositoryReservationIdentitySurvivesSymlinkAliases(t *testing.T) {
	repo, _ := testRepository(t)
	alias := filepath.Join(t.TempDir(), "repo-alias")
	if err := os.Symlink(repo, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SELF_RESERVATION_DIR", t.TempDir())
	first, err := acquireRepositoryReservation(repo, "first")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	if _, err := acquireRepositoryReservation(alias, "alias"); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("symlink alias did not share reservation: %v", err)
	}
}
