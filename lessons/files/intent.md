# files — content carried in the log

## purpose

The log is the only structured state; anything on disk beside it is a
squatter that `rehydrate` evicts. So a file this instance should keep must
travel *as an event*: its bytes ride in the payload, its identity is a
digest, and the page that serves it back is a pure replay like every other
projection. This is not a blob store — it is a pocket: a place to keep the
small files that belong to this instance's story (a config, a photo, a
receipt, a diagram) with the same guarantees as everything else it
remembers. What is not in the log did not happen; after this lesson, a kept
file is in the log.

## surface

- `self run file/add <path> [note…]` reads the file at `<path>` from disk
  and appends one `file.added` event carrying: `name` (the basename),
  `media_type` (guessed from the extension, `application/octet-stream` when
  unknown), `size` (bytes), `sha256` (hex digest of the raw bytes), `bytes`
  (the content, base64), and `note` (the remaining arguments joined — why
  this file matters, for a future reader).
- `self run file/drop <name>` appends one `file.removed` event carrying
  `name`. Removal is a later event: the bytes stay in the log forever, the
  index just stops showing them.
- `/files` renders the index: for each live file its name, size, short
  digest, note, and when it arrived. The content itself is served through a
  `data:` URI — a download link on every entry, and an inline `<img>` when
  the media type is `image/*`. The page is self-contained by construction:
  no external assets, nothing read from disk.

## constraints

- Exactly two commands (`file/add`, `file/drop`), one projection (`files`),
  two event names (`file.added`, `file.removed`).
- The latest `file.added` per name wins; a `file.removed` hides the name
  until a later add revives it. The projection derives this from events
  alone.
- The projection consumes only `file.added` and `file.removed`, and renders
  an empty log as a short invitation to add a file, not an error.
- The command may read the filesystem — commands are effectful and run at a
  moment; the projection may not — it is a pure function of its events.
- Base64 in JSON grows the log by ~4/3 of every file kept, and every replay
  carries it. That is the honest price of the guarantee; the note below
  says when not to pay it.

## anti-goals

- Not a blob store: no content-addressed directory, no upload route, no
  multipart handling, no deduplication. Files enter from a path the command
  can read, full stop.
- Not for large or churning files. A file bigger than a few hundred
  kilobytes, or one that changes often, does not belong in an append-only
  log that every projection replays — keep such files outside and record
  *about* them (a path, a digest, a note) instead of carrying them. That
  is the blobs lesson (`lessons/blobs`), the other pocket of this pair.
- No serving bytes outside the projection. The data: URI on `/files` is the
  one egress; nothing writes loose files into `site/`.

## what good looks like

`self run file/add ~/notes/recipe.jpg "grandmother's handwriting"` — one
event lands. `/files` shows the photo inline with its note and digest.
`self rehydrate` on a fresh clone of the log rebuilds the same page,
byte for byte, photo included. `self give file. <dir>` writes an account
that carries the files verbatim to another instance, digests intact —
evidence that travels like any other record. Six months later the log
still explains why the photo is there, because the note said so.
