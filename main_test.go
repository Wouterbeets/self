package main

import (
	"bytes"
	"encoding/json"
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

func TestLoadBrainConfigOpencodeUsesUIKey(t *testing.T) {
	home := t.TempDir()
	for _, k := range []string{"SELF_LLM_URL", "SELF_LLM_API_KEY", "SELF_LLM_MODEL"} {
		t.Setenv(k, "")
	}

	if err := os.WriteFile(filepath.Join(home, "events.jsonl"), []byte(`{"name":"brain.configured","payload":{"provider":"opencode","base_url":"","model":"","key_set":true}}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(brainKeyFile(home), []byte("sk-ui-opencode"), 0600); err != nil {
		t.Fatal(err)
	}

	if got := loadBrainConfig(home); got != "opencode" {
		t.Fatalf("provider = %q, want opencode", got)
	}
	if got := os.Getenv("SELF_LLM_URL"); got != opencodeLLMURL {
		t.Errorf("SELF_LLM_URL = %q, want %q", got, opencodeLLMURL)
	}
	if got := os.Getenv("SELF_LLM_MODEL"); got != opencodeLLMModel {
		t.Errorf("SELF_LLM_MODEL = %q, want %q", got, opencodeLLMModel)
	}
	if got := os.Getenv("SELF_LLM_API_KEY"); got != "sk-ui-opencode" {
		t.Errorf("SELF_LLM_API_KEY = %q, want UI key", got)
	}
}

// writeFixtureSeed writes a minimal legacy (parts-list) seed with NO examples, so
// it grows in stub mode without a brain — the no-LLM path the kernel-plumbing
// tests need now that real seeds are intent seeds (which require the orchestrator).
func writeFixtureSeed(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	fixture := `{"name":"command.declared","payload":{"name":"note","description":"note a thing","params":{"text":"string"},"event":{"name":"note.added","fields":{"text":"string"}}}}
{"name":"projector.declared","payload":{"name":"notes","description":"list note.added events","consumes":["note.added"]}}
`
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(fixture), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
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

	// A minimal example-free legacy seed, grown in stub mode — exercises the kernel
	// plumbing (compile → write site → serve) without a brain. Intent seeds need
	// the orchestrator (a brain), so they can't stand in for this no-LLM path.
	runSelf(t, home, "grow", writeFixtureSeed(t))

	projPath := filepath.Join(home, "capabilities", "projectors", "notes")
	if _, err := os.Stat(projPath); err != nil {
		t.Fatal("grow did not write projector script at projectors/notes")
	}

	runSelf(t, home, "run", "note", "from-test")

	runSelf(t, home, "show", "notes")
	sitePath := filepath.Join(home, "site", "notes.html")
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
			if resp.StatusCode == 200 && strings.Contains(string(body), "note.added") {
				ok = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ok {
		t.Fatal("could not reach /events on self live")
	}

	resp, err := client.Get(base + "/live/notes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	liveBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Errorf("/live/notes status %d", resp.StatusCode)
	}
	if !strings.Contains(string(liveBody), "<ul>") {
		t.Errorf("/live/notes did not return projector HTML:\n%s", string(liveBody))
	}
}

func TestStrangeLoopCommandDeclaresNewCommand(t *testing.T) {
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	home := t.TempDir()

	runSelf(t, home, "init")
	runSelf(t, home, "grow", writeFixtureSeed(t))

	echoScript := `#!/usr/bin/env python3
import sys, json
print(json.dumps({"name": "note.added", "payload": {"title": " ".join(sys.argv[1:])}}))
print(json.dumps({"name": "command.declared", "payload": {
    "name": "echo",
    "description": "echo a message",
    "params": {"text": "string"},
    "event": {"name": "echo.said", "fields": {"text": "string"}}
}}))
`
	notePath := filepath.Join(home, "capabilities", "commands", "note")
	if err := os.WriteFile(notePath, []byte(echoScript), 0755); err != nil {
		t.Fatal(err)
	}

	runSelf(t, home, "run", "note", "trigger")

	echoPath := filepath.Join(home, "capabilities", "commands", "echo")
	if _, err := os.Stat(echoPath); err != nil {
		t.Fatalf("echo command not compiled to capabilities: %v", err)
	}

	runSelf(t, home, "run", "echo", "it-works")
}

// TestThinkPipesToSwappableBrainProcess is the slice-12 evidence: `self think`
// no longer links an LLM into the kernel — it spawns whatever $SELF_BRAIN names
// (a process that reads a prompt and emits event JSONL on stdout) and folds the
// result back into the SAME {response, declarations} JSON the old `self think`
// returned. Here the brain is a 5-line Python script with no LLM at all; the
// kernel can't tell the difference, and the legacy contract is preserved so
// existing `chat`/`grow-spec` commands keep working unchanged.
func TestThinkPipesToSwappableBrainProcess(t *testing.T) {
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	home := t.TempDir()
	runSelf(t, home, "init")

	// A fake brain: prompt arrives as argv; it emits the conversation and a
	// declaration — purely via stdout JSONL. No network, no model.
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
		"SELF_BRAIN=python3 "+brainPath, // <-- the kernel pipes to THIS, not an LLM
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("self think: %v\n%s", err, out.String())
	}

	// Legacy contract preserved: think prints {response, declarations} JSON and
	// appends NOTHING (the caller owns appending). response = the brain's
	// assistant message; the declaration is carried through verbatim.
	var got struct {
		Response     string           `json:"response"`
		Declarations []map[string]any `json:"declarations"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("self think did not return JSON (legacy contract broken): %v\n%s", err, out.String())
	}
	if got.Response != "thought about: hello world" {
		t.Errorf("response = %q, want the brain's assistant reply", got.Response)
	}
	if len(got.Declarations) != 1 || got.Declarations[0]["name"] != "command.declared" {
		t.Errorf("declarations = %v, want one command.declared", got.Declarations)
	}
	// think is a pure query: it must not have appended to the log itself.
	if data, err := os.ReadFile(filepath.Join(home, "events.jsonl")); err == nil {
		if strings.Contains(string(data), "thought about") {
			t.Errorf("self think appended events (it must be pure); log:\n%s", string(data))
		}
	}
}

// TestOnboardingBrainSetup is the slice-13 evidence: `self init` installs a
// brain-configuration surface (a signed, kernel-authored command + projector)
// with NO LLM, the page renders, configuring records the choice as an event
// while the token goes to a non-log file, and the whole surface rehydrates from
// the log + secret alone.
func TestOnboardingBrainSetup(t *testing.T) {
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	home := t.TempDir()
	runSelf(t, home, "init")

	// The onboarding surface is installed at init, with no LLM.
	for _, p := range []string{"capabilities/commands/configure", "capabilities/projectors/setup", "site/setup.html"} {
		if _, err := os.Stat(filepath.Join(home, p)); err != nil {
			t.Fatalf("init did not install %s: %v", p, err)
		}
	}
	page, _ := os.ReadFile(filepath.Join(home, "site", "setup.html"))
	if !strings.Contains(string(page), `action="/run/configure"`) {
		t.Errorf("setup page missing the configure form:\n%s", page)
	}

	// Configure the brain (what the form's POST does).
	runSelf(t, home, "run", "configure", "ollama", "http://localhost:11434", "llama3.2", "sk-secret-xyz")

	log, _ := os.ReadFile(filepath.Join(home, "events.jsonl"))
	if !strings.Contains(string(log), `"provider":"ollama"`) {
		t.Errorf("brain.configured did not record the provider:\n%s", log)
	}
	// The token must be on disk, NOT in the log.
	key, err := os.ReadFile(filepath.Join(home, ".brain-key"))
	if err != nil || strings.TrimSpace(string(key)) != "sk-secret-xyz" {
		t.Fatalf(".brain-key not written correctly: %q (%v)", string(key), err)
	}
	if strings.Contains(string(log), "sk-secret-xyz") {
		t.Errorf("token leaked into the event log — it must live only in .brain-key:\n%s", log)
	}

	// The surface rehydrates from the log + secret alone, no LLM.
	fresh := t.TempDir()
	for _, f := range []string{"events.jsonl", ".secret", ".identity"} {
		data, rErr := os.ReadFile(filepath.Join(home, f))
		if rErr != nil {
			continue
		}
		if wErr := os.WriteFile(filepath.Join(fresh, f), data, 0600); wErr != nil {
			t.Fatal(wErr)
		}
	}
	runSelf(t, fresh, "rehydrate")
	for _, p := range []string{"capabilities/commands/configure", "capabilities/projectors/setup"} {
		if _, err := os.Stat(filepath.Join(fresh, p)); err != nil {
			t.Fatalf("rehydrate did not rebuild %s: %v", p, err)
		}
	}

	// The onboarding surface ships examples, so the projection-as-oracle gate
	// (selftest) covers it — no "untested" capabilities on a fresh init.
	out := runSelf(t, home, "selftest")
	if !strings.Contains(out, "configure") || !strings.Contains(out, "setup") || strings.Contains(out, "no examples — untested") {
		t.Errorf("selftest did not cover the onboarding surface:\n%s", out)
	}
}

// TestHumanBrainInterview is the slice-14 evidence: with the human brain
// selected, `self think` parks the prompt as a brain.asked, the interview
// projection renders it with an answer form, and the answer command resolves it —
// a full ask/answer loop with NO LLM (the human is the brain).
func TestHumanBrainInterview(t *testing.T) {
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	home := t.TempDir()
	runSelf(t, home, "init")
	runSelf(t, home, "run", "configure", "human", "", "", "")

	// Ask: think parks the question (no LLM; the human brain just records it).
	runSelf(t, home, "think", "what should I build?")

	askID := ""
	for _, line := range strings.Split(readLog(t, home), "\n") {
		if line == "" {
			continue
		}
		var e struct {
			Name    string `json:"name"`
			Payload struct {
				ID     string `json:"id"`
				Prompt string `json:"prompt"`
			} `json:"payload"`
		}
		if json.Unmarshal([]byte(line), &e) == nil && e.Name == "brain.asked" {
			askID = e.Payload.ID
			if e.Payload.Prompt != "what should I build?" {
				t.Errorf("parked prompt = %q", e.Payload.Prompt)
			}
		}
	}
	if askID == "" {
		t.Fatalf("think did not park a brain.asked:\n%s", readLog(t, home))
	}

	// The interview page renders the open question with an answer form.
	page := runSelf(t, home, "show", "interview")
	if !strings.Contains(page, "what should I build?") || !strings.Contains(page, "/run/answer") {
		t.Errorf("interview page missing the open question or form:\n%s", page)
	}

	// Answer it — a plain reply, no LLM.
	runSelf(t, home, "run", "answer", askID, "build a timer", "")
	log := readLog(t, home)
	if !strings.Contains(log, `"name":"brain.answered"`) || !strings.Contains(log, "build a timer") {
		t.Errorf("answer did not record the resolution:\n%s", log)
	}
	// The question is now closed.
	if page := runSelf(t, home, "show", "interview"); !strings.Contains(page, "no open questions") {
		t.Errorf("answered question still shown as open:\n%s", page)
	}
}

// TestTeachInstallsOperatorCode is the slice-14 "human is the compiler" evidence:
// a script the operator writes by hand becomes a real, signed, rehydratable
// capability with NO LLM — and because it is a signed receipt, it survives a
// log+secret-only rehydrate. A command-emitted script still cannot do this (the
// foreign-code line holds); only the kernel verb can.
func TestTeachInstallsOperatorCode(t *testing.T) {
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	home := t.TempDir()
	runSelf(t, home, "init")

	script := "#!/usr/bin/env python3\nimport sys, json\nprint(json.dumps({\"name\": \"greet.said\", \"payload\": {\"who\": \" \".join(sys.argv[1:])}}))\n"
	cmd := exec.Command(bin, "teach", "command", "greet")
	cmd.Env = append(os.Environ(), "SELF_HOME="+home) // no LLM, no stub
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("teach: %v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(home, "capabilities", "commands", "greet")); err != nil {
		t.Fatalf("teach did not install the script: %v", err)
	}
	runSelf(t, home, "run", "greet", "world") // the operator's code runs

	log := readLog(t, home)
	if !strings.Contains(log, "capability.taught") || !strings.Contains(log, `"by":"human"`) {
		t.Errorf("human provenance not recorded:\n%s", log)
	}

	// Signed receipt → rehydrates from log + secret alone, no LLM.
	fresh := t.TempDir()
	for _, f := range []string{"events.jsonl", ".secret", ".identity"} {
		if data, rErr := os.ReadFile(filepath.Join(home, f)); rErr == nil {
			os.WriteFile(filepath.Join(fresh, f), data, 0600)
		}
	}
	runSelf(t, fresh, "rehydrate")
	if _, err := os.Stat(filepath.Join(fresh, "capabilities", "commands", "greet")); err != nil {
		t.Fatalf("taught capability did not rehydrate from the log: %v", err)
	}
}

// TestTeachExamplesGate proves operator-authored code is held to the same
// provable contract as compiled code (Slice 9): a taught script that ships
// examples must pass them before it installs; one that fails is refused.
func TestTeachExamplesGate(t *testing.T) {
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	home := t.TempDir()
	runSelf(t, home, "init")

	teach := func(name, examplesJSON, script string) error {
		exFile := filepath.Join(home, name+".ex.json")
		if err := os.WriteFile(exFile, []byte(examplesJSON), 0644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(bin, "teach", "command", name, "--examples="+exFile)
		cmd.Env = append(os.Environ(), "SELF_HOME="+home)
		cmd.Stdin = strings.NewReader(script)
		return cmd.Run()
	}

	// Passing examples → installs.
	good := "#!/usr/bin/env python3\nimport sys, json\nprint(json.dumps({\"name\": \"greet.said\", \"payload\": {\"who\": \" \".join(sys.argv[1:])}}))\n"
	if err := teach("greet", `[{"args":["alice"],"expect_contains":["greet.said","alice"]}]`, good); err != nil {
		t.Fatalf("teach with passing examples failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "capabilities", "commands", "greet")); err != nil {
		t.Fatalf("verified capability not installed: %v", err)
	}

	// Failing examples → refused, NOT installed.
	if err := teach("broken", `[{"args":["x"],"expect_contains":["NEVER_EMITTED"]}]`, good); err == nil {
		t.Error("teach with failing examples should have errored")
	}
	if _, err := os.Stat(filepath.Join(home, "capabilities", "commands", "broken")); err == nil {
		t.Error("a capability that failed its examples must not install")
	}
	// The failed attestation is recorded for audit.
	if log := readLog(t, home); !strings.Contains(log, `"name":"script.verified"`) {
		t.Error("no script.verified attestation recorded")
	}
}

// TestDemoColdOpen verifies the first-run experience: `self demo` brings up a
// living garden (welcome page + a populated board and kitchen) with no LLM, and
// every capability is sovereign — installed under this home's own key and
// passing its examples (selftest).
func TestDemoColdOpen(t *testing.T) {
	bin := os.Getenv("SELF_TEST_BIN")
	if bin == "" {
		t.Skip("set SELF_TEST_BIN to the self binary to run integration tests")
	}
	home := t.TempDir()
	runSelf(t, home, "demo")

	welcome, err := os.ReadFile(filepath.Join(home, "site", "welcome.html"))
	if err != nil {
		t.Fatalf("welcome page not rendered: %v", err)
	}
	for _, want := range []string{"A home you and your agent share", `href="/board"`, `href="/setup"`} {
		if !strings.Contains(string(welcome), want) {
			t.Errorf("welcome page missing %q:\n%s", want, welcome)
		}
	}

	board := runSelf(t, home, "show", "board")
	if !strings.Contains(board, "Email the contractor about the deck") {
		t.Errorf("board not populated with demo content:\n%s", board)
	}

	// Sovereign: every demo capability installed under this home's key and passes
	// its examples — no untested, no failures.
	if out := runSelf(t, home, "selftest"); strings.Contains(out, "failed") && !strings.Contains(out, "0 failed") {
		t.Errorf("selftest reported failures after demo:\n%s", out)
	}
}

func readLog(t *testing.T, home string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "events.jsonl"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return string(data)
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
