package seed

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStubCommandPureStdout(t *testing.T) {
	c := &Compiler{Stub: true}
	cmd := Command{
		Name:        "note",
		Description: "capture a note",
		Event:       EventDecl{Name: "note.captured"},
	}
	script, err := c.CompileCommand(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "sys.argv") {
		t.Error("stub command should read argv")
	}
	if strings.Contains(script, "self_common") {
		t.Error("stub command should not reference self_common")
	}
}

func TestStubProjectorPureStdout(t *testing.T) {
	c := &Compiler{Stub: true}
	p := ProjectorDecl{
		Name:     "notes",
		Consumes: []string{"note.captured"},
	}
	script, err := c.CompileProjector(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "sys.stdin") {
		t.Error("stub projector should read from stdin")
	}
	if strings.Contains(script, "self_common") {
		t.Error("stub projector should not reference self_common")
	}
	if strings.Contains(script, "write_site") {
		t.Error("stub projector should not call write_site — kernel manages persistence")
	}
}

func TestReferenceImplementationReachesPrompt(t *testing.T) {
	marker := "MARKER_ref_impl_42"
	cmd := Command{Name: "post", Description: "post", Event: EventDecl{Name: "wall.posted"},
		Implementation: "#!/usr/bin/env python3\n# " + marker + "\n"}
	if got := buildCommandPrompt(cmd, ""); !strings.Contains(got, marker) || !strings.Contains(got, "REFERENCE IMPLEMENTATION") {
		t.Errorf("command prompt missing reference implementation:\n%s", got)
	}
	if got := buildCommandPrompt(Command{Name: "x", Event: EventDecl{Name: "x.done"}}, ""); strings.Contains(got, "REFERENCE IMPLEMENTATION") {
		t.Error("no implementation should mean no reference block")
	}
	proj := ProjectorDecl{Name: "wall", Consumes: []string{"wall.posted"}, Implementation: "# " + marker}
	if got := buildProjectorPrompt(proj, ""); !strings.Contains(got, marker) {
		t.Errorf("projector prompt missing reference implementation:\n%s", got)
	}
	// The whole-seed intent is woven into the per-trio compile prompt when present.
	if got := buildCommandPrompt(Command{Name: "x", Event: EventDecl{Name: "x.done"}}, "INTENT_MARKER_99"); !strings.Contains(got, "INTENT_MARKER_99") || !strings.Contains(got, "--- INTENT ---") {
		t.Errorf("intent not woven into command prompt:\n%s", got)
	}
}

func TestKernelPrimerInEveryPrompt(t *testing.T) {
	// The strange loop and the brain's turn protocol must be in the model's mental
	// model BEFORE it explores — not reverse-engineered from the garden. Every
	// compile/brain prompt opens with the primer that states them.
	prompts := map[string]string{
		"orchestrator": OrchestratorSystemPrompt,
		"command":      CommandSystemPrompt,
		"projector":    ProjectorSystemPrompt,
		"brain":        BrainSystemPrompt,
	}
	must := []string{
		"STRANGE LOOP",          // the heart of the project
		"command.declared",      // declaring is creating
		"self think",            // how a capability reaches the brain
		"{role, content} turns", // the turn protocol chat needs
		"append-only event log", // state is events
	}
	for name, p := range prompts {
		for _, m := range must {
			if !strings.Contains(p, m) {
				t.Errorf("%s prompt is missing the mental-model phrase %q", name, m)
			}
		}
	}
}

func TestConversationTurns(t *testing.T) {
	// A plain prompt → one user turn (the unchanged `self think "..."` path).
	if ts := conversationTurns("hello"); len(ts) != 1 || ts[0]["role"] != "user" || ts[0]["content"] != "hello" {
		t.Errorf("plain prompt: got %v", ts)
	}
	// A JSON turns array → expanded turns, in order.
	j := `[{"role":"system","content":"you are self"},{"role":"user","content":"hi"}]`
	ts := conversationTurns(j)
	if len(ts) != 2 || ts[0]["role"] != "system" || ts[1]["content"] != "hi" {
		t.Errorf("turns: got %v", ts)
	}
	// A JSON array without roles is NOT a conversation — treat as a single prompt.
	if ts := conversationTurns(`[1,2,3]`); len(ts) != 1 || ts[0]["role"] != "user" {
		t.Errorf("non-turns array should be one user message: got %v", ts)
	}
}

func TestWriteCommandScript(t *testing.T) {
	dir := t.TempDir()
	if err := WriteCommandScript(dir, "test-cmd", "#!/usr/bin/env python3\nprint(1)\n"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "commands", "test-cmd")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("command script not written: %v", err)
	}
	if info.Mode()&0100 == 0 {
		t.Error("command script should be executable")
	}
}

func TestWriteProjectorScript(t *testing.T) {
	dir := t.TempDir()
	if err := WriteProjectorScript(dir, "test-proj", "#!/usr/bin/env python3\nprint(1)\n"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "projectors", "test-proj")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("projector script not written: %v", err)
	}
	if info.Mode()&0100 == 0 {
		t.Error("projector script should be executable")
	}
}

func TestLoadParsesProjectorDeclared(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"name":"command.declared","payload":{"name":"note","description":"capture","params":{},"event":{"name":"note.captured","fields":{}}}}
{"name":"projector.declared","payload":{"name":"notes","description":"render notes","consumes":["note.captured"]}}
{"name":"note.captured","payload":{"title":"hello"}}
`), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(m.Commands))
	}
	if m.Commands[0].Name != "note" {
		t.Errorf("command name = %q, want note", m.Commands[0].Name)
	}
	if m.Commands[0].Event.Name != "note.captured" {
		t.Errorf("event name = %q, want note.captured", m.Commands[0].Event.Name)
	}
	if len(m.Projectors) != 1 {
		t.Errorf("expected 1 projector, got %d", len(m.Projectors))
	}
	if m.Projectors[0].Name != "notes" {
		t.Errorf("projector name = %q, want notes", m.Projectors[0].Name)
	}
	if len(m.Projectors[0].Consumes) != 1 || m.Projectors[0].Consumes[0] != "note.captured" {
		t.Errorf("projector consumes = %v, want [note.captured]", m.Projectors[0].Consumes)
	}
}

func TestLoadOpencodeGoConfig(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	if _, ok := loadOpencodeGoConfig(authPath); ok {
		t.Fatal("missing file should return ok=false")
	}

	if err := os.WriteFile(authPath, []byte(`not json`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadOpencodeGoConfig(authPath); ok {
		t.Fatal("invalid json should return ok=false")
	}

	if err := os.WriteFile(authPath, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadOpencodeGoConfig(authPath); ok {
		t.Fatal("empty auth should return ok=false")
	}

	if err := os.WriteFile(authPath, []byte(`{
		"opencode-go": {"type": "api", "key": "sk-test-123"}
	}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, ok := loadOpencodeGoConfig(authPath)
	if !ok {
		t.Fatal("opencode-go entry should return ok=true")
	}
	if cfg.Key != "sk-test-123" {
		t.Errorf("key = %q, want sk-test-123", cfg.Key)
	}
	if cfg.URL != "https://opencode.ai/zen/go" {
		t.Errorf("url = %q, want https://opencode.ai/zen/go", cfg.URL)
	}
	if cfg.Model != "glm-5.2" {
		t.Errorf("model = %q, want glm-5.2", cfg.Model)
	}
}

func TestNewCompilerStubFlag(t *testing.T) {
	original := os.Environ()
	t.Cleanup(func() { resetEnv(original) })

	clearLLMEnv(t)
	os.Setenv("SELF_LLM_STUB", "1")
	c := NewCompiler("")
	if !c.Stub {
		t.Fatal("SELF_LLM_STUB=1 should set Stub=true")
	}
	if c.Available() {
		t.Fatal("stub compiler should not be available")
	}
}

func clearLLMEnv(t *testing.T) {
	for _, k := range []string{"SELF_LLM_URL", "SELF_LLM_API_KEY", "SELF_LLM_MODEL", "SELF_LLM_STUB", "XDG_DATA_HOME"} {
		os.Unsetenv(k)
	}
}

func resetEnv(original []string) {
	for _, k := range []string{"SELF_LLM_URL", "SELF_LLM_API_KEY", "SELF_LLM_MODEL", "SELF_LLM_STUB", "XDG_DATA_HOME"} {
		os.Unsetenv(k)
	}
	for _, kv := range original {
		for _, k := range []string{"SELF_LLM_URL", "SELF_LLM_API_KEY", "SELF_LLM_MODEL", "SELF_LLM_STUB", "XDG_DATA_HOME"} {
			if len(kv) > len(k)+1 && kv[:len(k)+1] == k+"=" {
				os.Setenv(k, kv[len(k)+1:])
			}
		}
	}
}

// writeAuthJSON writes an opencode auth.json with the given provider entries
// under a temp dir and points XDG_DATA_HOME at it, so NewCompiler's
// opencode-go auto-detection resolves against a controlled fixture instead of
// the developer's real ~/.local/share/opencode/auth.json.
func writeAuthJSON(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	authDir := filepath.Join(dir, "opencode")
	if err := os.MkdirAll(authDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	os.Setenv("XDG_DATA_HOME", dir)
}

func TestNewCompilerDefaultsToOpencodeGo(t *testing.T) {
	original := os.Environ()
	t.Cleanup(func() { resetEnv(original) })
	clearLLMEnv(t)
	writeAuthJSON(t, `{"opencode-go": {"type": "api", "key": "sk-test-123"}}`)

	c := NewCompiler("")
	if c.Stub {
		t.Fatal("should not be stub")
	}
	if c.URL != "https://opencode.ai/zen/go" {
		t.Errorf("URL = %q, want https://opencode.ai/zen/go", c.URL)
	}
	if c.Key != "sk-test-123" {
		t.Errorf("Key = %q, want sk-test-123", c.Key)
	}
	if c.Model != "glm-5.2" {
		t.Errorf("Model = %q, want glm-5.2", c.Model)
	}
	if c.fallback == nil {
		t.Fatal("fallback should be set when primary is opencode-go")
	}
	if c.fallback.URL != "http://127.0.0.1:8080" {
		t.Errorf("fallback URL = %q, want http://127.0.0.1:8080", c.fallback.URL)
	}
	if c.fallback.Model != "local" {
		t.Errorf("fallback Model = %q, want local", c.fallback.Model)
	}
	if !c.Available() {
		t.Error("opencode-go with a key should be available")
	}
}

func TestNewCompilerLocalWhenNoAuth(t *testing.T) {
	original := os.Environ()
	t.Cleanup(func() { resetEnv(original) })
	clearLLMEnv(t)
	// XDG_DATA_HOME points at an empty temp dir (set by clearLLMEnv unsetting
	// it would fall back to $HOME/.local/share; instead force an empty dir).
	dir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", dir)

	c := NewCompiler("")
	if c.URL != "http://127.0.0.1:8080" {
		t.Errorf("URL = %q, want http://127.0.0.1:8080", c.URL)
	}
	if c.Model != "local" {
		t.Errorf("Model = %q, want local", c.Model)
	}
	if c.fallback != nil {
		t.Errorf("fallback should be nil when primary is already local, got %+v", c.fallback)
	}
	if !c.Available() {
		t.Error("local llama-server should be available without a key")
	}
}

func TestLLMTimeoutDefaultAllowsSlowRemoteOrchestration(t *testing.T) {
	t.Setenv("SELF_LLM_TIMEOUT", "")
	if got := llmTimeout(); got != 10*time.Minute {
		t.Errorf("llmTimeout default = %s, want 10m", got)
	}
}

func TestLLMTimeoutOverride(t *testing.T) {
	t.Setenv("SELF_LLM_TIMEOUT", "30s")
	if got := llmTimeout(); got != 30*time.Second {
		t.Errorf("llmTimeout override = %s, want 30s", got)
	}
}

func TestCallBrainReturnsDeclarationsWhenFinalSummaryNeverArrives(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "declare",
									"arguments": `{"name":"command.declared","payload":{"name":"note","description":"capture a note","params":{},"event":{"name":"note.captured","fields":{}}}}`,
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := &Compiler{URL: server.URL, Key: "sk-test", Model: "glm-5.2", Home: t.TempDir()}
	res, err := c.CallBrain("declare a note capability", nil, nil)
	if err != nil {
		t.Fatalf("CallBrain should return collected declarations after tool-round exhaustion: %v", err)
	}
	if len(res.Declarations) != 1 {
		t.Fatalf("declarations = %d, want duplicate declare calls deduped to 1", len(res.Declarations))
	}
	if res.Response == "" {
		t.Error("response should summarize collected declarations")
	}
}

func TestCallBrainStopsAfterToolCallLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "tree",
									"arguments": `{"path":".","depth":1}`,
								},
							},
							{
								"id":   "call_2",
								"type": "function",
								"function": map[string]any{
									"name":      "latest_state",
									"arguments": `{}`,
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := &Compiler{URL: server.URL, Key: "sk-test", Model: "glm-5.2", Home: t.TempDir()}
	_, err := c.CallBrain("loop forever", nil, nil)
	if err == nil {
		t.Fatal("expected runaway tool loop to stop with an error")
	}
	if !strings.Contains(err.Error(), "stopped after 60 brain tool calls") {
		t.Fatalf("error = %v, want tool-call limit", err)
	}
}

func TestInspectionToolsDenyPrivateFiles(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".brain-key"), []byte("sk-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	c := &Compiler{Home: home}
	for _, tool := range []string{"read", "tree", "list", "search"} {
		t.Run(tool, func(t *testing.T) {
			args := `{"path":".brain-key"}`
			if tool == "search" {
				args = `{"pattern":"sk-","path":".brain-key"}`
			}
			out := c.executeInspectTool(tool, args)
			if !strings.Contains(out, "private kernel files") {
				t.Fatalf("%s output = %q, want private-file denial", tool, out)
			}
			if strings.Contains(out, "sk-secret") {
				t.Fatalf("%s leaked secret: %q", tool, out)
			}
		})
	}
}

func TestTreeReportsCountsAndOmissions(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "site"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.html", "b.html", "c.html"} {
		if err := os.WriteFile(filepath.Join(home, "site", name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	c := &Compiler{Home: home}
	out := c.inspectTree("site", 1, 2)
	if !strings.Contains(out, "site/ (3 entries)") || !strings.Contains(out, "limit reached") {
		t.Fatalf("tree output missing counts/limit:\n%s", out)
	}
}

func TestIsQuotaExceeded(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"zero-value http error", &llmHTTPError{Status: 0, Body: ""}, false},
		{"429", &llmHTTPError{Status: 429, Body: `{"error":"rate limited"}`}, true},
		{"402 payment required", &llmHTTPError{Status: 402, Body: "payment required"}, true},
		{"403 with quota body", &llmHTTPError{Status: 403, Body: `{"error":"quota exceeded"}`}, true},
		{"403 with exceeded body", &llmHTTPError{Status: 403, Body: "Monthly limit reached"}, true},
		{"500 server error", &llmHTTPError{Status: 500, Body: "internal error"}, false},
		{"400 bad request", &llmHTTPError{Status: 400, Body: "invalid model"}, false},
		{"non-http error", errPlain("some network failure"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isQuotaExceeded(tc.err); got != tc.want {
				t.Errorf("isQuotaExceeded(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

type errPlain string

func (e errPlain) Error() string { return string(e) }

// TestCallLLMFallsBackOnQuotaError stands up two httptest servers: a primary
// that refuses every request with HTTP 429 (quota exceeded) and a fallback
// that returns a valid submit_command tool call. callLLM should detect the
// quota error, switch to the fallback endpoint, and return the fallback's
// submitted script.
func TestCallLLMFallsBackOnQuotaError(t *testing.T) {
	primaryHits := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits++
		w.WriteHeader(429)
		w.Write([]byte(`{"error":"quota exceeded"}`))
	}))
	defer primary.Close()

	fallbackHits := 0
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits++
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "submit_command",
									"arguments": `{"command_script":"#!/usr/bin/env python3\nprint(\"from fallback\")"}`,
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer fallback.Close()

	c := &Compiler{
		URL:   primary.URL,
		Key:   "sk-test",
		Model: "glm-5.2",
		Home:  t.TempDir(),
		fallback: &llmEndpoint{
			URL:   fallback.URL,
			Key:   "",
			Model: "local",
		},
	}

	out, err := c.callLLM(CommandSystemPrompt, "compile a command", submitCommandTool)
	if err != nil {
		t.Fatalf("callLLM failed: %v", err)
	}
	if primaryHits == 0 {
		t.Error("primary endpoint should have been tried first")
	}
	if fallbackHits == 0 {
		t.Error("fallback endpoint should have been hit after quota error")
	}
	if !strings.Contains(out, "from fallback") {
		t.Errorf("expected the fallback's submitted script, got %q", out)
	}
}

// TestCallLLMNoFallbackWhenPrimaryIsLocal verifies that a quota error from a
// local-only compiler (no fallback configured) propagates instead of being
// swallowed or retried.
func TestCallLLMNoFallbackWhenPrimaryIsLocal(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":"rate limit exceeded"}`))
	}))
	defer primary.Close()

	c := &Compiler{URL: primary.URL, Model: "local"}
	// No fallback set: a local-ish URL would normally leave fallback nil, but
	// here the URL is 127.0.0.1 (httptest) yet we intentionally leave fallback
	// nil to assert the error path.
	_, err := c.callLLM(CommandSystemPrompt, "compile a command", submitCommandTool)
	if err == nil {
		t.Fatal("expected an error when no fallback is configured")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention 429, got %v", err)
	}
}
