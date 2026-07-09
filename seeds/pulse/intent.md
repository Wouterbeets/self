# pulse — the instance takes its own pulse

An instance should be able to look at itself and say how it is doing, using
nothing but the log it already keeps. This seed grows that self-awareness: a
projection that reads the whole event log and reports the instance's **vital
signs**, plus a way to drop a dated marker so growth between two moments can be
read off later.

The point is honesty about scale. `self` keeps one append-only log and replays
all of it on every render, so cost grows with history — an instance that never
looks at its own size only discovers the wall by hitting it. `pulse` is the
mirror: it makes the current load, and the distance left to the known limits,
something you can see on a page instead of something you find out the hard way.

## The projection: `pulse`

Declare a projector named `pulse` (it consumes the whole log — every event kind
counts toward vitals). Rendered from the log alone, deterministically, it should
show:

- **Size.** Total events, total log bytes, and the single largest event line in
  bytes. Call out the largest line explicitly: a single event line is what a
  reader must hold at once, and an instance whose largest line is approaching a
  megabyte is approaching the size where naive line readers give up. Show it as
  a share of 1 MiB so the headroom is legible.
- **Shape.** A histogram of event names — how many of each kind — so the log's
  composition is visible at a glance (how much is domain content vs. compiled
  receipts vs. bookkeeping). Order it by count, most common first.
- **Capabilities.** How many commands and projections are currently live
  (declarations minus retirements), listed by name.
- **Tempo.** The timestamp of the first and most recent event, the span between
  them, and the count of `checkpoint.marked` markers (see below) with the note
  attached to the latest one.
- **A plain-language read.** One honest sentence on how the instance is doing
  for its size — comfortable, getting heavy, or near a wall — reasoning from the
  numbers above, not from a fixed threshold table.

It must be a pure function of the log: same log in, same HTML out. Emit only
bare semantic HTML — no CSS, no JavaScript, no inline styles — the kernel skins
it. Group the vitals under clear headings so the page reads top to bottom.

## The command: `checkpoint`

Declare a command named `checkpoint` that takes one optional `note` argument and
emits a single `checkpoint.marked` event carrying that note. A checkpoint is
just a labelled point in the log; `pulse` counts them and surfaces the latest
note, so a reader can mark "before the big import" and later read how far the
instance has travelled since.
