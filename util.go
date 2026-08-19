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

// ensureHome initializes an instance exactly once, under the log lock — two
// selves starting concurrently in one pipeline must not both initialize.
func ensureHome(home string) error {
	if _, err := loadSecret(home); err != nil {
		return err
	}
	unlock, err := lockLog(home)
	if err != nil {
		return err
	}
	defer unlock()
	last, err := lastSeq(home)
	if err != nil || last > 0 {
		return err
	}
	e := newEvent("kernel.initialized", json.RawMessage(`{}`))
	e.Via, e.Seq = "kernel", 1
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(logPath(home), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, string(line)); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
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
	return `self — a local-first, event-sourced runtime; a filter with a memory

One append-only event log + projections as deterministic replays. The kernel
holds no model: the mind is whatever the shell pipes between two selves.

the loop:

  echo "whats going on today?" | self | claude -p | self

  prose in       → the situated prompt out (brief, conversation, pending
                   work, answer contract); the ask is recorded (self.asked)
  prose returned → recorded whole as one self.replied event and echoed
  pure JSONL in  → low-level capability wire: append/install, no mixed prose
  nothing in     → at a terminal, the orientation brief plus what is in
                   flight (pending scripts, unresolved rejections, waiting
                   chat); in a pipe, the work prompt (pending scripts, else
                   unresolved rejections, else one reflection) — so bare
                   'self | claude -p | self' is a self-improvement cycle

usage: self [command] [args]

  self                 the filter (see the loop above)
  self serve           rehydrate from the log, then serve at :7777
  self run <cmd> ...   run a capability — append events, refresh projections
  self show <name>     render a projection to stdout
  self learn <account> deposit an account's record (moments preserved) and
                       print its learning prompt — pipe it to a mind:
                       self learn <dir> | claude -p | self
  self give <sel> <dir>
                       write an account from the log — <sel> is an event-name
                       prefix ("note.") or command/<name> | projector/<name>;
                       intent + record + manifest land in <dir> for you to
                       curate before passing on
  self rehydrate       rebuild capabilities/ + site/ from the log's signed receipts (no LLM)
  self retire <target> retire a capability — its script and page leave the
                       surface; the log keeps every event, re-declaring revives
  self protocol        print the pipe + capability wire protocol

environment:
  SELF_HOME         the instance — a dir holding events.jsonl and .secret
                    (default: current working directory; set it in your shell rc
                    to pin a shared instance, e.g. export SELF_HOME=~/.self)
  SELF_CALLER       claimed speaker recorded verbatim as by on events your
                    invocations append (over HTTP the X-Self-Caller header
                    carries the claim); the door (via) is stamped by the
                    kernel itself and cannot be claimed
  SELF_MIND_ID      author by-line signed into script.compiled receipts when
                    the pipe installs an authored script (default: SELF_CALLER,
                    else "pipe")
`
}

func protocolText() string {
	return `self protocol — the wire contracts

The pipe (the one seam)

  self is a filter; the mind is whatever process the shell puts between two
  invocations of it:

      echo "<ask>" | self | <mind> | self

  ask face     stdin is prose. self records it (self.asked) and writes the
               situated prompt to stdout: the orientation brief (also at
               site/brief.md), the recent conversation, any pending work
               (declarations awaiting scripts), the ask, and the answer
               contract. A mind MUST be able to inspect files under SELF_HOME
               (site/*.html, events.jsonl, capabilities/) with its own tools —
               the prompt is a wake-up card, not a context dump. Coding-agent
               minds (claude -p, opencode run) plug in with no adapter.
  reply face   stdin is prose returning from a mind to a terminal. self records
               the complete body as one self.replied event and echoes it
               unchanged. Minds perform durable work with self run commands;
               their stdout is communication, not a multiplexed event stream.
  hear face    stdin is entirely event JSONL. Event lines append to the log (the
               kernel stamps id, seq, occurred_at, via "pipe", and by from
               SELF_CALLER); script.authored lines install under a locally
               signed receipt. Any prose line makes the whole body prose; mixed
               streams are never partially ingested. A
               script.authored the kernel refuses (empty script, undeclared or
               unknown capability) is recorded as a kernel-stamped
               script.rejected event — the failure is memory, not a terminal
               incident. A rejection stays open until a verified receipt or a
               capability.retired for that capability postdates it; open
               rejections ride the pending section (when the declaration still
               stands), become the work prompt (when it does not), and show
               under bare 'self' at a terminal.
  work face    stdin is empty (or a terminal with stdout piped). self emits
               the pending-compile prompt if declarations await scripts (each
               carrying the reason for any rejected previous attempt), else
               the unresolved-rejection prompt, else the waiting-chat ask,
               else one reflection — bare 'self | <mind> | self' converges.

Low-level wire events (pure JSONL only; ordinary minds use self run commands)

  command.declared    declare a command capability (pending until authored):
                      {"name":"command.declared","payload":{"name":"note","description":"...","params":{"text":"string"},"event":{"name":"note.added","fields":{"text":"string"}}}}

  projector.declared  declare a projection (pending until authored):
                      {"name":"projector.declared","payload":{"name":"notes","description":"...","consumes":["note.added"]}}

  script.authored     the script for a declared capability; installs under a
                      signed receipt, never lands in the log raw:
                      {"name":"script.authored","payload":{"type":"command","name":"note","script":"#!/bin/sh\n..."}}

  capability.retired  retire a capability: its script and page leave the derived
                      surface; the log keeps all history and a re-declaration
                      revives it:
                      {"name":"capability.retired","payload":{"type":"projector","name":"notes"}}

  anything else       a domain event, appended verbatim.

Compiled capability contract

  command script      argv are command args; stdin is the current event log JSONL;
                      stdout is new event JSONL: {"name":"event.name","payload":{...}}
                      the kernel assigns id, seq, occurred_at, and provenance —
                      via, the door the events entered through (cli, pipe,
                      http:<addr>, learn:<account>, kernel), stamped from what
                      the kernel witnessed and never accepted from a script; and by,
                      the caller's claimed identity (SELF_CALLER locally, the
                      X-Self-Caller header over HTTP), recorded verbatim as a
                      claim — then appends the events and re-renders the
                      projections that consume them.

  projector script    stdin is the events matching the projector's declared
                      consumes list, as JSONL (an empty list or "*" means every
                      event); stdout is HTML. The kernel writes it to
                      SELF_HOME/site/<name>.html.

  environment         SELF_HOME is set for every compiled script.

Accounts (give / learn)

  An account is a directory — the one wire format between instances:

    intent.md      the telling: who this is from, what it means (required)
    record.jsonl   the evidence: events verbatim, moments preserved (optional)
    manifest.json  the attestation: event count + sha256 of the record (optional)

  self give writes one from the log (an event-name prefix selects a record;
  command/<name> selects a capability's declarations and receipts). self learn
  reads one: the receiver's mind reads the intent — and the record, with its
  own tools — against local state and declares its own capabilities; the
  record is then deposited verbatim with its own occurred_at and by (the
  speaker travels with its testimony), never routed through the mind. The
  deposit's via is stamped learn:<account> — doors are local facts and are
  never inherited from another body's log. The kernel's own vocabulary (command.declared,
  script.compiled, capability.retired, …) never travels raw: give renames
  such events to lineage.<name> and learn refuses them otherwise — a foreign
  account carries history as evidence but cannot speak in the receiving
  kernel's voice, so a hostile account cannot install anything.
  lesson.learned records the sha256 of what was actually deposited beside the
  manifest's claim: editing an account before learning it (the receiver's
  intervention) is visible in both logs. Curation is file editing — the
  account is plain text.

Declarations — not code — are what cross every boundary between instances. A
script installs only after the local kernel signs a script.compiled receipt
with SELF_HOME/.secret; the author by-line inside it is SELF_MIND_ID (else
SELF_CALLER, else "pipe"), a claim covered by the signature.
`
}

func commandHelp(cmd string) (string, bool) {
	switch cmd {
	case "serve":
		return "usage: self serve\n\nRehydrate from the log, then serve the instance at 127.0.0.1:7777 (SELF_BIND overrides): every projection a live replay, every action a plain HTML form.\n", true
	case "learn":
		return "usage: self learn <account-dir>\n       self learn <account-dir> | claude -p | self\n\nDeposit <account-dir>/record.jsonl verbatim (moments and speakers preserved), record the intent, and print the learning prompt on stdout — pipe it to a mind to grow capabilities fitted to this instance. The kernel's own vocabulary is refused in a record — it travels only as lineage.* events, which land inert.\n", true
	case "give":
		return "usage: self give <event-prefix> <dir>\n       self give command/<name> <dir>\n       self give projector/<name> <dir>\n\nWrite an account from this log: the selected events verbatim in record.jsonl, a manifest with their count and sha256, and an intent.md stub to edit — who you are, what this means, what you hope it becomes. Kernel-vocabulary events are renamed lineage.* so they arrive as evidence, never as installables. The giving is remembered as an account.given event.\n", true
	case "run":
		return "usage: self run <command> [args...]\n\nRun an installed command capability. Its emitted events are appended, then the projections consuming them re-render.\n", true
	case "show":
		return "usage: self show <projection>\n\nRender a projection to stdout by replaying the current log. Use 'kernel' for the instance index.\n", true
	case "rehydrate":
		return "usage: self rehydrate\n\nRebuild capabilities/ and site/ from events.jsonl + .secret without a mind.\n", true
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
