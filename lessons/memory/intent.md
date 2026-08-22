# memory — durable memory for a stateless mind

## purpose

A mind is a stateless process: every pass starts cold and orients from replayed
state. This is where it remembers. A memory is one durable fact worth carrying
across sessions — a lesson learned, a preference the human stated, an invariant
of this instance — written for a future mind that knows nothing except what it
can replay.

Nothing about memory lives outside the log. No session store, no harness state,
nothing a `rehydrate` would lose. If you are tempted to reach for
`claude -p --continue` as the mind's memory, this is the lesson that says no:
chain state outside the log and it is invisible to audit and gone on rebuild.

## surface

- `self run remember <text…>` appends one `memory.noted` event carrying the
  memory text.
- `self view memory` renders every memory, newest first, with its timestamp and
  the speaker (`by`) who laid it down — a memory records who said it.

## constraints

- Exactly one command (`remember`), one view (`memory`), one event name
  (`memory.noted`).
- One memory per event, self-contained: it must make sense to a reader with no
  other context. Corrections are later memories that say what changed; nothing
  is edited or deleted.
- The view consumes only `memory.noted` and renders an empty log as a short
  invitation to remember, not an error.
- Plain text, compact and scannable. It is an orientation page for a cold
  reader, not a diary.
