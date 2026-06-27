package seed

import (
	"encoding/json"
	"testing"
)

func raw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestVerifyScriptPassesAndFails(t *testing.T) {
	// A script that echoes its JSONL stdin back. Its output therefore contains
	// whatever was fed in.
	echo := "#!/bin/sh\ncat\n"
	ev := raw(map[string]any{"name": "observation.logged", "payload": map[string]any{"where": "Alpha"}})

	// passes: the substring is present in the (echoed) input
	pass := []Example{{Note: "has Alpha", Events: []json.RawMessage{ev}, ExpectContains: []string{"Alpha"}}}
	res, err := VerifyScript(echo, "projector", pass)
	if err != nil {
		t.Fatalf("VerifyScript: %v", err)
	}
	if !res.OK() || res.Passed != 1 || res.Ran != 1 {
		t.Fatalf("expected 1/1 pass, got %+v", res)
	}

	// fails: the substring is absent
	fail := []Example{{Note: "wants Zeta", Events: []json.RawMessage{ev}, ExpectContains: []string{"Zeta"}}}
	res, _ = VerifyScript(echo, "projector", fail)
	if res.OK() || res.Passed != 0 || len(res.Failures) != 1 {
		t.Fatalf("expected a failure, got %+v", res)
	}
}

func TestVerifyScriptFeedsArgv(t *testing.T) {
	// A command-style script that echoes its first argv.
	echoArg := "#!/bin/sh\nprintf '%s' \"$1\"\n"
	ex := []Example{{Args: []string{"hello-arg"}, ExpectContains: []string{"hello-arg"}}}
	res, err := VerifyScript(echoArg, "command", ex)
	if err != nil {
		t.Fatalf("VerifyScript: %v", err)
	}
	if !res.OK() {
		t.Fatalf("argv not passed to script: %+v", res)
	}
}

func TestVerifyScriptCountsBrokenScriptAsFailure(t *testing.T) {
	broken := "#!/bin/sh\necho oops 1>&2\nexit 1\n"
	ex := []Example{{ExpectContains: []string{"anything"}}}
	res, err := VerifyScript(broken, "command", ex)
	if err != nil {
		t.Fatalf("VerifyScript should not error on a failing script: %v", err)
	}
	if res.OK() || res.Passed != 0 || len(res.Failures) != 1 {
		t.Fatalf("expected the broken script to fail verification, got %+v", res)
	}
}

func TestVerifyScriptNoExamples(t *testing.T) {
	res, err := VerifyScript("#!/bin/sh\n:\n", "command", nil)
	if err != nil {
		t.Fatalf("VerifyScript: %v", err)
	}
	if res.Ran != 0 || res.OK() {
		t.Fatalf("no examples should run nothing, got %+v", res)
	}
}
