# snapshot — fold the log, keep the working set

## purpose

The log is unbounded by doctrine and the kernel refuses to own compaction —
the Limits section promises that a snapshot "can itself be modeled as a seed
and left to the user." This seed is that seed. One command folds the log:
the whole history is archived first, content-addressed into the store like
any other file, and then the live log is rewritten down to its working set —
under the kernel's own lock, with no kernel change and no new trust. Nothing
is deleted, only moved to cold storage: the archive *is* the old log, byte
for byte, named by its own hash, remembered by a `file.stored` event, served
at `/files/<sha256>` like every other blob, and covered by the same backup
discipline as the rest of `files/`.

This works because of what the kernel refuses to keep: it holds no state the
log file does not. The next sequence number is parsed from the last line;
page freshness is an mtime comparison; every read is fresh from disk; a
receipt proves itself with a signature. A command that respects those four
facts can fold the file the kernel reads, and the kernel never notices.

## surface

- `self run snapshot [prefix…]` — fold the log. Extra args are event-name
  prefixes to fold out as well (curation: `self run snapshot chat.` retires
  an old conversation to the archive). The command writes a
  `snapshot.folded` marker `{upto_seq, kept, dropped, archive_sha256}` into
  the fold itself, and emits one `file.stored` for the archive on stdout —
  the fourth ingress, so the kernel copies the bytes in and verifies the
  pinned hash before anything appends. When nothing would fold, it does
  nothing: no marker, no archive, no rewrite.
- `/snapshots` — the fold history: each `snapshot.folded` in order, with
  what was kept, what was folded, and a link to
  `/files/<archive_sha256>/events-upto-<upto_seq>.jsonl`. Consumes
  `["snapshot.folded"]`.

## the mechanics (exact — the log's integrity lives or dies on these)

- **Archive first, fold second.** The full original bytes are written to
  scratch (`.snapshot/` under the instance) and fsynced *before* the log is
  truncated. If anything fails after that, the history still exists in
  scratch; scratch from earlier folds is cleared only once its hash is
  already in the store.
- **The same lock as the kernel.** Read → archive → rewrite happens under an
  exclusive flock on `events.jsonl` — the very lock `appendEvent` takes — and
  the rewrite is in place (truncate, not rename), so the inode the kernel
  locks is always the inode being written. Racing appends serialize; the
  fold operates on the locked file's bytes, never on the stdin copy.
- **The last-line rule.** The kernel reads its next sequence number from the
  log's last line and nothing else. The fold's marker therefore carries
  `seq = old max + 1`, so every event appended after the fold lands *above*
  the archived range — live and archived sequence numbers never collide.
- **Kept events travel byte for byte.** A kept line is the original line,
  never re-serialized. Ids, moments, payloads, signatures: untouched.
- **What stays:** the latest *verified* `script.compiled` receipt per live
  capability (rehydrate needs exactly this one); every declaration and
  tombstone of live capabilities (the fold order that decides page order is
  sacred); all `timer.declared` and the latest `timer.fired` per timer — the
  epoch; drop it and the timer re-fires; all `file.stored` (the store's
  contents must stay discoverable); all prior `snapshot.folded` (the chain
  of folds is history too); `kernel.initialized`; and every domain event not
  named for folding.
- **What folds:** `self.heartbeat`, superseded and unverifiable receipts,
  older timer firings and failures, the whole trail of capabilities retired
  and never revived, and the prefixes you name.
- **Never fold what a live projector consumes.** The kernel bumps
  unconsumed pages' mtimes on append instead of re-rendering them, so a
  page must remain a pure function of the *kept* events. Any event named in
  a live projector's `consumes` list is kept no matter what asked for it —
  the shield that makes folding sound, not a courtesy. (A `"*"` projector
  re-renders on every append anyway; after a fold it honestly shows the
  working set, and the archive holds the rest.)
- **Refuse dangerous curation.** A prefix that matches any kernel lifecycle
  name (`script.compiled`, `command.declared`, `projector.declared`,
  `capability.retired`, `timer.declared`, `file.stored`, `snapshot.folded`,
  `kernel.initialized`) is refused outright, before anything is touched.

## anti-goals

- Never rewrite, reorder, or re-serialize a kept event. The fold selects; it
  does not edit.
- Never touch `files/` — blobs are user content, and the archive only ever
  adds one.
- Never print diagnostics to stdout. Stdout is event JSONL; everything else
  goes to stderr.
- Never fold an empty working set into existence: a log with nothing to
  fold is left untouched, byte for byte.
- Never depend on the kernel remembering anything between operations — the
  whole point is that it doesn't.

## what good looks like

1. An instance that has lived a while — revisions, heartbeats, a weekly
   timer — runs `self run snapshot`. The log shrinks to its working set; a
   `snapshot.folded` marker and one `file.stored` for the archive land on
   top.
2. Every page renders exactly as before. The timer does not re-fire. The
   next appended event takes the next sequence number above the archived
   range.
3. `self rehydrate` from the folded log + `.secret` rebuilds every installed
   script byte for byte — the latest receipts survived the fold.
4. `/snapshots` lists the fold with a link; the archive downloads as plain
   JSONL, replayable and readable, the old log exactly.

The mechanics above are pinned by the kernel's own test suite
(`main_test.go`, the snapshot tests), against a reference implementation a
brain can consult — so a grown `snapshot` can be checked against the
invariants the kernel actually guarantees.
