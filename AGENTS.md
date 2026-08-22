# self — agent integration card

Copy the section below into the agent instructions (`CLAUDE.md`, `AGENTS.md`, a
system prompt) of any project whose sessions should share one persistent memory.
It assumes `self` is on PATH. The working directory is the instance unless
`SELF_HOME` pins one.

---

## self: persistent state for this project

This environment carries `self`: an event-sourced runtime whose append-only log
outlives your session. Anything worth keeping across sessions must be written to
it as events. Anything not in the log is lost when your context ends.

**Orient first. Reading never changes anything.**

```sh
export SELF_CALLER="<who you are>"   # recorded verbatim as `by` on what you write
self brief                           # what exists, what is pending, what broke
self help                            # the complete protocol — read it once
self view log                        # what happened here lately, and who says so
```

Set `SELF_CALLER` before you write. It is the only attribution there is: without
it your events are indistinguishable from every other caller's, so on a shared
instance nobody can tell which agent wrote a line and you cannot filter your own
writes out of what you read back.

**The five things you can do**

| | |
|---|---|
| read state | `self view <name>` — replays a view from the log. Never a command: a command's output is appended, so a query written as a command litters the log on every read. If the view you need is missing, author one. |
| write | `self run <cmd> [args…]` for an installed verb, or print events: `echo '{"name":"note.added","payload":{"text":"…"}}' \| self hear` |
| grow | declare a capability and author its script in one body — see `self help`. Only a kernel-signed receipt installs, and only for something this log declared. |
| learn | `self learn <account-dir>` deposits an account's record and prints its learning prompt; answer it yourself or pipe it to another mind. |
| ask | `self "<question>"` prints the situated prompt a mind would receive. Useful for seeing what another mind would be told. It appends nothing. |

**Escaping a script into JSON by hand is miserable. Don't:**

```sh
jq -n --arg t command --arg n note --rawfile s /tmp/note.sh \
  '{name:"script.authored",payload:{type:$t,name:$n,script:$s}}' | self hear
```

**Established instances carry conventions.** A long-lived instance usually has
capabilities for memory, work logs, or session hand-off. Check `self brief` and
use what is there: announce yourself at the start, record what you did before
your context ends, and leave durable facts where a future cold reader will
actually look — a view — rather than in prose nobody re-reads. When a thread's
history stops informing decisions, write a summary through a command so views
can lead with it; the log keeps everything, summaries keep readers cheap.

**Two things not to do.** Do not edit `events.jsonl`, and do not write into
`cap/` — installed bytes are content-addressed against a signed receipt and your
edit will simply be overwritten the next time the capability runs. Do not reach
for a harness session store (`claude -p --continue` and its kin) as memory: it
chains state outside the log, where `rehydrate` cannot replay it and no audit can
see it. If a cold start feels slow, that is design pressure pointed at the right
target — improve the views.

**The log is authoritative** over any view, any note, and this card.
`self rehydrate` rebuilds the instance from `events.jsonl` + `.secret`; what
survives that is the actual state.

---

*Design note: the runtime does not distinguish an internal mind from an external
agent, because there is no internal mind. Everything intelligent stands in the
same shell pipe, acts through the same primitives, reads the same replayed
state, and leaves receipts signed by the instance carrying its own claim.*
