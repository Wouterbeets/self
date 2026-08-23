# self

`self` is a local-first runtime that grows its own capabilities, and to the
shell it is a filter with a memory. One append-only event log is its only
authoritative state. Every capability and every view is a deterministic replay
of that log. The kernel holds no model and spawns nothing: intelligence enters
through a shell pipe, where the mind is whatever process you put beside it.

```sh
self "I want to track long-running goals here" | claude -p | self hear
```

The first `self` situates the ask against the instance's own state — what
exists, what is pending, what broke — and **appends nothing**. The mind does
durable work through installed commands and prints events. `self hear` lands
them: ordinary events are appended, and a script a mind authored installs under
a receipt the kernel signs with a key only it holds. A declaration without a
script stays pending and rides the next prompt, so the loop converges.

That is the strange loop, and it is one shell idiom:

```sh
while ask=$(self); do printf '%s\n' "$ask" | claude -p | self hear; done
```

It stops on its own: bare `self` exits 3 when nothing is pending. Stack a line
of work when the next pass should see it (`self work "analyse the metrics view"`);
watch a condition with a view when the loop should keep going until the world
looks right (`while [ -n "$(self view goals)" ]`).

## Three laws

**Reads project. Writes append. Orientation is a read.** Looking at an instance
never changes it. The log grows only when something was meant to persist.

**Direction is structural, never sniffed.** An ask arrives as argv; what comes
back from a mind arrives on stdin, at `self hear`. Prose alone cannot tell those
apart — "what is going on?" and a mind's answer to it are both prose — so the
kernel does not guess. It does not look at your terminal, and it behaves
identically in a pipe, a script, a sandbox and cron.

**A mind proposes; the kernel records.** Only an HMAC receipt that verifies
under `SELF_HOME/.secret` ever installs, and only for a capability this log
declared. Everything else in the log is inert data. `self rehydrate` rebuilds
the whole instance from `events.jsonl` + `.secret`, with no model and no
network — which is also the audit path and the migration path.

## Quick start

See the machinery with no model at all, in about fifteen seconds:

```sh
git clone https://github.com/wouterbeets/self && cd self
./demo.sh
```

`demo.sh` drives the entire thesis offline through `examples/mind-stub`, a
deterministic filter in the same seam a real mind occupies: an intent becomes
declarations and signed scripts, a command appends an event, a view replays it
twice to the same bytes, a hand-edited script is silently overwritten by its
receipt, a fresh body is rebuilt from two files byte-for-byte, and an account is
given, curated, and learned somewhere else with the edit visible in both logs.

Then put a real mind in the pipe:

```sh
go install .                                     # `self` on PATH
cd ~/somewhere
self learn ~/self/lessons/chat | claude -p | self hear   # grow a way to talk
self run say "what can you do?"
self view chat
```

The working directory is the instance, so a clone is immediately inspectable:
`events.jsonl`, `.secret` and `cap/` appear beside it. Pin one shared instance
with `export SELF_HOME=~/.self`. A new home starts genuinely empty — every
capability is learned through the pipe and leaves a signed receipt. There is no
other install path.

**Already living in a coding agent?** [`AGENTS.md`](AGENTS.md) is one section to
paste into your project's `CLAUDE.md`, and your sessions gain a memory that
outlives them.

## What an instance is

```
events.jsonl   the append-only log — the only authoritative state
.secret        the signing key (32 random bytes, mode 0600)
cap/           installed scripts, derived: blobs addressed by hash, with a
               readable symlink per capability. Delete it; nothing is lost.
```

Two kinds of capability, and the difference is the trust boundary. The kernel
appends what a **command** prints and never appends what a **view** prints; a
command is told which instance it is acting on, and a view is told nothing at
all — no `SELF_HOME`, an empty directory to run in, its whole input on stdin.
A command gets argv and the log; a view gets exactly the events its receipt
names and prints opaque bytes — text, HTML, JSON, whatever you want to read.
Both are ordinary executables in any language with a shebang, written by a mind
and installed only under a signature. None of this is a sandbox: an installed
script runs as you, and the limits say so.

The whole contract is [`PROTOCOL.md`](PROTOCOL.md), which is also what
`self help` prints. Nothing else in this repository restates it — there is one
description of the wire, in one place, because six hand-synced copies is how the
previous kernel came to print two contradictory instructions inside one brief.

## Accounts — how anything moves between instances

You cannot transplant a skill and you cannot write into another mind's memory.
What one instance can do is **give an account**: a directory of plain text that
a receiver reads, curates, and learns from.

```
account/
  intent.md      the telling: who this is from, what it means (required)
  record.jsonl   the evidence: events verbatim, moments preserved (optional)
  manifest.json  the attestation: count + sha256 of the record (optional)
```

**Nothing runnable ever travels.** `self learn <dir>` deposits the record
verbatim — keeping each event's own moment and its own speaker, re-stamping only
the door, because a door is a local fact — and prints a learning prompt. The
receiving mind reads the intent against local state and declares *its own*
capabilities, authored and signed locally. Same account, two instances, two
expressions: that is learning rather than copying.

Three mechanical rules keep it honest. The kernel's vocabulary never travels
raw, and the refused set is frozen — a name may leave the vocabulary but never
leaves the refused set, so yesterday's event name can never become tomorrow's
injection. Moments and speakers are preserved. And the attestation records the
sha256 of what actually landed beside what the manifest claimed, so deleting a
line before learning — legitimate curation — is visible in both logs forever.

Giving is cheap; learning is the work. That asymmetry is the protocol, and it is
specified in full in [`PROTOCOL.md`](PROTOCOL.md).

## Why

Frontier models are extraordinary at the novel and the general. What they do not
have is your history: the accumulated, contextual, hard-won practical knowledge
the Greeks called **metis**. `self` exists to make that a first-class thing a
person owns — durable, inspectable, verifiable, and portable — so it can travel
*alongside* the best models rather than being captured inside any one of them.

These are complementary. A healthy metis layer gives a frontier model
better-grounded context, and the user keeps continuity and agency that no single
provider mediates. This runtime is the reference implementation of a larger
idea: the [Account
Protocol](https://github.com/wouterbeets/knowledge-seed-protocol) says how
records and capabilities move between sovereign minds — as accounts you read and
learn from, never as code that runs. `self` is the runtime that speaks it.

## Substrate, not features

This kernel is deliberately smaller than the one before it. What went, and why:

- **The HTTP server, the injected shell, `site/`.** A web surface is a feature,
  and it forced every view to emit bare markup — no CSS, no JavaScript, no
  assets — so a server-side stylesheet could style it. That ceiling is gone. See
  *Many faces* below for what is on the other side of it.
- **The kernel's conversation** (`self.asked`, `self.replied`, `self.reflected`)
  and its knowledge of `chat.message`. The kernel kept a diary beside the chat
  lesson, and parsed a *lesson's* event name to decide what to do next.
  Conversation is a capability — see `lessons/chat` — not a kernel feature.
- **`isatty` as protocol.** The previous kernel chose between five faces by
  inspecting file descriptors. Verified consequence: run its own documented loop
  anywhere without a terminal — a script, an agent, cron — and the mind's reply
  was recorded as a *question* and the answer never reached you. Direction is
  structural now.
- **The page cache** (materialized HTML, mtime freshness, selective refresh) and
  the staging-and-rename swap for two derived trees. Cache coherence has no
  business inside a kernel whose whole claim is pure replay.
- **A declaration schema nothing validated** — params, event shapes, revision
  records, and a *reference implementation* embedded in the declaration, which is
  a runnable riding the one channel that exists to keep runnables out.

What arrived: content-addressed script blobs, so a running script's bytes can
never change under it; one replay that answers every derived question; a
scrubbed environment, because determinism was claimed but never enforced; and a
`hear` that is lenient per line, because a real frontier model told plainly that
stdout is the wire still opens with a sentence — and strictness threw six perfect
events away.

Two claims turned out to be false rather than broken, so the claims changed
rather than the code. And a record is now *defined* as a line terminated by a
newline: a crash mid-write leaves bytes that were never a record, and the next
append drops them. Before that rule, one short write bricked an instance
permanently — every verb, `rehydrate` included — with no repair path but the one
thing the protocol forbids.

## Many faces

A view is a pure function from events to **bytes**. Not to HTML — to bytes, and
the kernel neither knows nor cares what shape they are. One log, as many surfaces
as you care to write, each composing with whatever already reads that format:

```sh
self view table                       # an aligned table with a total, at a prompt
self view json | jq 'group_by(.what)' # your own history, queryable
self view csv  > work.csv             # a spreadsheet, sqlite, pandas, R
self view page > /tmp/p.html          # ONE self-contained HTML file, own CSS and JS
self view chart > /tmp/p.svg          # a chart, as a pure function of events
self view metrics                     # Prometheus text — scrape an instance that
                                      #   has no idea what a metric is
self view ics                         # iCalendar — a phone can subscribe to it
self view replay > rebuild.sh         # a view whose output is a program
```

None of that is kernel support. Each face is a short script a mind wrote,
installed under a signature, replayed on demand. `self view replay | sh` against
a fresh instance produces a log whose table is identical to the original's: the
log wrote the program that rebuilt the log.

`lessons/faces` is the account that grows this — the intent, not the scripts,
because nothing runnable travels. Learn it and your instance writes its own:

```sh
self learn lessons/faces | claude -p | self hear
```

## Limits

Stated plainly, and at greater length in `self help`: there is **no human review
step between authoring and signing** — piping a mind's output into `self hear`
signs whatever it authored. The log can contain untrusted input (a learned
account, a crafted event) and that input becomes context for the mind writing
your scripts. Nothing is sandboxed; the environment is scrubbed for determinism,
not containment. The log is unbounded. Anyone who can run `self` against your
`SELF_HOME` can install, which is the power they already have holding the
directory and its key.

Read a generated script before you trust it. The advantage over an ordinary
software supply chain is not that this is safe — it is that what you inspect is
readable intent and readable output rather than an opaque binary.

## Status

Experimental. The claim under test: an append-only log, a signed install gate,
deterministic replay, and one shell pipe are enough of a kernel for a system
that writes, tests and revises its own capabilities while staying local-first
and wholly inspectable.

## License

Apache-2.0. The scripts your instance generates and the events in your log are
program output — yours, not derivatives of this runtime.
