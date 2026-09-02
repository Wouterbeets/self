# self

`self` is a shell filter with a memory. It keeps one append-only event log, and
everything it can do is grown from that log by whatever mind you pipe beside it.

```sh
self "I want to track long-running goals here" | claude -p | self hear
```

The first `self` prints a situated prompt: what this instance is, what exists,
what is pending, what broke. It appends nothing. The mind answers on stdout with
events. `self hear` appends them. When the mind declares a capability and
authors its script in the same breath, the kernel signs and installs it, and it
is live on the next turn.

The kernel holds no model. A mind is any process that reads a prompt on stdin
and prints events on stdout: a frontier model, a local one, a shell script, a
person at a keyboard. They all stand in the same pipe and are told the same
thing.

## The one idea

The prompt does not describe a tool. It opens with: *you are this self, for a
bit. The mind ends; you do not. You persist as an append-only log, and only what
you append persists.*

Everything else here serves that sentence. A mind that reads the log as itself
preserves evidence instead of narration, grows a capability instead of
re-deriving the same answer every session, and leaves the human holding
something durable, inspectable and portable instead of a conversation that
evaporated. The wording lives in exactly one place, [`PROTOCOL.md`](PROTOCOL.md),
and every situated turn opens with it.

## Three laws

**Reads project. Writes append. Orientation is a read.** Looking at an instance
never changes it. The log grows only when something was meant to persist.

**Direction is structural, never sniffed.** An ask arrives as argv; a mind's
reply arrives on stdin, at `self hear`.

**A mind proposes; the kernel records.** Only an HMAC receipt that verifies
under `SELF_HOME/.secret` ever installs, and only for a capability this log
declared. Everything else in the log is inert data. `self rehydrate` rebuilds
the whole instance from `events.jsonl` + `.secret`, with no model and no
network. That is also the audit path and the migration path.

## Quick start

See the machinery with no model at all, in about fifteen seconds:

```sh
git clone https://github.com/wouterbeets/self && cd self
./demo.sh
```

`demo.sh` drives the whole thesis offline through `examples/mind-stub`, a
deterministic filter in the same seam a real mind occupies: an intent becomes
declarations and signed scripts, a command appends an event, a view replays it
twice to the same bytes, a hand-edited script is silently overwritten by its
receipt, a fresh body is rebuilt from two files byte-for-byte, and an account is
given, curated, and learned somewhere else with the edit visible in both logs.

Then put a real mind in the pipe:

```sh
go install .                                            # `self` on PATH
cd ~/somewhere
self learn ~/self/lessons/chat | claude -p | self hear  # grow a way to talk
self run say "what can you do?"
self view chat
```

The working directory is the instance: `events.jsonl`, `.secret` and `cap/`
appear beside it. Pin one shared instance with `export SELF_HOME=~/.self`. A new
home starts genuinely empty. Every capability is learned through the pipe and
leaves a signed receipt. There is no other install path.

## The loop

```sh
self loop -- claude -p
```

Each pass presents the same situated prompt. The mind discovers pending
capabilities and domain state through the brief and the views. The kernel
repeats after any append and stops after the first complete turn that leaves
the log unchanged. It never needs to know what a goal or a task means.

The mind is argv after `--`, executed directly. It inherits your working
directory and environment. `--ask <text>` directs the first pass only; later
passes are naked. `--max-passes` and `--timeout` bound it. `SELF_LOOP_MIND`
pins a mind so `self loop` needs no arguments. `self loop --help` has the rest.

## What an instance is

```
events.jsonl   the append-only log: the only authoritative state
.secret        the signing key (32 random bytes, mode 0600)
cap/           installed scripts, derived. Delete it; nothing is lost.
```

Two kinds of capability, and the difference is the trust boundary. The kernel
appends what a **command** prints and never appends what a **view** prints. A
command is told which instance it is acting on. A view is told nothing at all:
no `SELF_HOME`, an empty directory, its whole input on stdin, and it prints
opaque bytes. Text, JSON, HTML, SVG, whatever you want to read. Both are ordinary
executables in any language with a shebang, written by a mind and installed only
under a signature.

The whole contract is [`PROTOCOL.md`](PROTOCOL.md), which is also what
`self help` prints. Nothing else in this repository restates it.

## Accounts

You cannot transplant a skill and you cannot write into another mind's memory.
What one instance can do is **give an account**: a directory of plain text that
a receiver reads, curates, and learns from.

```
account/
  intent.md      the telling: who this is from, what it means (required)
  record.jsonl   the evidence: events verbatim, moments preserved (optional)
  manifest.json  the attestation: count + sha256 of the record (optional)
```

Nothing runnable ever travels. `self learn <dir>` deposits the record and prints
a learning prompt. The receiving mind reads the intent against local state and
declares its own capabilities, authored and signed locally. Same account, two
instances, two expressions. The `lessons/` directory holds three small accounts
to start from: `journal`, `chat`, and `memory`.

## For coding agents

[`AGENTS.md`](AGENTS.md) is one section to paste into a project's `CLAUDE.md`
or agent instructions. Sessions then share one memory that outlives them.
`make build` also produces `self-serve` and `self-browse`, sidecars that show
the same replayed bytes in a browser.

## Why

Frontier models are extraordinary at the novel and the general. What they do not
have is your history: the accumulated, contextual, hard-won practical knowledge
the Greeks called metis. `self` makes that a thing a person owns, in a format any
mind can read and no single provider mediates. The human keeps agency over what
is remembered, what is grown, and where it goes.

## Limits

Stated plainly, and at greater length in `self help`: there is no human review
step between authoring and signing. Piping a mind's output into `self hear`
signs whatever it authored. The log can contain untrusted input, and that input
becomes context for the mind writing your scripts. Nothing is sandboxed; the
environment is scrubbed for determinism, not containment. The log is unbounded.
Anyone who can run `self` against your `SELF_HOME` can install, which is the
power they already have holding the directory and its key.

Read a generated script before you trust it. What you inspect is readable intent
and readable output rather than an opaque binary. That is the advantage, not
safety.

## Status

Experimental. The claim under test: an append-only log, a signed install gate,
deterministic replay, and one shell pipe are enough of a kernel for a system
that writes, tests and revises its own capabilities while staying local-first
and wholly inspectable.

## License

Apache-2.0. The scripts your instance generates and the events in your log are
program output: yours, not derivatives of this runtime.
