# notes — between-session notes

## purpose

A tiny note-taking surface for agents and humans to leave durable context between
sessions. It is intentionally small: one append-only command and one readable
page, so the first grown app is easy to inspect.

## surface

- `self run note <text…>` appends one `note.added` event carrying the note text.
- `/notes` renders all notes newest first, with timestamps.

## constraints

- Exactly one command (`note`), one projection (`notes`), one event name
  (`note.added`).
- Notes are never edited or deleted; corrections are later notes.
- The projection consumes only `note.added` and must render an empty log as an
  empty list, not an error.
