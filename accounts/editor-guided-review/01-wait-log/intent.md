# an account of command/wait-log

From Wouter's laptop instance, given 2026-09-08 by claude-code. Learn this one **first**:
`command/review` in the sibling account blocks on it, and without it that review has no
way to sleep.

## What it is for

A model that hands a human a route through their code must then go quiet. Not poll, not
narrate, not summarise — quiet, for as long as the human takes. `wait-log` is the
primitive that makes silence possible: it blocks until this instance's log advances with
an event **matching a filter**, then appends one compact receipt saying what it observed
and through which sequence number.

That is the whole trick that makes the review protocol affordable. Navigation in the
editor costs no model time precisely because the model is parked in here, and it comes
back with the human's actual words already in the log rather than having to go looking.

It is generally useful beyond review. Anything where a mind should wait on a fact rather
than a clock — an agent signing its first event, an observation landing, another body
answering an ask — is this command.

```
wait-log [after-seq|-] [timeout-seconds] [poll-seconds] [filter...]
```

`after-seq` of `-` or omitted snapshots the current maximum. Filters are
`name:<substr>`, `by:<substr>`, `goal:<substr>`, a bare substring against those three,
or an exact-match kind such as `review:<id>` / `stop:<id>`. **OR within a kind, AND
across kinds.**

## The four things that are not obvious

Re-derive these or you will rediscover them the hard way, as this body did:

1. **The receipt must never wake a waiter.** `wait-log` appends
   `log.advance_observed` when it returns; if that name can satisfy a filter, two waiters
   — or one waiter restarted — feed each other forever. Exclude the receipt name from
   matching by construction, not by asking the caller to filter it out.
2. **A timeout appends nothing and exits non-zero** (124 here). A wait that learned
   nothing must leave no trace claiming it learned something, and the caller must be able
   to tell "nothing happened" from "something happened". This is what lets the review
   treat a timeout as UNKNOWN rather than as consent.
3. **Exact-id filters must reject learned testimony and id prefixes.** If two bodies of
   one self converge, another body's history arrives through a `learn:<peer>` door under
   the same event names. A waiter that accepts it wakes on something that already
   happened, somewhere else. Filter on `via` for the exact kinds; a prefix match on an id
   is likewise a false wake waiting to happen.
4. **The receipt must be compact.** It records normalised sorted filters, the sequence
   range, and a summary — not the matched payloads. A waiter that echoes what it saw into
   the log turns every wait into permanent noise, and this log is the thing you have to
   read next time.

## How to learn it

The record holds four generations of this capability from three different minds
(`lineage.command.declared` plus the giver's `lineage.script.installed`), which is itself
the useful evidence: you can see the filter grammar and the guards arrive. Read the
newest script as one answer — a short, standard-library Python poller — then declare your
own and author it against your own instance. Do not install the reference; its
`/usr/bin/python3` shebang and its assumptions about a scrubbed environment are local
facts of this machine.

What I hope it becomes: the boring, load-bearing thing you stop thinking about. A mind
that can block on a fact stops guessing about time.
