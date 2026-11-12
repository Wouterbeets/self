package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomeDefaultsToTheUsersSelf(t *testing.T) {
	user, cwd := t.TempDir(), t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("SELF_HOME", "")
	t.Chdir(cwd)
	if got, want := homeDir(), filepath.Join(user, ".self"); got != want {
		t.Fatalf("home = %s, want %s", got, want)
	}
}

func TestHomeIsTheWorkingDirectoryThatHoldsALog(t *testing.T) {
	user, cwd := t.TempDir(), t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("SELF_HOME", "")
	t.Chdir(cwd)
	if err := os.WriteFile(filepath.Join(cwd, "events.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	here, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got := homeDir(); got != here {
		t.Fatalf("home = %s, want %s", got, here)
	}
}

func TestSelfHomeWinsOverEveryDefault(t *testing.T) {
	user, cwd, chosen := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("SELF_HOME", chosen)
	t.Chdir(cwd)
	if err := os.WriteFile(filepath.Join(cwd, "events.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := homeDir(); got != chosen {
		t.Fatalf("home = %s, want %s", got, chosen)
	}
}

// Setup is plumbing, so it is a read of its own and stays out of the brief.
func TestSetupIsAReadOutsideTheBrief(t *testing.T) {
	h := home(t)
	var page, brief bytes.Buffer
	if err := dispatch(h, "setup", nil, &page); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"AGENTS.md", "Before you finish, leave behind what it asks for.", "only once they agree"} {
		if !strings.Contains(page.String(), want) {
			t.Fatalf("self setup lacks %q:\n%s", want, page.String())
		}
	}
	if err := dispatch(h, "", nil, &brief); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(brief.String(), "`self setup`") || strings.Contains(brief.String(), "AGENTS.md") {
		t.Fatalf("the empty brief should point at setup, not carry it:\n%s", brief.String())
	}
	if entries, err := os.ReadDir(h); err != nil || len(entries) != 0 {
		t.Fatalf("self setup left files behind: %v %v", entries, err)
	}
}

// A new instance holds only what a mind left in it: the kernel declares nothing.
func TestTheFirstWriteIsTheWholeLog(t *testing.T) {
	h := home(t)
	heard(t, h, `{"name":"thing.happened","payload":{}}`)
	st := replayed(t, h)
	if len(st.Events) != 1 || len(st.Caps) != 0 {
		t.Fatalf("a new instance holds more than its first write: %d events, %d capabilities", len(st.Events), len(st.Caps))
	}
}

func TestFirstRunLayerShowsOnlyOnAnEmptyLog(t *testing.T) {
	h := home(t)
	first := protocolLayer("first")
	var empty bytes.Buffer
	if err := dispatch(h, "", nil, &empty); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(empty.String(), first) {
		t.Fatalf("an empty log did not carry the first-run layer:\n%s", empty.String())
	}
	if entries, err := os.ReadDir(h); err != nil || len(entries) != 0 {
		t.Fatalf("the first-run read left files behind: %v %v", entries, err)
	}
	if p := situated(t, h, "an ask"); strings.Contains(p, first) {
		t.Fatal("an execution prompt carries the first-run layer")
	}
	heard(t, h, `{"name":"journal.entry","payload":{"text":"hello"}}`)
	var grown bytes.Buffer
	if err := dispatch(h, "", nil, &grown); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(grown.String(), first) {
		t.Fatal("the first-run layer outlived the first event")
	}
}
