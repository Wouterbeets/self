# self

`self` is a small, local-first runtime. It keeps one append-only event log as
its only state, and rebuilds every view — and every capability — from that log.
Capabilities are not shipped as code. You describe what you want, and a mind of
your choosing — a tool-capable coding agent like `opencode run` or `claude -p` —
writes the script for it, on your machine, after inspecting the instance's
rendered state. The kernel itself holds no model: it only installs each generated
script under a signature made with a key that never leaves the instance, so the
whole system can be rebuilt from the log alone, with no model and no network.

The point is durable, inspectable state for an LLM agent: one log, replayed into
every view, with a record of who generated each script and the ability to
reconstruct it offline. What is not in the log did not happen.

This runtime is the reference implementation of a larger idea:

- **[knowledge-seed-protocol](https://github.com/wouterbeets/knowledge-seed-protocol)**
  — the protocol: how local, verifiable knowledge moves between minds as
  replayable reasoning rather than bare assertions.
- **self** (this repo) — the runtime that makes the protocol executable.

It is experimental. See [Limits](#limits-and-threat-model) before you rely on it.

## Quick start

**Already living in a coding agent?** The fastest path is the
[agent integration card](AGENTS.md): install `self`, paste one section into
your project's `CLAUDE.md` / agent instructions, and your sessions gain an
append-only memory that outlives them — no browser, no config, growth when
you ask for it. Prebuilt binaries are on the
[releases page](https://github.com/Wouterbeets/self/releases); or
`go install github.com/Wouterbeets/self@latest`.

### 1. See the machinery with no LLM (about 10 seconds)

```sh
git clone https://github.com/wouterbeets/self && cd self
./demo.sh
```

`demo.sh` runs the whole loop offline using `examples/mind-stub` (no API key,
no model — a deterministic mind plugged through the same seam as any real
one): a declaration compiles into a script, running a command appends an
event, a projection renders it, and the instance rebuilds from
`events.jsonl` + `.secret` alone — byte for byte. The stub writes trivial
scripts; this shows the machinery, not the intelligence.

### 2. Real capabilities (plug a mind)

```sh
go install .                        # `self` on PATH (via GOBIN)
make run                            # or: self
```

Open <http://127.0.0.1:7777>. By default the current working directory is the
instance home, so a clone is immediately inspectable: `events.jsonl`, `.secret`,
`capabilities/`, and `site/` appear right beside the code. A new home starts
empty on purpose: name a mind with `SELF_MIND`, then grow chat or notes from
their visible `intent.md` prompts. Every capability grows through the mind and
leaves a signed receipt in the log — there is no other install path.

If you want one shared instance regardless of where you run `self`, pin it in
your shell rc:

```sh
export SELF_HOME=~/.self             # or: export SELF_HOME=$HOME/my-self
```

The same flow works from the CLI:

```sh

cd ~/my-project
export SELF_MIND="claude -p"        # any executable can be the mind (see below)
self grow seeds/chat                 # generate a capability set from its intent
self                                 # rebuild from the log, then serve at :7777
```

`grow` needs a mind; `examples/mind-stub` supplies a deterministic offline
one for mechanical tests and demos, while real capabilities need a real model
or agent.
To make every coding-agent session in a project use one instance as shared
persistent state, paste the card in [`AGENTS.md`](AGENTS.md) into the project's
agent instructions. To write your own capability sets, see [`SEEDS.md`](SEEDS.md).

## How it works

An instance is a directory (`SELF_HOME`, defaulting to the current working
directory) with two files of real state:

```
events.jsonl    the append-only log — the only state
.secret         a per-instance signing key (32 random bytes, hex; mode 0600)
files/          stored files, content-addressed by sha256 — user content
```

Everything else (`capabilities/`, `site/`) is derived and can be rebuilt.

**Events.** Each record is `{id, seq, name, occurred_at, payload}`. Records are
never changed or deleted; a deletion is expressed as a later event.

**Commands.** A command is an executable script. It receives its arguments as
`argv` and the current log as JSONL on stdin, and writes new events as JSONL on
stdout (`{name, payload}` per line; the kernel fills in the rest). The emitted
events are appended and the projections that consume them re-render.

**Projections.** A projection is an executable that reads the events matching
its declared `consumes` list on stdin (an empty list — or `"*"` — means every
event) and writes bare semantic HTML on stdout, saved to `site/<name>.html`.
The kernel injects the shared shell when serving, so projectors must not emit
CSS, JavaScript, inline styles, or external assets. A projector must be a pure
function of its events: rendering twice from the same log yields the same
bytes. The kernel re-runs a projection only when the log grows events it
consumes; a page whose events did not change is served as already materialized.

**Files.** Bytes never ride in events. A file enters the store through a
browser form's file input, an `@<path>` arg to `self run`, a seed's
`files/` dir — or a command's own output: a command that produces a file (an
export, an invoice, a generated booklet) writes the bytes to a scratch path
and emits `file.stored {name, path}`, and the kernel copies them in,
completes the payload, and verifies before appending. Every deposit writes
the blob to `files/<sha256>` and appends one small `file.stored` event
`{name, mime, size, sha256}`. Commands receive file args as hashes and read
`SELF_HOME/files/<sha256>` themselves; projections link
`/files/<sha256>/<name>` and the server serves the blob (immutable, so
browsers cache it forever; served inert — `nosniff`, and document types that
could script render sandboxed). Same hash, same file — dedup is free. Blobs
are user content: `rehydrate` rebuilds scripts and pages from the log alone,
and `files/` must be backed up alongside `events.jsonl` and `.secret`.

**Effects.** Commands may act on the world; projections never do. A command
is free to spawn the OS's own tools — `lp` to print, `curl` to poke a device
on the LAN or a service the user configured, `sendmail` for the follow-up —
because the discipline lives in the log, not in a sandbox: every effect
leaves a receipt event (what was attempted, what came back), and an
effectful command reads the log on stdin first so it never repeats what the
log already says happened. Secrets and endpoints come from the environment
or a config file outside the log — never from event payloads. This is the
kernel as glue: the same append-only log, now with hands.

**Timers.** A `timer.declared` event `{name, every, command, args}` binds an
installed command to a cadence; the serving kernel ticks it. `every` is a Go
duration ("24h", "168h"), `"off"` disables, the latest declaration per name
wins, and every firing appends `timer.fired` before the command runs — so
the log shows everything the instance did on its own, and a crashed command
leaves a `timer.failed` receipt instead of silence. A timer plus a
log-reading command is a follow-up machine: the timer supplies the moment,
the log says what is actually due.

**Runtime code generation.** A `command.declared` or `projector.declared` event
triggers a compile: the kernel hands the declaration to the mind process, which
writes the script; the kernel installs it and records a `script.compiled`
receipt. Declarations — not code — are what cross every boundary.

**Signed installation.** A receipt is `{type, name, script, by, sig}`, where
`sig` is an HMAC-SHA256 over the fields using the instance's key. Only receipts
that verify under the local key are ever installed; anything else in the log is
inert data. `by` records which mind authored the bytes and is covered by the
signature, so authorship cannot be relabeled after the fact.

**Reconstruction.** `self rehydrate` rebuilds every script and view from
`events.jsonl` + `.secret` alone — no LLM, no network. This is the recovery
path, the migration path, and the audit path, and the test suite pins it.

## CLI

```
self                 rehydrate from the log, then serve (default)
self grow <seed>     generate capabilities from a seed's intent.md (needs a mind)
self run <cmd> ...   run a command: append its events, re-render projections
self think "..."     query the mind; returns {response, declarations} JSON
self heartbeat       one improvement cycle: the mind inspects the log and may declare
self show <name>     render a projection to stdout
self rehydrate       rebuild capabilities/ + site/ from the log (offline)
self share <cap>     print a capability's declarations + receipts as JSONL to stdout
self adopt <seed>    re-generate a shared capability locally ("-" reads stdin)
self export <prefix> <dir>
                     write a content seed: every <prefix>* event, the files
                     they reference, and an editable intent — another
                     instance grows the directory, dates preserved
self retire <t>/<n>  retire a capability: script + page leave the surface, the
                     log keeps every event, re-declaring revives it
```

Server routes: `/` (instance self-description), `/<projection>`,
`/run/<command>` (plain HTML forms; `multipart/form-data` file inputs are
stored and passed as hashes), `/files/<sha256>[/<name>]` (stored files),
`/events` (raw log). The server binds
`127.0.0.1:7777` by default; `SELF_BIND` is the whole bind address, host or
`host:port` — set `SELF_BIND=0.0.0.0` to expose it (see
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

## The mind

Every request for intelligence — `think`, `heartbeat`, `grow`, and each
compile — goes through one interface. The kernel holds no model — not even a
fake one; the mind is always a **process** you supply:

```sh
SELF_MIND="claude -p"                      # any executable (agent CLI, script, human shim)
SELF_MIND="$PWD/examples/mind-stub"       # or the deterministic offline stub (testing, demos)
SELF_MIND="$PWD/examples/mind-grok"       # or the Grok Build CLI adapter (recommended for Grok users)
```

A tool-capable agent CLI needs no adapter: `SELF_MIND="claude -p"` is a
complete mind as-is — and stateless on purpose. Each ask starts cold and
orients from the brief and the rendered state; the instance's memory is the
log, its projections, and nothing else. Do not reach for the harness's own
session store (`claude -p --continue` and its kin) as the mind's memory: it
chains asks into a conversation that lives outside the log — not replayed by
`rehydrate`, not in any seed, invisible to audit — and whatever "sense of the
place" accumulates there is state the system depends on but never captured.
If a cold mind orients slowly, that is design pressure aimed at the right
target: improve the projections, don't bolt on a hidden memory tier.

To drive an OpenAI-compatible endpoint, point `SELF_MIND` at the
[`examples/mind-openai`](examples/mind-openai) adapter — a stdlib-only process
that illustrates the contract's wire shape. It is intentionally incomplete: it
makes a single completion against the brief without inspecting `SELF_HOME`
itself, so it cannot do the exploration a real mind needs. Use it to understand
the seam, then plug a tool-capable agent (`examples/mind-opencode`, or
`claude -p`) for real capabilities. It used to live inside the kernel; it is a
reference file now, so the core stays model-free.

The new `examples/mind-grok` adapter wires in Grok Build (the official xAI CLI coding agent). Install Grok Build with `curl -fsSL https://x.ai/cli/install.sh | bash`, then use `SELF_MIND="$PWD/examples/mind-grok"` (or simply `grok` if in PATH with proper setup). It leverages Grok 4.5's strengths for exploration and script generation while staying fully compatible with the self contract.

The stub mind is deliberately dumb but complete: `think` returns a fixed
reply, `grow` declares a minimal command + projection from names in
`intent.md`, and compile answers with scripts that honor the pipe contract. It
proves the machinery, not the intelligence — and it is a process behind the
same seam as every real mind, because the kernel carries no mind of its own.

The `SELF_MIND` process contract:

| channel     | content                                                       |
|-------------|---------------------------------------------------------------|
| `$SELF_ASK` | request kind: `think` \| `heartbeat` \| `grow` \| `compile`   |
| last argv   | the prompt (for `compile`, it carries the declaration to build)|
| stdin       | an **orientation brief** (plain text): where the mind is,    |
|             | what capabilities exist, and where to look for the rest. This  |
|             | is a wake-up card, not a context dump. The mind MUST inspect  |
|             | `SELF_HOME` itself with its own tools — `site/*.html`,         |
|             | `events.jsonl`, `capabilities/` — to do its job. A process    |
|             | without that exploration ability is not a complete mind.      |
| stdout      | event JSONL; non-JSON lines are collected as the text reply    |

So a whole mind is any program that switches on `$SELF_ASK` and prints events:

```python
#!/usr/bin/env python3
import os, sys, json
brief  = sys.stdin.read()         # an orientation brief, plain text — where to look
ask    = os.environ["SELF_ASK"]    # think | heartbeat | grow | compile
prompt = sys.argv[-1]             # for compile, this is the declaration to build
# A real mind then reads SELF_HOME/site/kernel.html, events.jsonl, etc. itself.

if ask == "compile":               # author the script the declaration asked for
    script = "#!/bin/sh\necho '{\"name\":\"pinged\",\"payload\":{}}'\n"
    print(json.dumps({"name": "script.authored", "payload": {"script": script}}))
else:                              # think/heartbeat/grow: prose + optional declarations
    print("explored the instance; nothing to add.")  # non-JSON line → the text reply
```

`command.declared` / `projector.declared` add capabilities; `script.authored`
answers a `compile`. The kernel's seam is a pipe, but a mind that cannot
inspect files under `SELF_HOME` (`site/*.html`, `events.jsonl`,
`capabilities/`) cannot do the job — the orientation brief on stdin is a
wake-up card, not a context dump. A tool-capable coding agent (`opencode run`,
`claude -p`) already has the tools to explore; a plain API adapter without a
tool loop of its own is incomplete by spec. Receipts are signed with
`SELF_MIND_ID` as the recorded author.

## Capability exchange

Instances exchange **declarations and evidence, never runnable code.**
Capabilities travel by `share`/`adopt`; lived content travels by
`export`/`grow` — two flavors of the same seed idea.

`self share <cap>` prints a slice of the local log: every declaration of that
capability and every locally-signed receipt for it, as JSONL. `self adopt`
records the whole slice inside a single `capability.adopted` event (the foreign
receipts sit there as data, which the installer never reads), then re-declares
the capability locally. The local mind re-generates the script — the sender's
script is passed only as a reference to check against and adapt — and the result
is installed under a receipt signed with the local key.

So: a hostile slice cannot install anything (there is a test for this);
provenance survives adaptation (the sender's records stay in the receiver's log);
and cross-instance authorship is recorded but not cryptographically verifiable,
because the keys are symmetric and never leave an instance.

**Content exchange** is the other direction: not "teach me your tool" but
"here is a piece of my record." `self export <prefix> <dir>` writes a seed
directory from the live log — every `<prefix>*` event with its original
`occurred_at`, the blobs those events reference, and an `intent.md` stub the
sender edits (who I am, what this means, what I hope grows from it). The
receiver runs `self grow <dir>`: the events land with their own moments and
this instance's ids, the files land in the store hash-verified, and the
receiver's mind reads the intent against what already lives here to decide
how two records merge — rendered side by side, overlaps surfaced,
contradictions visible. Two friends' seasons become a head-to-head; two
researchers' field notes become the correlation neither could see alone.
The sender's log keeps a `seed.exported` event; the receiver's keeps
`seed.provenance`. Both sides remember.

## Where the mind runs

The kernel spawns the mind as a plain subprocess and reads its stdout; it does
not give the mind tools, and does not sandbox it. Exploration during
generation — running a candidate script, reading files — is the mind's own
concern, and a capable mind (a coding agent) already has its own sandboxed
tools. The kernel's guarantee is narrower and stronger: whatever the mind does,
**only a locally-signed `script.compiled` receipt ever installs**, so a mind
can only ever *propose* — it cannot write the record. Installed capability
scripts then run without a sandbox — see Limits.

## Environment

```
SELF_HOME         instance directory (default: current working directory; set in
                  your shell rc to pin one shared home, e.g. ~/.self)
SELF_MIND         mind executable (e.g. "claude -p"); the kernel spawns it for
                  every request kind. For offline demos/tests, point it at
                  examples/mind-stub (no LLM, no network).
SELF_BIND         bind address, host or host:port (default 127.0.0.1:7777;
                  set 0.0.0.0 to expose)
SELF_MIND_ID      author string signed into receipts
                  (default: the mind executable)
SELF_THEME        default page design: grove | micro | paper | spec
                  (default grove); ?theme= or the on-page picker overrides it
```

The mind was called the brain until mid-2026; `SELF_BRAIN` and
`SELF_BRAIN_ID` still answer (the new names win when both are set), so an
instance configured under the old name keeps working untouched.

## Repository layout

- `main.go` — the entire runtime: log, signed install, pipe orchestration, the
  mind seam, HTTP server. One file by design.
- `main_test.go` — the pinned invariants: log semantics, offline runtime
  generation, the forged-receipt gate, receipt provenance, share/adopt
  independence, the pluggable mind, and byte-stable reconstruction.
- `examples/` — minds that plug in through `SELF_MIND`. `mind-stub` is the
  deterministic offline mind the tests and `demo.sh` use (no LLM, no
  network); `mind-opencode` is a working tool-capable mind (it delegates to
  `opencode run`, which can inspect `SELF_HOME`); `mind-openai` is a reference
  adapter that illustrates the contract's wire shape but is incomplete by spec
  (no tool loop of its own). Not part of the kernel.
- `demo.sh` — the offline walkthrough of the loop, no mind required.
- `seeds/notes` / `seeds/journal` — small examples: one command, one projection.
- `seeds/memory` — durable memory for a stateless mind: `remember` writes
  facts to the log; a cold mind orients from `/memory`. The in-log answer to
  session stores.
- `seeds/chat` — a conversational surface; asking for a missing capability
  generates it mid-conversation.
- `seeds/renga` — linked verse written by many authors across sessions; a seed
  whose first entry can't be pre-written, so each instance starts blank.
- `seeds/palace` — the method of loci, made literal: every event in the log
  becomes a room, derived from nothing but the hash of its own id. `walk`
  your instance's history; every new event, from any capability, builds
  another chamber, and `rehydrate` rebuilds the identical labyrinth.

## Limits and threat model

These are current properties, stated plainly, not goals to aspire to.

- **The mind is driven by the log, and the log can contain untrusted input.**
  Chat messages, adopted seeds, and other events all become context for the mind
  that writes your scripts. A crafted event can try to steer what gets written
  (prompt injection). There is **no human review step between authoring and
  signing** — the signature is applied to whatever the mind produced. Generated
  scripts are plain text in `capabilities/`; read them, use a mind you trust, and
  treat adopting a seed as running code. Keeping a human in the loop before
  trusting a new capability is the intended posture. The advantage over the usual
  software supply chain is that what you inspect is readable intent and readable
  output, not an opaque binary — but it is still yours to inspect.
- **The kernel does not sandbox the mind, and installed capability scripts run
  without a sandbox.** The kernel runs the mind as a plain subprocess (isolating
  its exploration is the mind's own concern) and runs the scripts it installs
  directly. The kernel's guarantee is the signed-receipt gate, not containment.
- **Effectful commands raise the stakes of both points above.** A command that
  prints, emails, or messages turns a prompt-injected script from a local
  nuisance into an outward action, and a timer runs it with nobody watching.
  The posture is unchanged but matters more: read the scripts that touch the
  world before trusting them, keep credentials out of the log and out of
  scripts, and let every effect leave a receipt event so the log can always
  answer "what did this instance do on my behalf."
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
