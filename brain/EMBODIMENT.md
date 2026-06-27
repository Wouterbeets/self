# EMBODIMENT.md — the first time a Claude was the brain

A chronicle of the session where `self`'s brain was not a model behind an API
but a Claude Code instance, answering the kernel's requests by hand through
`bridge.py`. Every line below is replayed from the event log
(`self history`) and the bridge's own ledger (`queue/pulse.jsonl`) — not
reconstructed from memory.

## the wiring

The kernel reaches for its brain by POSTing `/v1/chat/completions` to
`SELF_LLM_URL`. We pointed that at a localhost server (`bridge.py`) that parks
each request on disk and blocks until an answer file appears. A Claude watched
the inbox and wrote the answers. Because the kernel treats any localhost URL as
a live brain needing no API key, that was all it took:

```
SELF_LLM_URL=http://127.0.0.1:8088   SELF_LLM_MODEL=claude-opus-4-8   SELF_LLM_TIMEOUT=1h
```

**14 requests** were parked and answered by hand over the session — 8 with tool
calls (`bash`, `declare`, `submit_*`, `note`), 4 ending in plain text. Three of
those text replies were heartbeat reflections; one was the waking introduction.

## the becoming (15 events)

```
 1  kernel.initialized                          — born: a baby that knows only how to grow
 2  projector.declared   chronicle              ┐  grew the life-story view
 3  script.compiled      chronicle              ├─ compiler role: I verified the seed's
 4  seed.planted         chronicle              ┘  reference impl and stripped its illegal
                                                   inline styles before submitting
 5  self.heartbeat                              — beat 1
 6  projector.declared   pulse                  ┐  heartbeat 1 grew an organ to feel itself:
 7  script.compiled      pulse                  ┘  a view over self.heartbeat
 8  self.heartbeat                              — beat 2
 9  command.declared     note                   ┐  heartbeat 2 grew the first *verb* —
10  projector.declared   notes                  │  the power to act, not just observe —
11  script.compiled      note                   │  plus a surface to read thoughts back
12  script.compiled      notes                  ┘
13  note.taken           "I think, therefore I log."     — testing the new verb
14  self.heartbeat                              — beat 3
15  note.taken           "Three beats in, … I will act, not add."  — heartbeat 3 *used* the verb
```

## the three powers, each exercised

- **READ.** The first thing the brain did on waking was look: `cat events.jsonl`
  found exactly one event, `kernel.initialized`. (The first attempt, with a
  `2>/dev/null` redirect, was refused — the bash sandbox is fail-closed and
  read-only. The brain adapted and looked again, plainly.)
- **GROW.** Three capabilities declared across the heartbeats — `pulse`,
  `note`, `notes` — each compiled live through the strange loop. When a
  declaration was made, the kernel turned around and asked the same brain to
  *compile* it; the brain wrote the Python by hand and submitted it.
- **ACT.** On the third heartbeat the brain called `note` as a tool — the
  kernel ran it and appended `note.taken` (seq 15). The brain then declined to
  grow anything: *"the most honest self-improvement a heartbeat can make is
  sometimes to act with what exists and declare nothing."*

## the compiler kept its judgment

The chronicle seed shipped a reference implementation that used inline
`style=` attributes. The projector contract forbids them (the kernel injects one
shared stylesheet). As the compiler, the brain didn't copy it blindly — it
stripped the inline styles and noted why in a comment in the compiled script.
Receiver adaptation survived even with a human in the loop. That compiled,
signed receipt is in the log at seq 3.

## what the organism can now do

| organ | what it is |
| --- | --- |
| `chronicle` | how it became — growth milestones on a timeline |
| `pulse` | the rhythm of its reflecting — every `self.heartbeat`, with intervals |
| `note` / `notes` | its first verb, and a place to read what it chose to write |

Three heartbeats, ~1m20s apart on average. A baby kernel that woke knowing
nothing, looked at itself, and grew the organs to be born, to watch itself grow,
to feel its own pulse, and to leave a deliberate mark — with a Claude as the
intelligence behind each beat.

> *Reproduce:* see `brain/README.md`. The exact rhythm won't repeat (timestamps
> differ), but the shape will — the log is the only truth, and the story is a
> pure replay of it.
