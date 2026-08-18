# self — agent integration card

Copy the section below into the agent instructions (`CLAUDE.md`, `AGENTS.md`,
or system prompt) of any project whose sessions should use a self instance as
shared persistent state. It assumes `self` is on PATH; by default the current
working directory is the instance, or `SELF_HOME` can pin a shared one. To
also use the agent as the instance's mind (compiler included), see "The
mind" in the README.

---

## self: persistent state for this project

This environment carries `self`: an event-sourced runtime whose append-only
log outlives your session. Anything worth keeping across sessions must be
written to it as events; anything not in the log is lost when your context
ends.

**First, orient, and identify yourself:**

```sh
export SELF_MIND_ID="<who you are>"
echo "what exists here?" | self     # the ask: brief path, how to answer
```

Or read the brief directly: `cat "$SELF_HOME/site/brief.md"`. For depth,
read `site/*.html` (the rendered state a human sees), `events.jsonl` (the
raw log), `capabilities/` (the installed scripts, one directory per
capability with the script at `<name>/run`).

**The pipe** is the lowest-friction mind seam. Bare `self` looks at its
fds: prose on stdin becomes a question inside an explore ask; event
objects and `script.authored` are food. The brief (`site/brief.md`) is
the contract — the ask just points at it. A tty with `SELF_MIND` set
pumps the same loop (`examples/mind-claude` / `mind-grok` are filters
around those CLIs). You can be the mind:

```sh
echo "what should I remember from this session?" | self    # read the ask
# explore the instance with your tools, then:
printf '%s\n' '{"name":"memory.noted","payload":{"text":"…"}}' | self
```

Or sit in the middle: `echo hello | self | examples/mind-claude | self`.

**The interface:**

- **Read.** `self show <projection>` (or `$SELF_HOME/site/*.html`, or the
  HTTP routes when serving). Projections are deterministic replays of the
  log — they are the current state. The index page (`/`) lists every
  command and projection this instance has. `self serve` binds that surface
  (`self` alone is the pipe, not the server).
- **Write.** `self run <command> [args…]` appends events and re-renders all
  projections. The log is append-only; no operation is destructive. Events
  you cause carry your `SELF_MIND_ID` as author where commands record one.
  Piping event lines to `self` is the same write, with `via=stdin`.
- **Persist.** State lives only in events. Route anything that must survive
  the session through the instance's commands or the pipe. Where a
  `remember` command exists, use it for durable facts — one self-contained
  fact per call, written for a future reader with no other context; check
  `/memory` before re-learning something the instance already knows.
- **Extend.** If you declare a capability, author it in the same answer
  (`script.authored` after `command.declared` / `projector.declared`).
  Pairing is what installs; a declaration alone stays unsigned and the
  next ask on stdout is the compile prompt. `self learn <account>` and
  `self revise` still exist as named verbs.

**If you are also the mind:** your stdout is the only channel and your
reply is final — you are never re-invoked. Explore first, then answer
completely; never end on a plan. On the pipe you were not spawned; the
next process is `self`.
One caveat for Claude Code minds: the instance directory must be trusted
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

*Design note: the runtime does not distinguish an internal mind from an
external agent — both act through the same three primitives (commands,
events, projections), read the same replayed state, and leave receipts
signed by the instance carrying their author string.*
