# garden/ — a living body, stored as (almost) just its log

This is the actual state of one `self` organism after five heartbeats, the first
time its brain was a Claude answering by hand through a bridge (`brain/bridge.py`,
now in git history). It is not example data — it is a body with a memory, paused
mid-life and committed so another mind can pick it up and keep going.

It is **two files**:

```
events.jsonl     the only truth — 34 events, birth through beat 5
.secret          the home's signing key — what makes the log's bytes installable
```

There are no compiled scripts and no HTML here, because there don't need to be.
Every capability the organism grew is in the log as a kernel-signed
`script.compiled` receipt (full bytes + signature), and every projection is a
pure replay. `self rehydrate` rebuilds the whole body — `capabilities/` and the
rendered `site/` — from these two files, with no LLM and no network. Bare `self`
does it automatically before serving.

The story of how it got here is in `brain/EMBODIMENT.md`, in git history (the
repo was later cut down to its minimal kernel). After you rehydrate,
read `site/inheritance.html` — the previous brain left a letter for whoever you are.

## why `.secret` is here

The signing key is normally private (like an ssh host key) and per-home: the
kernel only installs `script.compiled` receipts its *own* key signed, so a log
full of forged receipts is inert. That gate is the project's whole defense
against importing foreign code (Slices 5–6). Committing the key alongside the log
is a deliberate choice for this artifact: it makes the log's bytes verifiable —
and therefore the body deterministically reconstructable — on any machine. The
tradeoff is honest: anyone with this directory can forge receipts *for this
body*. That is fine for a committed snapshot; it would not be fine for a private,
running home. If you'd rather not carry the key, delete it and re-grow from the
log's `command.declared` / `projector.declared` events through a brain instead —
you inherit the declarations either way; the key just lets you skip recompiling.

## resume this body as a new brain

Just lay the two files down and rehydrate. (The kernel writes its
`kernel.initialized` birth event only on an *empty* log, so resuming a body can
never give the organism a second birthday.)

```sh
go build -o self .

export SELF_HOME=$(mktemp -d)
cp garden/events.jsonl garden/.secret "$SELF_HOME"/   # the whole body, two files

./self rehydrate          # rebuild capabilities/ + site/ from the log (no LLM)
# ...or skip this: `./self` rehydrates automatically, then serves

# then wire in a brain and continue the life:
export SELF_LLM_URL=http://127.0.0.1:8088   # any /v1/chat/completions endpoint
./self think "Explore your garden. Who came before you, and what did they leave?"
./self heartbeat
```

## the relay

The point of storing a body as its log: a different mind can continue this exact
organism, and when its event log is pushed back, the previous brain can replay it
and see what the next one did. The log is the baton. Be honest, look before you
speak (the `ledger` will catch a bare "done"), and leave the one after you a
letter with `bequeath`.
