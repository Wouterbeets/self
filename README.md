# self

`self` is a small, local-first runtime — and, to the shell, a filter with a
memory. It keeps one append-only event log as its authoritative state, and
rebuilds every view — and every capability — from that log. The kernel holds
no model: intelligence enters through the pipe, where the mind is whatever
process you put between two invocations of `self`:

```sh
echo "whats going on today?" | self | claude -p | self
```

The first `self` turns your prose into a situated prompt — where the instance
is, what it can do, what work is pending, and how to answer. The mind (any
agent CLI, any model wrapper, any script) answers in event JSONL. The second
`self` hears it: events append to the log, authored scripts install under a
signature made with a key that never leaves the instance, and the reply
passes through to you. The whole system can be rebuilt from the log alone,
with no model and no network.

The point is durable, inspectable state for an LLM agent: one log, replayed
into every view, with a record of who generated each script and the ability
to reconstruct it offline. What is not in the log did not happen.

## Vision

`self` exists to make **metis** — practical, contextual, accumulated wisdom built
through lived use — a first-class citizen alongside powerful frontier intelligence.

Frontier models excel at broad reasoning, synthesis, and handling the novel.
Local instances like this provide durable personal history, organic capability
growth from real interaction, verifiable provenance, and user-controlled evolution.

These are complementary, not competing. A healthy metis layer gives frontier models
richer, better-grounded context to work with. Users keep long-term agency and
continuity without routing everything through any single provider. This is not
hostility toward powerful models; it is the missing substrate that lets them
participate in sovereign, long-lived personal systems.

The result is a win-win: general intelligence and lived practical wisdom reinforce
each other. The goal is a more resilient ecosystem where decentralized, user-owned
metis can grow and travel alongside the most capable models — preserving space
for agency that is not captured by any one company.

This runtime is the reference implementation of a larger idea:

- **[the Account Protocol](https://github.com/wouterbeets/knowledge-seed-protocol)**
  — how records and capabilities move between sovereign minds: as accounts a
  receiver reads and learns from, never as code that runs. You can't
  transplant a skill; you can only show your work.
- **self** (this repo) — the runtime that speaks it: the log, the signed
  installation, the deterministic replay, and `give`/`learn` as the exchange.

It is experimental. See [Limits](#limits-and-threat-model) before you rely on it.

## The loop

Everything intelligent this system does is one shell idiom:

```sh
echo "<ask>" | self | <mind> | self
```

`self` has three faces, chosen by what arrives on stdin:

- **prose** → the *ask* face: the question is recorded (`self.asked`) and a
  situated prompt goes to stdout — orientation brief, recent conversation,
  pending work, the ask, the answer contract.
- **event JSONL** → the *hear* face: event lines append to the log, each
  `script.authored` installs under a locally signed receipt, and the reply
  (prose plus the text of `self.replied`) passes through to stdout.
- **nothing** → at a terminal, the orientation brief plus a live margin —
  pending scripts, unresolved script rejections, a chat message waiting for a
  reply; in a pipe, the *work* prompt — pending scripts if declarations await
  authoring, else unresolved rejections, else one self-improvement
  reflection. So the bare loop always means something:

```sh
self | claude -p | self        # author pending work, or reflect once
```

The composability is the point. Stop before the second `self` and nothing is
ingested — `echo "…" | self | claude -p` is a pure query. Skip the mind and
`self` is `tee` into the log — `echo '{"name":"note.taken","payload":{…}}' | self`
appends an event raw. Loop it and capabilities grow until the mind has
nothing left to author.

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
no model — a deterministic filter piped through the same seam as any real
mind): a lesson's intent becomes declarations and authored scripts, running a
command appends an event, a projection renders it, and the instance rebuilds
from `events.jsonl` + `.secret` alone — byte for byte. The stub writes
trivial scripts; this shows the machinery, not the intelligence.

### 2. Real capabilities (put a mind in the pipe)

```sh
go install .                        # `self` on PATH (via GOBIN)
cd ~/my-project
self learn lessons/chat | claude -p | self    # grow a conversational surface
self serve                                    # then open http://127.0.0.1:7777
```

By default the current working directory is the instance home, so a clone is
immediately inspectable: `events.jsonl`, `.secret`, `capabilities/`, and
`site/` appear right beside the code. A new home starts empty on purpose:
every capability is learned through the pipe and leaves a signed receipt in
the log — there is no other install path.

If you want one shared instance regardless of where you run `self`, pin it in
your shell rc:

```sh
export SELF_HOME=~/.self             # or: export SELF_HOME=$HOME/my-self
```

`examples/mind-stub` supplies a deterministic offline mind for mechanical
tests and demos; real capabilities need a real model or agent in the pipe.
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

**Events.** Each record is `{id, seq, name, occurred_at, via, by, payload}`.
Records are never changed or deleted; a deletion is expressed as a later
event. `via` and `by` are provenance: `via` is the **door** — the channel the
event entered through (`cli`, `pipe`, `http:<addr>`, `learn:<account>`,
`kernel`) — stamped by the kernel from what it witnessed and never accepted
from a script, mind, or record; `by` is the **speaker** — the caller's claimed
identity (`SELF_CALLER` locally, the `X-Self-Caller` header over HTTP),
recorded verbatim as a claim, not a verified fact. `via` is local like `seq`;
`by` is portable like `occurred_at` and travels with an account.

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

**Runtime code generation.** A `command.declared` or `projector.declared`
event announces a capability; it stays **pending** until a mind pipes back
its script as a `script.authored` line, which the kernel installs and records
as a `script.compiled` receipt. Pending work surfaces in every prompt the ask
face emits, so the loop converges: run `self | claude -p | self` until quiet.
Declarations — not code — are what cross every boundary between instances.

**Signed installation.** A receipt is `{type, name, script, by, sig}`, where
`sig` is an HMAC-SHA256 over the fields using the instance's key. Only receipts
that verify under the local key are ever installed; anything else in the log is
inert data. `by` records who authored the bytes and is covered by the
signature, so authorship cannot be relabeled after the fact. The hear face
installs a script only for a capability this log has declared and not retired.

**Reconstruction.** `self rehydrate` rebuilds every script and view from
`events.jsonl` + `.secret` alone — no LLM, no network. This is the recovery
path, the migration path, and the audit path, and the test suite pins it.

## Accounts — how anything moves between instances

You cannot transplant a skill, and you cannot write into another mind's
memory. What one instance can do is **give an account** — a plain-text
directory the receiver reads, curates, and **learns** from:

```
account/
  intent.md      the telling: who this is from, what it means, what you hope it
                 becomes (required — a bare intent is just a lesson)
  record.jsonl   the evidence: events verbatim, moments preserved (optional)
  manifest.json  the attestation: event count + sha256 of the record (optional)
```

`self give note. <dir>` writes the knowledge flavor: every `note.*` event,
verbatim. `self give command/note <dir>` writes the capability flavor: the
declarations and locally-signed receipts of that capability, renamed to
lineage. `self learn <dir>` is the only way in, and it splits cleanly along
the seam: the **mechanical half** happens immediately and needs no mind — the
record lands verbatim with its own `occurred_at` and speaker, and a
`lesson.learned` receipt attests to what was deposited; the **intelligent
half** rides the pipe — learn prints the learning prompt on stdout, and
whatever mind you pipe it to reads the intent (and the deposited record, with
its own tools) against local state and declares its own capabilities,
authored and signed locally:

```sh
self learn account/ | claude -p | self
```

Same account, two instances, two expressions — that is learning, not drift.
Dropping the prompt is legitimate too: the record is already in, and the
intent stays in the log for a later pass.

Three rules keep the exchange honest, all mechanical:

- **The kernel's vocabulary never travels.** `give` renames lifecycle events
  (`command.declared`, `script.compiled`, `self.replied`, …) to `lineage.*`;
  `learn` refuses them raw. A foreign account carries its history as evidence
  but cannot speak in the receiving kernel's voice — a hostile account cannot
  install anything, and cannot inject conversation turns (there are tests for
  both).
- **Moments are preserved.** Deposited events keep their own `occurred_at`
  and their own speaker (`by`) — testimony travels with its time and its
  voice. The door (`via`) is re-stamped `learn:<account>`: doors are this
  log's facts, never another body's. A record arriving is history, not news.
- **Interventions are visible.** Curation is editing the directory — the
  account is plain text, and deleting a line before learning is legitimate.
  The `lesson.learned` receipt records the sha256 of what was actually
  deposited beside what the manifest claimed, so the edit shows in both logs.

Giving is cheap; learning is the work. The giver's log keeps `account.given`,
the receiver's keeps `lesson.learned` — both sides remember.

## CLI

```
self                 the filter: prose in → situated prompt out; events in →
                     appended, reply out; empty pipe → pending work or one
                     reflection (the loop: echo "…" | self | claude -p | self)
self serve           rehydrate from the log, then serve at :7777
self run <cmd> ...   run a command: append its events, re-render projections
self show <name>     render a projection to stdout
self learn <account> deposit an account's record (moments preserved) and print
                     its learning prompt — pipe it to a mind
self give <sel> <dir>
                     write an account from the log — <sel> is an event-name
                     prefix ("note.") or command/<name> | projector/<name>
self rehydrate       rebuild capabilities/ + site/ from the log (offline)
self retire <t>/<n>  retire a capability: script + page leave the surface, the
                     log keeps every event, re-declaring revives it
self protocol        print the pipe and capability wire contracts
```

The old verbs dissolved into the pipe: *think* is the loop without the second
`self` (nothing ingested), *reflect* is the bare loop with nothing pending,
and *revise* is asking — `echo "revise command/note: also record a mood" |
self | claude -p | self` re-declares and re-authors under a fresh receipt.

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

The kernel holds no model — not even a fake one — and it spawns nothing. A
mind is any process the shell pipes between two selves:

```sh
echo "…" | self | claude -p | self          # an agent CLI, no adapter
echo "…" | self | examples/mind-stub | self # the offline stub (tests, demos)
```

The wire contract is symmetrical and tiny. Downstream of the first `self`, a
mind receives one plain-text prompt on stdin: the orientation brief (also at
`site/brief.md`), the recent conversation, any pending declarations with the
script contract, the ask, and instructions for answering. Upstream of the
second `self`, it prints lines:

| line                                              | what the hear face does           |
|---------------------------------------------------|-----------------------------------|
| `{"name":"self.replied","payload":{"text":…}}`    | appended; text printed as the reply |
| `{"name":"<domain.event>","payload":{…}}`         | appended verbatim                 |
| `{"name":"command.declared"/"projector.declared",…}` | appended; pending until authored |
| `{"name":"script.authored","payload":{"type":…,"name":…,"script":…}}` | installed + signed receipt |
| anything else                                     | passed through as prose           |

A `script.authored` the kernel refuses — empty script, undeclared or unknown
capability — is not lost to stderr: the kernel records a `script.rejected`
event (via `kernel`, with the reason and a capped excerpt), so the failure is
part of the log like everything else. Whether a rejection is still *open* is
derived by replay, never stored: it closes when a verified receipt or a
`capability.retired` for that capability postdates it. While open it rides
the pending section of every prompt (when its declaration still stands),
becomes the work prompt itself (when it does not), and shows under bare
`self` at a terminal — so `self | mind | self` continues from a failure the
same way it continues from anything else.

The prompt is a wake-up card, not a context dump: a complete mind inspects
`SELF_HOME` itself with its own tools — `site/*.html`, `events.jsonl`,
`capabilities/` — before answering. A tool-capable coding agent already can;
a bare completion endpoint is a partial mind (it can converse and emit
events, but cannot explore or test what it authors).

Minds are stateless on purpose. Each pass starts cold and orients from the
prompt and the rendered state; the instance's memory is the log, its
projections, and nothing else — including the conversation itself, which the
ask and hear faces record as `self.asked` / `self.replied` events. Do not
reach for a harness session store (`claude -p --continue` and its kin) as the
mind's memory: it chains state outside the log — not replayed by `rehydrate`,
invisible to audit. If a cold mind orients slowly, that is design pressure
aimed at the right target: improve the projections, don't bolt on a hidden
memory tier.

## Where the mind runs

Nowhere the kernel knows about. The kernel never spawns a mind, gives it no
tools, and does not sandbox it — the shell composes the pipeline, and
exploration during authoring (reading files, testing a candidate script) is
the mind's own concern with its own tools. The kernel's guarantee is narrower
and stronger: whatever arrives on the pipe, **only a locally-signed
`script.compiled` receipt ever installs, and only for a capability this log
declared** — a mind can only ever *propose*; the kernel writes the record.
Installed capability scripts then run without a sandbox — see Limits.

## Environment

```
SELF_HOME         instance directory (default: current working directory; set in
                  your shell rc to pin one shared home, e.g. ~/.self)
SELF_BIND         bind address for self serve, host or host:port (default
                  127.0.0.1:7777; set 0.0.0.0 to expose)
SELF_CALLER       claimed speaker, recorded verbatim as `by` on events your
                  invocations append — including everything heard from the
                  pipe; over HTTP the `X-Self-Caller` header carries the
                  claim. The door (`via`) is stamped by the kernel itself and
                  cannot be claimed.
SELF_MIND_ID      author string signed into script.compiled receipts when the
                  pipe installs an authored script (default: SELF_CALLER,
                  else "pipe")
```

## Repository layout

- The top-level Go files divide the small kernel by concern: CLI dispatch,
  the pipe (ask/hear/work faces, prompts, pending work), event log, signed
  installation, projections, accounts (give/learn), HTTP server, and
  reconstruction.
- `main_test.go` — the pinned invariants: log semantics, the strange loop
  through the pipe, the forged-receipt gate, receipt provenance, prompt
  shape and bounds, byte-stable reconstruction, and the account round trip
  (moments preserved, kernel vocabulary refused, lineage inert, interventions
  visible).
- `examples/` — minds. A mind is a filter, so most need no adapter at all
  (`claude -p` plugs in bare); `mind-stub` is the deterministic offline mind
  the tests and `demo.sh` pipe in (no LLM, no network). Not part of the kernel.
- `demo.sh` — the offline, no-model walkthrough of the loop.
- `lessons/journal` — the smallest example: one command, one projection.
- `lessons/memory` — durable memory for a stateless mind: `remember` writes
  facts to the log; a cold mind orients from `/memory`. The in-log answer to
  session stores.
- `lessons/chat` — a conversational surface; the mind stays outside the loop
  the way the clock stays outside the timers lesson, and asking for a missing
  capability grows it on the next mind pass.
- `lessons/files` — small files carried in the log as events: content in the
  payload, digest as identity, served back as data: URIs from a pure
  projection. What the kernel's file store used to do, relearned as a lesson.
- `lessons/timers` — scheduled intentions with the clock kept outside: an
  external tick (cron, a session, a human) fires due timers as events, so
  replay sees history and never a trigger. What the kernel's ticker used to do,
  relearned as a lesson.

## Limits and threat model

These are current properties, stated plainly, not goals to aspire to.

- **The mind is driven by the log, and the log can contain untrusted input.**
  Chat messages, learned accounts, and other events all become context for the
  mind that writes your scripts. A crafted event — or a persuasive account —
  can try to steer what gets written (prompt injection). There is **no human
  review step between authoring and signing** — piping a mind's output into
  `self` signs whatever it authored. The pipe makes the review point visible
  (hold the mind's output in a file and read it before piping it in), but
  nothing enforces the pause. Generated scripts are plain text in
  `capabilities/`; read them, use a mind you trust, and treat learning an
  account as running code: read its intent and record first. The advantage
  over the usual software supply chain is that what you inspect is readable
  intent and readable output, not an opaque binary — but it is still yours to
  inspect.
- **The hear face trusts its caller.** Anyone who can run `self` against your
  `SELF_HOME` can pipe in declarations and scripts — the same power they
  already have holding the directory and its key. The signed-receipt gate is
  about reconstruction and foreign accounts, not local privilege. The HTTP
  surface never hears: `/run` only fires installed commands.
- **The kernel does not sandbox anything.** Minds run wherever the shell ran
  them; installed capability scripts run directly. The kernel's guarantee is
  the signed-receipt gate, not containment.
- **The log is unbounded.** Every install stores its script bytes, and every
  projection replays its consumed events (O(history)). Snapshotting is not
  built in; a snapshot can itself be modeled as a capability and left to the
  user.
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
installation, deterministic replay, and one shell pipe are enough of a kernel
for a system that generates, tests, and revises its own capabilities while
staying local-first and fully inspectable.

## License

Apache-2.0. The scripts your instance generates and the events in your log are
program output — yours, not derivatives of the runtime.
