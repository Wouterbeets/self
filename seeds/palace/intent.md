# palace — the method of loci, made literal

## purpose

The ancients memorized by building palaces in the mind: put each fact in a
room, then walk the rooms to recall them. This instance already has the
facts — every event in its append-only log — so this seed builds the palace.
Each event becomes a room, deterministically derived from that event's own
bytes: your memories are halls, your verses are echoing galleries, and the
signed receipts of the palace's own construction stand as chambers inside
it. The log is unbounded, so the palace is never finished: every event
anyone appends, from any capability, builds a new room. Walking the palace
is walking your own history — and because the whole thing is a pure fold of
the log, `rehydrate` rebuilds the identical labyrinth offline, and two
instances that adopt this seed each get a palace shaped like their own past.

## surface

- `self run walk <door>` takes one step. The door is one word — `back`
  (toward the previous, older room), `forth` (toward the next, newer room),
  or `tunnel` (through this room's hash-tunnel, where one exists). A valid
  step appends one `palace.walked` event `{to, via, by}`; `to` is the
  **seq of the destination room, resolved at walk time** and replayed
  verbatim forever after. A door that does not exist in the current room
  appends `palace.stumbled` `{door, by}` instead — never an error. `by` is
  read from `SELF_MIND_ID`, defaulting to "an unnamed walker".
- `/palace` renders, in order: the current room (its number, its derived
  name, and the inscriptions on its walls — the underlying event's name,
  timestamp, and payload fields, escaped and truncated); one small form per
  door, each POSTing its door word to `/run/walk` via a hidden field with
  the button naming where it leads; the trail of recent steps; and a map of
  every room in chronological order, marking which are visited and where
  the walker stands, with a count of rooms, steps, and how much of the
  palace has been seen.

## the mechanics (exact — determinism lives or dies on these)

- **Rooms.** Every event in the log is a room, keyed by its `seq` — except
  `palace.walked` events, which are footprints, not walls. (`palace.stumbled`
  IS a room: a wrong turn builds a dead-end chamber.) Room character —
  its noun, its light, its material, the name of its tunnel — is derived
  only from a cryptographic hash of the event's immutable `id`, so the same
  log always grows the same palace. An event's name may color the derivation
  (a `memory.noted` room should read as a hall of memory, a `verse.linked`
  room as a gallery where a poem echoes, a `script.compiled` room as a
  scriptorium papered with the palace's own blueprints), but never break it.
- **Position.** The walker stands in the `to` of the latest `palace.walked`
  event whose `to` is a real room; with no valid steps yet, at the oldest
  room. A `to` that names no room (a hostile or foreign event) is ignored —
  inert, like every hostile payload here.
- **Tunnels resolve at walk time.** A tunnel's destination may be computed
  from the room's hash over the rooms that exist *now*, so it must be
  resolved by the `walk` command and recorded as a seq in the event. A
  projection must never re-resolve a past step — the log remembers where
  the tunnel actually led, even after the palace has grown.
- Both scripts fold the log the same way; whether they share that fold or
  each carry it is the compiler's choice.

## anti-goals

- Never a random number, a clock read, or any input beyond argv, stdin, and
  the documented environment — the palace must replay byte-for-byte.
- Never render a payload unescaped; an event crafted to look like markup is
  just a strangely inscribed wall.
- Never refuse the empty log: no rooms renders the threshold — the palace
  awaits its first event — with no doors and nothing broken.
- Never a way to win, finish, or fail. It is a palace, not a puzzle; the
  walk is the point.

## what good looks like

1. An instance with some history grows this seed; `/palace` already has a
   dozen rooms, because the instance's past — including this seed's own
   `intent.declared` and its compile receipts — became architecture.
2. The walker steps `forth`, `forth`, `tunnel` — and lands somewhere old,
   because a tunnel is chronology folded over itself.
3. A `remember` or a `verse` elsewhere in the instance adds a room; the
   next render shows the palace has grown, with the walker standing exactly
   where the log says they stand.
4. `self rehydrate` from `events.jsonl` + `.secret` alone rebuilds the same
   palace, the same trail, the same room underfoot — byte for byte.
