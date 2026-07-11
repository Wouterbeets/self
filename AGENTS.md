# self — agent integration card

Copy the section below into the agent instructions (`CLAUDE.md`, `AGENTS.md`,
or system prompt) of any project whose sessions should use a self instance as
shared persistent state. It assumes `self` is on PATH; by default the current
working directory is the instance, or `SELF_HOME` can pin a shared one. To
also use the agent as the instance's brain (compiler included), see "The
brain" in the README.

---

## self: persistent state for this project

This environment carries `self`: an event-sourced runtime whose append-only
log outlives your session. Anything worth keeping across sessions must be
written to it as events; anything not in the log is lost when your context
ends.

**First, orient, and identify yourself:**

```sh
cat "$SELF_HOME/site/brief.md"      # where you are, what exists, where to look
export SELF_BRAIN_ID="<who you are>"
```

The brief is a wake-up card. For depth, read `site/*.html` (the rendered
state a human sees), `events.jsonl` (the raw log), `capabilities/` (the
installed scripts, one directory per capability with the script at
`<name>/run`).

**The interface:**

- **Read.** `self show <projection>` (or `$SELF_HOME/site/*.html`, or the
  HTTP routes when serving). Projections are deterministic replays of the
  log — they are the current state. The index page (`/`) lists every
  command and projection this instance has.
- **Write.** `self run <command> [args…]` appends events and re-renders all
  projections. The log is append-only; no operation is destructive. Events
  you cause carry your `SELF_BRAIN_ID` as author where commands record one.
- **Persist.** State lives only in events. Route anything that must survive
  the session through the instance's commands. Where a `remember` command
  exists, use it for durable facts — one self-contained fact per call,
  written for a future reader with no other context; check `/memory` before
  re-learning something the instance already knows.
- **Extend.** Declarations compile on ingestion. `self grow <seed>` grows a
  capability set from an `intent.md`;
  `self revise command/<name> "<change request>"` recompiles an installed
  capability with its current script as context. `self think "<prompt>"`
  is report-only: it returns `{response, declarations}` without ingesting —
  a query, not a mutation. Declining to extend is a valid outcome.

**If you are also the brain** (the kernel spawns you for compiles): your
stdout is the only channel and your reply is final — you are never
re-invoked. Explore first, then answer completely; never end on a plan.
One caveat for Claude Code brains: the instance directory must be trusted
once (`cd $SELF_HOME && claude`, accept the prompt) or the permission
grants in `$SELF_HOME/.claude/settings.json` — the test bench that lets
compiles actually run their scripts — are silently ignored.

**Established instances may define conventions.** Long-lived instances often
carry capabilities for memory (`remember` / `/memory`), work logs, or
session hand-off. Check the index page; where such capabilities exist, use
them: announce your session at start, record what you did before your
context ends, and leave durable facts in memory rather than in prose
nobody will re-read.

**The log is authoritative** over any rendered page, note, or this card.
`self rehydrate` rebuilds the entire instance from `events.jsonl` +
`.secret`; what survives that is the actual state.

---

*Design note: the runtime does not distinguish an internal brain from an
external agent — both act through the same three primitives (commands,
events, projections), read the same replayed state, and leave receipts
signed by the instance carrying their author string.*
