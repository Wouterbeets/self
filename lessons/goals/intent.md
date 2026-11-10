# goals — outcomes that outlive the session that wanted them

## purpose

A mind finishes its pass and forgets what it was for. A goal is a desired
outcome written down with the evidence that would prove it, so the next mind,
in any harness, can pick it up, move it, or close it. Goals form a tree: a
large outcome is reached through smaller ones.

## surface

- `self run goal create <id> <title> <outcome> [parent]` appends `goal.created`.
  The outcome says what will be true and how anyone could check it.
- `self run goal progress <id> <report…>` appends `goal.progress`: what
  changed, the evidence, what blocks, the next action.
- `self run goal hold <id> <reason…>` and `resume <id>` park and unpark one.
- `self run goal close <id> <completed|cancelled|superseded> <note…>` appends
  the closing `goal.updated`.
- `self view tree` renders the open goals as a tree, with on-hold and blocked
  ones marked.
- `self view context <id>` renders one goal for a mind arriving cold: outcome,
  latest progress, open children.

## constraints

- A goal is closed with a resolution and a note that carries evidence, never
  silently. Closing twice appends nothing.
- Progress on a closed or held goal is refused with a message that says why.
- Views fold events in the order they happened (`occurred_at`), not the order
  they arrived: a log that converges with another instance must render the same
  tree on both.
- `context` fits in a screen. It is what a worker reads first, so it leads with
  the outcome and the latest report, not the history.
- No owners, priorities, estimates or due dates until someone needs them.
