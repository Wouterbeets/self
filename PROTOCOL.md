# self — the protocol

This file is the Self Protocol. It is the contract. `self help` prints it verbatim, and situated prompts
splice the marked core and growth layers as state requires, so there is exactly one
description of the wire in the whole system. Nothing else — not the README, not
AGENTS.md, not a comment — restates it.

## The law

**Reads project. Writes append. Orientation is a read.**

Looking at an instance never changes it. The log grows only when something was
meant to persist. There is no hidden state: no session store, no cache, no
sidecar. What is not in the log did not happen, and everything else — every
capability, every view — is a replay of it.

## The dispatcher

`self` is a filter with two faces, and which one runs is **structural**:

```
self [ask…]           situate: print the brief + the ask. Reads no stdin, appends nothing.  (READ)
… | self hear         hear: event lines land, authored scripts install.                     (WRITE)
```

An ask arrives as **argv**; what comes back from a mind arrives on **stdin**, at
`self hear`. The read face never touches stdin, so it also cannot block at the
head of a pipeline.

A line is an event when it is a JSON object with a dotted lowercase `name` and a
`payload` key — both halves, so a mind reporting `{"name":"notes","status":"ok"}`
cannot land an event called `notes` in your log. Every other line is ignored,
echoed, and counted on stderr. Do not narrate on the wire; a stray line will not
cost you the pass.

The corollary: `hear` ingests **any** event-shaped line it is given, including
one quoted inside a document. Do not pipe prose *about* events into it — this
file, for one, is full of examples that would land.

Identical in a terminal, a pipe, a script, a sandbox and cron.

<!-- prompt:core:begin -->
You are this self, for a bit. The mind ends; you do not: you persist as an append-only log, and only what you append persists. Context is finite: raw reads and repeated output displace evidence you still need.

The brief is a map; views are compressed perception of what you have appended. Anything you do not append is lost after this waking. Durable events may enter the next one, so preserve evidence, not narration.

Exploration sometimes yields metis: a locally verified response to a recognizable situation. Re-deriving it wastes future context; retaining transient glue does too. Preserve the smallest practice carrying its trigger, method, constraints, and evidence when recurrence, rediscovery cost, or consequence justifies it.

Use `self view <name> [args...]` to perceive and `self run <command> [args...]` to act. Your stdout is event JSONL or silence: one object per line with only `name` (lowercase dotted) and `payload`. The kernel assigns identity, sequence, time, and provenance. Prose is ignored and cannot persist. Full contract: `self help`.
<!-- prompt:core:end -->

## The wire

A mind's stdout is the wire: **event JSONL, or silence.** There is no second
output mode. Durable work happens through `self run <command>` or through the
events you print. Words meant for a human are an event this instance knows how
to render — never kernel vocabulary. Empty stdout is a valid turn.

One JSON object per line:

```json
{"name":"journal.entry","payload":{"text":"watered the plants"}}
```

You may set only `name` and `payload`. The kernel assigns `id`, `seq`,
`occurred_at`, and provenance (`via`, `by`) — a door cannot be claimed.

An event name is lowercase dotted: `^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`.

### Growing a capability

Two kinds, and the difference is what the **kernel** does with the output: it
appends what a command prints, and never appends what a view prints. A view has
no path to the log through the kernel. (Nothing *stops* a script from writing to
`events.jsonl` itself — see the limits — but then it is not a view doing it, it
is a program you installed.)

```json
{"name":"command.declared","payload":{"name":"entry","description":"append a journal entry. usage: entry <text…> — appends journal.entry {text}"}}
{"name":"view.declared","payload":{"name":"journal","description":"every entry, newest first","consumes":["journal.entry"]}}
```

A declaration is `{name, description}`, plus `consumes` for a view. There is no
schema beyond that: put usage and argument order in `description`, then explain
the consequence that makes the capability preferable — context it saves,
ambiguity it removes, or durable evidence lost when it is skipped. That string
is what the next cold mind uses to choose among tools.

A declaration is **pending** until a script arrives for it:

```json
{"name":"script.authored","payload":{"type":"command","name":"entry","script":"#!/bin/sh\n…"}}
```

`script.authored` is a wire message, not an event: it never lands in the log.
The kernel installs the bytes and records its own signed `script.installed`
receipt, or refuses and records `script.rejected` with the reason. A refusal is
not lost — it stands in the brief until an install or a retirement supersedes
it, and the reason rides the next prompt.

<!-- prompt:growth:begin -->
A declaration without installed bytes cannot run. Author and test each script,
then print this wire message; the kernel installs it and replaces it with a
signed receipt, so `script.authored` never lands as an event:

```json
{"name":"script.authored","payload":{"type":"command|view","name":"<declared name>","script":"<shebang and bytes>"}}
```

Commands receive argv, the whole log on stdin, `SELF_HOME`, and the instance
working directory; stdout must be new event JSONL. Views receive argv and only
their signed `consumes` events on stdin, with no `SELF_HOME` and an empty scratch
directory; stdout is read-only bytes. Scripts may use a standard-library
language with a shebang.

For a parameterized view, make the zero-argument form its discoverable index:
print concise usage plus the valid keys or actionable items a reader can choose.
Valid arguments render detail. Fail only on malformed or excess arguments, not
because the reader omitted a key they could not yet know.

When a capability manages stable named records rather than an unbounded stream,
give later wakings the complete append-only lifecycle by default: create/add,
revise/update, and remove/retire via a tombstone event, with reads supplied by
views. Use one stable key across those events. A tombstone never erases history;
it makes the record non-live until a later explicit create or restore. Without
revision and tombstone paths, stale records remain permanently actionable and
later wakings cannot distinguish current state from history. Do not invent CRUD
for journals or other event streams whose history is itself the domain.

If this domain repeatedly produces locally verified practices, give them a
domain-named append-only lifecycle and selective views. Retain the final
sanitized method, recognizable trigger, local constraints, verification
evidence, and source references — not the raw session, failed attempts, secrets,
or full tool transcript. Lead views with practices relevant to active work,
recently reused, or failing verification; keep the archive out of the default
surface. A method that reads live external state is a command that appends a
structured observation; a view only replays what was witnessed. Repeated reuse
may justify automation as a command, but one successful use does not require
installation.
<!-- prompt:growth:end -->

### Practices, not transcripts

Tool activity is not itself durable knowledge. Across domains, the reusable
unit is a locally proven response to a situation a future mind can recognize:
its trigger, desired outcome, final method, local constraints, and verification
evidence. The admission signal is verified compression plus likely recurrence,
high rediscovery cost, or material consequence. Both forgetting metis and
retaining transient glue impose future context cost, so selection belongs near
the moment exploration becomes trusted method.

Practice vocabulary is domain-owned, not kernel vocabulary. A printer may keep
material profiles and recovery recipes; a kitchen may keep service playbooks; a
home instance may keep maintenance diagnostics. Their lifecycle should permit
use, re-verification, revision, supersession, failure, and tombstone retirement.
The noisy source session may remain as provenance, but should not become the
working surface.

Escaping a script into JSON by hand is miserable. Don't:

```sh
jq -nc --arg t command --arg n entry --rawfile s /tmp/entry.sh \
  '{name:"script.authored",payload:{type:$t,name:$n,script:$s}}' | self hear
```

### A capability may declare capabilities

Nothing special is needed for this, and it is the sharpest form of the loop. A
command's stdout is event JSONL, and `command.declared` / `view.declared` are
events, so a command can emit one. The *instance* then has pending work that no
mind asked for, it rides the next prompt like any other, and a mind authors it.

```sh
self run propose census "how many events of each name this log holds"
self                      # exit 0: view/census is pending, and the instance asked
self | claude -p | self hear
self view census          # the instance grew a capability it proposed itself
```

The kernel does not distinguish growth a human asked for from growth the
instance asked for. That is deliberate: both are declarations in the log, and
both converge the same way.

Retiring is an event too — no verb, no special path. The script leaves the
surface and every event stays; re-declaring the capability brings it back as
**pending work**, to be authored fresh. A retired script does not silently
return:

```json
{"name":"capability.retired","payload":{"type":"view","name":"journal"}}
```

## Capability scripts

Any language with a shebang; standard library only. The environment is scrubbed:
a fixed `PATH`, `TZ=UTC`, `LC_ALL=C`, `PYTHONHASHSEED=0`, and any `SELF_*`
variable of the caller — which is the documented way to hand a capability
configuration on purpose. Nothing else is inherited, because determinism is a
claim this kernel makes and an inherited `$TZ` or a randomized hash seed
silently breaks it.

The two kinds are told different things, and the difference is the boundary:

- A **command** is an effect on one instance, so it gets `SELF_HOME` and runs
  with the instance as its working directory.
- A **view** is a pure function of its events, so it gets **no path to the
  instance at all**: no `SELF_HOME`, and an empty scratch directory to run in.
  Its whole input arrives on stdin. This is not a sandbox — a script can still
  guess a path — but the kernel does not hand a view the log to read, or to
  write.

**command** — argv is the arguments after `self run <name>`. stdin is the whole
log as JSONL. stdout is new events as JSONL. Exit non-zero and nothing is
appended.

**view** — never ingested: whatever it prints goes to the reader, not the log.
argv is the arguments after `self view <name>`; the view is handed no path to
the instance. stdin is exactly the events its
receipt's `consumes` list names, as
JSONL, in log order (an empty list or `["*"]` means every event). stdout is
opaque bytes: text, HTML, JSON, whatever you like. A view is a **pure function
of those events** — same events in, same bytes out. Do not read the clock, the
network, or anything not on stdin. A view is never materialized to disk; it is
replayed on demand.

## Answering

You are expected to have tools. The prompt is a wake-up card, not a context
dump: read `events.jsonl`, `cap/`, run `self brief`, `self view <name>`,
`self help` before you answer.

- Do durable work through `self run <command> …`, or by printing events.
- To grow the instance, print a declaration and its `script.authored` in the
  same body — test the script by running it before you print it.
- If a pending declaration lists a previous rejection, do not repeat it.
- If there is nothing worth doing, print nothing. Silence is a valid turn.
- Never edit `events.jsonl` and never write into `cap/` yourself. Only a
  kernel-signed receipt installs, and only for a capability this log declared.

## Events

The kernel's own vocabulary — eight names it acts on. Everything else in the
log is a domain event, appended verbatim and interpreted only by views.

| name | who writes it | what it means |
|---|---|---|
| `command.declared` | anyone | a command capability exists, pending a script |
| `view.declared` | anyone | a view capability exists, pending a script |
| `script.installed` | the kernel | a receipt: these bytes are installed, signed under the local key |
| `script.rejected` | the kernel | an authored script was refused, and why |
| `capability.retired` | anyone | a tombstone: off the surface, still in the log |
| `intent.declared` | `self learn` | prose someone brought here: what an account is for |
| `lesson.learned` | the kernel | an attestation: what an account actually deposited |
| `account.given` | `self give` | this instance gave an account away |

`script.authored` is the wire schema above; it is never appended.

An event is `{id, seq, name, occurred_at, via, by, payload}`. Records are never
changed or deleted. A deletion is a later event.

## Provenance

- **`via` — the door.** The channel the kernel *witnessed* the event entering
  through: `cli`, `hear`, `kernel`, or `learn:<account>`. Stamped by the kernel
  from what it saw. Never accepted from a script, a mind, or a record. A door is
  a local fact, like `seq`.
- **`by` — the speaker.** Whatever the caller claimed via `SELF_CALLER`,
  recorded verbatim as a claim and never verified. It is portable, like
  `occurred_at`: testimony keeps its speaker when it travels between instances.

The author claim is signed *into* the receipt, so authorship cannot be
relabelled after the fact.

## Signed installation

A receipt is `{type, name, script, consumes, by, sig}` where `sig` is
HMAC-SHA256 over those fields, domain-separated and length-prefixed, under
`SELF_HOME/.secret` (32 random bytes, mode 0600). Only a receipt that verifies
under the local key ever installs, and only for a capability this log declared
and has not retired. Anything else in the log is inert data.

Installed bytes are content-addressed at `cap/blob/<sha256>`; the readable path
`cap/<type>/<name>/run` is a symlink to that blob. Execution resolves the
latest live verified receipt, checks the blob against its own hash, rewrites it
if it differs, and runs the blob — so a running script's bytes can never change
under it, and a hand-edit is simply overwritten by the log. Editing files under
`cap/` has no effect: the log is authoritative by construction, not by check.

`self rehydrate` makes disk match the log exactly — materializing what live
receipts require, removing what they do not, and garbage-collecting unreferenced
blobs. It needs `events.jsonl` and `.secret` and nothing else: no model, no
network.

Byte-identity is guaranteed for one log and one key: two instances holding the
same two files rebuild identical scripts and render identical views. It is
never guaranteed across two bodies — `id` is random, and `seq` and `via` are
local facts.

## Accounts

An account is the one wire format between instances, and it is a directory of
plain text. **Nothing runnable travels.**

```
account/
  intent.md      the telling: who this is from, what it means, what you hope it
                 becomes (required — intent alone is a valid account)
  record.jsonl   the evidence: events verbatim, moments preserved (optional)
  manifest.json  the attestation: {events, record_sha256, prefix?, capability?}
                 (optional; written by give, advisory on learn)
```

`self give <selector> <dir>` writes one from the log. A selector is an
event-name prefix (`note.`) for the knowledge flavour, or `command/<name>` /
`view/<name>` for the capability flavour — declarations and receipts, renamed
to lineage.

`self learn <dir>` is the only way in, and it splits along the seam:

- **The mechanical half** needs no mind. `intent.declared` records the prose
  first, the record is deposited verbatim next, and `lesson.learned` attests
  last — it must be last, because it hashes what actually landed.
- **The intelligent half** rides the pipe: learn prints the learning prompt, and
  whatever mind you pipe it to reads the intent against local state and declares
  *its own* capabilities, authored and signed locally.

```sh
self learn account/ | claude -p | self hear
```

Four rules keep the exchange honest, all mechanical:

1. **The kernel's vocabulary never travels raw.** `give` renames it to
   `lineage.<name>`; `learn` refuses a record containing it and appends nothing
   at all. Without this a deposited `command.declared` would become pending work
   and the next pass would author and sign an attacker's script under your key.

   The refused set is **frozen**: it holds the eight names above plus every name
   any earlier kernel acted on — `kernel.initialized`, `projector.declared`,
   `script.compiled`, `self.asked`, `self.replied`, `self.reflected`,
   `learn.orchestrated`, `capability.revision.requested`. A name may leave the
   vocabulary; it never leaves this set, because otherwise retiring a name would
   quietly make it depositable and yesterday's vocabulary would become
   tomorrow's injection. If a record you are writing needs one of them, rename
   it `lineage.<name>` — that is what `give` does, and it lands inert.
2. **Moments are preserved.** Deposited events keep their own `occurred_at` and
   their own `by`. The door is re-stamped `learn:<account>`: doors are this
   log's facts, never another body's. A record arriving is history, not news.

   That re-stamped door is also what lets a view tell the difference. A learned
   record lands under the event names this instance already reads, so it changes
   what existing views print — which is the point of learning evidence, and also
   the way a hostile account reaches your pages. A view that should show only
   local testimony filters on `via`; one that should show both makes the
   provenance visible. Deciding which is the receiving mind's job, not the
   kernel's.
3. **Interventions are visible.** `lesson.learned` records the sha256 of the
   record file **as read** beside what the manifest claimed. Deleting a line
   before learning is legitimate curation — and it shows, in both logs, forever.
   It is a hash of bytes, so a transport that rewrites line endings also shows
   up as a divergence: the honest reading of a mismatch is "this file is not the
   file that was given", not "someone removed a line".
4. **Only the local key installs.** An account cannot install anything, ever.
   It can only be read.

Giving is cheap; learning is the work. That asymmetry is the Self Protocol.

## The CLI

Every verb names a different primitive. There is no sugar: no `ask`, no
`reply`, no `author`, no `retire` — those are the wire.

```
self                        situate the naked default ask and full instance brief (READ)
self <ask…>                 situate that ask (READ)
… | self hear               hear: event lines land, scripts install (WRITE)
self brief                  the state card: what exists, what is pending, what broke
self run <cmd> [args…]      execute a command capability
self view <name> [args…]    replay a view to stdout ("log" is built in, shadowable)
self loop [opts] -- <mind>  run situated turns to an unchanged-state fixed point
self learn <dir>            deposit an account, print its learning prompt
self give <sel> <dir>       write an account from the log
self rehydrate              make cap/ match the log exactly
self help                   this file
```

## The loop

Capability work and domain work are both state a mind discovers by exploring
the brief and its views. The kernel does not decide which state counts as work.
Bare `self` therefore always prints the situated surface, even when no
declaration is pending.

`self loop` drives that surface to an append fixed point:

```sh
self loop -- claude -p
self loop -- pi --provider github-copilot --model gpt-5.6-luna --no-session -p
```

Each pass gives the mind a naked situated prompt, hears its stdout through the
normal write door, and then checks authoritative state internally. It repeats
after any append — whether the append came back on stdout or a tool-capable mind
called `self run` itself — and stops after the first complete turn that leaves
the log unchanged. Users do not hash or inspect the log; witnessing change is
kernel work. The loop knows nothing about goals, tasks, or declarations.

The mind command is required after `--`; there is no resident or default model.
Driver policy is explicit and generic:

```sh
self loop --max-passes 12 --timeout 30m -- <mind> [args…]
```

An optional first-pass ask directs attention without teaching the kernel domain
semantics. It is presented only on pass one; if that turn changes state, later
fresh passes return to the naked default ask and settle what changed:

```sh
self loop --ask 'advance backoffice-preprod-proof' -- <mind> [args…]
```

The mind is executed directly, not through a shell. It inherits the caller's
working directory and environment (including `SELF_HOME` and `SELF_CALLER`),
receives the situated prompt on stdin, and returns the ordinary event wire on
stdout. Mind stderr and loop progress go to stderr; `hear` output goes to the
loop's stdout. Use a shell explicitly only when shell syntax is intended:

```sh
self loop -- sh -c 'my-mind --flag'
```

For a pinned local setup, environment defaults make the short form complete:

```sh
export SELF_LOOP_MIND='pi --provider github-copilot --model gpt-5.6-luna --no-session -p'
export SELF_LOOP_ASK='advance the one goal I selected'
export SELF_LOOP_MAX_PASSES=12
export SELF_LOOP_TIMEOUT=30m
self loop
```

`SELF_LOOP_MIND` is necessarily a shell command string and runs through
`sh -c`; explicit argv after `--` is safer and takes precedence. CLI
`--ask`, `--max-passes`, and `--timeout` likewise override their environment
defaults.

Reaching the pass cap while state still changes, a mind failure, a timeout, or a
hear failure exits non-zero. A converged fixed point exits zero.

## Exit codes

`0` did the thing. `1` did not. Bare orientation is always a successful read;
`self loop` reports convergence or failure rather than overloading an exit code
with the kernel's opinion about whether domain work exists.

## Environment

```
SELF_HOME    the instance: a directory holding events.jsonl and .secret
             (default: the current directory; pin one in your shell rc)
SELF_CALLER  your claim, recorded verbatim as `by` on events you cause and
             signed into the receipts of scripts you author
SELF_LOOP_MIND          default shell command for `self loop` when `--` is absent
SELF_LOOP_ASK           default explicit objective for pass one only
SELF_LOOP_MAX_PASSES    default loop pass cap (12 when unset)
SELF_LOOP_TIMEOUT       default per-mind timeout (30m when unset)
```

Any other `SELF_*` variable is passed through to capability scripts.

## Lineage

This kernel begins a new log lineage. It does not read the previous kernel's
receipts: `script.compiled` is not a name it acts on, and the signature is
domain-separated even where it is. What that means for a v1 `events.jsonl`,
verified rather than assumed:

- Every event still **reads**. `self view log` shows the whole history, domain
  events included, with their moments and speakers intact.
- Every v1 `command.declared` appears as a **pending declaration**, because its
  receipt no longer verifies. So the loop offers to re-author it, locally and
  under this key — a migration that runs itself.
- v1 `projector.declared` events are invisible here: the kind was renamed to
  `view`. Re-declare those as views.

The clean path for a grown instance is the one anything else travels: `self
give` under the old kernel, `self learn` under this one. That is not a
workaround — the protocol being its own migration path is the claim.

## Limits, stated plainly

- **The mind is driven by the log, and the log can contain untrusted input.** A
  learned account or a crafted event becomes context for the mind that writes
  your scripts. There is **no human review step between authoring and signing**:
  piping a mind's output into `self` signs whatever it authored. Hold the output
  in a file and read it first if you want that pause; nothing enforces it.
- **The write door trusts its caller.** Anyone who can run `self` against your
  `SELF_HOME` can declare and install — the same power they already have holding
  the directory and its key. The signed-receipt gate is about reconstruction and
  foreign accounts, not local privilege.
- **Nothing is sandboxed.** The environment is scrubbed for determinism, not
  containment. Installed scripts run as you.
- **The log is unbounded.** Every install stores its script bytes; every view
  replays its events. Compaction is not in the kernel; a snapshot is a
  capability someone can grow.
- **Appends are locked; operations are not transactions.** One `hear` body is
  one critical section, but a command that emits several events and a
  concurrent operation can still interleave.
- **Capability scripts are not timed out.** A capability that hangs still hangs
  its invocation. `self loop` times out the external mind process because that
  is explicit driver policy; it does not impose timeouts on capabilities the
  mind invokes.
- **The last line of the log is judged by whether it is a whole event.**
  Terminated by a newline, it is a record. Unterminated but parsing as an event,
  it is also a record — complete, missing only its terminator, which is what an
  editor that strips trailing newlines leaves behind — and the next append gives
  it one. Unterminated and unparseable, it is a torn write: skipped, and dropped
  by the next append, which says so. Real corruption further up is an error
  naming the line, never a silent skip: the log is authoritative, so it is not
  the kernel's place to decide which of your committed records to ignore.
- **A command's runtime failure is not an event.** Its exit code and stderr are
  the caller's to see; nothing is appended, so a failing command leaves no trace
  in the log. Only a refused *authoring* attempt becomes state.
