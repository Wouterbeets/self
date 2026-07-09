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
                      events, then re-renders all projections.

  projector script    stdin is the full event log JSONL; stdout is HTML.
                      The kernel writes it to SELF_HOME/site/<name>.html.

  environment         SELF_HOME is set for every compiled script.

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
		return "usage: self run <command> [args...]\n\nRun an installed command capability. Its emitted events are appended, then projections re-render.\n", true
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
