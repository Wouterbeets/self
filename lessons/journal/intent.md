# journal — the smallest lesson

## purpose

The smallest useful capability set, as a worked example: one command that
records an entry, one view that renders the record.

## surface

- `self run entry <text…>` appends one `journal.entry` event carrying the entry
  text.
- `self view journal` renders every entry, newest first, with its timestamp.

## constraints

- Exactly one command (`entry`), one view (`journal`), one event name
  (`journal.entry`).
- The view consumes only `journal.entry` and renders an empty log as an empty
  list, not an error.
- Plain text output. There is no browser here; the reader is a terminal or an
  agent.
