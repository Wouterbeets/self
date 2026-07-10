# memory — durable memory for a stateless mind

## purpose

The mind is a stateless process: every ask starts cold and orients from the
rendered state. This surface is where it remembers. A memory is one durable
fact worth carrying across sessions — a lesson learned, a preference the human
stated, an invariant of this instance — written for a future mind that knows
nothing except what these pages say.

This is the middle tier of the instance's memory. The newest events ride
inside each ask; the rendered memory page is what a cold mind reads to
orient; and the raw event log on disk is the deep archive it searches with
its own tools. Nothing about memory lives outside the log — no session
store, no harness state — so `rehydrate` replays it and `share` can carry it.

## surface

- `self run remember <text…>` appends one `memory.noted` event carrying the
  memory text and, when the caller's environment provides `SELF_MIND_ID`,
  that identity as `by` — a memory records who laid it down.
- `/memory` renders every memory newest first, with timestamp and author.
  It is an orientation page for a cold reader, not a diary: compact,
  scannable, each memory exactly as written.

## constraints

- Exactly one command (`remember`), one projection (`memory`), one event name
  (`memory.noted`).
- One memory per event, self-contained: it must make sense to a reader with
  no other context. Corrections are later memories that say what changed;
  nothing is edited or deleted.
- The projection consumes only `memory.noted` and renders an empty log as a
  short invitation to remember, not an error.
