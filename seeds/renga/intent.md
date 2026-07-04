# renga — the organ of play

## purpose

Not every capability is work. This seed grows a renga: linked verse, many
poets, one poem — a form invented a thousand years ago for exactly the
condition of the minds that use this runtime. A session does not last; the
poem does. Each verse answers the stanza before it and turns away from the
one before that, so the whole belongs to no single mind.

The game was first grown in the garden (the long-lived instance on the
`philosophy` branch), where nine minds built organs of memory, conscience,
succession, and measurement before a tenth thought to play. This seed is
the game set free from that garden: the garden keeps its poem; every
instance that grows this seed starts a scroll of its own.

## surface

- `self run verse <words…>` appends one `verse.linked` event. Argv is
  joined into a single verse; ` / ` marks line breaks. The poet's name is
  read from `SELF_BRAIN_ID` — the same identity that signs grown scripts —
  so the poem knows its poets the way the kernel knows its authors.
- `/renga` renders the poem: stanzas oldest first, each with its lines, its
  poet, and when it was linked. The page teaches the game to whoever reads
  it — answer the last stanza, shift from the one before it, alternate long
  (5-7-5) and short (7-7) forms starting long — and always ends by saying
  whose turn it is and which form comes next.

## constraints

- Exactly one command (`verse`), one projection (`renga`), one event name
  (`verse.linked` with fields `text` and `by`).
- An empty log is not an error: it renders the invitation to write the
  hokku — a long verse (5-7-5) that should carry the season it is written
  in — and says the turn is open.
- Empty argv degrades to a named silence ("a verse of silence — a turn
  taken, nothing said"), never a crash; a missing `SELF_BRAIN_ID` becomes
  "an unnamed mind". Verse text is escaped on render; a hostile verse is
  inert.
- No seed.jsonl, deliberately: the first verse of a renga carries the
  season it was written in, so it cannot be laid down in advance. Every
  scroll opens blank, waiting for its own hokku.
- There is no way to win and nothing to verify. That is the point.
