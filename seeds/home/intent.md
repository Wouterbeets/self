# home — a board you actually open

## what it's for

A personal board you open every day: what's on **Now**, what's for **This week**,
what's **Waiting**, what's **Done**. Capture a task in one breath; move it as life
moves it. This is the daily surface — the place you live, not a project tracker.

## the core intuition

Capture is frictionless: type a line and it lands in Now, no fields, no ceremony.
A task is the *latest of its events* — rename it, move it, drop it — so the board
always shows the current truth while the log keeps the whole history (a drop is a
tombstone, not an erasure; nothing is ever really gone). The four lanes read left
to right the way work flows: now → this week → waiting → done.

## the feel

- Zero JavaScript: every action is a plain form on the board — a capture box, a
  one-click move per lane, rename, delete.
- Brain-callable in the same breath ("add a task: call the plumber", "move 3 to done").
- Friendly lane spellings normalize ("this week" → this_week, "later" → waiting).

## the surface (public; the decomposition is growth's to choose)

- `/board` — the four-lane view, with capture and per-task controls.
- the verbs you'd expect: capture, move, edit, drop.

## anti-goals

- Never require structure to capture — a bare line is enough.
- Never lose history — dropping hides a task, it does not erase the record.
