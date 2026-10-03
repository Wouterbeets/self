package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func trees(t *testing.T, h string) {
	t.Helper()
	for _, d := range []string{"view", "bin"} {
		if err := os.MkdirAll(filepath.Join(h, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

// view/<name> holds what `self view <name>` prints, until the view is retired.
func TestProjectionMatchesViewsAndRetires(t *testing.T) {
	h := home(t)
	trees(t, h)
	growJournal(t, h)
	if _, err := runCommand(h, replayed(t, h), "entry", []string{"hello"}, doorCLI, ""); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"journal", "log"} {
		want, err := runView(h, replayed(t, h), name)
		got, _ := os.ReadFile(filepath.Join(h, "view", name))
		if err != nil || string(got) != string(want) || len(got) == 0 {
			t.Fatalf("view/%s = %q, self view prints %q (%v)", name, got, want, err)
		}
	}
	shim, _ := os.ReadFile(filepath.Join(h, "bin", "entry"))
	if !strings.Contains(string(shim), "SELF_HOME='"+h+"' exec self run 'entry' \"$@\"") {
		t.Fatalf("shim: %q", shim)
	}
	heard(t, h, line(t, "capability.retired", map[string]string{"type": "view", "name": "journal"})+
		line(t, "capability.retired", map[string]string{"type": "command", "name": "entry"}))
	for _, p := range []string{"view/journal", "bin/entry"} {
		if _, err := os.Stat(filepath.Join(h, p)); !os.IsNotExist(err) {
			t.Fatalf("%s survived retirement: %v", p, err)
		}
	}
}

func TestProjectionIsOptIn(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	for _, d := range []string{"view", "bin"} {
		if _, err := os.Stat(filepath.Join(h, d)); !os.IsNotExist(err) {
			t.Fatalf("%s/ appeared uninvited: %v", d, err)
		}
	}
}

// Only views the new events could change rerun; identical bytes keep their mtime.
func TestProjectionRerunsOnlyAffectedViews(t *testing.T) {
	h := home(t)
	trees(t, h)
	runs := filepath.Join(t.TempDir(), "runs")
	t.Setenv("SELF_TEST_RUNS", runs)
	growJournal(t, h)
	heard(t, h, line(t, "view.declared", decl{Name: "other", Description: "d", Consumes: []string{"other.thing"}})+
		line(t, "script.authored", authored{Type: "view", Name: "other", Script: "#!/bin/sh\necho run >>\"$SELF_TEST_RUNS\"\n"})+
		line(t, "view.declared", decl{Name: "same", Description: "d"})+
		line(t, "script.authored", authored{Type: "view", Name: "same", Script: "#!/bin/sh\necho same\n"}))
	old := time.Unix(1e9, 0)
	same := filepath.Join(h, "view", "same")
	if err := os.Chtimes(same, old, old); err != nil {
		t.Fatal(err)
	}
	count := func() int { b, _ := os.ReadFile(runs); return strings.Count(string(b), "run") }
	runCommand(h, replayed(t, h), "entry", []string{"x"}, doorCLI, "")
	if n := count(); n != 1 {
		t.Fatalf("other ran %d times; want once, at install", n)
	}
	if fi, err := os.Stat(same); err != nil || !fi.ModTime().Equal(old) {
		t.Fatalf("unchanged output was rewritten: %v", err)
	}
	heard(t, h, line(t, "other.thing", map[string]any{}))
	if n := count(); n != 2 {
		t.Fatalf("other ran %d times after its event; want 2", n)
	}
	os.Remove(same)
	runCommand(h, replayed(t, h), "entry", []string{"y"}, doorCLI, "")
	if got, _ := os.ReadFile(same); string(got) != "same\n" {
		t.Fatalf("a missing view file was not restored: %q", got)
	}
}

func TestShimRunsCommandAndRehydrateRebuilds(t *testing.T) {
	h := filepath.Join(t.TempDir(), "o'self")
	t.Setenv("SELF_HOME", h)
	t.Setenv("SELF_CALLER", "")
	trees(t, h)
	growJournal(t, h)
	tools := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(tools, "self"), ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(filepath.Join(h, "bin", "entry"), "via", "shim")
	cmd.Env = append(os.Environ(), "PATH="+tools+":"+os.Getenv("PATH"), "SELF_HOME=/nowhere")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shim: %v\n%s", err, out)
	}
	journal := filepath.Join(h, "view", "journal")
	if got, _ := os.ReadFile(journal); string(got) != "- via shim\n" {
		t.Fatalf("view/journal after the shim: %q", got)
	}
	os.WriteFile(journal, []byte("hand edit"), 0644)
	os.Remove(filepath.Join(h, "bin", "entry"))
	os.WriteFile(filepath.Join(h, "view", "stale"), nil, 0644)
	if err := rehydrate(h); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(journal); string(got) != "- via shim\n" {
		t.Fatalf("rehydrate left view/journal %q", got)
	}
	for p, want := range map[string]bool{"bin/entry": true, "view/stale": false} {
		if _, err := os.Stat(filepath.Join(h, p)); (err == nil) != want {
			t.Fatalf("after rehydrate %s exists=%v, want %v", p, err == nil, want)
		}
	}
}
