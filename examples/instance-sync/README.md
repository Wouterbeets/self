# instance-sync — two bodies of one self, converging

Built and verified 2026-09-05 between `home` (Arch, 192.168.1.59) and `laptop`
(macOS, 192.168.1.66). Answers `instance-sync.prompt.md`. Installed on both
bodies; the sources are kept here because a log holds installed *bytes*, and
these are the split sources those bytes were assembled from.

## What the probe found, before anything was built

The prompt's central premise is false, and the whole design turns on it.

| the prompt says | the kernel actually does |
| --- | --- |
| `id` is global and survives `learn` | `newEvent` mints 16 random bytes on **every** append, and `learn` calls it per deposited event. `readAccount` parses only `name`, `occurred_at`, `by`, `payload` and discards `id`, `seq`, `via`. |
| `learn` dedups on `id` | It does not. Learning one account twice deposits the same moment twice, under two ids. |

The "same `id` in `give`'s record" observation was circular: `give` reads the
*local* log, after the deposit already landed under a fresh local id.

What **does** survive transport verbatim is `name`, `occurred_at`, `by` and
`payload` — so that tuple, canonicalised, is the reconciliation key.

## The key

```
fingerprint = sha256( "self-sync-fp-v1" ‖ len‖wire_name ‖ len‖occurred_at ‖ len‖by ‖ len‖canonical_payload )
```

- **canonical payload** — sorted keys, no whitespace. `give` compacts payloads
  through Go's encoder, so pre- and post-transport bytes differ; canonicalising
  both sides makes them agree.
- **length-prefixed** — otherwise `("a","bc")` and `("ab","c")` collide.
- **wire name** — the name the moment carries *after* it crosses. Refused kernel
  vocabulary and pre-dotted v1 names (`identity`) become `lineage.<name>`,
  exactly as `self give` renames them. Computing the fingerprint over the wire
  name is what makes a renamed moment match itself on the far side; over the
  local name it would cross forever.
- **`occurred_at` is used verbatim, never parsed.** Go emits RFC3339Nano with
  trailing zeros stripped, so the strings do not sort lexicographically
  (`".2Z"` > `".25Z"` by byte order, `0.2 < 0.25` by clock). Comparison pads the
  fraction to nine digits; parsing to a float would lose nanoseconds.

## The capabilities

| | |
| --- | --- |
| `sync.digest [lines\|hash\|names]` | view — the fingerprint set this body will talk about. `hash` is the cheap "has it moved?" probe. |
| `sync.export [all\|since <ts>]` | view — crossable events as an account's `record.jsonl` lines. `since` is inclusive, so a boundary moment is re-offered rather than lost. |
| `sync.pull <peer> [--full]` | command — read the peer, deposit the delta through `learn:<peer>`, append one `sync.observed`. **Reads the peer only.** |
| `sync.push <peer>` | command — the mirror; the only direction that writes to the other body. |
| `peer here\|add\|set\|retire` | command — the roster, with a full append-only lifecycle. |
| `peers [<name>\|--names]` | view — this body, its siblings, how converged each one is. |
| `ask.peer` / `inbox` / `answer.peer` | work requests riding the same wire. |

`self-sync` plus the two systemd units run both directions for every live peer
every 15 minutes, under `flock`.

## Three things that decide whether it terminates

1. **The sync's own bookkeeping never crosses.** `sync.observed`, and the
   `intent.declared` / `lesson.learned` receipts a learn writes, are excluded.
   Without this a sync never goes quiet: giving N moments writes 3 more, which
   are themselves moments to give, forever. Measured before the fix: every push
   had exactly one more thing to give, indefinitely.
2. **Exclusion matches with `lineage.` stripped.** A name held back here comes
   back as `lineage.<name>` from the peer otherwise, and never settles.
3. **The digest covers exactly what export gives.** So the hash moves only when
   a body has something new to *say*, never because it wrote down that it synced.

## What does not travel

Capabilities. A peer's declarations and scripts land as `lineage.*` reference
material; only the local key signs a script into the local `cap/`. There is no
same-owner trust mode: ssh between these two boxes already grants full code
execution in that direction, so a trust mode would buy nothing and cost the
protocol's one invariant. The accepted threat is unchanged from learning any
account — a peer can deposit *evidence* under names local views read, and every
such moment carries `via:learn:<peer>` for a view to filter on. `self view
identity` does exactly that, so a sibling's identity cannot make this body
introduce itself as the other one.

## Verified

- 2,352 moments crossed laptop→home in 20 s; every fingerprint given was present
  afterwards, and a repeat `--full` pull took 0.
- After converging both bodies print the same digest hash, and a tick is silent
  in both directions.
- A real ask (home→laptop, "which live goals cite an unmerged PR") was answered
  from the laptop's own `prs` view and collected at home on the next tick.

## Rebuilding

`src/build.sh` assembles `src/prelude.py` + each `src/body_*.py` into `out/`.
`src/emit.py <spec.json>` turns a capability list into a hear body;
`src/install.py` installs one directly. Both bodies already hold the installed
bytes, so `self rehydrate` restores them from the log alone.

## The goal surface, made two-body correct

Syncing the events was only half of it: a goal you can read on one body and
update on the other needs capabilities that agree. Porting the laptop's goal
command and views unchanged would have produced two bodies that disagree, for
two reasons that were invisible while there was one log.

1. **`seq` is assigned per body.** The views sorted "most recently touched
   first" by `seq`. A moment learned from the other body today lands with a
   higher `seq` than everything written here yesterday, so the same goals come
   out in a different order on each body.
2. **"Last one wins" folded over log order.** Learned events arrive in one block
   at the point of the learn, not in the order they happened, so a goal's
   "latest" note was whichever body's copy was learned last rather than
   whichever was written last.

Both are fixed by sorting on `occurred_at` before anything folds, with name and
canonical payload as a deterministic tie-break — never `id`, which differs
across bodies for the same moment. Printed `[seq]` locators became `[here]` or
`[<peer>]`, which is the useful thing to show a reader who has two logs; the
detail view keeps the local seq beside it, because there it really is a locator
into this log.

`goal-surface/patch.py` applies this to the five capabilities and is the
record of exactly what changed. Verified: with the patch, `next`, `brief`,
`tree` and `context` render byte-identical output on both bodies once the
body tag is normalised, over 1,369 identical goal events.

**What stayed on one body.** `dispatch`, `reclaim`, `observe-agents`,
`observe-prs`, `dbprod`, `preprod-test` and `dm-wouter` are bound to the
laptop's Herdr, its gh auth and its network position. They are reached by
asking, not by copying. `home` keeps its own capabilities the laptop has no use
for. The bodies are not meant to be the same — only to agree about what
happened.
