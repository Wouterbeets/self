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
)

func runKs(t *testing.T, home string, args ...string) string {
	t.Helper()
	bin := os.Getenv("KS_TEST_BIN")
	if bin == "" {
		t.Skip("set KS_TEST_BIN to the ks binary to run integration tests")
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "KS_HOME="+home, "KS_LLM_STUB=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("ks %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func TestEndToEndStubWritesSiteAndServes(t *testing.T) {
	bin := os.Getenv("KS_TEST_BIN")
	if bin == "" {
		t.Skip("set KS_TEST_BIN to the ks binary to run integration tests")
	}
	home := t.TempDir()

	runKs(t, home, "init")
	if _, err := os.Stat(filepath.Join(home, "site")); err != nil {
		t.Fatal("ks init did not create site/")
	}

	runKs(t, home, "plant", "seeds/chat")

	projPath := filepath.Join(home, "registry", "projectors", "chat")
	if _, err := os.Stat(projPath); err != nil {
		t.Fatal("plant did not write projector script at projectors/chat")
	}

	runKs(t, home, "invoke", "chat", "from-test")

	runKs(t, home, "project", "chat")
	sitePath := filepath.Join(home, "site", "chat.html")
	data, err := os.ReadFile(sitePath)
	if err != nil {
		t.Fatalf("projector did not write site file: %v", err)
	}
	if !strings.Contains(string(data), "from-test") {
		t.Errorf("site HTML missing invoked message:\n%s", string(data))
	}

	srv := exec.Command(bin, "serve", "18777")
	srv.Env = append(os.Environ(), "KS_HOME="+home, "KS_LLM_STUB=1")
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
		t.Fatal("could not reach /events on ks serve")
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
	bin := os.Getenv("KS_TEST_BIN")
	if bin == "" {
		t.Skip("set KS_TEST_BIN to the ks binary to run integration tests")
	}
	home := t.TempDir()

	runKs(t, home, "init")
	runKs(t, home, "plant", "seeds/chat")

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
	chatPath := filepath.Join(home, "registry", "commands", "chat")
	if err := os.WriteFile(chatPath, []byte(echoScript), 0755); err != nil {
		t.Fatal(err)
	}

	runKs(t, home, "invoke", "chat", "trigger")

	echoPath := filepath.Join(home, "registry", "commands", "echo")
	if _, err := os.Stat(echoPath); err != nil {
		t.Fatalf("echo command not compiled to registry: %v", err)
	}

	runKs(t, home, "invoke", "echo", "it-works")
}
