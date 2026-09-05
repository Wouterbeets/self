# Built: two `self` instances that talk

> **Status 2026-09-05: built, installed on both bodies, and running.** The
> implementation, the verification and the design notes live in
> `examples/instance-sync/`. Nine capabilities are installed on `home` and
> `laptop`; a systemd timer converges them every 15 minutes; both bodies print
> the same digest hash and a tick is silent. What follows is the plan as it
> stood before the build, kept because its probe results are the reasoning the
> design rests on. Two things changed in contact with the code:
>
> - The **fingerprint is computed over the wire name**, not the local one, so a
>   moment renamed to `lineage.*` on the way out matches itself on the far side.
> - The **sync's own bookkeeping is excluded from the wire**. Without that a
>   sync never terminates: each one writes observations and learn receipts that
>   are themselves moments to give.

# Plan: two `self` instances that talk

Companion to `instance-sync.prompt.md`. Written 2026-09-04 after probing the kernel
at `bbab4f4` in scratch instances. Facts first, then the design, then the order of work.

## 1. Ground truth (verified, not assumed)

| claim in the prompt | reality |
|---|---|
| `id` is global and survives `learn` | **False.** `newEvent` mints 16 random bytes on every append; `learn` calls `newEvent` per deposited event. `readAccount` parses only `name, occurred_at, by, payload` and discards `id/seq/via`. The "same id in give's record.jsonl" observation was circular: `give` reads the *local* log, after landing. |
| `learn` dedups on id | **False.** Learning one account twice into a scratch instance appended two copies of the same event (seq 2 and seq 5, different ids, identical `occurred_at/by/payload`). |
| a deposited intent rides later wakings | **False.** Only pending declarations ride the brief. `intent.declared` is prose in the log; the learning prompt prints once, at `learn` time. |
| capability sync is the hard edge | **Narrower than feared.** The laptop runs the *current* kernel (protocol text sha-identical, 10 commands / 11 views live). `goal`, `howto` and the goal views (`brief`, `tree`, `next`, `context`, `prs`, `health`, `employees`) are portable plain-text scripts; `dispatch`, `reclaim`, `observe-agents`, `preprod-test`, `dbprod`, `dm-wouter` are bound to the laptop (Herdr, `/Users/wouterbeets` paths, gh auth). |

What *does* survive transport verbatim: `name`, `occurred_at` (nanosecond precision), `by`, `payload`.
That is enough for a content key.

Runtime and wire facts, all verified this session:
- `ssh` works from a command's scrubbed environment (`env -i HOME=/nonexistent PATH=<fixed> ssh laptop` succeeded). `SELF_*` variables pass through — the documented config channel.
- **Reverse ssh fails**: `sshd` is inactive on the Linux box (192.168.1.59). Today only home → laptop exists. See decision 4.
- Laptop `self` is at `~/go/bin/self`, not on the non-interactive `PATH`; call it by full path over ssh.
- Interpreters: python3 **3.9.6** on the laptop, 3.13 here. Scripts target 3.9 stdlib.
- `occurred_at` strings are already canonical on both sides (Go RFC3339Nano UTC, trailing zeros stripped, 5 distinct precisions observed). Use the string verbatim; never parse it (Python datetime would truncate nanoseconds).
- Payloads are **not** byte-canonical: 259 laptop lines carry `": "` whitespace, and `give` compacts them through `json.Encoder`. The fingerprint must canonicalize.
- `by` is `omitempty`: absent, `null` and `""` are the same speaker (821 laptop events have no `by`).
- Shippable laptop set: **2,253 of 2,819**. 542 are refused kernel vocabulary (`script.compiled` 178, `projector.declared` 118, `script.installed` 84, `command.declared` 72, `view.declared` 40, `account.given` 30, `self.*` 20, `kernel.initialized` 1). 24 are the dotless v1 name `identity`, which fails `validEventName` and would make `learn` refuse the whole record. The export view must rename those `lineage.identity` (or drop them), and skip refused names entirely.

## 2. The reconciliation key: a fingerprint, computed in a view

```
fp(e) = sha256( name "\n" occurred_at(RFC3339Nano, UTC) "\n" by "\n" canonical(payload) )
canonical = JSON with sorted keys, no whitespace (python json.dumps(sort_keys, separators=(',',':')))
```

Identical on both bodies for the same event, before and after `learn`. No kernel change.

**Syncable set** = events whose `name` is not in the kernel's refused set and does not
start with `lineage.`, minus an optional exclude-prefix policy (`SELF_SYNC_EXCLUDE`).

**Digest** = sorted fingerprints, one per line. 2,819 × 65 B ≈ 180 KB. A Merkle tree
is premature; `sha256(digest)` is the one-line "has the peer changed?" probe.

**Watermark** = per peer, the max `occurred_at` received, kept in `sync.observed`.
Fast path: export from the peer everything after the watermark, drop what my digest
already holds. Correct for two peers (anything old the peer holds that I lack cannot
exist: it came from me). Repair path `--full`: ignore the watermark, compare full digests.
Move to full-digest-always if a third body ever joins.

## 3. Capabilities

### peers — "you are many"
- `peer.declared / peer.updated / peer.retired` — stable record keyed by name:
  `{name, ssh, self_home, os, role}` e.g. `laptop → wouterbeets@192.168.1.66, /Users/wouterbeets/.self, macos, work`.
- view `peers` — each peer, how to reach it (`ssh <ssh> 'SELF_HOME=<self_home> self brief'`),
  last sync each direction. Its description is what teaches a cold mind it has a sibling.
- `self.identity` rewritten on both: *"one self, two bodies: `home` (Linux, always on,
  runs the sync timer) and `laptop` (macOS, work). Work is done from home and personal
  things from work; the other body is one `ssh` away."*
  Caution: the existing `identity` view is "newest wins" and would adopt the peer's
  identity after a sync. Fix: identity events carry `instance`, or the view filters
  `via != learn:*`. This is the first concrete case of Q4 (semantic conflict is a view problem).

### sync
- view `sync.digest` (consumes `*`) — `sync.digest` prints sorted fps; `sync.digest hash` prints one sha.
- view `sync.export <since|all>` (consumes `*`) — syncable events after `since` as
  record.jsonl lines carrying only the four portable fields.
- command `sync.pull <peer> [--full]` —
  1. `ssh peer 'self view sync.digest hash'`; if equal to the last observed, exit 0 printing nothing (quiet tick appends nothing).
  2. `ssh peer 'self view sync.export <watermark>'` → drop fps I hold → write account
     `$SELF_HOME/sync/in/<peer>/{intent.md,record.jsonl,manifest.json}`. Basename = peer, so the door is `learn:<peer>`.
  3. `self learn` it (mechanical half only, no mind). Empty delta → skip the learn entirely, so no `intent.declared/lesson.learned` noise.
  4. print `sync.observed {peer, direction:pull, received, skipped_known, peer_digest_sha, watermark}`.
- command `sync.push <peer>` — mirror: write the account on the peer over ssh into
  `<peer self_home>/sync/in/<me>/`, run `self learn` there, print `sync.observed {direction:push…}`.
- view `sync` — status card from `sync.observed`: per peer, last pull/push, counts, watermark.
- driver: systemd timer on the Linux box (the always-on one), every 15 min, under `flock`:
  `self run sync.pull laptop && self run sync.push laptop`. Laptop asleep → ssh fails →
  non-zero exit → nothing appended. Next tick retries. No live-lock possible: union is
  idempotent under fingerprint dedup, and `learn` is all-or-nothing.

### dispatch — asking the other body to do something
Rides sync; no second transport.
- command `ask.peer <peer> <text…>` → `peer.asked {key, from, to, ask}`.
- view `inbox` — asks addressed to *this* body without a `peer.answered`. Its description
  ("asks from your other body awaiting action") puts it in every waking's brief.
- command `answer.peer <key> <text…>` → `peer.answered {key, summary}`; syncs back.
- `dispatch <peer> <text…> --now` = `ask.peer` + `sync.push` + `ssh peer 'self loop --ask "answer your inbox" -- claude -p'` for a synchronous wake.
- The laptop already has `agent.*` orchestration (757 events); `inbox` should be where that
  machinery looks for work. Inspect it over ssh before designing further.

## 4. Capabilities across bodies, and the threat model
- Nothing runnable crosses on the wire. Kernel vocabulary is refused; no same-owner trust mode. Ssh between the two boxes already grants full code execution in that direction, so a trust mode would buy nothing and cost the protocol's one invariant.
- **Portable capabilities are installed deliberately, not synced.** For `goal`, `howto` and the goal views, read the laptop's live script over ssh (`cat ~/.self/cap/command/goal/run`), test it here, and print `script.authored` locally so the *home key* signs it. A mind or the human chooses per script; that is authoring, not transport. Without this, the 1,425 synced `goal.*` events land here with no view to read them.
- **Machine-bound capabilities stay where they are** and are reached through the inbox: from home, ask the laptop to `dispatch`; the laptop answers with what it observed.
- A compromised peer can deposit *evidence* (events under names my views read). That is the accepted threat, identical to learning any account; `via:learn:<peer>` is on every such event and views decide what to trust.

## 5. Decisions for the human
1. **Kernel untouched (recommended)** vs. teaching `learn` to skip fingerprints it already holds. The latter makes every account idempotent, which is nice, but it changes what `lesson.learned` counts and is a protocol edit. Start at the sync layer; revisit after a week of ticks.
2. **Exclude prefixes?** Recommend excluding `script.`, `*.declared` (dead v1 vocabulary — refused anyway) and probably `agent.reclaim_skipped` (224 events of noise). `goal.*`, `howto.*`, `pr.*`, `dbprod.*`, `project.*`, `memory.*`, `preprod.*`, `agent.started/failed/observed` come over.
3. **No separate snapshot step.** The first `sync.pull laptop --full` *is* the bridge the prompt asked for, with provenance intact and no clobbered identity or key.
4. **Enable `sshd` on the Linux box?** Without it, only this box can drive transport, and "wake the home body now" from work is impossible. Async dispatch through the inbox still works both ways because home pulls. Recommend enabling it (key-only, VPN-side) so either body can wake the other; otherwise the plan stands with a single driver here.
5. **The 24 dotless `identity` events**: carry as `lineage.identity` (recommended, they are the laptop's early self-description) or drop.

## 6. Order of work
1. **Reach.** Done for home → laptop (brief read, scrubbed-env ssh verified, kernels at parity). Remaining: decide on `sshd` here; declare peers and install `peers` on both bodies.
2. **Digest and export views.** Author, test on the scratch harness already in the session scratchpad (two instances, give/learn round trip, fp equality before/after learn).
3. **`sync.pull`.** Run once from here with `--full`: 2,253 events land under `learn:laptop`. Then install the portable capabilities here (section 4) so `self view next`, `tree`, `howto` read them.
4. **`sync.push`, `sync` view, systemd timer.** Watch two or three ticks be quiet.
5. **`inbox` / `ask.peer` / `answer.peer`, identity rewrite** on both bodies. First real dispatch: from home, ask laptop to summarize its open goals; watch the answer arrive by sync.
6. **Author on the laptop.** `sync.digest`, `sync.export`, `peers`, `inbox`, `ask.peer`, `answer.peer` — same scripts, signed by its key. `sync.pull`/`sync.push` only where reverse ssh exists.

Steps 1–3 fit one sitting. Nothing in the kernel changes.
