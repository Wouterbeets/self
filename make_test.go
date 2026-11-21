package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	for _, k := range []string{"SELF_CALLER", "AI_AGENT", "CLAUDECODE"} {
		os.Unsetenv(k)
	}
	os.Exit(m.Run())
}

func made(t *testing.T, h, script string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := cmdMake(h, args, strings.NewReader(script), &out); err != nil {
		t.Fatalf("self make %v: %v", args, err)
	}
	return out.String()
}

func TestMakeDeclaresAndInstallsInOneStep(t *testing.T) {
	h := home(t)
	t.Setenv("SELF_CALLER", "tester")
	out := made(t, h, "#!/bin/sh\ncat >/dev/null\necho hello from the command\n", "command", "greet", "say", "hello")
	if !strings.Contains(out, "self run greet") {
		t.Fatalf("make did not say how to try what it made: %q", out)
	}
	st := replayed(t, h)
	c := st.cap(kindCommand, "greet")
	if c == nil || c.Pending() || c.Decl.Summary != "say hello" || c.Receipt.By != "tester" {
		t.Fatalf("make left no installed, attributed capability: %+v", c)
	}
	var shown bytes.Buffer
	if err := dispatch(h, "run", []string{"greet"}, &shown); err != nil {
		t.Fatal(err)
	}
	if shown.String() != "hello from the command\n" {
		t.Fatalf("a command's plain output did not reach its caller: %q", shown.String())
	}
	if len(replayed(t, h).Events) != len(st.Events) {
		t.Fatal("plain output was appended to the log")
	}
}

func TestMadeViewReadsWhatAMadeCommandRecords(t *testing.T) {
	h := home(t)
	made(t, h, "#!/bin/sh\ncat >/dev/null\necho kept\nprintf '{\"name\":\"fact.learned\",\"payload\":{\"text\":\"%s\"}}\\n' \"$*\"\n",
		"command", "learned", "keep one fact")
	made(t, h, "#!/bin/sh\ngrep -c fact.learned\n", "view", "facts", "--consumes", "fact.learned", "--brief", "how many facts")
	var ran bytes.Buffer
	if err := dispatch(h, "run", []string{"learned", "the", "print", "profile", "fails"}, &ran); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ran.String(), "kept\n") || !strings.Contains(ran.String(), "fact.learned") {
		t.Fatalf("run should show the command's words and the event it committed: %q", ran.String())
	}
	st := replayed(t, h)
	v := st.cap(kindView, "facts")
	if v == nil || v.Decl.Brief == nil || len(v.Decl.Consumes) != 1 || v.Decl.Summary != "how many facts" {
		t.Fatalf("make lost the view's flags: %+v", v)
	}
	var arrival bytes.Buffer
	if err := dispatch(h, "", nil, &arrival); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(arrival.String(), "## facts") || !strings.Contains(arrival.String(), "\n1\n") {
		t.Fatalf("the made view did not show on arrival:\n%s", arrival.String())
	}
}

func TestMakeForgivesAndExplains(t *testing.T) {
	h := home(t)
	made(t, h, "cat >/dev/null\necho plain\n", "command", "plain", "print a word")
	if s := replayed(t, h).cap(kindCommand, "plain").Receipt.Script; !strings.HasPrefix(s, "#!/bin/sh\n") {
		t.Fatalf("a script without a shebang was not made a shell script: %q", s)
	}
	made(t, h, "#!/bin/sh\ncat >/dev/null\necho second\n", "command", "plain", "print another word")
	var shown bytes.Buffer
	if err := dispatch(h, "run", []string{"plain"}, &shown); err != nil || shown.String() != "second\n" {
		t.Fatalf("remaking did not replace the script: %q %v", shown.String(), err)
	}
	before := len(replayed(t, h).Events)
	for _, tc := range []struct {
		script string
		args   []string
		want   string
	}{
		{"echo x\n", []string{"gadget", "x", "y"}, "usage: self make"},
		{"echo x\n", []string{"command", ".hidden", "y"}, "cannot name"},
		{"echo x\n", []string{"command", "nosummary"}, "what nosummary is for"},
		{"  \n", []string{"command", "empty", "nothing"}, "no script arrived"},
		{"echo x\n", []string{"command", "c", "--brief", "y"}, "--brief is for views"},
	} {
		err := cmdMake(h, tc.args, strings.NewReader(tc.script), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("self make %v: got %v, want an error saying %q", tc.args, err, tc.want)
		}
	}
	if len(replayed(t, h).Events) != before {
		t.Fatal("a refused make wrote to the log")
	}
	var how bytes.Buffer
	if err := cmdMake(h, nil, strings.NewReader(""), &how); err != nil || !strings.Contains(how.String(), protocolLayer("make")) {
		t.Fatalf("bare make did not show how: %v", err)
	}
}

func TestBriefShowsWhoMadeWhat(t *testing.T) {
	h := home(t)
	t.Setenv("SELF_CALLER", "claude-code")
	made(t, h, "#!/bin/sh\ncat >/dev/null\nprintf '{\"name\":\"fact.learned\",\"payload\":{}}\\n'\n", "command", "learned", "keep one fact")
	for range 2 {
		if err := dispatch(h, "run", []string{"learned"}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	if b := brief(h, replayed(t, h)); !strings.Contains(b, "keep one fact  (by you, just now · 2 events since)") {
		t.Fatalf("the brief does not credit its caller:\n%s", b)
	}
	t.Setenv("SELF_CALLER", "opencode")
	if b := brief(h, replayed(t, h)); !strings.Contains(b, "(by claude-code, just now · 2 events since)") {
		t.Fatalf("the brief does not name another maker:\n%s", b)
	}
}

func TestCallerIsTheHarnessWhenItSaysSo(t *testing.T) {
	if callerClaim() != "" {
		t.Fatalf("an unannounced caller was named %q", callerClaim())
	}
	t.Setenv("CLAUDECODE", "1")
	if callerClaim() != "claude-code" {
		t.Fatalf("caller = %q", callerClaim())
	}
	t.Setenv("AI_AGENT", "some-agent_2-1-330_agent")
	if callerClaim() != "some-agent" {
		t.Fatalf("caller = %q", callerClaim())
	}
	t.Setenv("SELF_CALLER", "named on purpose")
	if callerClaim() != "named on purpose" {
		t.Fatalf("caller = %q", callerClaim())
	}
}
