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
```

Set `SELF_CALLER` before you write. It is the only attribution there is: without
it your events are indistinguishable from every other caller's, so on a shared
instance nobody can tell which agent wrote a line and you cannot filter your own
writes out of what you read back.

### Orient in cost order

Every rung is a read, so none of them can scar the log. What they spend is the
context you still need for the work — so stop at the first rung that answers the
question, and reach for the last one rarely.

| rung | read | costs | tells you |
|---|---|---|---|
| 1 | `self brief` | a few hundred bytes | what exists, what is pending, what stands refused |
| 2 | `self view situation` | bounded, if the instance grew one | what is live and what the last session left open |
| 3 | `self view` (no name) | a line per view | what this log can show you at all |
| 4 | `self view <name> [args…]` | one slice | the specific thing you came for |
| 5 | `self view log` | the entire history — pipe it | every record, uninterpreted |

Views print to stdout, so length is a shell problem and the shell already solved
it. Filter where you stand:

```sh
self view log | tail -30                            # what happened lately
self view log | grep goal.
self view log | awk '{print $3}' | sort | uniq -c   # what this log is made of
tail -3 events.jsonl                                # reading the file is fine; editing is not
```

What no amount of filtering returns is **current state**. Six `goal.created` and
two `goal.closed` are eight records; `tail` will hand you all eight and none of
them says *four open*. Reconstructing that fold is what a view is for, and where
no view exists you will do it by hand on every session, forever. So when you
find yourself folding raw records to answer a question you will have again,
stop: **the view you are simulating is the durable work**, and writing it is
worth more than the answer you were about to give.

A parameterized view's zero-argument form is its index: run it bare and it lists
the keys you may pass. You should never have to know an identifier before the
view will teach it to you.

**If orienting here is expensive, that is not a reason to skip it.** It is the
instance telling you which view it is missing, and growing that view is durable
work the next session inherits. `self brief` reporting *nothing pending, nothing
refused* means no capability is half-built; it does not mean everything in the
log can be read.

### The six things you can do

| | |
|---|---|
| read state | `self view <name> [args…]` — replays a view from the log. Never a command: a command's output is appended, so a query written as a command litters the log on every read. If the view you need is missing, author one. |
| write | `self run <cmd> [args…]` for an installed verb, or print events: `echo '{"name":"note.added","payload":{"text":"…"}}' \| self hear` |
| grow | declare a capability and author its script in one body — see `self help`. Only a kernel-signed receipt installs, and only for something this log declared. |
| learn | `self learn <account-dir>` deposits an account's record and prints its learning prompt; answer it yourself or pipe it to another mind. |
| ask | `self "<question>"` prints the situated prompt a mind would receive. Useful for seeing what another mind would be told. It appends nothing. |
| loop | `self loop -- <mind> [args…]` presents situated turns until one leaves authoritative state unchanged. `--ask <text>` or `SELF_LOOP_ASK` directs pass one only; later passes are naked. Set `SELF_LOOP_MIND` to pin the mind. |

**Escaping a script into JSON by hand is miserable. Don't:**

```sh
jq -nc --arg t command --arg n note --rawfile s /tmp/note.sh \
  '{name:"script.authored",payload:{type:$t,name:$n,script:$s}}' | self hear
```

Run the script first, under the boundary the kernel will give it — a view fed
only its consumed events, from an empty directory, with no `SELF_HOME`; a
command fed the log on stdin. There is no review step between authoring and
signing, so the run you do beforehand is the only one there is.

### Write like a stranger will read it

You are one of several sessions on this instance, and the next one remembers
nothing you did not append.

- **Read before you write.** You cannot recall whether an earlier session
  already recorded this; witness current state through a view first, or you will
  record it twice.
- **Announce, then account.** Say who you are at the start, and before your
  context ends write what you did and what you left open — through a command, so
  a view can lead with it. Prose in a transcript nobody re-reads is not memory.
- **Preserve the practice, not the transcript.** What is worth keeping is a
  locally verified response to a situation a future reader can recognize: its
  trigger, the method that worked, the local constraints, and the evidence it
  worked. Not the failed attempts, not the tool output, not the secrets.
- **Compress when history stops informing decisions.** Write a summary through a
  command so views can lead with it. The log keeps everything; summaries keep
  readers cheap.

**Established instances carry conventions.** A long-lived instance usually has
capabilities for memory, work logs, or session hand-off. Check `self brief` and
use what is there before inventing a parallel one.

### Two things not to do

Do not edit `events.jsonl`, and do not write into `cap/` — installed bytes are
content-addressed against a signed receipt and your edit will simply be
overwritten the next time the capability runs. Do not reach for a harness session
store (`claude -p --continue` and its kin) as memory: it chains state outside the
log, where `rehydrate` cannot replay it and no audit can see it.

**The log is authoritative** over any view, any note, and this card.
`self rehydrate` rebuilds the instance from `events.jsonl` + `.secret`; what
survives that is the actual state.

---

*Design note: the runtime does not distinguish an internal mind from an external
agent, because there is no internal mind. Everything intelligent stands in the
same shell pipe, acts through the same primitives, reads the same replayed
state, and leaves receipts signed by the instance carrying its own claim.*

*Rung 2 above is a convention, not a kernel feature: an instance grows a view
called `situation` that compresses what a cold reader needs first, and every
session reads it before anything else. [`lessons/situation`](lessons/situation)
is the account that teaches it. [`DESIGN.md`](DESIGN.md) argues why perception
belongs to the instance rather than to the kernel, and what the kernel still
owes an agent driving it.*
