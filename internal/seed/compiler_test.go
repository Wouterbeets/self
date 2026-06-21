package seed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if strings.Contains(script, "ks_common") {
		t.Error("stub command should not reference ks_common")
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
	if strings.Contains(script, "ks_common") {
		t.Error("stub projector should not reference ks_common")
	}
	if strings.Contains(script, "write_site") {
		t.Error("stub projector should not call write_site — kernel manages persistence")
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
	os.Setenv("KS_LLM_STUB", "1")
	c := NewCompiler()
	if !c.Stub {
		t.Fatal("KS_LLM_STUB=1 should set Stub=true")
	}
	if c.Available() {
		t.Fatal("stub compiler should not be available")
	}
}

func clearLLMEnv(t *testing.T) {
	for _, k := range []string{"KS_LLM_URL", "KS_LLM_API_KEY", "KS_LLM_MODEL", "KS_LLM_STUB"} {
		os.Unsetenv(k)
	}
}

func resetEnv(original []string) {
	for _, k := range []string{"KS_LLM_URL", "KS_LLM_API_KEY", "KS_LLM_MODEL", "KS_LLM_STUB"} {
		os.Unsetenv(k)
	}
	for _, kv := range original {
		for _, k := range []string{"KS_LLM_URL", "KS_LLM_API_KEY", "KS_LLM_MODEL", "KS_LLM_STUB"} {
			if len(kv) > len(k)+1 && kv[:len(k)+1] == k+"=" {
				os.Setenv(k, kv[len(k)+1:])
			}
		}
	}
}
