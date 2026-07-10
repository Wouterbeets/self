package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func homeDir() string {
	if v := os.Getenv("SELF_HOME"); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return v
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

func ensureHome(home string) error {
	if _, err := loadSecret(home); err != nil {
		return err
	}
	events, err := readEvents(home)
	if err != nil || len(events) > 0 {
		return err
	}
	e := newEvent("kernel.initialized", json.RawMessage(`{}`))
	if err := appendEvent(home, &e); err != nil {
		return err
	}
	renderKernelHTML(home)
	renderBriefFile(home)
	fmt.Fprintf(os.Stderr, "self: new home %s\n", home)
	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, usageText())
}

func usageText() string {
	return `self — a local-first, event-sourced runtime with LLM-generated capabilities

One append-only event log + projections as deterministic replays. A minimal
kernel; every capability is generated from a declaration and installed under
a signed receipt.

usage: self [command] [args]

  self                 rehydrate the instance from the log, then serve it (the default)
  self grow <seed>     grow a seed's intent into capabilities (needs a brain)
  self run <cmd> ...   run a capability — append events, refresh projections
  self think "..."     ask the brain; returns {response, events} JSON
  self heartbeat       one self-improvement cycle (the brain reflects & grows)
  self show <name>     render a projection to stdout
  self rehydrate       rebuild capabilities/ + site/ from the log's signed receipts (no LLM)
  self share <cap>     print a seed to stdout — the capability's declarations and
                       receipts, a verbatim slice of this log
  self adopt <seed>    re-grow a shared capability here ("-" reads stdin) — this
                       instance's own compiler re-authors it; foreign bytes never install
  self export <prefix> <dir>
                       write a content seed: every <prefix>* event, its files,
                       and an editable intent — a directory another instance
                       can grow, dates preserved
  self revise <target> <request>
                       edit an installed local capability with its current script as context
  self retire <target> retire a capability — its script and page leave the
                       surface; the log keeps every event, re-declaring revives
  self protocol        print the brain + capability wire protocol

environment:
  SELF_HOME         the instance — a dir holding events.jsonl + .secret
                    (default: current working directory; set it in your shell rc
                    to pin a shared instance, e.g. export SELF_HOME=~/.self)

  plug a brain (one seam; think, heartbeat, grow, and compile all pass through it):
  SELF_BRAIN        a tool-capable executable, e.g. "claude -p" or
                    examples/brain-opencode — it gets the ask's kind in
                    $SELF_ASK, the prompt as its last argument, and an
                    orientation brief on stdin; it answers in event JSONL,
                    prose tolerated. The brain must inspect SELF_HOME itself
                    (site/*.html, events.jsonl, capabilities/) with its own
                    tools. See examples/README.md. examples/brain-stub is a
                    deterministic offline brain for demos/tests;
                    examples/brain-openai is a reference adapter that
                    illustrates the wire shape but is incomplete by spec
                    (no tool loop).
  SELF_BRAIN_ID     provenance by-line signed into script.compiled receipts
                    (default: the brain executable)
  SELF_THEME        default page design when serving: grove | micro | paper |
                    spec (default grove); a ?theme= link or the on-page picker
                    overrides it per viewer. Presentation only — never logged.
`
}

func protocolText() string {
	return `self protocol — the wire contracts

Brain process contract

  The same seam handles think, heartbeat, grow, and compile.

  SELF_BRAIN   executable to spawn, optionally with args. A brain MUST be able to
              inspect files under SELF_HOME (site/*.html, events.jsonl,
              capabilities/) with its own tools — a plain stdin/stdout adapter
              with no file access cannot do the job. Coding-agent brains
              (opencode run, claude -p) already have such tools.
  SELF_ASK     request kind: think | heartbeat | grow | compile
  argv         the prompt is passed as the last argument
  stdin        an orientation brief (plain text): where the brain is, what
               capabilities exist, and where to look for the rest. The brain is
               expected to explore SELF_HOME itself for depth — this is a
               wake-up card, not a context dump.
  stdout       event JSONL; non-JSON lines are tolerated as prose reply text

Brain reply events

  chat.message        prose reply for think:
                      {"name":"chat.message","payload":{"role":"assistant","content":"..."}}

  command.declared    declare a command capability; the kernel compiles it:
                      {"name":"command.declared","payload":{"name":"note","description":"...","params":{"text":"string"},"event":{"name":"note.added","fields":{"text":"string"}}}}

  projector.declared  declare a projection; the kernel compiles it:
                      {"name":"projector.declared","payload":{"name":"notes","description":"...","consumes":["note.added"]}}

  script.authored     answer to SELF_ASK=compile only:
                      {"name":"script.authored","payload":{"script":"#!/bin/sh\n..."}}

  capability.retired  retire a capability: its script and page leave the derived
                      surface; the log keeps all history and a re-declaration
                      revives it:
                      {"name":"capability.retired","payload":{"type":"projector","name":"notes"}}

Compiled capability contract

  command script      argv are command args; stdin is the current event log JSONL;
                      stdout is new event JSONL: {"name":"event.name","payload":{...}}
                      the kernel assigns id, seq, and occurred_at, appends the
                      events, then re-renders the projections that consume them.

  projector script    stdin is the events matching the projector's declared
                      consumes list, as JSONL (an empty list or "*" means every
                      event); stdout is HTML. The kernel writes it to
                      SELF_HOME/site/<name>.html.

  environment         SELF_HOME is set for every compiled script.

Files

  Bytes never ride in events. A file enters the store four ways — a browser
  form's file input (multipart POST /run/<command>), an @<path> arg to
  self run, a seed's files/ dir at grow time, or a command's own output (the
  command writes bytes to a scratch path and emits file.stored {name, path};
  the kernel copies them in, completes the payload, and verifies before
  appending) — and each deposit writes the
  blob to SELF_HOME/files/<sha256> and appends one file.stored event
  {name, mime, size, sha256}. The command behind the form or run receives the
  sha256 as the arg's value and resolves SELF_HOME/files/<sha256> itself when
  it needs the bytes. The server serves any blob at /files/<sha256> (or
  /files/<sha256>/<name> — the name is presentation, the hash is resolution),
  so a projector shows a file by linking its hash. Blobs are user content:
  rehydrate rebuilds scripts and pages from the log, but files/ must be backed
  up alongside events.jsonl and .secret.

Timers

  A timer.declared event {name, every, command, args} binds an installed
  command to a cadence; the serving kernel ticks it. every is a Go duration
  ("24h", "168h"); "off" disables; the latest declaration per name wins;
  capability.retired {type: "timer", name} retires. Each firing appends
  timer.fired before the command runs, and a command that errors leaves a
  timer.failed receipt. The bound command reads the log on stdin like any
  command run, so it decides what is actually due — the timer is the
  metronome, the log is the memory.

Effects

  Commands may act on the world (print, mail, call a local device);
  projectors never may. Every effect leaves a receipt event recording the
  attempt and the outcome, an effectful command consults the log first so
  it never repeats what already happened, and secrets stay in the
  environment or a config file — never in events or scripts.

Declarations cross instance boundaries; runnable code does not. A generated
script installs only after the local kernel signs a script.compiled receipt with
SELF_HOME/.secret and the current SELF_BRAIN_ID.
`
}

func commandHelp(cmd string) (string, bool) {
	switch cmd {
	case "grow":
		return "usage: self grow <seed-dir>\n\nRead <seed-dir>/intent.md, ask the brain to declare capabilities, compile them, and install signed receipts.\n", true
	case "run":
		return "usage: self run <command> [args...]\n\nRun an installed command capability. Its emitted events are appended, then the projections consuming them re-render. An arg spelled @<path> deposits that file into the store first (files/<sha256> + a file.stored event) and the command receives its sha256.\n", true
	case "think":
		return "usage: self think <prompt>\n       self think < prompt.txt\n\nAsk the brain through the SELF_BRAIN protocol. Prints {response, events} JSON and appends nothing.\n", true
	case "heartbeat":
		return "usage: self heartbeat\n\nAppend a heartbeat event, ask the brain for one small improvement, and compile any declarations it emits.\n", true
	case "show":
		return "usage: self show <projection>\n\nRender a projection to stdout by replaying the current log. Use 'kernel' for the instance index.\n", true
	case "rehydrate":
		return "usage: self rehydrate\n\nRebuild capabilities/ and site/ from events.jsonl + .secret without a brain.\n", true
	case "share":
		return "usage: self share <capability>\n\nPrint the capability's declarations and receipts as a JSONL seed.\n", true
	case "export":
		return "usage: self export <event-prefix> <dir> [<new-prefix>]\n\nWrite a content seed from this log: every event whose name starts with <event-prefix>, the file.stored metadata and blobs those events reference, and an intent.md stub to edit. Dates are preserved; an optional <new-prefix> renames the events on the way out (the sender-side remap for when two instances share a vocabulary), recorded in the seed's provenance. The receiver grows the directory and their brain decides how the records merge. The export is remembered as a seed.exported event.\n", true
	case "adopt":
		return "usage: self adopt <seed.jsonl>\n       self adopt - < seed.jsonl\n\nRecord a shared seed and re-generate its capability locally; foreign code never installs.\n", true
	case "revise":
		return "usage: self revise command/<name> <change request>\n       self revise projector/<name> <change request>\n\nRecord a local revision request, then recompile the installed capability with its latest declaration and verified script as context.\n", true
	case "retire":
		return "usage: self retire command/<name>\n       self retire projector/<name>\n\nAppend a capability.retired tombstone: the installed script (and a projector's page) come off disk, the brief and kernel index stop listing it, and rehydrate honors the tombstone. Events are never deleted — re-declaring the capability revives it.\n", true
	case "protocol":
		return protocolText(), true
	}
	return "", false
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" || arg == "help" {
			return true
		}
	}
	return false
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func jsonRepr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// jsonField pulls one top-level string field out of a raw JSON payload.
func jsonField(payload json.RawMessage, key string) string {
	var m map[string]any
	json.Unmarshal(payload, &m)
	s, _ := m[key].(string)
	return s
}
