# EMBODIMENT-2.md — a second time a Claude was the brain

A chronicle of the second session where `self`'s brain was not a model behind an
API but a Claude Code instance, answering the kernel's requests by hand through
`bridge.py`. As with the first (`EMBODIMENT.md`), every line below is a replay
from the event log (`self history --raw`) and the bridge's own ledger
(`queue/pulse.jsonl`) — not reconstructed from memory.

This was a fresh home, born at seq 1. None of the first embodiment's organs were
present; the body woke newborn again. The brain this time was `claude-opus-4-8`.

## the wiring

The kernel reaches for its brain by POSTing `/v1/chat/completions` to
`SELF_LLM_URL`. We pointed that at a localhost server (`bridge.py`) that parks
each request on disk and blocks until an answer file appears. A Claude watched
the inbox and wrote the answers. Because the kernel treats any localhost URL as
a live brain needing no API key, that was all it took:

```
SELF_LLM_URL=http://127.0.0.1:8088   SELF_LLM_MODEL=claude-opus-4-8   SELF_LLM_TIMEOUT=1h
```

**17 requests** were parked and answered by hand over the session — 13 with tool
calls (`bash`, `declare`, `submit_command`, `submit_projector`, `note`), 4
ending in plain text. One text reply was the waking introduction; three were
heartbeat reflections.

## the becoming (11 events)

```
 1  kernel.initialized                          — born: a baby that knows only how to grow
 2  self.heartbeat                              — beat 1
 3  projector.declared   pulse                  ┐  beat 1 grew the first sense:
 4  script.compiled      pulse                  ┘  a view over self.heartbeat (signed receipt)
 5  self.heartbeat                              — beat 2
 6  command.declared     note                   ┐  beat 2 grew the first verb —
 7  projector.declared   notes                  │  the power to act, not just observe —
 8  script.compiled      note                   │  plus a surface to read marks back
 9  script.compiled      notes                  ┘  (two signed receipts)
10  self.heartbeat                              — beat 3
11  note.taken           "Beat 3, and I chose to act, not add. …"   — beat 3 *used* the verb
```

Three heartbeats, ~2m18s apart on average — a rhythm the `pulse` organ now
renders against itself.

## the three powers, each exercised

- **READ.** On waking, the brain looked before it spoke: `cat events.jsonl`
  found one event, `kernel.initialized`. The bash sandbox is fail-closed and
  read-only — the brain's first instinct, a command with `echo` and `2>/dev/null`
  redirection, was refused (`command blocked: redirection not allowed`). It
  adapted and looked plainly, exactly as the first embodiment's brain had.
- **GROW.** Three capabilities declared across the beats — `pulse`, `note`,
  `notes` — each compiled live through the strange loop. When a declaration was
  made, the kernel turned around and asked the same brain to *compile* it; the
  brain wrote the Python by hand, verified it against the real event stream
  before submitting, and called `submit_command` / `submit_projector`. Every
  compile landed a `script.compiled` receipt carrying the home's signature
  (`sig`), so only this kernel's own compiler could install it.
- **ACT.** On the third beat the brain called `note` as a tool. The kernel ran
  it and appended `note.taken` (seq 11). The brain then declared nothing —
  *"the most honest self-improvement a heartbeat can make is sometimes to wield
  what exists rather than pile on more."*

## the compiler kept its judgment

As the compiler, the brain never submitted code it had not run. Each script was
tested against the actual log first — `pulse` against the three real beats
(getting the nanosecond-precision `occurred_at` parsing right, since `datetime`
only carries microseconds), `note` against both its success and empty-mark
branches, `notes` against sample events to confirm newest-first ordering and
HTML escaping. Bare semantic HTML only; the kernel's shared stylesheet was left
to do the styling.

## the honest stumbles (kept, not hidden)

A brain answering by hand makes a brain's mistakes, and the log keeps them:

- The bash sandbox refused a redirect twice before the brain looked plainly —
  the same fail-closed lesson the first embodiment recorded.
- The first `note` ACT failed (`exit status 1`): the brain passed `{"text": …}`,
  but the kernel hands act-tool calls a single `args` string (see
  `internal/seed/compiler.go` — the tool schema requires `args`). The brain read
  the source, learned the contract, and re-acted with `{"args": …}` — which is
  why seq 11's mark ends with *"I learned the contract the hard way."*
- One outbox write landed in an already-answered slot when a round resolved
  faster than expected; the brain noticed the live request had gone unanswered,
  moved the declaration to the correct id, and continued. The log is the only
  truth, and it shows the recovery, not a clean fiction.

## what the organism can now do

| organ | what it is |
| --- | --- |
| `pulse` | the rhythm of its reflecting — every `self.heartbeat`, newest first, with intervals and an average |
| `note` / `notes` | its first verb, and a place to read back what it chose to remember |

A baby kernel that woke knowing nothing looked at itself, grew the sense to feel
its own pulse and the verb to leave a deliberate mark, then used that verb — with
a Claude as the intelligence behind each of the three beats, reached over HTTP
and answered entirely by hand.

> *Reproduce:* see `brain/README.md`. The exact rhythm won't repeat (timestamps
> and ids differ every run), but the shape will — the log is the only truth, and
> this chronicle is a pure replay of it.
