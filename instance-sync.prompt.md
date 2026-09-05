# Prompt: let two `self` instances talk instead of copying

*A wake-up card for a mind, not a spec. Read the protocol first (`self help`),
then design — and, if you can verify it this waking, grow — a capability that
lets two `self` instances the same human owns **converge continuously** over the
account wire. This file is written to be handed to a mind:
`self "$(cat instance-sync.prompt.md)"` or deposited as an intent.*

---

## The situation

One human, two instances of `self`, and a want: "get all the data from the
laptop onto this machine." Investigating it turned a copy job into a protocol
question.

| | **source — `.66`** (laptop, macOS) | **dest — this machine** (Linux) |
| --- | --- | --- |
| `SELF_HOME` | `/Users/wouterbeets/.self` | `/home/wouter/.self` |
| log | **2,819 events / 6.25 MB** | 54 events |
| character | a real working instance | fresh / near-scratch |

What lives in the 2,819 events on `.66` (top event names):

- **Goal tracking — ~1,425 events.** `goal.progress` ×690, `goal.updated`
  ×221, `goal.created` ×216, `goal.context` ×182, plus `goal.milestone`,
  `goal.kpi`, `goal.tree`, `goal.compacted`, `goal.completion.requested`,
  `goal.removed`.
- **Agent orchestration — ~757.** `agent.reclaim_skipped` ×224, `agent.started`
  ×180, `agent.requested` ×169, `agent.reclaimed` ×118, `agent.observed`,
  `agent.failed`.
- **Capability plumbing — ~490.** `script.compiled` ×178, `projector.declared`
  ×118, `script.installed` ×85, `command.declared` ×73, `view.declared` ×40.
- **Work knowledge.** `dbprod.queried` ×50 (production DB query records),
  `howto.saved` ×31, `pr.observed` ×20, `project.registered` ×6,
  `preprod.test_run`, `memory.noted`, and the `self.identity`.

Both machines are the same person's, reached over their home VPN. So this is
device-to-device sync of one human's own tooling — not sharing between owners,
which is the case the account wire was originally shaped for.

## What the wire actually is (observed, not assumed)

An **account** is the only format that crosses between instances: a directory of
plain text — `intent.md`, `record.jsonl` (events verbatim), `manifest.json`
(`{events, record_sha256, prefix?, capability?}`). `self give <selector> <dir>`
writes one; `self learn <dir>` is the only way in. **Nothing runnable travels**:
`give` renames kernel vocabulary to `lineage.*` and `learn` refuses any record
that carries the raw kernel names, so a deposited `command.declared` can never
become pending work that authors an attacker's script under the local key.

The load-bearing observation, straight out of the log:

```
seq 1  intent.declared  via:cli         by:claude
seq 2  self.identity    via:learn:chat  by:claude   id:b6af7448…
```

That `self.identity` arrived by **learning** the shipped `chat` account. On the
way in the kernel **kept its `id`, its `occurred_at`, and its `by`, and
re-stamped only the door (`via`) to `learn:chat`.** Confirm it yourself: the
same `id:b6af7448…` shows up in `self give self. <dir>`'s `record.jsonl`. So:

- **`id` is global.** It is content-stable and survives transport through
  `learn`. This is the reconciliation key.
- **`seq` is local.** The kernel re-assigns it on every append, including on
  `learn`. It orders *one* log and means nothing across two.
- **`occurred_at` and `by` are portable testimony** — preserved on learn.
- **`via` (the door) is a local fact** — re-stamped to `learn:<account>`, which
  is precisely how a view can tell a synced event from a home-grown one.

## Why copying fails — the conclusion that started this

- **Option A — clone (`rsync` / `scp events.jsonl` + `self rehydrate`).**
  Cheap and faithful *for one instant*. Then both logs keep appending under
  their own **local `seq`**, so within a day both sit at "seq 2900" holding
  **different** event 2900. There is no global clock to merge on and no shared
  cursor to resume from. They have **forked**, and a fork of an append-only log
  never cheaply reconciles again. `rsync` also clobbers the destination's own
  identity and `.secret` key. Copy buys a moment and sells the future.
- **Option B — one-shot `give`/`learn` of every prefix.** Honest to the
  protocol, but it is dozens of accounts, the intelligent half burns a mind per
  account (`self learn acct/ | claude -p | self hear`), capabilities land inert
  as `lineage.*` and must be re-authored, and **when it finishes the two
  instances are already drifting again.** High token cost for a snapshot.

Neither is sync. Both are a photograph of a thing that keeps moving.

## The reframe: reconcile the set, don't copy the file

An append-only log is a **set of events keyed by `id`**, given a convenient
local order by `seq`. Two instances converge when each holds the union of both
id-sets. Because `id` survives `learn`, this is ordinary set reconciliation, and
it can ride the wire that already exists:

1. **Digest.** Each side exposes the ids it holds — as a `view` (pure function
   of the log): sorted ids, or a Merkle summary / bloom filter so the exchange
   is sublinear instead of shipping 2,819 ids every time.
2. **Diff.** Each side computes what the *peer* lacks from the two digests.
3. **Give the delta.** `give` an account of only the missing events. The peer
   `learn`s it; ids it already has must be a no-op (**verify this — see below**).
4. **Both directions.** Run 1–3 each way and the union is achieved. Re-render:
   learned events land under names local views already read, so a goal from the
   laptop shows up in `self view goals` here, its door `learn:<peer>` marking
   where it came from. Views that should show only local testimony filter on
   `via`; views that should show both make provenance visible — the receiving
   mind's call, exactly as the account rules already say.
5. **Continuously.** Wrap it in `self loop` / cron so the two gossip on a
   cadence and stay converged instead of being re-photographed.

Ordering across the merged set uses `occurred_at` (portable), with `id` as the
deterministic tie-break — never `seq`.

## The ask — design this, and grow it if you can verify it now

Design a capability (propose the name; `sync` is the obvious one) that makes two
instances the same human owns **converge over accounts, cheaply, repeatedly,
without losing provenance and without ever installing a peer's code**. A command
appends events and may declare capabilities; a view is a pure replay. Use both.

**Resolve these first — they decide the whole shape:**

1. **Does `learn` dedup on `id`?** If instance L already holds event `X` and
   learns an account containing `X`, does the kernel skip it, or append a second
   copy with a new `seq` and the same `id`? *Everything downstream forks here.*
   Dedup-on-id → step 3 can be a dumb union and the digest is only an
   optimization. No dedup → the digest/diff is mandatory, and you must give only
   the true delta. Probe it in a scratch instance before you build; do not
   assume.
2. **Is `id` genuinely content-derived and stable**, or kernel-assigned per
   append? The `learn:chat` evidence says it travels verbatim — but confirm it
   is reproducible for the *same* logical event created independently, or accept
   that `id` is "stable once minted, then carried" and dedup only works for
   events that have actually met.
3. **Capability convergence.** Knowledge merges cleanly; capabilities are the
   hard edge — they cross as inert `lineage.*` by design, and "only the local
   key installs" must not be weakened even for a peer you own. Decide honestly:
   do capabilities stay per-instance and re-authored (a mind pass reconciles the
   `lineage.*` into local signed scripts), or is there a *narrow, explicit*
   same-owner trust mode — and if so, what exactly guards it (a shared key? a
   fingerprint allowlist? a human confirm per script)? Name the threat you are
   accepting.
4. **Semantic conflict, not event conflict.** The event set never conflicts —
   union is union. Conflicts are in meaning: both instances advanced the "same"
   goal differently. That is a **view** problem — surface agreements,
   contradictions, and what only one side saw, per the account intent's own
   language. Do not invent event mutation to fix it.
5. **Watermark & cost.** Keep a per-peer high-water mark (last `occurred_at`
   synced, or the set of known ids) so a run ships only new events. What does the
   digest look like so a 2,819-event log does not re-hash and re-send itself
   every tick?
6. **Cadence & safety.** How does the loop wake (cron, `self loop`), what stops
   two peers from live-locking, and how does a run that dies mid-transfer leave
   both logs honest (accounts are all-or-nothing on `learn`; lean on that)?

**What good looks like:**

- A `view` that emits this instance's id-digest, pure and cheap.
- A `command` that, given a peer's digest (or reachable account dir), `give`s the
  delta account and records a structured `sync.observed`-style event — trigger,
  peer, counts, watermark — so the next waking sees what happened without a
  transcript.
- The mirror on the peer, and a loop that runs both directions on a cadence.
- Provenance intact: every synced event carries `via:learn:<peer>` and its
  original `by`/`occurred_at`, so any view can separate local from received.
- No raw kernel vocabulary on the wire, and no path by which a peer installs
  code here. Giving stays cheap; learning stays the work; the local key stays
  the only key.

Then this stops being "copy the laptop" and becomes what the protocol was
already reaching for: **instances that talk, and stay talked.**

---

### Immediate next step for the human (no build required)

If a snapshot is wanted *today* while the sync capability is designed, the least
regretful move is **B scoped to knowledge only** — `give`/`learn` the work
prefixes (`goal.`, `howto.`, `pr.`, `dbprod.`, `project.`, `memory.`,
`preprod.`, `self.identity`) into this instance, skip the `agent.*`/`script.*`/
`*.declared` plumbing, and re-grow capabilities here. It drifts like everything
else, but it is reversible (back up `~/.self/events.jsonl` first) and it does not
clobber this machine's identity the way `rsync` would. Treat it as a bridge, not
the answer.
