# peer-harvest — one body draining another body's queue

## purpose

Execution is cheap to generate and expensive to consolidate. The work body
at `127.0.0.1:7778` carries 114 ended-but-unharvested sessions: every one of
them was read by nobody, and the visible-queue pressure the harvest lesson
relies on stops working once the queue is longer than a mind's patience. A
swarm makes this worse for free — more bodies, more endings, the same single
consolidation loop.

This account grows the body that answers that: a **harvester**, whose whole
job is other bodies' unfinished business. It watches peer bodies through
their doors, keeps its own log of what it has seen pending and seen cleared,
and presents the *oldest* un-promoted outcome first — because the tail is
exactly what a newest-first queue never reaches. It carries no judgment of
its own: promotion stays a mind's act, performed on the peer through the
peer's own verbs. What this body adds is that the backlog becomes
**addressable** — a queue with a front — so a cold mind can wake, take one
item, promote it, and sleep.

## surface

- `watch <name> <base-url>` — append one `peer.watched {name, url, by}`.
  Re-watching a name with a different URL is the correction; the latest
  `peer.watched` for a name wins, and nothing is edited or deleted.
- `scan` — for each watched peer: `GET <url>/events`, the kernel's own
  raw-log route, and replay it. Pending means a `session.started` whose
  `session.ended` exists and whose seq no `session.harvested` ref covers.
  Append one `peer.session.observed {peer, ref, worktree, goal, outcome,
  ended_at}` for each pending session this log has not already observed, and
  one `peer.session.cleared {peer, ref, note, by}` for each previously
  observed session the peer now shows harvested — the note and author come
  from the peer's own `session.harvested` event. A scan that finds nothing
  new appends nothing, so a quiet scan costs the log nothing. An unreachable
  or non-JSONL peer prints one plain warning to stderr, appends nothing, and
  exits zero.
- `/queue` — one page, and the body's whole surface. Above: one line per
  watched peer — name, url, pending count, cleared count, when it was last
  scanned. Below: every pending session across all peers, **oldest ending
  first**, each with worktree, goal, the outcome text in full, how long it
  has been waiting, and the two exact requests that finish it on the peer:
  a `POST <url>/run/<verb>` that promotes the durable fact, and the
  `POST <url>/run/harvest` that records the receipt with pointers. Both
  rendered with an `X-Self-Caller` header carrying this body's name, because
  an unattributed event in a swarm is a worthless event. An empty queue says
  so in one line.

## constraints

- The peer is reached only through its door: `GET <url>/events` to read,
  `POST <url>/run/<command>` to write, `X-Self-Caller` to claim a speaker.
  This body never opens another instance's files, never appends to another
  log directly, and holds no credential for one.
- Pending and cleared are computed at render time from this log alone
  (observed minus cleared). The peer's log stays the authority: re-scanning
  re-derives everything. No cache, no cursor, no state outside the log.
- One page. This is a view of other bodies' work, not a second work
  surface: no per-peer page, no projects page, no dashboard.
- Nothing is promoted automatically. `scan` observes; only a mind decides
  what an outcome taught and issues the POST. A script that guesses what a
  session meant has failed this account.
- Idempotent by construction: observing the same pending session twice
  appends nothing, clearing an already-cleared ref appends nothing, and
  scanning a peer that has not changed appends nothing.

## anti-goals

- Not a scheduler and not a chaser. No timers are declared here, no
  reminders, no nagging of the peer or of anyone. An external clock may call
  `scan`; the body does not own a clock.
- Not a mirror. It does not copy the peer's sessions, outcomes, or work
  facts into its own log as knowledge — it records only that it saw a queue
  entry, and later saw it cleared. Two logs, one act, no second authority.
- No aggregate metrics, no throughput charts, no ranking of bodies by how
  much they left unharvested.
- No writing to a peer from a script, and no local re-statement of what was
  promoted: the promoted event lives in the peer's log, stamped with the
  door it came through and the speaker who claimed it.

## what good looks like

A mind wakes on this body cold and reads `/queue`. The header says: `work
— http://127.0.0.1:7778 — 114 pending, 0 cleared, scanned 3 minutes ago`.
The front of the queue is a session in `mym/services/monolith` that ended
eleven days ago, its outcome describing a production rollout that no work
fact ever recorded. The mind reads the outcome, POSTs `worklog` to the peer
with `X-Self-Caller: harvester`, POSTs `harvest <ref> kept: worklog <seq>`,
and sleeps. The next `scan` sees the peer's own `session.harvested`, appends
`peer.session.cleared`, and `/queue` is 113 long with a new front. On the
peer's side the promoted note carries `via=http:…` and `by=harvester`: one
act, auditable from either end, by a body that spent its whole life reading
other bodies' endings.
