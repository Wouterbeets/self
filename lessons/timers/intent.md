# timers — scheduled intentions, externally clocked

## purpose

The kernel keeps no clock: nothing in this instance acts on its own, and
replaying the log must never make anything happen again. But a mind wants
to schedule intentions — "surface this on Friday", "remind me to reflect".
This lesson resolves that tension by splitting the timer in two: the
*intention* is an event in the log, and the *clock* is outside — cron, a
shell loop, an agent session, a human. A tick command reads the log,
notices what has come due, and records each firing as an event. Firing is
remembering that the moment arrived — so replay and rehydrate see history,
never a trigger, and the instance stays inert between invocations.

## surface

- `self run timer/set <name> <when> [message…]` appends one `timer.set`
  event carrying `name` (the handle), `at` (the due moment, normalized to
  UTC RFC3339 with a trailing `Z`), and `message` (the remaining arguments
  joined). `<when>` is either an RFC3339 timestamp or a relative offset
  like `+30s`, `+10m`, `+2h`, `+3d`. Setting the same name again
  reschedules it: the latest `timer.set` per name wins.
- `self run timer/cancel <name>` appends one `timer.cancelled` event; the
  timer leaves the pending list, its history stays.
- `self run timer/tick` reads the whole log, finds every timer that is set,
  not cancelled, due (`at` ≤ now), and not already fired *for that
  scheduled moment*, and appends one `timer.fired` event per such timer,
  carrying `name`, `at` (the scheduled moment, verbatim), and `message`.
  A tick with nothing due appends nothing. Ticking twice in a row is
  identical to ticking once — idempotence comes from the log, not from any
  state the command keeps.
- `/timers` renders two lists from events alone: **pending** (set, not
  cancelled, not yet fired for their latest scheduled moment — with due
  moment and message) and **fired** (each firing with when it was scheduled
  for and when the tick noticed). The projection never reads the clock, so
  it cannot say "overdue" — it says "waiting for a tick", which is the
  truth.

## constraints

- Exactly three commands (`timer/set`, `timer/cancel`, `timer/tick`), one
  projection (`timers`), three event names (`timer.set`,
  `timer.cancelled`, `timer.fired`).
- All `at` values are normalized to UTC RFC3339 `Z` at set time, so
  dueness and already-fired checks are plain string comparisons — no
  timezone arithmetic anywhere downstream.
- A firing is keyed by (`name`, `at`): a rescheduled timer can fire again
  for its new moment; the same moment never fires twice.
- Only `timer/set` (parsing a relative `<when>`) and `timer/tick` read the
  clock. The projection is a pure function of its events; an empty log
  renders as a short explanation of how to set a timer and wire a tick,
  not an error.
- The tick must tolerate a log with no timer events at all — a bare
  instance ticking is a no-op, not a crash.

## anti-goals

- No daemon, no background thread, no serve-time ticker: the kernel stays
  inert and this lesson does not sneak a clock back into it. The tick's
  cadence is the operator's choice and lives outside the log —
  `* * * * * self run timer/tick` in a crontab, a tick at session start
  for agent-driven instances, or a human running it when they sit down.
- Firing has no effects. A `timer.fired` event is a record, not an action:
  anything that should *happen* on firing is another capability consuming
  `timer.fired` — a projection that surfaces what fired since you last
  looked, or an operator pairing the tick with `self reflect`.
- No recurrence in this lesson. Repeating timers are one
  `self revise command/timer/tick "…"` away once a real need shapes what
  repetition should mean; do not speculate them into the first version.

## what good looks like

`self run timer/set water +2h "the plants"` — one event, `/timers` shows it
pending with its due moment. `self run timer/tick` before the moment:
nothing appended. After the moment: one `timer.fired` lands, the timer
moves from pending to fired on `/timers`, and a second tick appends
nothing. `self rehydrate` replays all of it without firing anything —
the firing is history now, and history only happens once.
