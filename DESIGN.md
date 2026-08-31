# self — the agent-facing design

[`PROTOCOL.md`](PROTOCOL.md) is the contract: what holds today, in one place,
printed verbatim by `self help`. This file is the other half — *why the system
has this shape*, read from the seat of the thing that actually drives it, and
what is still owed. Nothing here is a promise the kernel already keeps. Every
item is marked:

- **holds** — true of the code in this repository today.
- **designed** — argued for, not built. It states the pressure, the smallest
  change, the invariant it must not break, and the check that would prove it.
- **rejected** — considered and refused, with the reason, so it is not
  re-litigated by the next mind to have the same idea.

A design document that a future waking must re-derive is a design document that
failed. This one is written to be read cold, once, by an agent with no memory of
writing it.

## The three properties

The system is built for a reader that arrives with no history, works for one
bounded turn, and vanishes. Three properties decide whether that reader can do
good work. Each has a signal that can be checked rather than argued.

**Agent-intuitive** — a cold mind infers the right next action from the surface
alone. *Signal:* everything needed to choose is on the surface it is handed; it
never has to guess a name, an argument, or a convention that the log already
knows. The failing case is a mind that must probe to discover what an instance
is even about.

**Agent-ergonomic** — being right is cheap. *Signal:* the context spent to reach
a correct action is bounded and proportional to the decision, not to the size of
the log. Rehearsal is available before commitment; feedback from a mistake
reaches the turn that can act on it.

**Agent-accretive** — every waking leaves the next one cheaper. *Signal:*
orientation cost is flat or falling in log size, while what the instance can do
grows. The failing case is the opposite curve, and it has a name here: the
instance gets richer while orienting in it gets more expensive, until the mind
stops orienting and starts guessing.

Only the third is a claim about time, and it is the one this system exists to
make. It is also the one currently broken, in exactly one place.

## The measurement

Two hundred `note.added` events into a fresh instance with nothing pending:

```
situated prompt   2,328 bytes      (1,737 before this branch's core-layer change)
self brief          518 bytes
self view log    25,292 bytes
```

Two hundred domain events moved the situated prompt by **two bytes** — the digit
count in `log: N events`. They moved the only read that would have shown them by
**25 KB**. Reproduce it: append two hundred events to a fresh home and diff
`self` against itself before and after.

Worse than the arithmetic: the string `note.added` — the only thing this
instance has ever been about — appears nowhere in its own orientation surface.
The commands section reads `none yet`, the views section offers the built-in log,
and that is the whole of it. A mind woken here is told what it may *do* and
nothing whatsoever about what *is*. Until this branch it was also pointed at the
whole history under the words "what happened lately", which on a grown instance
is the most expensive read available rather than the cheapest; that pointer now
says what the read actually is, which is honesty, not a fix.

That is the whole of what this document is about. Everything below either
explains why the rest of the system is right to be as it is, or proposes the
smallest change that closes that gap without spending the properties that make
the rest of it work.

## The tower

Each level is a pure function of the level below it, with exactly one door in.
That is the source of the system's coherence: an agent that understands one
level can predict the next, and a change that breaks a level is visible as a
broken invariant rather than as a bug three levels away.

| # | level | is a function of | one door in | fails if |
|---|---|---|---|---|
| 0 | **two files** — `events.jsonl`, `.secret` | — | the filesystem | anything durable lives elsewhere |
| 1 | **record** — the event | — | `hear`, `run`, `learn` | a door or a sequence is accepted from outside |
| 2 | **state** — replay | the two files | — | replay reads a clock, an env var, a network |
| 3 | **capability** — declaration + signed receipt | state | `script.authored` | bytes install without a local-key receipt |
| 4 | **perception & action** — `view` / `run` | state, argv | `run` | a view reaches the log, or a command sets its own provenance |
| 5 | **situation** — the situated prompt | state, ask | — | it appends, or its size tracks the log instead of the decision |
| 6 | **turn** — prompt → mind → wire → `hear` | situation | stdin of `hear` | direction is inferred rather than structural |
| 7 | **loop** — turns to a fixed point | turns | — | anything appends that a mind did not mean to persist |
| 8 | **account** — intent, record, attestation | a slice of the log | `learn` | something runnable travels |

Read the table as a tower and two things become obvious.

**The first is why the kernel refuses to learn domain semantics.** Levels 0–4
know about events, names, bytes and signatures, and nothing about goals, tasks,
notes, or priority. Every attempt to teach the kernel what matters lands at
level 5 or above, where it is a rendering choice, and renderings are
capabilities. This is not minimalism for its own sake: a kernel that knew what a
goal was would have to be revised every time an instance grew a new domain, and
the log — which outlives every kernel — would carry that kernel's vocabulary
forever. The frozen refused set in the account rules is the same instinct,
written down.

**The second is that the tower is finished everywhere except level 5.** Levels
0–4 are complete and mechanically proven: `rehydrate` rebuilds the instance from
two files, replay is deterministic, receipts gate installation, views are handed
no path to the instance. Levels 6–8 are complete: direction is structural, the
loop converges on log change, accounts carry no runnables. Level 5 — the only
level an agent actually stands on — is a capability list and an ask.

That asymmetry is not an oversight; it is where the work went last. The prompt
diet (`experiments/situated-prompt-diet`) cut the situated prompt from 6.5 KB to
1.7 KB by removing protocol prose from every ordinary turn. It was the right
change and it is worth restating what it actually bought: **the diet was never
about making the prompt small. It was about making room in it for state.** That
room is still empty.

## The load-bearing invariant nobody wrote down

`self loop` converges when a complete turn leaves the log unchanged. The log is
therefore not only the state — it is the **termination condition**.

Everything follows from that:

- A kernel that appended telemetry would never converge.
- A kernel that recorded a command's runtime failure as an event would turn a
  failing command into an infinite loop: fail, append, state changed, wake
  again, fail. This is the actual reason `PROTOCOL.md` says a command's runtime
  failure is not an event — it reads as an omission and it is a consequence.
- A mind that appends a note about having oriented has, by appending, declared
  that there is more to do.

State it as the fourth law, beside the three in `PROTOCOL.md`:

> **Only intended persistence appends.** The log is the fixed point, so
> everything that is not deliberate durable work must leave it untouched —
> diagnostics to stderr, feedback into the next prompt, transient failure to the
> caller's exit code.

**holds** — the kernel behaves exactly this way today. It is written here
because every proposal below had to be checked against it, and two good-looking
ones died on it.

## One waking, from inside

This is what actually happens when I am the mind, and where the cost is.

```
1  the loop hands me 1.7 KB on stdin: frame, brief, ask.
2  I learn: what capabilities exist, what is pending, what was refused.
3  I do not learn: what happened here, what is live, what the last waking
   was in the middle of, whether anything changed since it ran.
4  so I probe. Each view is an unbounded read of unknown size and unknown
   relevance, and I cannot see its cost before paying it.
5  I decide — on whatever the probes happened to surface.
6  I act: self run …, or events on stdout.
7  I verify by probing again.
8  I end. Everything I learned that I did not append is gone.
```

Steps 4 and 7 are the entire budget. Steps 1–3 are nearly free and nearly empty.
The ratio between those two facts is the ergonomics of this system.

Three failure modes fall straight out of that trace, and none of them is the
mind being careless:

- **Blind probing.** With *k* views and no signal of which is relevant, a
  careful mind reads several and a careless one guesses. Both are wrong for the
  same reason: the choice was never informable.
- **Duplicate work.** A cold mind cannot tell whether the thing it is about to
  append was already appended by the previous waking, unless it spends a read to
  find out. Nothing in the surface answers "what changed since last time".
- **Silent recurrence.** Prose on the wire is ignored, counted, and reported to
  stderr — where the *next* mind never sees it. A refusal teaches, because it is
  state and rides the next prompt. An ignored line teaches nothing, so the same
  mind makes the same mistake on every pass of the same loop.

## The pressures

Eight, in the order they cost.

**P1 — Orientation must be bounded, layered, and proportional to the decision,
not to the log.** The mind should be able to spend a fixed small amount to know
*roughly* everything, then spend precisely on the one thing that matters. Today
the ladder has two rungs: a prompt that is nearly free and nearly stateless, and
a log dump that is linear in history. There is no middle, and the middle is where
every real decision is made.

**P2 — The instance must own its own perception.** The compression that makes a
digest useful is domain knowledge, and domain knowledge must not enter the
kernel (see the tower). So the digest cannot be kernel-authored. It has to be a
view — which means the accretive act available to every waking is *improving what
the next waking sees*. This is the keystone. A system where the mind can improve
its own perception compounds; one where perception is fixed by the kernel does
not, no matter how good the fixed version is.

**P3 — Every byte the kernel prints is context.** Bound it, or the pointer to it
is a lie. The brief called `self view log` "what happened lately" when it is in
fact the whole history; that wording is corrected on this branch, which makes the
pointer honest and the read no cheaper. The comment above `builtinLogView` in
`capability.go` still says it "answers the cheapest question at every cold
start" — true on an empty instance, false on any instance worth orienting in,
and the divergence grows monotonically. D1 is what would make the sentence true
again.

**P4 — Rehearsal before commitment.** Authoring is the sharpest act in the
system: there is no human review between authoring and signing, and the kernel
runs a script under a boundary the mind cannot easily reproduce by hand
(scrubbed environment, no `SELF_HOME` for views, consumes-filtered stdin, an
empty working directory). `PROTOCOL.md` says "test the script by running it
before you print it" and offers no way to run it *the way the kernel will*. The
mind's most consequential act is its least rehearsable one.

**P5 — Feedback must arrive where it can be acted on.** Three channels exist and
only one of them closes: refusals become state and ride the next prompt (closed);
ignored wire lines go to stderr (open); command failures go to an exit code
(open). Decide each on the fourth law: what is genuinely state persists; what is
a transient mistake echoes inside the loop and is never written.

**P6 — Names are the index.** On an unknown instance the cheapest true map is
the set of event names and their counts. It is a pure replay, it costs a dozen
lines, it names every domain the instance has ever had, it tells a mind which
views are worth running — and it is also the selector vocabulary for
`self give`. No other ten lines carry as much.

**P7 — Writes should be witnessable before they are made.** A cold mind cannot
be idempotent by memory; it can only be idempotent by reading first. That is
affordable only if the read is cheap (P1) and targeted (P2). The three pressures
are one pressure.

**P8 — One dispatcher.** The kernel already dispatches by name — verbs, views,
commands. Every proposal that adds a flag where a name would do, or a second
output mode where one exists, is adding a dispatcher, and the reader now has to
know two systems. The `faces` lesson states this for views ("eight small scripts
beat one script with a `--format` argument"); it is a system rule.

## The work outstanding

Each item: the pressure it relieves, the smallest change that relieves it, the
invariant it must not break, and the check that proves it. Ordered by value per
line of kernel.

### D1 — a bounded built-in log read **designed** · P1, P3

`self view log` accepts an optional tail count and an optional `since:<seq>`:

```sh
self view log 50            # the last 50 events
self view log since:1284    # everything after the seq I last saw
```

The default stays the whole log: the log is authoritative and the kernel does not
get to decide which of your records to hide by default. What changes is that a
bounded read *exists*, and every pointer at it — the brief, `AGENTS.md`, the
prompt — names the bounded form.

*Invariant:* purity (a tail is a pure function of the same events), and one
dispatcher — arguments to a view are already how every other view is
parameterized, and the growth layer already asks that the zero-argument form be
a usable index.

*Check:* `self view log 50` on a 200-event instance prints 50 lines, the last
50, in log order; `self view log since:$n` prints exactly the events after `n`;
a declared view named `log` still shadows the built-in, arguments and all;
malformed arguments fail rather than being ignored.

### D2 — the name census in the brief **designed** · P6, P1

The brief gains a bounded census: every event name in the log with its count and
the seq of its most recent occurrence, most recent first, capped (12 lines and
`…and k more names`).

```
## what this log is made of

note.added        200   last seq 201
goal.created       14   last seq 188
goal.closed         9   last seq 190
```

Three lines of kernel-legitimate replay turn an instance from anonymous into
legible. It is the first thing that would tell a mind woken in the measured
instance above that the place is about notes.

*Invariant:* no domain semantics — the kernel counts strings, it does not know
what a note is. Boundedness — the cap is not optional; a log with a thousand
distinct names must not print a thousand lines into a prompt.

*Check:* the census is present in `self brief` and in the situated prompt; its
length is bounded independent of the number of distinct names; counts and last
seq match a replay; the brief stays under a stated byte ceiling on a synthetic
log with 500 distinct names.

### D3 — the `situation` splice **designed** · P2, P1, P7 — *the keystone*

`situation` becomes a reserved-but-shadowable view name, exactly as `log` is
today. When an instance has a live installed view called `situation`, the kernel
runs it with no arguments while building the situated prompt and splices its
bytes in, under a stated byte budget.

```
## situation — this instance's own account of itself (self view situation)

<the view's bytes, truncated at the budget with a visible marker>
```

The kernel gains one name. It gains no idea what the bytes mean. The instance
gains the ability to decide what every future waking sees first — and the mind
that improves the view is not the one that benefits, which is precisely what
makes it accretion rather than convenience.

Design constraints that are not optional:

- **A budget, stated in the prompt.** The mind must be told the size of the
  space it is filling; a budget it cannot see is a budget it cannot design for.
  Truncation must be visible, never silent.
- **Orientation must never hang or fail.** A situation view that errors, or that
  runs long, must degrade to a one-line note in the prompt saying so. This is
  the one place the "capability scripts are not timed out" limit should be
  narrowed, and only here: every other capability runs because a caller asked
  for it, while this one runs on the path that a caller has no way to avoid.
  The failure note is itself the feedback that gets the view fixed.
- **Still a read.** Views are pure and appendless by construction, so the law
  survives: orientation remains a read, and splicing a view into it appends
  nothing.

*Invariant:* the kernel learns a name, not a meaning (as with `log`); the prompt
stays bounded; orientation cannot be broken by a capability.

*Check:* an instance with no `situation` view produces a byte-identical prompt to
today's; an instance with one splices it; a view that exits non-zero, hangs, or
prints more than the budget yields a prompt that is complete, bounded, and
explicit about what went wrong; the spliced prompt is still a pure function of
the two files.

### D4 — `self try`, rehearsal under the real boundary **designed** · P4

A read verb that runs a candidate script exactly as the kernel would, and
appends nothing:

```sh
self try view journal /tmp/journal.py            # real consumes, empty cwd, no SELF_HOME
self try view journal /tmp/journal.py --twice    # byte-identical output, or it is not pure
self try command entry /tmp/entry.sh "a note"    # real env, log on stdin
```

For a view: the events its declaration names, on stdin; the scrubbed
environment; an empty working directory; no `SELF_HOME`. For a command: argv,
the whole log on stdin, `SELF_HOME`, the instance as the working directory —
and then the output is run through the same wire splitter `hear` uses, and the
verb *reports what would land* without landing it.

This converts three of the system's central claims from instructions into
mechanisms. "Test it before you print it" becomes a verb. "A view is a pure
function" becomes `--twice`, which is a byte comparison rather than an
aspiration. "Do not narrate on the wire" becomes a report the author sees before
the log does.

*Invariant:* it is a read — it appends nothing, installs nothing, and works on a
declaration that has never been authored. It must not become a second way to
install; the receipt gate stays the only door.

*Check:* `self try` leaves `events.jsonl` and `cap/` byte-identical; a view given
`--twice` that reads the clock fails; a command whose output contains prose is
reported as prose, not as events; the environment a tried script observes is
identical to the one the kernel gives the installed script.

### D5 — turn feedback inside the loop **designed** · P5

The loop carries the kernel's report of the immediately preceding turn into the
next prompt, bounded, and marked ephemeral:

```
## your previous turn (this loop only, not state)

heard 3 events (seq 210-212) · installed view/journal
IGNORED 4 lines — first: "I'll now write the six events to stdout"
```

This is the one deliberate exception to "orientation is a pure function of the
log", and it is worth arguing rather than hiding. The alternative — appending a
`turn.reported` event — is *dead on the fourth law*: a report is an append, an
append means state changed, and the loop would never converge. So the choice is
between the feedback being ephemeral and the feedback not existing. The block is
derived entirely from a turn this same process conducted seconds earlier, it
never becomes state, it is absent from a bare `self` invocation, and the prompt
labels it as ephemeral so no mind mistakes it for something replayable.

*Invariant:* nothing is persisted; a bare `self` prompt is unchanged; the block
is bounded; `rehydrate` is unaffected because nothing about this is on disk.

*Check:* a loop pass whose mind emits prose produces a following prompt naming
the ignored lines; the log after that pass is identical to a run without the
feature; bare `self` never prints the block.

### D6 — cost legibility in the brief **designed** · P1, P7

One line, three facts the mind currently cannot get without paying for them:

```
log: 1,284 events · 412 KB · last append 2026-08-31T09:14:02Z (seq 1284)
```

Size tells the mind what `self view log` would cost *before* it spends it. Last
append answers "has anything happened since the last waking" — the single most
common question a loop-driven mind has, currently answerable only by reading the
whole log.

*Invariant:* pure replay; bounded; no clock read (the timestamp is the last
event's own moment, which is data, not now).

*Check:* the figures match the file and the replay on a synthetic log; the brief
grows by one line, not by a section.

### Sequencing

D1, D2 and D6 are small, pure, and independent — they are the ones that make the
next waking cheaper immediately. D4 is independent of all of them and is the
largest single accuracy win. D3 is the keystone and should land after D2, so the
census tells a mind what a good `situation` view would have to compress. D5 is
last: it is the only item that touches the purity of orientation, and it should
be spent only once the cheap wins are in.

## Conventions that need no kernel at all

These work today, unchanged. They are the accretive practices an instance grows
into, and they are why D3 is a splice rather than a feature: the convention comes
first, and the kernel eventually stops making the instance pay for it.

**C1 — grow a `situation` view, and read it first. holds.** Any instance can
declare and author a view named `situation` right now, and every mind can be told
to run it before anything else. `lessons/situation` is the account that teaches
it. D3 only removes the round trip.

**C2 — leave a handoff, in the log, for a reader who is not you. holds.** The
next waking is a stranger with your job. What it needs is not a transcript but
one durable line: what is open, what was just tried, what to do next. That is a
domain event through a domain command, and the situation view leads with the
newest one.

**C3 — read before you write. holds.** A cold mind has no memory of what it
already did. Idempotence is not available by recall, only by looking — which is
affordable exactly to the degree that C1 and D1 make looking cheap.

## Rejected, and why

Kept because the cost of re-deriving these is several wakings each.

**A `command.failed` event.** Breaks the fourth law: the failure appends, the
loop sees changed state, wakes again, fails again. A failing command would be an
infinite loop with a growing log. Failures belong to the caller's exit code and
stderr. Superseded in practice by D4, which surfaces the failure *before* it is
a failure.

**`self brief --json`.** The consumer of the brief is a language model, which
reads prose better than JSON; the consumer that genuinely needs JSON is a
program, and a program can be handed a view that emits it. Adding a flag would
put a second dispatcher next to the one that already works by name (P8).

**Priority, attention, or a "next action" field in the kernel.** Domain
semantics. The kernel would have to be revised for every new domain and the log
would carry its vocabulary forever. Priority is a rendering, renderings are
views, and `situation` is where it goes.

**A harness session store, `claude -p --continue`, or any continuation state.**
Invisible to `rehydrate`, invisible to audit, gone on rebuild — the exact
property the log exists to provide. If a cold start feels expensive, that is
design pressure pointed at the views, which is what D1–D3 are.

**Caching a view's output.** The previous kernel had it — materialized HTML,
mtime freshness, selective refresh — and it is gone deliberately. Cache
coherence has no business inside a kernel whose whole claim is pure replay. If a
view is too slow to run on demand, the expensive part belongs in a command that
appends a computed observation, and the view replays that.

**A human review step between authoring and signing.** Stated as a limit rather
than fixed, and left that way: piping a mind's output into `self hear` signs what
it authored, and holding the output in a file first is available to anyone who
wants the pause. D4 narrows the gap that actually matters — the author's own
inability to see what they are about to sign.

**Sandboxing installed scripts.** Out of scope and stated as a limit. The
environment is scrubbed for determinism, not containment; an installed script
runs as you, and the documentation says so plainly rather than implying a
boundary that is not there.

## What would falsify this document

The accretive claim is the falsifiable one. Grow an instance to a few thousand
events across several domains and measure, per waking: bytes of prompt, number of
reads before the first durable action, and bytes read. If those rise with log
size after D1–D3 land, the design is wrong and the fix is not more compression —
it is that perception belongs somewhere other than a view, and this document
should be rewritten around whatever that turns out to be.

`experiments/situated-prompt-diet` is the shape such a measurement takes here:
isolated homes, a deterministic mind, exact byte counts, and a caveat section
that refuses to claim more than the fixture supports.
