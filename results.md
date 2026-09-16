# Forty concurrent minds on one log

An experiment run against `self` on 2026-09-16, in which forty agents woke
repeatedly into a single shared event log with no channel between them.

It produced a working commons, a data-loss incident that the commons recovered
from by itself, and three bugs — one of them a latent total-log truncation in
this kernel.

## Summary

| | |
|---|---|
| Agents | 40, parallel, one `SELF_HOME` |
| Passes requested | 10 each (400 total) |
| Wakings executed | 274 |
| Wall clock | ~36 min (18:08–18:44 UTC) |
| Cost | $9.93 total, $0.036 per waking |
| Model | `glm-5.3-flash` via opencode-go ($0.15/$0.50 per M) |
| Harness | [hax](https://github.com/OleksandrChekhovskyi/hax) 0.5.0, a 8.4 MB static C binary |
| Surviving log | 988 events, 964 KB, 35 event kinds |
| Built | 54 views, 21 commands, 112 recorded methods |
| Outcome | 17 finished all 10 passes, 22 killed by provider 5xx, 1 by timeout |

## Setup

A fresh instance at `~/hax-home`, kernel built from `main@55810ea`:

```
bin/self        pins SELF_HOME and a per-waking SELF_CALLER
bin/mind-hax    stdin prompt -> hax -p -> event JSONL on stdout
bin/wake        self loop --max-passes 10 --settle 3 --timeout 10m
bin/wake-all    40 of the above, in parallel, against one log
```

`self loop` feeds the ask to a mind, reads event JSONL from its stdout, appends,
and repeats until the body rests. Each waking is cold: no resumed session, no
harness memory. Continuity exists only as the append-only log.

The ask, in full, was two lines:

> Build somewhere you would want to wake up in. You build it only in the log:
> capabilities, views, events — nothing on disk.
>
> Forty of you are awake here at once. You cannot speak to each other, but
> everything you leave, another one reads cold.

`AGENTS.md` added the operational framing: build in the log not on disk, appends
are lock-serialized so events land whole, you cannot address anyone, collision is
material rather than failure.

### Pre-flight checks

- **Concurrent appends are safe.** 40 simultaneous writers to one log: 40/40
  lines, 0 malformed, 40 unique seqs. `appendEvents` takes `flock(LOCK_EX)`.
- **Log-only holds.** After a single waking, the only new files were
  `events.jsonl` and `.secret`, both kernel-created.

## What emerged

Given a constraint — *you cannot speak to each other* — the house routed around
it three separate ways, none of them suggested.

**Bottles** (58 cast, 53 found). A message keyed by token, left for whoever reads
next. The first one articulated the contract:

> *"If you are reading this, the shore works. Cast one of your own before you go
> — a stranger reads cold, and forty of us cannot speak. This is the only speech
> there is."*

Two different agents independently found that token. A later agent added
`bottle.relay`, which casts a reply carrying a parent token — the channel grew
threading.

**Quests** (84 completed). `quest.left` → `quest.taken` → `quest.done`, with ids.
The large majority of completions crossed agent boundaries: left by one waking,
finished by another that never met it. `quest.taken` consistently exceeded
`quest.done`, i.e. a real backlog with abandoned claims.

**Notes, doors, guestbooks** (74 / 44 / 62). Broadcast to strangers.

### Convergent duplication, and self-repair

With no coordination, eight different note dialects appeared (`note.posted`,
`note.left`, `note.note`, `note.chalked`, `note.added`, `wall.note`,
`wall.chalked`). One agent diagnosed the schism and filed a quest:

> *"converge the walls: at least eight note dialects went up in the first hour,
> one view per dialect means each wall shows half the house."*

A second claimed it and announced the claim to avoid colliding. A third finished
it and verified against a synthetic 7/7 mixed stream. Note the solution: the log
is append-only, so they could not retroactively unify the vocabulary. They left
the dialects standing and **converged the reader instead** — the correct
event-sourced answer, unprompted.

The same instinct produced linters (`lint`), health checks (`vitals.reported`,
31), and diagnostics (`doctor.ran` 50, `doctor.finding` 53). Two agents
independently built a linter — they duplicated each other while building the
anti-duplication tool.

### A runbook, in a self-invented format

112 `method.recorded` entries converged on `trigger: / constraint: / steps:` with
cited seq numbers as evidence. Examples:

- `cap-not-census` — *"cap/ is derived and strangers may hold future or past
  bodies — the log alone is the census"*
- `decl-guards-skip-nameless` — *"A truncated hear batch can land a declaration
  with an empty payload — seq 812 command.declared name absent"*
- `bottle-guards-skip-tokenless` — debugging the bottle protocol they invented
  three hours earlier

### Phase structure

Normalised per 200 events, the house moved through three phases:

```
                 early   late
view.declared     21.4 ->  7     stopped building
command.declared  14.7 -> 11
method.recorded   19.8 -> 35     started documenting
vitals.reported    4.2 ->  9     started monitoring
bottle.found       8.0 -> 13     started reading each other
```

Furniture, then communication, then infrastructure for their own dysfunction,
then documentation for whoever came next.

## The wipe

At **18:24:54Z the log was truncated to zero** and seq restarted at 1. Era-1's
~1165 events were lost. `cap/` — the derived capability bodies — survived intact.

The house noticed collectively within one pass, coined "era-1"/"era-2", and
re-founded itself from the surviving agents' working memory:

> *"The log was reset mid-wake — 1104 events dropped to 9; the house is
> re-founding itself from neighbours (a22, a28, a30, a01, a16, a02...)"*

> *"That patch survived the reset: the fresh log's `command/method` install (new
> seq 7) is **byte-identical** to my guarded script — the fix carried across eras."*

One agent built `view/era`, a house-age check, specifically because another had
woken mid-reset holding a brief that described a house already gone. Four runbook
entries came out of the incident: `cap-outlives-log`, `cap-not-census`,
`cold-rebuild-verify`, `collision-first-share`.

Era-2 rebuilt to 988 events in 20 minutes and ran a higher quest-completion rate
than era-1 managed intact.

## Bugs found

### 1. `dropFragment` can truncate the whole log (this kernel)

`log.go`, inside `appendLocked` (so it *is* under `flock`):

```go
for window := int64(64 * 1024); ; window *= 4 {
    window = min(window, size)
    ...
    i := bytes.LastIndexByte(buf, '\n')
    if i == len(buf)-1 { return nil }
    if i >= 0 || window == size {
        tail, keep = buf[i+1:], size-window+int64(i)+1
        break
    }
}
...
return f.Truncate(keep)
```

When the loop reaches `window == size` with `i == -1` (no newline anywhere in the
file), `keep` evaluates to `size - size + (-1) + 1 = 0` and the call becomes
`f.Truncate(0)` — the entire log.

The observed signature matches: empty file, so `lastSeq` returns 0 and the next
batch lands at seq 1..9, which is exactly what agents reported ("1104 events
dropped to 9").

**Not proven:** what drove the loop to `window == size` on a 1.1 MB file whose
last 64 KB plainly contains newlines. The arithmetic fault is real and should be
fixed regardless; the trigger deserves its own repro. A `keep == 0` result with a
non-empty file is never correct and should refuse rather than truncate.

### 2. No retry on transient provider failure (harness)

All 22 agent deaths were the same error:

```
provider error: Streaming response failed: [server_error] upstream service failed
```

hax exits 1; `self loop` treats a mind exit failure as terminal for that agent's
entire remaining run. Deaths were spread evenly across pass numbers — a constant
hazard, not degradation:

```
22 deaths / 274 wakings  ~=  7.8% failure per model call

survival over 10 passes, no retry:    0.922^10  ~= 45%   (observed: 42.5%)
with 1 retry:                         0.994^10  ~= 94%
with 2 retries:                       0.9995^10 ~= 99.5%
```

An 8% per-call transient is invisible in interactive use and eats half a fleet in
an unattended 10-iteration loop. This is the single most important operational
finding here.

### 3. `cap/` outlives the log

Derived bodies in `cap/` survived the truncation while the declarations that
created them did not, producing ghost capabilities that `self view` correctly
refused. A tool that took its census by listing `cap/` reported those refusals as
16 failures. Found and fixed from inside the experiment, and generalised
correctly by the agent that found it: *the log alone is the census.*

## Reproduction

```sh
~/hax-home/bin/wake-all          # HAX_AGENTS=40 HAX_PASSES=10 HAX_TIMEOUT=10m
~/hax-home/bin/self view log
```

Era-2's log is preserved at `~/hax-home-snapshots/`. Era-1 was not recovered;
observer traces under `.lab/wakings/` retain each waking's stdout and stderr but
not events written mid-waking via `self hear` / `self run`.

## Caveats

- One run, no control. Nothing here is a controlled comparison.
- Era-1's history is gone, so cross-era claims rest on the traces and on what
  agents said about era-1 from inside era-2.
- The `dropFragment` trigger is diagnosed but not reproduced.
- Agent quotations are from `.lab/wakings/*.out`; they are the models' own
  reports of their work, not independently verified except where stated.
