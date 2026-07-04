# self

A single-binary, event-sourced runtime whose capabilities are generated at
runtime by an LLM from declarative specifications, installed only under
HMAC-signed receipts, and deterministically reconstructible from an
append-only log.

`self` gives LLM agents durable, auditable state that outlives any session,
model, or vendor: one log, replayed into every view, with cryptographic
provenance on every generated script.

## Architecture

An instance is a directory (`SELF_HOME`) containing exactly two files of
source state:

```
events.jsonl    append-only event log — the only state
.secret         per-instance HMAC key (32 random bytes, hex)
```

Everything else (`capabilities/`, `site/`) is derived and reproducible.

**Events.** Each log record is `{id, seq, name, occurred_at, payload}`.
Records are never mutated or deleted; deletion semantics, when needed, are
expressed as later events.

**Commands.** A command is an executable script invoked as a Unix pipeline
node: arguments as argv, the current log as JSONL on stdin, new events as
JSONL on stdout (`{name, payload}` per line; the runtime assigns the rest).
Emitted events are appended and all projections re-render.

**Projections.** A projection is a deterministic function of the log: a
script that reads all events on stdin and writes HTML to stdout, persisted to
`site/<name>.html`. Rendering twice from the same log yields the same bytes.

**Runtime code generation.** Ingesting a `command.declared` or
`projector.declared` event triggers compilation: an LLM authors the script
from the declaration, the runtime installs it and appends a `script.compiled`
receipt. Declarations — not code — are the transport format at every
boundary.

**Signed installation.** A receipt is `{type, name, script, by, sig}` with
`sig = HMAC-SHA256(secret, domain-separated length-prefixed fields)`. Only
receipts that verify under the local key are ever installed; anything else in
the log is inert data. `by` records which brain authored the bytes and is
covered by the signature, so authorship cannot be relabeled after the fact.

**Reconstruction.** `self rehydrate` rebuilds all scripts and views from
`events.jsonl` + `.secret` alone — no LLM, no network. This is the recovery
path, the migration path, and the audit path; the test suite pins it.

## Quick start

```sh
git clone <this repo> && cd self
go install .                    # `self` on PATH (via GOBIN)

cd ~/my-project
export SELF_HOME=$PWD/.self     # instance lives beside the code
export SELF_BRAIN="claude -p"   # any executable can be the brain (see below)
self                            # init (key + first event) and serve at :7777
```

To make every coding-agent session in a project use the instance as shared
persistent state, paste the integration card in [`AGENTS.md`](AGENTS.md) into
the project's agent instructions.

## CLI

```
self                 rehydrate, then serve (default)
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
`/run/<command>` (HTML forms, no JS), `/events` (raw log).

## The brain interface

Every request for intelligence — `think`, `heartbeat`, `grow`, and each
compilation — passes through one seam. Three implementations:

```sh
SELF_BRAIN="claude -p"     # any executable (agent CLI, script, human shim)
SELF_LLM_URL=http://...    # or any OpenAI-compatible endpoint (built-in loop)
SELF_LLM_STUB=1            # or deterministic offline stubs (testing)
```

The `SELF_BRAIN` process contract:

| channel   | content                                                        |
|-----------|----------------------------------------------------------------|
| `$SELF_ASK` | request kind: `think` \| `heartbeat` \| `grow` \| `compile`  |
| last argv | the prompt                                                     |
| stdin     | the full event log, JSONL                                      |
| stdout    | event JSONL; non-JSON lines are collected as the text reply    |

Reply events: `command.declared` / `projector.declared` to add capabilities;
`script.authored` (`{"script": "..."}`) to answer a `compile` request. A
process that reads stdin and prints JSON is a complete brain, compiler
included; its receipts are signed with `SELF_BRAIN_ID` as the recorded
author.

## Capability exchange

Instances exchange **declarations and evidence, never installable code**.

`self share <cap>` emits a verbatim slice of the local log: every declaration
of that capability and every locally-signed receipt for it, as JSONL. `self
adopt` records the whole slice inside a single `capability.adopted` event
(foreign receipts are stored as payload data, which the installer never
reads), then re-declares the capability locally. The local brain re-generates
the script — the sender's script is passed only as a reference implementation
to verify and adapt — and the result is installed under a receipt signed by
the local key.

Properties: a hostile slice cannot install anything (verified by test);
provenance survives adaptation (the sender's records persist verbatim in the
receiver's log); cross-instance authorship is recorded but not
cryptographically verifiable, since HMAC keys are symmetric and never leave
an instance.

## Sandbox

During generation, the brain's exploratory shell runs in a jail built from
Linux user namespaces: an ephemeral copy of the instance at `/body` (never
`.secret`), no network interfaces, writes confined to the jail. Nothing
executed inside installs anything — declarations remain the only ingress.
Where namespaces are unavailable (or `SELF_SANDBOX=0`), the shell falls back
to a fail-closed read-only allowlist.

## Environment

```
SELF_HOME         instance directory (default ~/.self)
SELF_BRAIN        brain executable; replaces the built-in for all request kinds
SELF_LLM_URL      OpenAI-compatible endpoint (default http://127.0.0.1:8080)
SELF_LLM_API_KEY  its key
SELF_LLM_MODEL    its model
SELF_LLM_STUB     "1" → offline stub generation (no LLM, no network)
SELF_SANDBOX      "0" → disable the namespace jail (read-only fallback)
SELF_BRAIN_ID     author string signed into receipts
                  (default: model @ endpoint, or "stub (no LLM)")
```

## Repository layout

- `main.go` — the entire runtime: log, signed install, pipe orchestration,
  LLM compiler/brain, HTTP server. Single file by design.
- `main_test.go` — the invariants, pinned: log semantics, runtime generation
  (offline via stubs), the forged-receipt gate, sandbox containment, receipt
  provenance, share/adopt sovereignty, the pluggable brain seam, and
  byte-stable reconstruction.
- `seeds/journal` — minimal example seed: one command, one projection.
- `seeds/chat` — a conversational surface over the instance; asking for a
  missing capability generates it mid-conversation.

## Threat model and limits

Trust is provenance, not containment: the compiler is the single code
ingress and therefore the attack surface; installed scripts run unsandboxed.
The log is unbounded: every compile stores its script bytes, and every
projection replays the full log (O(history)). Cross-instance identity is
asserted, not proven. These are current properties, not goals.

## Status

Experimental. The claim under test: an append-only log, HMAC-gated script
installation, and deterministic replay are a sufficient kernel for a system
that generates, tests, and revises its own capabilities while remaining
local-first and fully inspectable.
