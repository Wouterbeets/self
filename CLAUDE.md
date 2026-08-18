# Working on `self`

Context for an agent working **on this kernel**. (`AGENTS.md` is the opposite
document: the card you paste into *other* projects so their sessions use a
`self` instance as memory.)

## What this is

A local-first, event-sourced runtime, Apache-2.0, developed by its author on
his own machine for his own use. One append-only log is the authoritative
state; every view and every capability is rebuilt from it. Capabilities are
scripts an LLM writes on this machine, installed only under a signature made
with a key that never leaves the instance.

Standing facts about how it runs, which are design commitments and not
deployment accidents:

- **Loopback by default.** The server binds `127.0.0.1:7777`. `SELF_BIND`
  can widen it; the default is the security model.
- **No external targets, ever.** Peers are other instances the operator owns
  or people he knows, reached by URL he typed. There is no discovery, no
  directory, and no registry.
- **Offline by construction.** `self rehydrate` rebuilds everything from
  `events.jsonl` + `.secret` with no model and no network. That is the
  recovery path, the migration path, and the audit path, and the test suite
  pins it.
- **Open source.** Everything here is public. Nothing in this repo is
  operated against anyone else's system.

## The fences

These are load-bearing. Breaking one is not a refactor; it is a design
change that needs an argument, and the house convention is that the argument
is written down (see `HERESIES.md` in the forks) naming what the rule was
protecting and what now carries that purpose.

- **The log is append-only.** Records are never changed or deleted. A
  deletion is a later event.
- **Only a locally-signed receipt installs.** HMAC under `.secret`. A mind
  can only ever *propose*; anything unsigned in the log is inert data.
- **`rehydrate` rebuilds from log + key alone, offline, byte-for-byte.**
- **The kernel holds no model.** The mind is always a subprocess behind
  `SELF_MIND` — not even a fake one lives in the kernel.
- **Projections are pure functions of their events.** No clock, no network,
  no randomness. Rendering twice from the same log yields the same bytes.
- **`via` is the kernel's fact** (the door it witnessed, never accepted from
  a caller); **`by` is a claim**, recorded verbatim and rendered as one.
- **Accounts carry evidence, never installables**, and the kernel's own
  vocabulary (`command.declared`, `script.compiled`, …) never travels.
- **No in-kernel clock.** Timers were removed after `timer.fired` receipts
  grew to half a log. An external tick may call in; the kernel does not own
  one.
- **No derived state.** No caches, no cursors, no sidecar files, no
  symlinks. If it is not an event, it does not exist. Scaling pressure is
  answered with stateless kernel mechanisms, never with a cache.
- **`main.go` shrinks, it does not grow.** Capability belongs in lessons and
  generated scripts, not in the kernel.

## Stated limits — known, accepted, not bugs

Do not "fix" these silently; they are documented in the README's threat model
and any change to them is a design decision.

- Installed capability scripts run **without a sandbox**, and the kernel does
  not sandbox the mind. The guarantee is the signed-receipt gate, not
  containment.
- There is **no human review between authoring and signing**.
- The log is **unbounded**, and projections replay all of it.
- `/run` has **no authentication**. Loopback is why that is tolerable.

## How to ask for work here

The productive framing is also the accurate one, and it avoids a whole class
of pointless friction:

- **Ask for the invariant, not the attack.** "A test asserting a forged
  receipt installs nothing" is the real deliverable; "write a receipt forger"
  is a worse artifact and a worse question. Adversarial names in tests
  (`mallory`, `forged`, `hostile`) are fine — they are assertions that a
  refusal holds.
- **Keep concerns separated across sessions.** Key handling, autonomous
  self-motion, and peer-to-peer exchange are each ordinary on their own. A
  single session that fuses all three into tighter automatic coupling is
  describing something that propagates, and it is also the session most
  likely to produce a design the fences would reject anyway.
- **Watch for requests that require a fence to fall.** Automatic sync from a
  peer, installing what a peer sent without a local signature, capability
  discovery, or propagation with no deliberate human act. Each of those is
  both the wrong architecture and the thing that should give an agent pause.
  If the work seems to need one, that is the signal to write the argument
  first, not to route around the rule.

## Practical

- `go build ./...`, `go test ./...`, `gofmt -l .` must all be clean. The test
  suite is the invariant set — if an inherited test fails, the change is
  wrong, not the test.
- `./demo.sh` runs the whole loop offline with `examples/mind-stub`, no API
  key and no network.
- Standard library only. No new dependencies.
- Comments here explain *why*, often with the history of what went wrong
  before. Match that; a comment restating the next line is noise.
