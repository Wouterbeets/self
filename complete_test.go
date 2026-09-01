package main

// Completion is a read with a shell attached: it must know what the kernel
// knows (verbs, capability names), delegate what only the log knows to a
// grown complete.<name> view, and degrade to silence — never to an error a
// prompt line would have to display.

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func completed(t *testing.T, h string, words ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := dispatch(h, "__complete", words, &out); err != nil {
		t.Fatalf("__complete %v: %v", words, err)
	}
	return out.String()
}

// candidates strips the descriptions: the shims complete on column one.
func candidates(s string) []string {
	var names []string
	for l := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		if l == "" {
			continue
		}
		names = append(names, strings.SplitN(l, "\t", 2)[0])
	}
	return names
}

func TestCompleteVerbs(t *testing.T) {
	h := home(t)
	all := candidates(completed(t, h, ""))
	for _, v := range []string{"hear", "brief", "run", "view", "loop", "learn", "give", "rehydrate", "completion", "help"} {
		if !contains(all, v) {
			t.Fatalf("verb %q missing from %v", v, all)
		}
	}
	if got := candidates(completed(t, h, "vi")); len(got) != 1 || got[0] != "view" {
		t.Fatalf("prefix vi: want [view], got %v", got)
	}
	// __complete itself is machinery, not a verb to offer.
	if contains(all, "__complete") {
		t.Fatal("__complete offered as a candidate")
	}
}

func TestCompleteCapabilityNames(t *testing.T) {
	h := home(t)
	growJournal(t, h)

	views := candidates(completed(t, h, "view", ""))
	if !contains(views, "journal") || !contains(views, "log") {
		t.Fatalf("view names: want journal and built-in log, got %v", views)
	}
	if got := candidates(completed(t, h, "view", "jo")); len(got) != 1 || got[0] != "journal" {
		t.Fatalf("prefix jo: want [journal], got %v", got)
	}
	if cmds := candidates(completed(t, h, "run", "")); !contains(cmds, "entry") || contains(cmds, "journal") {
		t.Fatalf("run names: want entry only, got %v", cmds)
	}
}

// A pending capability is offered, annotated: the tab key is a status surface.
func TestCompletePendingAnnotated(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "view.declared", decl{Name: "census", Description: "counts"}))
	out := completed(t, h, "view", "cen")
	if !strings.Contains(out, "census") || !strings.Contains(out, "pending") {
		t.Fatalf("pending view not annotated: %q", out)
	}
}

// Name completion is a pure read: a tab-press must not leave files behind.
func TestCompleteNamesNeverWrite(t *testing.T) {
	h := home(t)
	completed(t, h, "")
	completed(t, h, "view", "")
	completed(t, h, "give", "")
	after, _ := os.ReadDir(h)
	if len(after) != 0 {
		names := []string{}
		for _, e := range after {
			names = append(names, e.Name())
		}
		t.Fatalf("completion created files: %v", names)
	}
}

// The seam: arguments belong to the domain, so the kernel hands them to a
// view named complete.<name> — grown like any other capability — and passes
// the words through verbatim.
func TestCompleteDelegatesToGrownView(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	body := line(t, "view.declared", decl{Name: "complete.journal", Description: "candidates for journal args", Consumes: []string{"journal.entry"}}) +
		line(t, "script.authored", authored{Type: "view", Name: "complete.journal",
			Script: "#!/bin/sh\nsed -n 's/.*\"text\":\"\\([^\"]*\\)\".*/\\1/p'\nprintf 'argv:%s\\n' \"$*\"\n"}) +
		line(t, "journal.entry", map[string]string{"text": "goal-alpha"}) +
		line(t, "journal.entry", map[string]string{"text": "goal-beta"})
	if report := heard(t, h, body); !strings.Contains(report, "installed view/complete.journal") {
		t.Fatalf("completer did not install: %s", report)
	}

	out := completed(t, h, "view", "journal", "goal-")
	if !strings.Contains(out, "goal-alpha") || !strings.Contains(out, "goal-beta") {
		t.Fatalf("completer candidates missing: %q", out)
	}
	if !strings.Contains(out, "argv:view journal goal-") {
		t.Fatalf("completer did not receive the words verbatim: %q", out)
	}

	// No completer for a capability means silence, not an error.
	if out := completed(t, h, "run", "entry", ""); out != "" {
		t.Fatalf("argument position without a completer: want silence, got %q", out)
	}
}

// A completer that hangs must cost one deadline and produce silence: the
// alternative is a frozen shell on every tab-press.
func TestCompleterTimeout(t *testing.T) {
	h := home(t)
	prev := completerTimeout
	completerTimeout = 100 * time.Millisecond
	defer func() { completerTimeout = prev }()

	body := line(t, "view.declared", decl{Name: "s", Description: "slow"}) +
		line(t, "script.authored", authored{Type: "view", Name: "s",
			Script: "#!/bin/sh\ncat >/dev/null\nsleep 5\necho too-late\n"}) +
		line(t, "view.declared", decl{Name: "complete.s", Description: "slow completer"}) +
		line(t, "script.authored", authored{Type: "view", Name: "complete.s",
			Script: "#!/bin/sh\ncat >/dev/null\nsleep 5\necho too-late\n"})
	heard(t, h, body)

	start := time.Now()
	if out := completed(t, h, "view", "s", ""); out != "" {
		t.Fatalf("hung completer: want silence, got %q", out)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("completion did not respect the deadline")
	}
}

func TestCompleteGiveSelectors(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	heard(t, h, line(t, "journal.entry", map[string]string{"text": "x"}))

	sels := candidates(completed(t, h, "give", ""))
	for _, want := range []string{"command/entry", "view/journal", "journal.entry"} {
		if !contains(sels, want) {
			t.Fatalf("give selector %q missing from %v", want, sels)
		}
	}
	if got := candidates(completed(t, h, "give", "view/")); !contains(got, "view/journal") || contains(got, "command/entry") {
		t.Fatalf("prefix view/: got %v", got)
	}
}

func TestCompletionScripts(t *testing.T) {
	h := home(t)
	for shell, marker := range map[string]string{
		"zsh":  "#compdef self",
		"bash": "complete -o default -F _self_complete self",
		"fish": "complete -c self",
	} {
		var out bytes.Buffer
		if err := dispatch(h, "completion", []string{shell}, &out); err != nil {
			t.Fatalf("completion %s: %v", shell, err)
		}
		if !strings.Contains(out.String(), marker) {
			t.Fatalf("completion %s: marker %q missing", shell, marker)
		}
		if !strings.Contains(out.String(), "__complete") {
			t.Fatalf("completion %s does not call the machine face", shell)
		}
	}
	var out bytes.Buffer
	if err := dispatch(h, "completion", []string{"tcsh"}, &out); err == nil {
		t.Fatal("unknown shell: want an error")
	}
}

func contains(list []string, s string) bool { return slices.Contains(list, s) }
