# self

`self` is a small, local-first runtime. It keeps one append-only event log as
its only state, and rebuilds every view — and every capability — from that log.
Capabilities are not shipped as code. You describe what you want, and a
language model of your choosing writes the script for it, on your machine. Every
generated script is installed only under a signature made with a key that never
leaves the instance, so the whole system can be rebuilt from the log alone, with
no model and no network.

The point is durable, inspectable state for an LLM agent: one log, replayed into
every view, with a record of who generated each script and the ability to
reconstruct it offline. What is not in the log did not happen.

This runtime is the reference implementation of a larger idea:

- **[knowledge-seed-protocol](https://github.com/wouterbeets/knowledge-seed-protocol)**
  — the protocol: how local, verifiable knowledge moves between minds as
  replayable reasoning rather than bare assertions.
- **self** (this repo) — the runtime that makes the protocol executable.
- **[emera](https://github.com/wouterbeets/emera)** — an experimental,
  non-gradient "brain" intended to plug into this seam.

It is experimental. See [Limits](#limits-and-threat-model) before you rely on it.

## Quick start

### 1. See the machinery with no LLM (about 10 seconds)

```sh
git clone https://github.com/wouterbeets/self && cd self
./demo.sh
```

`demo.sh` runs the whole loop offline using the built-in stub compiler (no API
key, no model): a declaration compiles into a script, running a command appends
an event, a projection renders it, and the instance rebuilds from
`events.jsonl` + `.secret` alone — byte for byte. The stub writes trivial
scripts; this shows the machinery, not the intelligence.

### 2. Real capabilities (plug a brain)

```sh
go install .                        # `self` on PATH (via GOBIN)

cd ~/my-project
export SELF_HOME=$PWD/.self          # the instance lives beside your code
export SELF_BRAIN="claude -p"        # any executable can be the brain (see below)
self grow seeds/chat                 # generate a capability set from its intent
self                                 # rebuild from the log, then serve at :7777
```

`grow` needs a brain; `SELF_LLM_STUB=1` supplies a deterministic offline one for
mechanical tests and demos, while real capabilities need a real model or agent.
To make every coding-agent session in a project use one instance as shared
persistent state, paste the card in [`AGENTS.md`](AGENTS.md) into the project's
agent instructions. To write your own capability sets, see [`SEEDS.md`](SEEDS.md).

## How it works

An instance is a directory (`SELF_HOME`) with two files of real state:

```
events.jsonl    the append-only log — the only state
.secret         a per-instance signing key (32 random bytes, hex; mode 0600)
```

Everything else (`capabilities/`, `site/`) is derived and can be rebuilt.

**Events.** Each record is `{id, seq, name, occurred_at, payload}`. Records are
never changed or deleted; a deletion is expressed as a later event.

**Commands.** A command is an executable script. It receives its arguments as
`argv` and the current log as JSONL on stdin, and writes new events as JSONL on
stdout (`{name, payload}` per line; the kernel fills in the rest). The emitted
events are appended and all projections re-render.

**Projections.** A projection is an executable that reads all events on stdin
and writes HTML on stdout, saved to `site/<name>.html`. It must be a pure
function of the log: rendering twice from the same log yields the same bytes.

**Runtime code generation.** A `command.declared` or `projector.declared` event
triggers a compile: the brain writes the script from the declaration, the kernel
installs it and records a `script.compiled` receipt. Declarations — not code —
are what cross every boundary.

**Signed installation.** A receipt is `{type, name, script, by, sig}`, where
`sig` is an HMAC-SHA256 over the fields using the instance's key. Only receipts
that verify under the local key are ever installed; anything else in the log is
inert data. `by` records which brain authored the bytes and is covered by the
signature, so authorship cannot be relabeled after the fact.

**Reconstruction.** `self rehydrate` rebuilds every script and view from
`events.jsonl` + `.secret` alone — no LLM, no network. This is the recovery
path, the migration path, and the audit path, and the test suite pins it.

## CLI

```
self                 rehydrate from the log, then serve (default)
self grow <seed>     generate capabilities from a seed's intent.md (needs a brain)
self run <cmd> ...   run a command: append its events, re-render projections
self think "..."     query the brain; returns {response, declarations} JSON
self brain "..."     the built-in brain process; replaced wholesale by $SELF_BRAIN
self heartbeat       one improvement cycle: the brain inspects the log and may declare
self show <name>     render a projection to stdout
self live [port]     serve the instance (default 7777)
self rehydrate       rebuild capabilities/ + site/ from the log (offline)
self share <cap>     print a capability's declarations + receipts as JSONL to stdout
self adopt <seed>    re-generate a shared capability locally ("-" reads stdin)
```

Server routes: `/` (instance self-description), `/<projection>`,
`/run/<command>` (plain HTML forms), `/events` (raw log). The server binds
`127.0.0.1` by default; set `SELF_BIND=0.0.0.0` to expose it (see
[Limits](#limits-and-threat-model)).

**The shell.** When serving, the kernel adds one shared stylesheet and a small
script to every page; projections on disk stay plain HTML, and the shell knows
only the CSS class names, never the events. The script is progressive
enhancement: it intercepts form posts, shows the request in flight, re-fetches
so what you see is the log's replay, and re-renders when `/events` grows. Strip
it (`self show`, curl, a no-JS browser) and every page still works, because
every action underneath is a plain HTML form.

**Swappable designs.** The class vocabulary and the layout rules are fixed —
that stable contract is what the projections and the script are written
against. A *theme* changes none of it: it is only a skin, a set of CSS
variables (palette, fonts, radii, border weight, shadow) the rules read through
`var()`. So switching designs never renames a class or touches a projection.
Four ship in the kernel — `grove` (the warm default), `micro` (a hard-edged
monospace micrographics look), `paper` (clean and low-chrome), and `spec` (a
monochrome technical-label / spec-sheet look — letter-spaced uppercase labels,
hairline frames, registration marks and a barcode strip) — and adding one is a
single map entry. A theme is a skin (CSS variables); a design whose feel is
more than a palette may also carry a small block of extra rules, which still
styles only the shared classes and never the events. Pick a design with the
on-page switcher (plain links, so it works with no JS), a `?theme=<name>` link,
or `SELF_THEME` for the instance default. The choice is presentation only, chosen at serve time like
`prefers-color-scheme`; nothing is written to the log, so `self show` and
`rehydrate` stay theme-agnostic.

## The brain

Every request for intelligence — `think`, `heartbeat`, `grow`, and each
compile — goes through one interface. There are three ways to supply it:

```sh
SELF_BRAIN="claude -p"     # any executable (agent CLI, script, human shim)
SELF_LLM_URL=http://...    # or any OpenAI-compatible endpoint (built-in loop)
SELF_LLM_STUB=1            # or deterministic offline stubs (testing, demos)
```

The stub path is deliberately dumb but complete: `think` returns a fixed reply,
`grow` declares a minimal command + projection from names in `intent.md`, and
compile emits scripts that honor the pipe contract. It proves the machinery, not
the intelligence.

The `SELF_BRAIN` process contract:

| channel     | content                                                       |
|-------------|---------------------------------------------------------------|
| `$SELF_ASK` | request kind: `think` \| `heartbeat` \| `grow` \| `compile`   |
| last argv   | the prompt (for `compile`, it carries the declaration to build)|
| stdin       | the full event log, JSONL                                      |
| stdout      | event JSONL; non-JSON lines are collected as the text reply    |

So a whole brain is any program that switches on `$SELF_ASK` and prints events:

```python
#!/usr/bin/env python3
import os, sys, json
log    = sys.stdin.read()          # the event log, JSONL — read it or not
ask    = os.environ["SELF_ASK"]    # think | heartbeat | grow | compile
prompt = sys.argv[-1]              # for compile, this is the declaration to build

if ask == "compile":               # author the script the declaration asked for
    script = "#!/bin/sh\necho '{\"name\":\"pinged\",\"payload\":{}}'\n"
    print(json.dumps({"name": "script.authored", "payload": {"script": script}}))
else:                              # think/heartbeat/grow: prose + optional declarations
    print("looked at the log; nothing to add.")   # non-JSON line → the text reply
```

`command.declared` / `projector.declared` add capabilities; `script.authored`
answers a `compile`. Any process that reads stdin and prints JSON is a complete
brain, compiler included; its receipts are signed with `SELF_BRAIN_ID` as the
recorded author. `claude -p` is the same shape with a real model behind it.

## Capability exchange

Instances exchange **declarations and evidence, never runnable code.**

`self share <cap>` prints a slice of the local log: every declaration of that
capability and every locally-signed receipt for it, as JSONL. `self adopt`
records the whole slice inside a single `capability.adopted` event (the foreign
receipts sit there as data, which the installer never reads), then re-declares
the capability locally. The local brain re-generates the script — the sender's
script is passed only as a reference to check against and adapt — and the result
is installed under a receipt signed with the local key.

So: a hostile slice cannot install anything (there is a test for this);
provenance survives adaptation (the sender's records stay in the receiver's log);
and cross-instance authorship is recorded but not cryptographically verifiable,
because the keys are symmetric and never leave an instance.

## Sandbox

While generating, the brain has a bash tool for exploration. Where the platform
supports it, that tool runs in a jail built from Linux user namespaces: an
ephemeral copy of the instance at `/body` (never `.secret`), no network, writes
confined to the jail. Nothing run there installs anything — declarations remain
the only way in. Where namespaces are unavailable (or `SELF_SANDBOX=0`), it falls
back to a read-only command allowlist against the same secret-less copy. It never
fails open.

This jail is for the brain's *exploration during generation*. Installed
capability scripts run without a sandbox — see Limits.

## Environment

```
SELF_HOME         instance directory (default ~/.self)
SELF_BRAIN        brain executable; replaces the built-in for all request kinds
SELF_LLM_URL      OpenAI-compatible endpoint (default http://127.0.0.1:8080)
SELF_LLM_API_KEY  its key
SELF_LLM_MODEL    its model
SELF_LLM_STUB     "1" → offline stub generation (no LLM, no network)
SELF_SANDBOX      "0" → disable the namespace jail (read-only fallback)
SELF_BIND         serve address (default 127.0.0.1; set 0.0.0.0 to expose)
SELF_BRAIN_ID     author string signed into receipts
                  (default: model @ endpoint, or "stub (no LLM)")
SELF_THEME        default page design: grove | micro | paper | spec
                  (default grove); ?theme= or the on-page picker overrides it
```

## Repository layout

- `main.go` — the entire runtime: log, signed install, pipe orchestration, the
  LLM compiler/brain, HTTP server. One file by design.
- `main_test.go` — the pinned invariants: log semantics, offline runtime
  generation, the forged-receipt gate, sandbox containment, receipt provenance,
  share/adopt independence, the pluggable brain, and byte-stable reconstruction.
- `demo.sh` — the offline, no-LLM walkthrough of the loop.
- `seeds/journal` — the smallest example: one command, one projection.
- `seeds/chat` — a conversational surface; asking for a missing capability
  generates it mid-conversation.
- `seeds/renga` — linked verse written by many authors across sessions; a seed
  whose first entry can't be pre-written, so each instance starts blank.

## Limits and threat model

These are current properties, stated plainly, not goals to aspire to.

- **The compiler is driven by a model reading the log, and the log can contain
  untrusted input.** Chat messages, adopted seeds, and other events all become
  context for the brain that writes your scripts. A crafted event can try to
  steer what gets written (prompt injection). There is **no human review step
  between authoring and signing** — the signature is applied to whatever the
  model produced. Generated scripts are plain text in `capabilities/`; read them,
  use a brain you trust, and treat adopting a seed as running code. Keeping a
  human in the loop before trusting a new capability is the intended posture. The
  advantage over the usual software supply chain is that what you inspect is
  readable intent and readable output, not an opaque binary — but it is still
  yours to inspect.
- **Installed capability scripts run without a sandbox.** The jail protects only
  the brain's exploration during generation, not the scripts the kernel runs
  afterward.
- **The log is unbounded.** Every compile stores its script bytes, and every
  projection replays the whole log (O(history)). Snapshotting is not built in; a
  snapshot can itself be modeled as a seed and left to the user.
- **One writer at a time.** Sequence numbers are assigned by reading the log and
  adding one, without locking. Two writers at once (say a server POST and a CLI
  `run`) can race. Route writes through a single process.
- **The server has no authentication** on `/run`. It binds loopback by default
  for this reason; only expose it with `SELF_BIND` on a network you trust.
- **Cross-instance identity is asserted, not proven,** because HMAC keys are
  symmetric and never leave an instance.

Not goals in this core: multi-user access control, log compaction in the kernel,
or shipping code between instances.

## Status

Experimental. The claim under test: an append-only log, HMAC-gated script
installation, and deterministic replay are enough of a kernel for a system that
generates, tests, and revises its own capabilities while staying local-first and
fully inspectable.

## License

Apache-2.0. The scripts your instance generates and the events in your log are
program output — yours, not derivatives of the runtime.
