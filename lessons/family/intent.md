# family — who this home belongs to

## purpose

A cold mind reading `/memory` gets a diary of facts. A family page is
something else: the household as a structured roster — names, roles,
optional birth dates — that a morning face, a meal plan, or a future
world-schooling log can fold without parsing prose. Memory stays the
orientation essay; `/family` is the index of people.

This organ is how the instance knows *whose* home it is.

## surface

- `self run person <who> <role> [born] [note…]` pins one household
  member. `who` is a stable slug (`wouter`, `noam`). `role` is one of
  `self | partner | child | parent | kin | other` (friendly spellings
  normalized). If the next token looks like `YYYY-MM-DD` it is `born`;
  the rest is free-text note (display name first, then what a stranger
  should know). Latest `person.noted` for a slug wins. Emits
  `person.noted` `{who, role, born, text}`.
- `self run unperson <who>` hides a member (tombstone; the log keeps
  history). Emits `person.dropped` `{who}`. Unknown or already-hidden
  slugs append nothing and warn on stderr.
- `/family` renders the live roster grouped by role (household, children,
  parents, kin, others), each card with name, role, age if born is
  known, and the note. Age is computed from the newest event timestamp
  (the log is the clock), so replay stays pure. A pin form at the top
  (`/run/person`); a hide button on each card (`/run/unperson`). Empty
  log: a short invitation, not an error.

## constraints

- Two commands (`person`, `unperson`), one projection (`family`), two
  event names (`person.noted`, `person.dropped`).
- The projection consumes only those two events.
- `who` is snake-ish: `[a-z][a-z0-9_]{0,31}`.
- No photos, no medical files, no school reports. A card is a name, a
  role, an optional birthday, and a sentence a future mind can trust.
- Corrections are a later `person` for the same slug, not an edit.

## anti-goals

- Not a scrape of `memory.noted`. Family structure is first-class data.
- Not a CRM. No emails, phones, or addresses required — those stay in
  memory if they must live at all.
- Not the mother's clinical dossier. That lives at `mam.beets.cloud`.
  A parent card may point there; it must not copy the notes.

## what good looks like

`self run person noam child 2014-06-25 Noam Beets. Eldest.` then open
`/family`: one child card, age derived from the log's newest date, a
hide button, and a pin form ready for the next sibling. Re-running
`person` for `noam` with a fuller note replaces the card; the old
event stays in the log.
