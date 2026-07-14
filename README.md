# self

`self` is a small, local-first runtime. It keeps one append-only event log as
its authoritative state, and rebuilds every view — and every capability — from that log.
Capabilities are not shipped as code. You describe what you want, and a mind of
your choosing — a tool-capable coding agent like `opencode run` or `claude -p` —
writes the script for it, on your machine, after inspecting the instance's
rendered state. The kernel itself holds no model: it only installs each generated
script under a signature made with a key that never leaves the instance, so the
whole system can be rebuilt from the log alone, with no model and no network.

The point is durable, inspectable state for an LLM agent: one log, replayed into
every view, with a record of who generated each script and the ability to
reconstruct it offline. What is not in the log did not happen.

This runtime is the reference implementation of a larger idea:

- **[the Account Protocol](https://github.com/wouterbeets/knowledge-seed-protocol)**
  — how records and capabilities move between sovereign minds: as accounts a
  receiver reads and learns from, never as code that runs. You can't
  transplant a skill; you can only show your work.
- **self** (this repo) — the runtime that speaks it: the log, the signed
  installation, the deterministic replay, and `give`/`learn` as the exchange.

It is experimental. See [Limits](#limits-and-threat-model) before you rely on it.

## Quick start

**Already living in a coding agent?** The fastest path is the
[agent integration card](AGENTS.md): install `self`, paste one section into
your project's `CLAUDE.md` / agent instructions, and your sessions gain an
append-only memory that outlives them — no browser, no config, growth when
you ask for it. Prebuilt binaries are on the
[releases page](https://github.com/Wouterbeets/self/releases); or
`go install github.com/Wouterbeets/self@latest`.

### 1. See the machinery with no LLM (about 10 seconds)

```sh
git clone https://github.com/wouterbeets/self && cd self
./demo.sh
```

`demo.sh` runs the whole loop offline using `examples/mind-stub` (no API key,
no model — a deterministic mind plugged through the same seam as any real
one): a lesson's intent becomes declarations, a declaration compiles into a
script, running a command appends an event, a projection renders it, and the
instance rebuilds from `events.jsonl` + `.secret` alone — byte for byte. The
stub writes trivial scripts; this shows the machinery, not the intelligence.

### 2. Real capabilities (plug a mind)

```sh
go install .                        # `self` on PATH (via GOBIN)
make run                            # or: self
```

Open <http://127.0.0.1:7777>. By default the current working directory is the
instance home, so a clone is immediately inspectable: `events.jsonl`, `.secret`,
`capabilities/`, and `site/` appear right beside the code. A new home starts
empty on purpose: name a mind with `SELF_MIND`, then learn chat or a journal
from their visible `intent.md` prompts. Every capability is learned through the
mind and leaves a signed receipt in the log — there is no other install path.

If you want one shared instance regardless of where you run `self`, pin it in
your shell rc:

```sh
export SELF_HOME=~/.self             # or: export SELF_HOME=$HOME/my-self
```

The same flow works from the CLI:

```sh

cd ~/my-project
export SELF_MIND="claude -p"        # any executable can be the mind (see below)
self learn lessons/chat              # generate a capability set from its intent
self                                 # rebuild from the log, then serve at :7777
```

`learn` needs a mind; `examples/mind-stub` supplies a deterministic offline
one for mechanical tests and demos, while real capabilities need a real model
or agent.
To make every coding-agent session in a project use one instance as shared
persistent state, paste the card in [`AGENTS.md`](AGENTS.md) into the project's
agent instructions. To write your own lessons, see [`LESSONS.md`](LESSONS.md).

## How it works

An instance is a directory (`SELF_HOME`, defaulting to the current working
directory) with one authoritative log and one local key:

```
events.jsonl    the append-only log — the only structured state
.secret         a per-instance signing key (32 random bytes, hex; mode 0600)
```

Everything else (`capabilities/`, `site/`) is derived and can be rebuilt.

**Events.** Each record is `{id, seq, name, occurred_at, payload}`. Records are
never changed or deleted; a deletion is expressed as a later event.

**Commands.** A command is an executable script. It receives its arguments as
`argv` and the current log as JSONL on stdin, and writes new events as JSONL on
stdout (`{name, payload}` per line; the kernel fills in the rest). The emitted
events are appended and the projections that consume them re-render.

**Projections.** A projection is an executable that reads the events matching
its declared `consumes` list on stdin (an empty list — or `"*"` — means every
event) and writes bare semantic HTML on stdout, saved to `site/<name>.html`.
The kernel injects the shared shell when serving, so projectors must not emit
CSS, JavaScript, inline styles, or external assets. A projector must be a pure
function of its events: rendering twice from the same log yields the same
bytes. The kernel re-runs a projection only when the log grows events it
consumes; a page whose events did not change is served as already materialized.

**Runtime code generation.** A `command.declared` or `projector.declared` event
triggers a compile: the kernel hands the declaration to the mind process, which
writes the script; the kernel installs it and records a `script.compiled`
receipt. Declarations — not code — are what cross every boundary.

**Signed installation.** A receipt is `{type, name, script, by, sig}`, where
`sig` is an HMAC-SHA256 over the fields using the instance's key. Only receipts
that verify under the local key are ever installed; anything else in the log is
inert data. `by` records which mind authored the bytes and is covered by the
signature, so authorship cannot be relabeled after the fact.

**Reconstruction.** `self rehydrate` rebuilds every script and view from
`events.jsonl` + `.secret` alone — no LLM, no network. This is the recovery
path, the migration path, and the audit path, and the test suite pins it.

## Accounts — how anything moves between instances

You cannot transplant a skill, and you cannot write into another mind's
memory. What one instance can do is **give an account** — a plain-text
directory the receiver reads, curates, and **learns** from:

```
account/
  intent.md      the telling: who this is from, what it means, what might
                 grow from it (required — a bare intent is just a lesson)
  record.jsonl   the evidence: events verbatim, moments preserved (optional)
  manifest.json  the attestation: event count + sha256 of the record (optional)
```

`self give note. <dir>` writes the knowledge flavor: every `note.*` event,
verbatim. `self give command/note <dir>` writes the capability flavor: the
declarations and locally-signed receipts of that capability. `self learn
<dir>` is the only way in: the receiver's mind reads the intent — and the
record, with its own tools — against local state and declares its own
capabilities, compiled and signed locally; the record then lands verbatim
with its own `occurred_at`, never routed through the mind. Same account,
two instances, two expressions — that is learning, not drift.

Three rules keep the exchange honest, all mechanical:

- **The kernel's vocabulary never travels.** `give` renames lifecycle events
  (`command.declared`, `script.compiled`, …) to `lineage.*`; `learn` refuses
  them raw. A foreign account carries its history as evidence but cannot
  speak in the receiving kernel's voice — a hostile account cannot install
  anything (there is a test for this).
- **Moments are preserved.** Planted events keep their own `occurred_at`;
  a record arriving is history, not news.
- **Interventions are visible.** Curation is editing the directory — the
  account is plain text, and deleting a line before learning is legitimate.
  The `lesson.learned` receipt records the sha256 of what was actually
  planted beside what the manifest claimed, so the edit shows in both logs.

Giving is cheap; learning is the work. The giver's log keeps `account.given`,
the receiver's keeps `lesson.learned` — both sides remember.

## CLI

```
self                 rehydrate from the log, then serve (default)
self learn <account> learn an account: capabilities from its intent.md (needs a
                     mind), its record planted verbatim, moments preserved
self give <sel> <dir>
                     write an account from the log — <sel> is an event-name
                     prefix ("note.") or command/<name> | projector/<name>
self run <cmd> ...   run a command: append its events, re-render projections
self think "..."     query the mind; returns {response, declarations} JSON
self reflect         one improvement cycle: the mind inspects the log and may declare
self show <name>     render a projection to stdout
self rehydrate       rebuild capabilities/ + site/ from the log (offline)
self revise <t>/<name> <request>
                     recompile a local capability with its current source as context
self protocol        print the mind and capability wire contracts
self retire <t>/<n>  retire a capability: script + page leave the surface, the
                     log keeps every event, re-declaring revives it
```

Server routes: `/` (instance self-description), `/<projection>`,
`/run/<command>` (plain HTML forms), `/events` (raw log). The server binds
`127.0.0.1:7777` by default; `SELF_BIND` is the whole bind address, host or
`host:port` — set `SELF_BIND=0.0.0.0` to expose it (see
[Limits](#limits-and-threat-model)).

**The shell.** When serving, the kernel adds one shared stylesheet and a nav
to every page; projections on disk stay plain HTML, and the shell knows only
the CSS class names, never the events. Strip it (`self show`, curl, a no-JS
browser) and every page still works, because every action underneath is a
plain HTML form.

## The mind

Every request for intelligence — `think`, `reflect`, `learn`, and each
compile — goes through one interface. The kernel holds no model — not even a
fake one; the mind is always a **process** you supply:

```sh
SELF_MIND="claude -p"                      # any executable (agent CLI, script, human shim)
SELF_MIND="$PWD/examples/mind-stub"       # or the deterministic offline stub (testing, demos)
```

A tool-capable agent CLI needs no adapter: `SELF_MIND="claude -p"` is a
complete mind as-is — and stateless on purpose. Each ask starts cold and
orients from the brief and the rendered state; the instance's memory is the
log, its projections, and nothing else. Do not reach for the harness's own
session store (`claude -p --continue` and its kin) as the mind's memory: it
chains asks into a conversation that lives outside the log — not replayed by
`rehydrate`, invisible to audit — and whatever "sense of the place"
accumulates there is state the system depends on but never captured.
If a cold mind orients slowly, that is design pressure aimed at the right
target: improve the projections, don't bolt on a hidden memory tier.

The stub mind is deliberately dumb but complete: `think` returns a fixed
reply, `learn` declares a minimal command + projection from names in
`intent.md`, and compile answers with scripts that honor the pipe contract. It
proves the machinery, not the intelligence — and it is a process behind the
same seam as every real mind, because the kernel carries no mind of its own.

The `SELF_MIND` process contract:

| channel     | content                                                       |
|-------------|---------------------------------------------------------------|
| `$SELF_ASK` | request kind: `think` \| `reflect` \| `learn` \| `compile`   |
| last argv   | the prompt (for `compile`, it carries the declaration to build)|
| stdin       | an **orientation brief** (plain text): where the mind is,    |
|             | what capabilities exist, and where to look for the rest. This  |
|             | is a wake-up card, not a context dump. The mind MUST inspect  |
|             | `SELF_HOME` itself with its own tools — `site/*.html`,         |
|             | `events.jsonl`, `capabilities/` — to do its job. A process    |
|             | without that exploration ability is not a complete mind.      |
| stdout      | event JSONL; non-JSON lines are collected as the text reply    |

So a whole mind is any program that switches on `$SELF_ASK` and prints events:

```python
#!/usr/bin/env python3
import os, sys, json
brief  = sys.stdin.read()         # an orientation brief, plain text — where to look
ask    = os.environ["SELF_ASK"]    # think | reflect | learn | compile
prompt = sys.argv[-1]             # for compile, this is the declaration to build
# A real mind then reads SELF_HOME/site/kernel.html, events.jsonl, etc. itself.

if ask == "compile":               # author the script the declaration asked for
    script = "#!/bin/sh\necho '{\"name\":\"pinged\",\"payload\":{}}'\n"
    print(json.dumps({"name": "script.authored", "payload": {"script": script}}))
else:                              # think/reflect/learn: prose + optional declarations
    print("explored the instance; nothing to add.")  # non-JSON line → the text reply
```

`command.declared` / `projector.declared` add capabilities; `script.authored`
answers a `compile`. The kernel's seam is a pipe, but a mind that cannot
inspect files under `SELF_HOME` (`site/*.html`, `events.jsonl`,
`capabilities/`) cannot do the job — the orientation brief on stdin is a
wake-up card, not a context dump. A tool-capable coding agent (`opencode run`,
`claude -p`) already has the tools to explore; a plain API adapter without a
tool loop of its own is incomplete by spec. Receipts are signed with
`SELF_MIND_ID` as the recorded author.

## Where the mind runs

The kernel spawns the mind as a plain subprocess and reads its stdout; it does
not give the mind tools, and does not sandbox it. Exploration during
generation — running a candidate script, reading files — is the mind's own
concern, and a capable mind (a coding agent) already has its own sandboxed
tools. The kernel's guarantee is narrower and stronger: whatever the mind does,
**only a locally-signed `script.compiled` receipt ever installs**, so a mind
can only ever *propose* — it cannot write the record. Installed capability
scripts then run without a sandbox — see Limits.

## Environment

```
SELF_HOME         instance directory (default: current working directory; set in
                  your shell rc to pin one shared home, e.g. ~/.self)
SELF_MIND        mind executable (e.g. "claude -p"); the kernel spawns it for
                  every request kind. For offline demos/tests, point it at
                  examples/mind-stub (no LLM, no network).
SELF_BIND         bind address, host or host:port (default 127.0.0.1:7777;
                  set 0.0.0.0 to expose)
SELF_MIND_ID     author string signed into receipts
                  (default: the mind executable)
```

## Repository layout

- The top-level Go files divide the small kernel by concern: CLI dispatch,
  event log, signed installation, orchestration, projections, accounts
  (give/learn), mind seam, HTTP server, and reconstruction.
- `main_test.go` — the pinned invariants: log semantics, offline runtime
  generation, the forged-receipt gate, receipt provenance, the pluggable
  mind, byte-stable reconstruction, and the account round trip (moments
  preserved, kernel vocabulary refused, lineage inert, interventions
  visible).
- `examples/` — minds that plug in through `SELF_MIND`. `mind-stub` is the
  deterministic offline mind the tests and `demo.sh` use (no LLM, no
  network); `mind-opencode` is a working tool-capable mind (it delegates to
  `opencode run`, which can inspect `SELF_HOME`). Not part of the kernel.
- `demo.sh` — the offline, no-mind walkthrough of the loop.
- `theater/` — the flashy, optional, deletable front end (`make theater`): the
  log as a galaxy of splats, capabilities as an orbiting ring, voice-in /
  speech-out `think`. A lens over the same log and seams, holding no state —
  see `theater/README.md`. Not part of the kernel.
- `lessons/journal` — the smallest example: one command, one projection.
- `lessons/memory` — durable memory for a stateless mind: `remember` writes
  facts to the log; a cold mind orients from `/memory`. The in-log answer to
  session stores.
- `lessons/chat` — a conversational surface; asking for a missing capability
  generates it mid-conversation.
- `lessons/files` — small files carried in the log as events: content in the
  payload, digest as identity, served back as data: URIs from a pure
  projection. What the kernel's file store used to do, regrown as a lesson.
- `lessons/timers` — scheduled intentions with the clock kept outside: an
  external tick (cron, a session, a human) fires due timers as events, so
  replay sees history and never a trigger. What the kernel's ticker used to
  do, regrown as a lesson.

## Limits and threat model

These are current properties, stated plainly, not goals to aspire to.

- **The mind is driven by the log, and the log can contain untrusted input.**
  Chat messages, learned accounts, and other events all become context for the
  mind that writes your scripts. A crafted event — or a persuasive account —
  can try to steer what gets written (prompt injection). There is **no human
  review step between authoring and signing** — the signature is applied to
  whatever the mind produced. Generated scripts are plain text in
  `capabilities/`; read them, use a mind you trust, and treat learning an
  account as running code: read its intent and record first. Keeping a human in the loop before
  trusting a new capability is the intended posture. The advantage over the usual
  software supply chain is that what you inspect is readable intent and readable
  output, not an opaque binary — but it is still yours to inspect.
- **The kernel does not sandbox the mind, and installed capability scripts run
  without a sandbox.** The kernel runs the mind as a plain subprocess (isolating
  its exploration is the mind's own concern) and runs the scripts it installs
  directly. The kernel's guarantee is the signed-receipt gate, not containment.
- **The log is unbounded.** Every compile stores its script bytes, and every
  projection replays the whole log (O(history)). Snapshotting is not built in; a
  snapshot can itself be modeled as a capability and left to the user.
- **Individual appends are locked; operations are not transactions.** Sequence
  assignment is serialized with an advisory file lock, but a command or learn
  may append several events and perform derived-state work between them. Two
  concurrent operations can therefore interleave. Route writes through one
  serving process when operation-level ordering matters.
- **The server has no authentication** on `/run`. It binds loopback by default
  for this reason; only expose it with `SELF_BIND` on a network you trust.

Not goals in this core: multi-user access control, log compaction in the kernel,
or shipping code between instances — an account carries evidence, never
installables.

## Status

Experimental. The claim under test: an append-only log, HMAC-gated script
installation, and deterministic replay are enough of a kernel for a system that
generates, tests, and revises its own capabilities while staying local-first and
fully inspectable.

## License

Apache-2.0. The scripts your instance generates and the events in your log are
program output — yours, not derivatives of the runtime.
