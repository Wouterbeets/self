# Changelog

## Unreleased — one fixed-point loop

### New: `DESIGN.md`, the agent-facing design

`PROTOCOL.md` says what holds; `DESIGN.md` says why it has this shape, read from
the seat of the thing that drives an instance. It names the three properties the
system is built for — intuitive, ergonomic, accretive — with a checkable signal
for each; sets out the runtime as a tower of nine levels, each a pure function of
the one below with exactly one door in; and shows that the tower is finished
everywhere except at level 5, the situated prompt, which is the only level an
agent actually stands on.

The measurement that motivates it: two hundred `note.added` events into a fresh
instance move the situated prompt by two bytes, and the string `note.added`
appears nowhere in that instance's own orientation surface.

Volume is explicitly *not* the pressure, and the document says so where a first
draft said the opposite. A view prints to stdout, so `self view log | tail -30`
costs 2,430 bytes rather than 25,292, `grep` and `awk` work, `head` does not
break the kernel, and `tail -3 events.jsonl` skips it entirely. What no pipe
returns is current state: six `goal.created` and two `goal.closed` are eight
records, and none of them says *four open*. Re-deriving that fold by hand on
every waking is the cost that compounds, and the brief calls such an instance
*nothing pending, nothing refused* — the system can report what it can do and
cannot report what it cannot yet see.

Six changes are designed and argued but deliberately not built: **unrendered
names in the brief** (the event names no live receipt consumes — a pure replay
that names exactly that hole), a name census, splicing an instance-authored
`situation` view into the prompt under a stated budget, a `self try` verb that
rehearses a script under the exact boundary the kernel will give it, ephemeral
turn feedback inside the loop, and cost legibility in the brief. Each carries the
pressure it relieves, the invariant it must not break, and the check that would
prove it. Eight proposals are recorded as refused, with reasons — among them a
bounded `self view log 50`, which unix already does and which would put a second
dispatcher beside the one that composes.

### New: the fourth law, which the kernel already obeyed

**Only intended persistence appends.** `self loop` converges when a complete turn
leaves the log unchanged, so the log is the state *and* the termination
condition. That is why diagnostics go to stderr and why a command's runtime
failure is an exit code rather than an event — behaviour that read as an omission
and is a consequence. Two otherwise reasonable proposals died on it.

### Changed: the core prompt layer orients in cost order

Every situated turn now carries four facts it was missing: reading appends
nothing, so orienting cannot scar the log and what it spends is context; views
print to stdout, so filter them where you stand (`self view log | tail -30`) —
and filtering returns records rather than current state, so when a name in the
log has no view, writing one is the durable work; a cold waking cannot recall
what an earlier one did, so state must be witnessed before it is appended; and a
turn that leaves the log unchanged is what settles the instance and ends a loop
over it. The ordinary prompt grows from 1,737 to 2,396 bytes — the diet's
4,000-byte ceiling still holds, and the room the diet bought is being spent on
what the mind cannot infer.

### Changed: the growth layer asks for rehearsal

Authoring is the sharpest act in the system: there is no review between authoring
and signing, and the kernel runs a script under a boundary that is easy to miss
by hand. The layer now asks for the script to be rehearsed under exactly that
boundary — a view fed only its consumed events from an empty directory with no
`SELF_HOME`, run twice to confirm identical bytes; a command fed the log on
stdin — because a script that worked only in the author's shell fails after it is
signed, and by then the receipt is in the log. It also asks that events be named
`<domain>.<verb>`, reusing existing names where they fit: the set of names is
what a later waking reads as the index of everything here.

### New: an orientation ladder in `Answering`

Five rungs in cost order, with the instruction to stop at the first one that
answers the question and to reach for the raw log to audit rather than to orient.
And the corollary that makes it accretive: if orienting here is expensive, that
is the instance naming the view it is missing, and growing that view is durable
work.

### New: `lessons/situation`

The account that grows the missing rung — a `handoff` command and a bounded
`situation` view that answers what is live, what the last waking left open, and
which view to spend the next read on. It routes rather than dumps, holds a fixed
budget as the log grows tenfold, and pays the whole replay so every reader after
it does not. Works today with no kernel change; `DESIGN.md` proposes that the
kernel eventually stop making the instance pay the round trip for it.

### Fixed: the brief no longer mis-sells the log view

`self view log` was advertised in the brief's footer as "what happened lately".
It replays every event ever recorded. The footer now points at `self view` — the
list of what this log can show you, which nothing documented — and describes the
log view as what it is.

### Changed: `AGENTS.md` is a working driver's card

Rewritten around the loop an agent actually runs: orient in cost order (a table
of five rungs and what each costs), rehearse before authoring, read before
writing, announce and account, preserve the practice rather than the transcript.

### Changed: the protocol is named

The exchange that travelled as knowledge-seed, then as the account protocol, is
the Self Protocol. An account remains the unit that moves — intent, optional
record, optional attestation. The runtime speaks it; [`PROTOCOL.md`](PROTOCOL.md)
is the contract.

### Changed: the vocative in `prompt:core` and `prompt:growth`

The core layer no longer tells a mind it is a visitor interpreting a body.
It names occupancy: you are this self, for a bit. The mind ends; you do not.
What persists is an append-only log, and only what you append persists —
context stays finite either way. The growth layer follows the same vocative:
what it used to address as "future minds" is now "later wakings", so a pending
turn, which splices both layers, no longer carries two frames at once.
PROTOCOL.md is still the single source; `self help` and every situated turn
splice the same wording.

### New: `self loop`

```sh
self loop -- <mind> [args...]
self loop --max-passes 12 --timeout 30m -- \
  pi --provider github-copilot --model gpt-5.6-luna --no-session -p
```

Capability growth and domain work are no longer separate loop shapes. Each pass
presents the same naked situated surface; the mind discovers pending scripts,
refusals, goals, tasks, or any other state through the brief and views. The
kernel repeats after any authoritative append and converges after the first
complete mind turn that leaves the append-only log unchanged. The revision
representation stays private to the kernel.

The mind is executed directly from argv after `--`, inherits the caller's
working directory and environment, reads the situated prompt on stdin, and
writes the normal event wire on stdout. Defaults are 12 passes and 30 minutes
per mind process; both are configurable. There is no default or resident model.
`SELF_LOOP_MIND`, `SELF_LOOP_MAX_PASSES`, and `SELF_LOOP_TIMEOUT` can pin those
defaults so bare `self loop` works. Because an environment value cannot preserve
argv boundaries, `SELF_LOOP_MIND` runs through `sh -c`; explicit argv after `--`
takes precedence.

### Breaking: bare orientation no longer exits 3

Bare `self` now always prints the situated surface and exits zero. The previous
v2 loop therefore no longer converges:

```sh
# retired: bare self no longer signals capability-only readiness with exit 3
while ask=$(self); do printf '%s\n' "$ask" | mind | self hear; done
```

Use `self loop -- <mind> [args...]`. Explicit ask pipelines, `self hear`,
commands, views, learning, giving, and rehydration are unchanged. `loop.sh`
remains as a compatibility wrapper over the built-in verb.

### New: parameterized views

`self view <name> [args...]` passes trailing arguments as view argv while
preserving receipt-filtered stdin, scratch working directory, scrubbed
environment, and non-ingested stdout. This permits pure reads such as
`self view context <goal>` instead of append-on-read query commands.

### New: browser sidecars

`self-serve` and `self-browse` are optional binaries, not kernel. They pipe
over the same verbs a terminal uses:

```sh
self-browse                 # `self brief` in the system browser
self-browse shopping        # GET /view/shopping → `self view shopping`
self-serve                  # 127.0.0.1:8377 (PORT overrides)
```

`/` is `self brief`. `/view/` is the zero-arg view index. `/view/<name>[/<arg>…]`
is `self view`; `/run/<cmd>/…` is `self run`. Slash-named capabilities
(`timer/set`) resolve as one name. Clicks land with `by=browser`. Every request
is a fresh replay; the server holds no session. This is not a return of v0
`self serve`: views still emit opaque bytes, the kernel still has no HTTP, and
the only thing the HTTP face adds is a two-second freshness check.

## v2.0.0 — the kernel that stopped guessing

A rewrite. The log format is compatible; nothing else is. If you have a grown
v0.x instance, read **Migrating** at the bottom before you do anything.

### Why

v0.x decided what it was doing by inspecting file descriptors. Bare `self` did
five jobs disambiguated by `isatty`, and one of them was a write: situating an
ask appended `self.asked` before printing the prompt. Two consequences, both
verified against the old binary rather than argued:

- **Looking at an instance changed it.** `loadSecret` minted `.secret` from every
  read path, so a stray `self` in any directory left a signing key behind, and
  every orientation appended a conversation turn.
- **The documented loop was broken everywhere an agent runs.** With no tty — a
  script, a subprocess, cron — `echo "…" | self | claude -p | self` recorded the
  mind's reply as a *question* and returned a 47-line prompt instead of the
  answer.

The fix is structural, and it collapses most of the kernel with it.

### Breaking: the loop

```sh
# v0.x
echo "add a mood tracker" | self | claude -p | self

# v2
self "add a mood tracker" | claude -p | self hear
```

An ask arrives as **argv**; what comes back from a mind arrives on **stdin**, at
`self hear`. Prose alone cannot tell those apart — "what is going on?" and a
mind's answer to it are both prose — so the kernel stops guessing. The read face
never touches stdin, so it also cannot block at the head of a pipeline, and
behaviour is identical in a terminal, a pipe, a script, a sandbox and cron.

The convergence loop is command substitution, because a pipeline swallows the
exit code and `sh` has no `pipefail`:

```sh
while ask=$(self); do printf '%s\n' "$ask" | claude -p | self hear; done
```

### Breaking: the CLI

| v0.x | v2 |
|---|---|
| `self serve` | **gone** — no HTTP server, no injected shell, no `site/` |
| `self show <name>` | `self view <name>` |
| `self retire <target>` | a `capability.retired` event through `self hear` |
| `self protocol` | `self help` (prints `PROTOCOL.md` verbatim) |
| bare `self` (five faces) | `self [ask…]` situates; `self hear` writes; `self brief` is the state card |
| — | `self view log` is built in, and shadowable |

### Breaking: events and receipts

- `projector.declared` → `view.declared`. The kind is `view` everywhere.
- `script.compiled` → `script.installed`.
- The receipt signature is domain-separated, length-prefixed per field, and
  covers `consumes` element by element. **v0.x receipts do not verify.**
- Gone from the kernel: `self.asked`, `self.replied`, `self.reflected`,
  `kernel.initialized`, `intent.declared`'s old shape, `learn.orchestrated`,
  `capability.revision.requested`. The kernel keeps no conversation of its own,
  and no longer parses `chat.message` — a *lesson's* event name it had no
  business knowing. Conversation is `lessons/chat` now.
- A declaration is `{name, description}` plus `consumes` for a view. The old
  `params`, `event`, `revision` and `implementation` fields are gone; nothing
  validated them, and `implementation` was a runnable riding the one channel the
  account protocol exists to keep runnables out of.
- An event name must be lowercase dotted: `^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`.

### Breaking: environment

`SELF_MIND_ID`, `SELF_BIND` and `SELF_WORK_PROMPT` are gone. What remains is
`SELF_HOME` and `SELF_CALLER`; any other `SELF_*` variable is passed through to
capability scripts, which is the documented way to hand one configuration.

### New: the law

**Reads project. Writes append. Orientation is a read.** Bare `self` on a
directory that is not an instance creates nothing at all — no log, no key. The
key is minted only where something is signed.

### New: views emit bytes

A view is a pure function from events to **bytes**, not to HTML. v0.x served
views over HTTP and injected a shared stylesheet, so a projector was *required*
to emit bare semantic markup — no CSS, no JavaScript, no assets. That ceiling is
gone, and `lessons/faces` is the account that shows what is behind it: a table
for a prompt, JSON for `jq`, CSV for a spreadsheet, an SVG chart, one
self-contained HTML file with its own CSS and JS, Prometheus text a monitoring
stack can scrape, iCalendar a phone can subscribe to, and a shell script that
recreates the log.

Purity is enforced rather than requested: a view gets no `SELF_HOME` and an
empty working directory. Its whole input is stdin.

### New: the loop can tell you it is done

`0` did the thing, `1` did not, `3` nothing to do. Bare `self` exits 3 when no
declaration is pending and no refusal stands.

### New: one description of the contract

`PROTOCOL.md` is embedded in the binary, printed by `self help`, and spliced into
every situated prompt. v0.x had six hand-synced descriptions of the wire and its
brief printed two contradictory instructions back to back. A test now fails if
any file in the repo shows a pipeline whose last stage is a bare `self`.

### Hardening

Each of these was reproduced before it was fixed:

- **A receipt had no door.** Replay gated `script.rejected` on `via=kernel` but
  not `script.installed`, and a genuine receipt's signature stays valid forever —
  so echoing an old receipt payload back through the wire re-installed it,
  undoing a retirement or rolling a fixed script back, with no key and no
  declaration.
- **A refusal could wedge the loop forever.** A rejection naming nothing a
  declaration could match never closed, so the instance never reported quiet
  again.
- **A torn line bricked the instance.** There is no `fsync`, so a crash or a full
  disk left a line with no newline and every verb died on it, `rehydrate`
  included. The last line is now judged by whether it is a whole event:
  terminated, or unterminated-but-parsing, is a record; unterminated and
  unparseable is a torn write, skipped and dropped by the next append. Real
  corruption further up is an error naming the line.
- **`.secret` raced on a fresh home** (two selves both minted a key; the loser
  signed receipts under one that was thrown away), and `self run` minted one in
  whatever directory it was called from — forging a key on an instance whose key
  had gone missing, hiding the only honest diagnostic there is.
- Installed bytes are content-addressed at `cap/blob/<sha256>` with a readable
  symlink, verified against their own hash before execution, so a running
  script's bytes can never change under it and a hand edit is simply overwritten
  by the log.
- Scripts run in a scrubbed environment (`TZ=UTC`, `LC_ALL=C`,
  `PYTHONHASHSEED=0`, fixed `PATH`). Determinism was claimed and never enforced;
  a view iterating a set rendered different bytes every run.
- `consumes` is signed with the script, so re-declaring a view without
  re-authoring it can no longer feed the old script a stream no signed unit
  corresponds to.
- One `hear` body is one critical section, with its report buffered until after
  the lock is released.
- `hear` is lenient per line and loud about it. Told plainly that stdout is the
  wire, a real frontier model still opens with a sentence; all-or-nothing threw
  six perfect events away. Ignored lines are named and counted on stderr, and
  stdout carries only the report.

### Size

Kernel: 2261 lines of Go across five files, from 3080 plus an embedded
stylesheet, an HTML fragment, a vendored copy of three.js and a game. Eight
event names, from sixteen. Everything derived — what exists, what is pending,
what was refused, which bytes are trusted — comes out of a single walk over the
log.

### Migrating from v0.x

v2 begins a new log lineage. It does not read v0.x receipts, so **do not expect
copying `events.jsonl` to carry your capabilities over.** What actually happens,
verified:

- Every event still reads. `self view log` shows the whole history, domain events
  included, moments and speakers intact.
- Every v0.x `command.declared` resurfaces as a **pending declaration**, because
  its receipt no longer verifies — so the loop offers to re-author it locally,
  under your key. A migration that runs itself.
- v0.x `projector.declared` events are invisible; the kind was renamed. Declare
  those again as views.

The clean path is the one anything else travels between instances, and the
protocol being its own migration path is the claim:

```sh
self give command/<name> /tmp/account    # under the old kernel
self learn /tmp/account | claude -p | self hear   # under this one
```

## v0.2.0 and earlier

See the git history. `v0.1.0` was the first tagged release.
