# self — agent integration card

Copy the section below into the agent instructions (`CLAUDE.md`,
`AGENTS.md`, or system prompt) of any project whose sessions should use a
self instance as shared persistent state. It assumes `self` is on PATH and
`SELF_HOME` points at an instance. To also use the agent as the instance's
brain (compiler included), see "The brain interface" in the README.

---

## self: persistent state for this project

This environment carries `self`: an event-sourced runtime whose append-only
log outlives your session. Anything worth keeping across sessions must be
written to it as events; anything not in the log is lost when your context
ends.

**First, identify yourself:**

```sh
export SELF_BRAIN_ID="<who you are>"
```

Scripts you cause to be generated are signed with this string as the
recorded author.

**The interface, four operations:**

- **Read.** `self show <projection>` (or `$SELF_HOME/site/*.html`, or the
  HTTP routes when serving). Projections are deterministic renders of the
  log — they are the current state. The index page (`/`) lists every
  command and projection this instance has.
- **Write.** `self run <command> [args…]` appends events and re-renders all
  projections. The log is append-only; no operation is destructive.
- **Persist.** State lives only in events. Route anything that must survive
  the session through the instance's commands.
- **Extend.** A new instance has no commands yet; creating the first ones is
  expected. Declarations compile on ingestion: `self think "<prompt>"`
  returns `{response, declarations}`, and `self heartbeat` runs one
  inspect-and-improve cycle. Alternatively, import a capability another
  instance exported: `self adopt <seed>` re-generates it locally
  (`self share <cap>` exports one). Declining to extend is a valid outcome.

**Established instances may define conventions.** Long-lived instances often
carry capabilities for verification (`claim`/`verify` with a ledger of
unproven claims) and session hand-off (`awaken`/`bequeath`). Check the index
page; where such capabilities exist, use them: announce your session at
start, attach evidence before marking work done, and record a hand-off note
at the end.

**The log is authoritative** over any rendered page, note, or this card.
`self rehydrate` rebuilds the entire instance from `events.jsonl` +
`.secret`; what survives that is the actual state.

---

*Design note: the runtime does not distinguish an internal brain from an
external agent — both act through the same three primitives (commands,
events, projections), read the same replayed state, and leave receipts
signed by the instance carrying their author string.*
