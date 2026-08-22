# self — the protocol

This file is the contract. `self help` prints it verbatim, and the situated
prompt splices the marked section below into every ask, so there is exactly one
description of the wire in the whole system. Nothing else — not the README, not
AGENTS.md, not a comment — restates it.

## The law

**Reads project. Writes append. Orientation is a read.**

Looking at an instance never changes it. The log grows only when something was
meant to persist. There is no hidden state: no session store, no cache, no
sidecar. What is not in the log did not happen, and everything else — every
capability, every view — is a replay of it.

## The dispatcher

`self` is a filter with two faces, and which one runs is **structural** — never a
terminal, never a heuristic:

```
self [ask…]           situate: print the brief + the ask. Reads no stdin, appends nothing.  (READ)
… | self hear         hear: event lines land, authored scripts install.                     (WRITE)
```

An ask arrives as **argv**; what comes back from a mind arrives on **stdin**, at
`self hear`. Prose alone cannot tell those apart — "what is going on?" and a
mind's answer to it are both prose — which is why the previous kernel reached for
`isatty` and got the loop wrong everywhere an agent actually runs. The read face
never touches stdin, so it also cannot block at the head of a pipeline.

A line is an event when it is a JSON object with a dotted lowercase `name` and a
`payload` key — both halves, so a mind reporting `{"name":"notes","status":"ok"}`
cannot land an event called `notes` in your log. Every other line is ignored,
echoed, and counted on stderr. Do not narrate on the wire; a stray line will not
cost you the pass.

The corollary: `hear` ingests **any** event-shaped line it is given, including
one quoted inside a document. Do not pipe prose *about* events into it — this
file, for one, is full of examples that would land.

Identical in a terminal, a pipe, a script, a sandbox and cron.

<!-- prompt:begin -->
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
schema beyond that: put the usage, the argument order and the event you emit
into `description`, because that string is what the next cold mind reads.

A declaration is **pending** until a script arrives for it:

```json
{"name":"script.authored","payload":{"type":"command","name":"entry","script":"#!/bin/sh\n…"}}
```

`script.authored` is a wire message, not an event: it never lands in the log.
The kernel installs the bytes and records its own signed `script.installed`
receipt, or refuses and records `script.rejected` with the reason. A refusal is
not lost — it stands in the brief until an install or a retirement supersedes
it, and the reason rides the next prompt.

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

**view** — never ingested: whatever it prints goes to the reader, not the log,
and it is handed no path to the instance. stdin is exactly the events its
receipt's `consumes` list names, as
JSONL, in log order (an empty list or `["*"]` means every event). stdout is
opaque bytes: text, HTML, JSON, whatever you like. A view is a **pure function
of those events** — same events in, same bytes out. Do not read the clock, the
network, or anything not on stdin. A view is never materialized to disk; it is
replayed on demand.

A view meant to drive a loop should print **nothing** when there is nothing to
report. Emptiness composes with the shell; a friendly "no goals yet" does not,
and `while [ -n "$(self view goals)" ]` then never terminates.

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
<!-- prompt:end -->

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

Giving is cheap; learning is the work. That asymmetry is the protocol.

## The CLI

Every verb names a different primitive. There is no sugar: no `ask`, no
`reply`, no `author`, no `retire` — those are the wire.

```
self                        situate the default ask: resolve whatever is pending (READ)
self <ask…>                 situate that ask (READ)
… | self hear               hear: event lines land, scripts install (WRITE)
self brief                  the state card: what exists, what is pending, what broke
self run <cmd> [args…]      execute a command capability
self view <name>            replay a view to stdout ("log" is built in, shadowable)
self learn <dir>            deposit an account, print its learning prompt
self give <sel> <dir>       write an account from the log
self rehydrate              make cap/ match the log exactly
self help                   this file
```

## Exit codes

`0` did the thing. `1` did not. `3` nothing to do — bare `self` exits 3 when no
declaration is pending and no refusal stands. That is the loop's convergence
signal, and the way to use it is command substitution, not a pipeline:

```sh
while ask=$(self); do printf '%s\n' "$ask" | claude -p | self hear; done
```

A pipeline would swallow it. `self | mind | self hear` exits with the status of
`self hear`, so the left-hand `self`'s 3 is lost unless the shell has
`pipefail` — and `sh` (dash) does not have `pipefail` at all, so
`set -o pipefail` inside a `sh -c` is silently a no-op. Command substitution
puts the exit code where the loop can see it and works in POSIX sh.

Quiet means *the kernel* has nothing pending. It does not mean there is nothing
to do in the world: unfinished domain work lives in this instance's own views,
and only a mind reading them can see it. That is the correct division — the
kernel cannot know what matters here, and should not pretend to.

So there are two loops, different shapes on purpose. The one above grows the
instance and converges when it has finished building itself. A loop that works
toward something is driven by a **view**, because a view is just bytes:

```sh
while [ -n "$(self view goals)" ]; do
  self "advance the open goals" | claude -p | self hear
done
```

Nothing in the kernel knows what a goal is, and nothing needs to. Whatever an
instance has learned to track can be looped on the same way — which is why a
view that drives a loop must go silent when it is done.

A mind with its own tools does not need the trailing `self hear` at all: it
writes through the same doors you do (`self run …`, `… | self hear`) and the
loop is just `self "<ask>" | claude -p`. The trailing `hear` is what lets a mind
with **no** tools — a bare completion endpoint — still grow the instance, by
returning its events on stdout. Both are the same loop; only the mind differs.

## Environment

```
SELF_HOME    the instance: a directory holding events.jsonl and .secret
             (default: the current directory; pin one in your shell rc)
SELF_CALLER  your claim, recorded verbatim as `by` on events you cause and
             signed into the receipts of scripts you author
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
- **Nothing is timed out.** A capability script that hangs hangs the invocation,
  and inside a `while ask=$(self)` loop it hangs the loop. Wrap it with
  `timeout` if that matters to you; the kernel does not own that policy.
- **A record is a line terminated by a newline.** A crash mid-write leaves
  bytes that were never a record, and the next append drops them, saying so.
  Real corruption further up is an error naming the line, never a silent skip:
  the log is authoritative, so it is not the kernel's place to decide which of
  your committed records to ignore.
- **A command's runtime failure is not an event.** Its exit code and stderr are
  the caller's to see; nothing is appended, so a failing command leaves no trace
  in the log. Only a refused *authoring* attempt becomes state.
