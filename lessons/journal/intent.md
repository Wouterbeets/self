# journal — minimal example lesson

## purpose

The smallest useful capability set, as a worked example: one command that
records an entry, one projection that renders the record.

## surface

- `self run entry <text…>` appends one `journal.entry` event carrying the
  entry text.
- `/journal` renders all entries, newest first, with their timestamps.

## constraints

- Exactly one command (`entry`), one projection (`journal`), one event name
  (`journal.entry`).
- The projection consumes only `journal.entry` and must render an empty log
  as an empty list, not an error.
