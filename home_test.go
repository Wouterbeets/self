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
