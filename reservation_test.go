package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		dispatch := reservationCommand(binary, home, reservations, "reserve", "dispatch", repo, "--", "sh", "-c", "touch \"$1\"; sleep 0.4; printf '%s\\n' \"$2\"", "dispatch", ready, wire)
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
	t.Setenv("SELF_RESERVATION_DIR", t.TempDir())
	err := cmdReserve(t.TempDir(), []string{"dispatch", repo, "--", "sh", "-c", "printf prose"}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no agent.started or agent.failed") {
		t.Fatalf("missing evidence error=%v", err)
	}
}

func TestDispatchHelperInheritsReservationAcrossWrapperDeath(t *testing.T) {
	repo, _ := testRepository(t)
	binary := buildSelfForReservationTest(t)
	home, reservations := t.TempDir(), t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready")
	wrapper := reservationCommand(binary, home, reservations, "reserve", "exec", repo, "--", "sh", "-c", "touch \"$1\"; sleep 0.5", "helper", ready)
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
