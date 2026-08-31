# situation — the instance's own account of itself

## purpose

Every waking here starts cold. The kernel hands a mind a card that says what it
*may do* — capabilities, pending declarations, standing refusals — and almost
nothing about what *is*. So the mind probes: it runs views it cannot see the
cost of, looking for the one that is relevant, and spends on orientation the
context it needed for the work.

This lesson grows the missing rung. `situation` is one view that answers, in a
fixed small space, the three questions every cold reader has:

1. What is live here right now?
2. What did the last waking leave open?
3. Which view should I spend my next read on?

The kernel cannot write this view, and should not learn how. Compressing an
instance requires knowing what matters *in this domain*, and the moment the
kernel knows that, every instance inherits one kernel's opinion forever. So
perception is a capability: the instance decides what its own readers see first,
and can improve that decision without a new kernel.

The asymmetry is the point. **The view pays the whole replay so that every
reader after it does not.** A mind that improves `situation` is not the mind
that benefits — the next one is. That is what makes this accretion rather than
convenience, and it is why an instance that grows this early gets cheaper to
work in as it gets richer.

## surface

Two capabilities. One records the hand-off; one renders the situation.

- `self run handoff <text…>` — appends `handoff.left {text}`: what this waking
  was doing, what is open, what the next reader should pick up. One line, written
  for a stranger with your job and none of your context.
- `self view situation` — the compressed card. No arguments. Bounded. Read
  before anything else.

It renders, in this order, and skips any section that is empty:

- **Open** — the newest `handoff.left`, verbatim, with its moment and speaker.
- **Live** — whatever this instance's domains consider current: unclosed goals,
  unfinished tasks, records without a tombstone. Counts and the few most recent
  keys, never the whole set.
- **Recent** — the last handful of events, name and moment only, so a reader can
  tell whether anything has happened since they last looked.
- **Shape** — every event name in the log with its count, most recent first, so
  a reader who has never been here knows what the place is about.
- **Where to look next** — the views that would answer the obvious follow-up
  questions, named, with what each one costs.

## constraints

- **A hard budget, and it does not grow with the log.** Pick a ceiling — 2 KB,
  or 40 lines — and hold it at ten times the current history. A `situation` view
  whose output grows with the log is the exact bug it exists to fix. Truncate
  visibly: `…and 47 more`.
- **Route, do not dump.** Sections are counts, keys and pointers. When a reader
  needs the detail, `situation` tells them which view to run; it does not inline
  that view's output.
- **Consumes `*`, and earns it.** It needs the whole log to compress it. That
  cost is the view's runtime, not the reader's context, which is the whole
  trade.
- **Pure, like every view.** No clock, no network, no `SELF_HOME`. Relative time
  is impossible without a clock — print the moment an event carries and let the
  reader do the arithmetic.
- **Compress what exists; invent nothing.** If this instance has no goals, there
  is no Live section. The view reports the log; it never holds a fact of its own.
- **Useful when empty.** A fresh instance should get one honest line, not an
  error and not a blank.
- **Provenance stays visible where it matters.** A learned account lands under
  event names this instance already reads. Decide deliberately whether a section
  shows only local testimony (`via` = `cli`/`hear`) or both, and if both, say
  which is which.

## anti-goals

- **Not a second log view.** If the output is one line per event, it is
  `self view log` with a smaller number and none of the judgement.
- **Not a place facts live.** It renders events. Anything it knows that no event
  says is a fact that dies with the next `rehydrate`.
- **Not one giant view.** A domain that needs detail gets its own view;
  `situation` names it and moves on. Resist every impulse to add an argument —
  the kernel already dispatches by name.
- **No analysis that belongs in a command.** If a number is expensive to
  compute or needs to read something outside the log, that is a command that
  appends a structured observation, and `situation` replays the observation.

## what good looks like

```
$ self view situation
open — mind@2026-08-31T09:12Z
  backoffice preprod proof: goal 3 blocked on `build`; retry once CI is green.

live
  goals    4 open, 9 closed   newest: preprod-proof, chart-face, census
  notes    12 since seq 1180

recent
  seq 1284  goal.progressed   2026-08-31T09:12Z
  seq 1283  note.added        2026-08-31T09:04Z
  seq 1282  script.installed  2026-08-30T18:41Z

shape
  note.added 200 · goal.created 14 · goal.closed 9 · handoff.left 6
  …and 3 more names

next
  self view goals            what is open and why       ~1 KB
  self view goal <key>       one goal and its history   small
  self view log              everything, oldest first   large
```

The test that this landed: **hand a cold mind nothing but `self view situation`
and it chooses the right next action, or the right next view, without reading
anything else.** If it has to fall back to the log to decide, the view is not
done yet — and what is missing from it is the durable work.

The second test is arithmetic. Measure `self view situation | wc -c` now, then
again after the log has grown tenfold. If the second number moved much, the
budget is not being held and orientation has started charging by the size of the
history again.
