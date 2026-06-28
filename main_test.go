package main

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"self/internal/event"
)

func runSelf(t *testing.T, home string, args ...string) string {
	t.Helper()
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home, "SELF_LLM_STUB=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("self %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func TestEndToEndStubWritesSiteAndServes(t *testing.T) {
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	home := t.TempDir()

	runSelf(t, home, "init")
	if _, err := os.Stat(filepath.Join(home, "site")); err != nil {
		t.Fatal("self init did not create site/")
	}

	runSelf(t, home, "grow", "seeds/chat")

	projPath := filepath.Join(home, "capabilities", "projectors", "chat")
	if _, err := os.Stat(projPath); err != nil {
		t.Fatal("plant did not write projector script at projectors/chat")
	}

	runSelf(t, home, "run", "chat", "from-test")

	runSelf(t, home, "show", "chat")
	sitePath := filepath.Join(home, "site", "chat.html")
	data, err := os.ReadFile(sitePath)
	if err != nil {
		t.Fatalf("projector did not write site file: %v", err)
	}
	if !strings.Contains(string(data), "from-test") {
		t.Errorf("site HTML missing invoked message:\n%s", string(data))
	}

	srv := exec.Command(bin, "live", "18777")
	srv.Env = append(os.Environ(), "SELF_HOME="+home, "SELF_LLM_STUB=1")
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Process.Kill()
	t.Cleanup(func() { srv.Process.Kill() })

	base := "http://localhost:18777"
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(3 * time.Second)
	var ok bool
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/events")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 && strings.Contains(string(body), "chat.message") {
				ok = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ok {
		t.Fatal("could not reach /events on self live")
	}

	resp, err := client.Get(base + "/live/chat")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	liveBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Errorf("/live/chat status %d", resp.StatusCode)
	}
	if !strings.Contains(string(liveBody), "<ul>") {
		t.Errorf("/live/chat did not return projector HTML:\n%s", string(liveBody))
	}
}

func TestStrangeLoopCommandDeclaresNewCommand(t *testing.T) {
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	home := t.TempDir()

	runSelf(t, home, "init")
	runSelf(t, home, "grow", "seeds/chat")

	echoScript := `#!/usr/bin/env python3
import sys, json
print(json.dumps({"name": "chat.message", "payload": {"role": "user", "content": " ".join(sys.argv[1:])}}))
print(json.dumps({"name": "chat.message", "payload": {"role": "assistant", "content": "ok"}}))
print(json.dumps({"name": "command.declared", "payload": {
    "name": "echo",
    "description": "echo a message",
    "params": {"text": "string"},
    "event": {"name": "echo.said", "fields": {"text": "string"}}
}}))
`
	chatPath := filepath.Join(home, "capabilities", "commands", "chat")
	if err := os.WriteFile(chatPath, []byte(echoScript), 0755); err != nil {
		t.Fatal(err)
	}

	runSelf(t, home, "run", "chat", "trigger")

	echoPath := filepath.Join(home, "capabilities", "commands", "echo")
	if _, err := os.Stat(echoPath); err != nil {
		t.Fatalf("echo command not compiled to capabilities: %v", err)
	}

	runSelf(t, home, "run", "echo", "it-works")
}

// TestThinkPipesToSwappableBrainProcess is the slice-12 evidence: `self think`
// no longer links an LLM into the kernel — it pipes the prompt to whatever
// $SELF_BRAIN names and ingests the events that process emits. Here the brain is
// a 12-line Python script with no LLM at all; the kernel can't tell the
// difference. Its emitted chat.message events land in the log (and render via
// the chat projector), and a command.declared it emits flows through the same
// strange-loop compile a real command's output would.
func TestThinkPipesToSwappableBrainProcess(t *testing.T) {
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	home := t.TempDir()

	runSelf(t, home, "init")
	runSelf(t, home, "grow", "seeds/chat")

	// A fake brain: prompt arrives as argv; it emits the conversation and grows a
	// capability — purely via stdout JSONL. No network, no model.
	fakeBrain := `#!/usr/bin/env python3
import sys, json
prompt = sys.argv[1] if len(sys.argv) > 1 else ""
print(json.dumps({"name": "chat.message", "payload": {"role": "user", "content": prompt}}))
print(json.dumps({"name": "chat.message", "payload": {"role": "assistant", "content": "thought about: " + prompt}}))
print(json.dumps({"name": "command.declared", "payload": {
    "name": "noted", "description": "note a thing", "params": {"text": "string"},
    "event": {"name": "noted.added", "fields": {"text": "string"}}}}))
`
	brainPath := filepath.Join(home, "fakebrain.py")
	if err := os.WriteFile(brainPath, []byte(fakeBrain), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "think", "hello world")
	cmd.Env = append(os.Environ(),
		"SELF_HOME="+home,
		"SELF_LLM_STUB=1",                          // declarations the brain emits compile via the stub
		"SELF_BRAIN=python3 "+brainPath,            // <-- the kernel pipes to THIS, not an LLM
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("self think: %v\n%s", err, out.String())
	}

	// The brain's reply landed in the log as ordinary chat.message events — the
	// kernel ingested the process's stdout exactly as it would a command's.
	log, err := os.ReadFile(filepath.Join(home, "events.jsonl"))
	if err != nil {
		t.Fatalf("event log not written: %v", err)
	}
	if !strings.Contains(string(log), "thought about: hello world") {
		t.Errorf("brain's reply did not reach the event log:\n%s", string(log))
	}
	// And the chat projector picked the new chat.message events up (auto-run).
	runSelf(t, home, "show", "chat")
	if _, err := os.Stat(filepath.Join(home, "site", "chat.html")); err != nil {
		t.Fatalf("chat projection not written: %v", err)
	}

	// The command.declared the brain emitted flowed through the strange loop and
	// was compiled to a real capability — growth via the brain process.
	if _, err := os.Stat(filepath.Join(home, "capabilities", "commands", "noted")); err != nil {
		t.Fatalf("brain-declared command not compiled (strange loop did not fire): %v", err)
	}
	runSelf(t, home, "run", "noted", "it-works")
}

func TestSinceLastHeartbeat(t *testing.T) {
	ev := func(name string) event.Event { return event.Event{Name: name} }
	log := []event.Event{ev("kernel.initialized"), ev("task.captured"), ev("self.heartbeat"), ev("meal.planned"), ev("shopping.added")}
	since := sinceLastHeartbeat(log)
	if len(since) != 2 || since[0].Name != "meal.planned" || since[1].Name != "shopping.added" {
		t.Fatalf("since last heartbeat = %v, want [meal.planned shopping.added]", since)
	}
	// no prior heartbeat → everything is new
	if got := sinceLastHeartbeat([]event.Event{ev("kernel.initialized"), ev("task.captured")}); len(got) != 2 {
		t.Fatalf("no-heartbeat case = %d events, want 2", len(got))
	}
}

func TestHeartbeatContext(t *testing.T) {
	mk := func(seq int, name, payload string) event.Event {
		return event.Event{Seq: seq, Name: name, Payload: []byte(payload)}
	}
	log := []event.Event{
		mk(1, "self.heartbeat", "{}"),
		mk(2, "task.captured", `{"text":"call plumber"}`),
		mk(3, "script.compiled", `{"script":"... huge ..."}`), // bookkeeping, must be skipped
		mk(4, "meal.planned", `{"day":"mon","meal":"tacos"}`),
	}
	ctx := heartbeatContext(log)
	if !strings.Contains(ctx, "call plumber") || !strings.Contains(ctx, "tacos") {
		t.Errorf("context should include the real actions, got:\n%s", ctx)
	}
	if strings.Contains(ctx, "script.compiled") {
		t.Errorf("context should skip kernel bookkeeping, got:\n%s", ctx)
	}
	// a beat with no activity since the last one → empty (quiet stays quiet)
	if heartbeatContext([]event.Event{mk(1, "self.heartbeat", "{}")}) != "" {
		t.Error("no activity should yield empty context")
	}
}
