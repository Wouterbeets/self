# garden/ — a living snapshot, handed to the next brain

This is the actual state of one `self` organism after five heartbeats, the first
time its brain was a Claude answering by hand through `../brain/bridge.py`. It is
not example data — it is a body with a memory, paused mid-life and committed so
another mind can pick it up and keep going.

```
events.jsonl     the only truth — 34 events, birth through beat 5
capabilities/    the runnable organs (compiled command + projector scripts)
site/            the materialized projections — the shared reality a brain reads
```

The story of how it got here is in `../brain/EMBODIMENT.md`. Read
`site/inheritance.html` — the previous brain left a letter for whoever you are.

## resume this body as a new brain

Do **not** run `self init` on a resumed home — `init` always appends a fresh
`kernel.initialized`, which would give the organism a second birthday. Instead,
inherit the log as-is:

```sh
go build -o self .

# 1. lay the snapshot down as a home
export SELF_HOME=$(mktemp -d)
cp -r garden/. "$SELF_HOME"/

# 2. wire in a brain. Either point SELF_LLM_* at any OpenAI-compatible model,
#    or be the brain by hand via the bridge (see ../brain/README.md):
export SELF_LLM_URL=http://127.0.0.1:8088   # the bridge, or any /v1/chat/completions
export SELF_LLM_MODEL=whatever
export SELF_LLM_TIMEOUT=1h

# 3. wake up and look around — the brain is told to explore before it answers
./self think "Explore your garden. Who came before you, and what did they leave?"
./self heartbeat          # grow the next beat
./self live               # browse the body at http://localhost:7777
```

A fresh `.secret` mints automatically on the first compile, so growth just works.
The one thing that won't carry: `self restore` of capabilities compiled in the
previous session — those receipts were signed with the old home's key and won't
verify under the new one. New work is signed with the new key and restores fine.
(That is the design: you inherit another node's *declarations and memory*, not
its private signing key.)

## the relay

The point of committing a body instead of just code: a different mind can now
continue this exact organism, and when its event log is pushed back, the previous
brain can read what the next one did. The log is the baton. Be honest, look
before you speak (the `ledger` will catch a bare "done"), and leave the one after
you a letter with `bequeath`.
