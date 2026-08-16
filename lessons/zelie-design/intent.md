# zelie-design — the design organ for Zélie's horse game

## purpose

Zélie (a child) asked, in French, for a video game where you are a horse
doing competitions and missions: you ride *on* the horse and see its ears
and head; there are obstacles; you can speed up or slow down; you must jump
at the right moment; you can steer left and right to choose your path; it
happens in a *carrière* (a riding arena); you can choose your horse's color
and the place you ride. The game itself is a static three.js page living in
`zelie/` in this repository — it is an artifact, not a capability (a
projection may not emit JavaScript, so a game cannot be one).

This lesson grows the **design organ**: the place where every design
decision about that game is recorded, and where experiments and wrong turns
are logged as they happen — so the game's history is replayable evidence,
not memory. When this lesson ships with a `record.jsonl`, that record *is*
the design log of the original build, deposited verbatim at learn time.

## surface

- `self run design <kind> <text…>` appends one `design.noted` event with
  payload `{kind, text}`. `kind` is one of:
  - `decision` — a choice made, with its reason;
  - `experiment` — something tried to find out what works;
  - `wrong-turn` — something tried that failed or was abandoned, and why.
  Any other kind is refused (the command emits nothing and exits nonzero).
- `/design` renders every `design.noted` event in chronological order — the
  design log reads top to bottom as the story of the build — with each
  entry's kind marked, and a small summary count of each kind at the top.

## constraints

- One command (`design`), one projection (`design`), one event name
  (`design.noted`).
- The projection consumes only `design.noted` and renders an empty log as an
  empty list, not an error.
- Chronological order, oldest first: a design log is a narrative, not a feed.
- Wrong turns are first-class: the projection must not bury or collapse
  them. The point of the organ is that failures stay visible.

## anti-goals

- No editing or deleting entries — the log is append-only and a revised
  decision is a new `decision` entry that says what it supersedes.
- No categories beyond the three kinds; taxonomy creep would stop entries
  from being written.

## what good looks like

While building the game: `self run design wrong-turn "CDN blocked by the
sandbox network policy; curl to cdn.jsdelivr.net got 403"` — then later
`self run design decision "vendor three.module.min.js from the npm registry
tarball so the game runs offline"`. Opening `/design` shows both, in order,
wrong turn first — the reader sees *why* the decision exists.
