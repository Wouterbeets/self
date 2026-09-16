package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestCommandOutputIsAnAtomicEventBatch(t *testing.T) {
	for _, tc := range []struct {
		name, output, exit string
		ok                 bool
	}{
		{"valid", `{"name":"note.added","payload":{"text":"hello"},"via":"forged","by":"forged","seq":999}`, "0", true},
		{"null", `{"name":"note.added","payload":null}`, "0", true},
		{"missing payload", `{"name":"note.added"}`, "0", false},
		{"invalid name", `{"name":"note","payload":{}}`, "0", false},
		{"prose", `not an event`, "0", false},
		{"quoted JSON", "`{\"name\":\"note.added\",\"payload\":{}}`", "0", false},
		{"authored script", `{"name":"script.authored","payload":{}}`, "0", false},
		{"failed producer", `{"name":"note.added","payload":{}}`, "7", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := home(t)
			script := "#!/bin/sh\ncat >/dev/null\ncat <<'EVENTS'\n" +
				"{\"name\":\"batch.started\",\"payload\":{}}\n\n" + tc.output + "\nEVENTS\nexit " + tc.exit + "\n"
			heard(t, h, line(t, "command.declared", decl{Name: "emit"})+
				line(t, "script.authored", authored{Type: kindCommand, Name: "emit", Script: script}))
			before, err := os.ReadFile(logPath(h))
			if err != nil {
				t.Fatal(err)
			}
			events, err := runCommand(h, replayed(t, h), "emit", nil, doorCLI, "test-caller")
			if !tc.ok {
				if err == nil {
					t.Fatal("invalid batch succeeded")
				}
				after, readErr := os.ReadFile(logPath(h))
				if readErr != nil {
					t.Fatal(readErr)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("failed command changed the log")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 2 {
				t.Fatalf("got %d events", len(events))
			}
			for _, e := range events {
				if e.Via != doorCLI || e.By != "test-caller" || e.Seq == 999 {
					t.Fatalf("forged provenance: %+v", e)
				}
			}
			if tc.name == "null" && strings.TrimSpace(string(events[1].Payload)) != "{}" {
				t.Fatal("null was not normalized")
			}
		})
	}
}

func TestViewOutputIsOpaqueAndFailureIsAnError(t *testing.T) {
	for _, tc := range []struct {
		name, script, want string
		fail               bool
	}{
		{"bytes", "#!/bin/sh\ncat >/dev/null\nprintf 'a\\000b\\377\\n'\n", "a\x00b\xff\n", false},
		{"failed", "#!/bin/sh\ncat >/dev/null\nprintf 'partial'\nexit 7\n", "", true},
		{"cannot start", "#!/no/such/interpreter\n", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := home(t)
			heard(t, h, line(t, "view.declared", decl{Name: "render"})+
				line(t, "script.authored", authored{Type: kindView, Name: "render", Script: tc.script}))
			before, err := os.ReadFile(logPath(h))
			if err != nil {
				t.Fatal(err)
			}
			out, err := runView(h, replayed(t, h), "render")
			if (err != nil) != tc.fail || string(out) != tc.want {
				t.Fatalf("output = %q, error = %v", out, err)
			}
			after, err := os.ReadFile(logPath(h))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("view changed the log: %v", err)
			}
		})
	}
}

// A successful no-op composes as silence, without inventing a log entry.
func TestSilentCommand(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "command.declared", decl{Name: "quiet"})+
		line(t, "script.authored", authored{Type: kindCommand, Name: "quiet", Script: "#!/bin/sh\ncat >/dev/null\n"}))
	before := len(replayed(t, h).Events)
	var out bytes.Buffer
	if err := dispatch(h, "run", []string{"quiet"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || len(replayed(t, h).Events) != before {
		t.Fatalf("silent command produced output or events: %q", out.String())
	}
}

type closedOutput struct{}

func (closedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// Reporting failure does not undo an event that the command already committed.
func TestCLIOutputFailure(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	for _, tc := range []struct {
		verb string
		args []string
	}{
		{"run", nil}, {"view", nil}, {"brief", nil}, {"help", nil},
		{"loop", []string{"--help"}},
		{"view", []string{"journal"}}, {"run", []string{"entry", "committed"}},
	} {
		if err := dispatch(h, tc.verb, tc.args, closedOutput{}); !errors.Is(err, io.ErrClosedPipe) {
			t.Errorf("%s %v: expected output failure, got %v", tc.verb, tc.args, err)
		}
	}
	page, err := runView(h, replayed(t, h), "journal")
	if err != nil || !strings.Contains(string(page), "committed") {
		t.Fatalf("reporting failure lost committed event: %q, %v", page, err)
	}
}

func TestCallerAttributionIsVerbatim(t *testing.T) {
	h := home(t)
	claim := "  caller with whitespace\t"
	t.Setenv("SELF_CALLER", claim)
	growJournal(t, h)
	if err := dispatch(h, "run", []string{"entry", "attributed"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	st := replayed(t, h)
	for _, e := range st.Events {
		if e.Via != doorKernel && e.By != claim {
			t.Fatalf("event attribution changed: %q", e.By)
		}
	}
	for _, c := range st.Caps {
		if c.Receipt == nil || c.Receipt.By != claim {
			t.Fatalf("signed author claim changed for %s", c.key())
		}
	}
}

// Commands and hear must apply the same kernel-event effects, while a failed
// producer still cannot commit any prefix or retire a capability.
func TestCommandRetirementUsesSharedIngestion(t *testing.T) {
	for _, tc := range []struct {
		name, tail string
		succeeds   bool
	}{
		{"success", "", true},
		{"failed producer", "exit 7\n", false},
		{"malformed suffix", "printf 'not an event\\n'\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := home(t)
			growJournal(t, h)
			script := "#!/bin/sh\ncat >/dev/null\ncat <<'EVENTS'\n" +
				line(t, "capability.retired", map[string]string{"type": "view", "name": "journal"}) + "EVENTS\n" + tc.tail
			heard(t, h, line(t, "command.declared", decl{Name: "remove-journal"})+
				line(t, "script.authored", authored{Type: kindCommand, Name: "remove-journal", Script: script}))
			before, err := os.ReadFile(logPath(h))
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			err = dispatch(h, "run", []string{"remove-journal"}, &out)
			if (err == nil) != tc.succeeds {
				t.Fatalf("unexpected result: %v", err)
			}
			_, linkErr := os.Lstat(linkPath(h, kindView, "journal"))
			if tc.succeeds {
				if !os.IsNotExist(linkErr) || replayed(t, h).cap(kindView, "journal") != nil {
					t.Fatal("committed retirement did not remove capability and link")
				}
				if !strings.Contains(out.String(), "capability.retired") || strings.Contains(out.String(), "heard ") {
					t.Fatalf("run output changed: %q", out.String())
				}
			} else {
				after, err := os.ReadFile(logPath(h))
				if err != nil {
					t.Fatal(err)
				}
				if linkErr != nil || !bytes.Equal(before, after) {
					t.Fatal("failed producer partially committed retirement")
				}
			}
		})
	}
}
