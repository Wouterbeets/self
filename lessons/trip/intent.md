# trip — a countdown, not a packing list

## purpose

A year-scale journey dies in a markdown file. This organ is the few
dates that define it — departure, borders, landings — replayed as a
countdown against the log's newest timestamp. The board still holds
the work (car door, remorque cable). `/trip` holds the spine.

## surface

- `self run milestone <who> <YYYY-MM-DD> [note…]` pins one milestone.
  `who` is a slug (`departure`, `egypt`, `cape_town`). Latest per slug
  wins. Emits `trip.noted` `{who, at, text}`.
- `self run unmilestone <who>` hides one (tombstone). Emits
  `trip.dropped` `{who}`. Unknown or already-hidden slugs append
  nothing and warn on stderr.
- `/trip` lists live milestones by date, each with days-until (or days
  ago) measured from the newest event timestamp — the log is the clock,
  so replay stays pure. A milestone named `departure` is the countdown
  at the top. Pin form (`/run/milestone`); hide per card.

## constraints

- Two commands (`milestone`, `unmilestone`), one projection (`trip`),
  two event names (`trip.noted`, `trip.dropped`).
- The projection consumes only those two events.
- Dates are calendar days (`YYYY-MM-DD`), not clock times. No packing
  checklist, no visas, no shopping — those are board cards.
- Corrections are a later `milestone` for the same slug.

## what good looks like

`self run milestone departure 2026-09-01 Wheels rolling south` then
open `/trip`: the page says how many days remain, from the log, not
the wall clock. Rehydrate does not move the countdown.
