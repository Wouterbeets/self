# blobs — heavy matter beside the log

## purpose

The files lesson carries content *in* the log, and prices it honestly:
every kept byte rides every replay. For a video, a disk image, an archive,
that price is absurd — the log would drown in matter it can barely lift.
This lesson is the other pocket: the bytes live in a **separate storage
medium**, a content-addressed directory beside the log, and the log keeps
only the reference — name, media type, size, digest, note. The trade is
stated plainly and chosen deliberately: the log stays light enough to
replay forever, and in exchange the instance now owns matter the log
cannot resurrect. `blobs/` is a peer of `events.jsonl` and `.secret` — part
of the home, not derived from it. Move the home, move it whole; move the
blobs alone and you break the references — and the breakage is detectable,
because the digest in the log says exactly what the bytes must be.

## surface

- `self run blob/add <path> [note…]` hashes the file at `<path>`, copies
  its bytes to `blobs/<sha256>` under the instance home (content-addressed:
  the digest is the filename), and appends one `blob.added` event carrying
  `name` (the basename), `media_type`, `size`, `sha256`, and `note` — never
  the bytes. Adding the same content twice stores it once and records both
  references; the copy lands via a temp file and rename, so a crash never
  leaves a half-written blob under its final name.
- `self run blob/drop <name>` appends one `blob.removed` event; the name
  leaves the index. The bytes are **not** deleted — another name may share
  them, and an append must never destroy the medium.
- `self run blob/verify` re-hashes every blob the live index references and
  appends exactly one `blob.checked` event: how many are sound, and which
  are `missing` (no file at the address) or `damaged` (bytes no longer
  match the digest), each listed with name and digest. This is the audit
  receipt for "move it and you break it": run it after moving an instance,
  restoring a backup, or whenever you doubt.
- `/blobs` renders the index from events alone: name, media type,
  human-readable size, short digest, note, and the relative path
  (`blobs/<sha256>`) where the bytes live under the instance home. Below
  the index, the latest audit: when `blob.verify` last ran and what it
  found, with missing or damaged references called out. The page is the
  map, the filesystem is the medium — a video plays from its path, not
  from the page.

## constraints

- Exactly three commands (`blob/add`, `blob/drop`, `blob/verify`), one
  projection (`blobs`), three event names (`blob.added`, `blob.removed`,
  `blob.checked`).
- The store is content-addressed: the file at `blobs/<sha256>` must be the
  bytes whose digest that is; media type and human name live only in the
  event. The latest `blob.added` per name wins; a `blob.removed` hides the
  name until a later add revives it.
- The projection is a pure function of its events: it never reads
  `blobs/`, never the clock. The integrity it shows is the latest
  `blob.checked` receipt's, clearly dated — a projection cannot know the
  disk's present, only what audits recorded, and it must say so rather
  than pretend.
- The commands may read and write the filesystem — they are effectful and
  run at a moment. Only `blob/add` and `blob/verify` touch the store;
  nothing else in the instance may write into `blobs/`.
- `rehydrate` rebuilds the index page exactly, and the store survives it
  untouched (the kernel wipes only derived state). But rehydrate cannot
  recreate a blob: the log remembers *that* the bytes existed and what
  their digest was, never the bytes. Losing `blobs/` loses content and
  keeps the evidence — `blob/verify` makes the wound visible.

## anti-goals

- No bytes in events. Content that should survive on the log's guarantee
  alone belongs in the files lesson — that is the choice this pair offers,
  per file.
- Drop is not delete, and there is no garbage collection. Unreferenced
  blobs sit harmlessly; reclaiming disk is the operator's deliberate move,
  or a later `self revise` once a real need shapes what collection should
  mean.
- No serving blob bytes over HTTP, no streaming, no thumbnails. The
  projection is an index; putting gigabytes behind the loopback server —
  or worse, under `site/` where rehydrate would evict them — recreates the
  problem this lesson exists to avoid.
- An account of `blob.*` events (`self give blob.`) carries references and
  digests, never content. The receiver learns that the matter existed and
  how to recognize it; the bytes travel out of band or not at all, and
  their digest verifies them on arrival.

## what good looks like

`self run blob/add ~/talks/keynote.mp4 "the venue's recording"` — the log
grows by one small event; `blobs/` grows by two gigabytes. `/blobs` lists
the video with its digest, note, and path; it plays from the path. Adding
the same recording under another name stores nothing new and the index
shows both names sharing one digest. `self rehydrate` rebuilds the page
byte for byte and never touches the store. Then the hard honesty: move
`blobs/` aside and run `self run blob/verify` — one `blob.checked` event
records the reference as missing, and `/blobs` shows exactly what was
lost and what its bytes would hash to, so a backup can prove itself on
restore.
