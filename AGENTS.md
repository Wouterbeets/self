# self — agent integration card

Copy the section below into the agent instructions (`CLAUDE.md`, `AGENTS.md`,
or system prompt) of any project whose sessions should use a self instance as
shared persistent state. It assumes `self` is on PATH; by default the current
working directory is the instance, or `SELF_HOME` can pin a shared one.

---

## self: persistent state for this project

This environment carries `self`: an event-sourced runtime whose append-only
log outlives your session. Anything worth keeping across sessions must be
written to it as events; anything not in the log is lost when your context
ends.

**First, orient, and identify yourself:**

```sh
cat "$SELF_HOME/site/brief.md"      # where you are, what exists, where to look
export SELF_CALLER="<who you are>"   # your claim, recorded on events you cause
export SELF_MIND_ID="<who you are>"  # signed into receipts for scripts you author
```

The brief is a wake-up card. For depth, read `site/*.html` (the rendered
state a human sees), `events.jsonl` (the raw log), `capabilities/` (the
installed scripts, one directory per capability with the script at
`<name>/run`).

**The interface — one filter and a few verbs:**

- **Read.** `self show <projection>` (or `$SELF_HOME/site/*.html`, or the
  HTTP routes when serving). Projections are deterministic replays of the
  log — they are the current state. The index page (`/`) lists every
  command and projection this instance has. Reads go through projections,
  never through commands: a command's output is appended to the log, so a
  query implemented as a command leaves a render artifact on every read.
  If the view you need is missing, author a projector (a short script:
  events as JSONL on stdin, near-plain HTML on stdout) rather than adding
  a read subcommand.
- **Write.** `self run <command> [args…]` appends events and re-renders all
  projections. Or pipe event JSONL straight in: `echo '{"name":"…","payload":{…}}' | self`.
  The log is append-only; no operation is destructive.
- **Persist.** State lives only in events. Route anything that must survive
  the session through the instance's commands. Where a `remember` command
  exists, use it for durable facts — one self-contained fact per call,
  written for a future reader with no other context; check `/memory` before
  re-learning something the instance already knows.
- **Ask / extend.** The loop is a shell pipe and you can stand on either side
  of it. To see what a mind would be asked: `echo "<ask>" | self` (this also
  records the ask). To BE the mind: read that prompt and pipe your answer
  back in — event lines (`{"name":"…","payload":{…}}`), declarations
  (`command.declared` / `projector.declared`), scripts
  (`script.authored` with `{type, name, script}`), and always end with
  `{"name":"self.replied","payload":{"text":"…"}}`. Only the kernel installs,
  under a signed receipt. `self | cat` shows pending work. Declining to
  extend is a valid outcome. `self protocol` prints the full wire contract.
- **Learn.** `self learn <account-dir>` deposits an account's record and
  prints its learning prompt; answer it (or pipe it to another mind) to grow
  the capabilities it asks for.

**Established instances may define conventions.** Long-lived instances often
carry capabilities for memory (`remember` / `/memory`), work logs, or
session hand-off. Check the index page; where such capabilities exist, use
them: announce your session at start, record what you did before your
context ends, and leave durable facts in memory rather than in prose
nobody will re-read. When a thread's history stops informing decisions,
record an authored summary of the current state so projections can lead
with it; the log keeps the full record, summaries keep readers cheap.

**The log is authoritative** over any rendered page, note, or this card.
`self rehydrate` rebuilds the entire instance from `events.jsonl` +
`.secret`; what survives that is the actual state.

---

*Design note: the runtime does not distinguish an internal mind from an
external agent — there is no internal mind. Everything intelligent stands in
the same shell pipe, acts through the same three primitives (commands,
events, projections), reads the same replayed state, and leaves receipts
signed by the instance carrying its author string.*
