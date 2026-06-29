# self

A local-first, self-modifying capability system. The kernel knows almost
nothing; capabilities grow from **seeds** — a seed is *intent* (the genotype: how
a surface should work) that an LLM grows into running capabilities fitted to this
receiver. State is one append-only event log; every view is a pure replay of it,
rendered as HTML that you and your agent read identically.

> One append-only event log + shared projections. A small kernel; everything
> else grows as seeds. Every capability, projection, and byte of state is a
> plain file you can open.

## mental model

- **One truth.** `events.jsonl` is an append-only log — the only source of
  truth. Nothing is ever mutated or destroyed; a "delete" is just another event.
- **Projections are replays.** A projection is a pure function of the log. Run
  it twice, get byte-identical HTML. The HTML in `site/` *is* the shared memory
  the human reads in a browser and the agent reads as context — the same reality.
- **Seeds are intent; growth is development.** A seed is the *genotype* — prose
  intent plus invariants (a fitness function) — not a parts-list. An orchestrator
  (the LLM, holding the whole intent) reads the garden and grows the decomposition
  that realizes the intent here, then the phenotype is selected against the
  invariants. The kernel stays minimal; `self` grows itself.
- **The strange loop.** A running capability can declare *new* capabilities, and
  the kernel compiles them on the spot — so `self` can grow itself while it runs.
  The loop always carries *specs*, never code: the LLM is always the compiler, so
  every binary is authored for this receiver and nothing foreign ever runs.

## the loop

```sh
self                       # the default — first run brings up a demo garden, then serves it at :7777
                           #   land on /, connect your intelligence on /setup
self init                  # initialize a bare kernel (welcome + setup pages, no demo)
self live                  # start the web server explicitly (default port 7777)
self teach command timer   # hand-write a capability yourself (script on stdin)
self grow seeds/chat       # grow a seed's intent into capabilities (the orchestrator decomposes + compiles)
self run chat "add a ..."  # run a capability; chat asks the brain, which can grow more
self think "summarize ..." # ask the brain (a swappable process; default = LLM)
self heartbeat             # one self-improvement cycle: the brain reflects & grows
self show board            # render a projection (browser, or stdout when piped)
self history               # recent events, human-readable
self ls                    # what capabilities exist (self ls commands|projectors|seeds)
self where                 # SELF_HOME and every important path
self which capture         # full path to a command or projection
```

Grow the chat seed and the rest follows: ask it to add a note command, a board,
a tracker — the brain reads the garden, emits declarations, the kernel compiles
them. A capability can declare new capabilities, so one seed bootstraps the rest.

## CLI

| command | behavior |
| --- | --- |
| `self` | Default: rehydrate the body from the log, then start the live garden (the most common action) |
| `self init` | Initialize a bare kernel (and the brain-setup page) |
| `self rehydrate` | Rebuild `capabilities/` + `site/` from the log's signed receipts — no LLM, no network |
| `self selftest` | Re-run every installed capability's examples against its binary — a regression gate (the projection/output is the oracle) |
| `self identity` | Print this home's public verification key (shareable) |
| `self verify-attestation` | Check a `script.verified` attestation piped on stdin — no secret needed |
| `self grow <seed>` | Grow a new capability from a seed |
| `self run <command> [args]` | Run a capability — append events, refresh affected projections |
| `self think "..."` | Ask the brain (runs the brain process, returns `{response, declarations}`; swap via `$SELF_BRAIN`) |
| `self brain "..."` | The brain process itself — prompt in, event JSONL out (the default is the in-tree LLM) |
| `self teach <command\|projector> <name> [consumes-csv] [--examples=file]` | Install a hand-written script as a capability (script on stdin); kernel-signed, examples-gated |
| `self heartbeat` | One self-improvement cycle (the brain reflects on the garden and may grow a capability) |
| `self restore <name> [seq]` | Roll a capability back to an earlier compiled version (kernel-only, audit-faithful) |
| `self show <name>` | Render a projection. Piped → HTML on stdout; otherwise render and open in a browser |
| `self live [port]` | Start the web server explicitly (default port 7777) |
| `self history [-n N] [--raw]` | Recent events, human-readable by default |
| `self ls [commands\|projectors\|seeds]` | List what exists, with full file paths |
| `self where` | Show `SELF_HOME` and every important path |
| `self which <name>` | Show the full path to a command or projection |

### live garden routes (`self live`, default port 7777)

| route | behavior |
| --- | --- |
| `/` | my identity page — capabilities, paths, wiring |
| `/<projection>` | a projection, re-rendered live against current events |
| `/live/<projection>` | re-run a projection against current events |
| `/run/<command>` | run a capability from the browser (plain HTML forms, zero JS) |
| `/events` | the raw event log |

## the trio

The unit a *grown* capability falls into is a **trio** — what the orchestrator
emits and the compiler turns into scripts:

- **command** — what you run (params, intent, the event it produces)
- **event** — what the command produces (name, payload schema)
- **projector** — how events become a view (consumed events, desired output)

But the trio is the *output* of growth, not the seed. You don't hand-write trios;
the orchestrator designs them from the seed's intent against the garden. Same
intent, different receiver, different decomposition — receiver-controlled
adaptation, all the way down to which trios even exist here.

## self-improvement (the strange loop)

`self run` doesn't just append events — it scans them for `command.declared` and
`projector.declared`. If a capability emits a declaration, the kernel compiles it
on the spot and the script lands in `capabilities/`, so a capability can grow new
capabilities — including re-declaring itself with a fresh spec.

```sh
self grow seeds/chat               # grow the chat interface (command + projection)
self run chat "add a summarize command that ..."
# → chat asks the brain; the brain reads site/chat.html + the garden and declares
# → chat emits chat.message + command.declared + projector.declared
# → the kernel compiles the new capability immediately
self run summarize "..."           # the new capability works right away
```

**The loop carries specs, never code.** The LLM is *always* the compiler, so
every binary is authored for this receiver — adaptation is never skipped, and the
only way code enters the system is through the compiler (the original, finite
attack surface). When the kernel compiles, it logs the bytes as a
`script.compiled` receipt **signed with a per-home secret** (`SELF_HOME/.secret`,
never in the log). Anything may append a `script.compiled`, but only a
kernel-signed one ever installs — provenance is intrinsic to the receipt, not
enforced by filtering who may write it. A forged receipt is inert: it sits in the
log and is ignored on install.

Two consequences worth naming:

- **Precision without code injection.** A seed that wants exact, complex behavior
  ships a *reference implementation* — an `implementation` field on a declaration.
  The compiler verifies it against the pipe contract and adapts it to the local
  garden; it is never installed as-is. Near-identical power to handing over code,
  but coherent with receiver adaptation and with zero new attack surface (see
  `poc/wall`).
- **Rollback splits cleanly into trigger and install.** Every compile is logged
  as a signed `script.compiled` receipt. *Installing* an earlier one is the
  kernel's job — it verifies the signature, so it only ever reinstates code its
  own compiler authored here. But *triggering* a rollback is just a data-only
  `restore.requested {name, seq}` event, which anything may emit. So
  `restore` is an ordinary **seed** (`seeds/restore`), the brain rolls back by
  calling it like any other capability, and `self restore <name> [seq]` is a thin
  always-on built-in that emits the same event — a safety net on a bare kernel.
  Either way the install reads only the kernel's own receipts: no drift, no
  foreign bytes, no special power.

## self heartbeat

`self heartbeat` runs one self-improvement cycle: it asks the brain to explore
the garden, pick one small high-value improvement, and — if warranted — declare
it. Any declarations flow through the strange loop and become real capabilities.
A heartbeat needs the brain (an LLM); without one it's a clear no-op.

## pipe contract

Compiled scripts are standalone executables orchestrated by the kernel via Unix
pipelines. Any language works — Python, bash, node, anything `os/exec` can run:

- **command script**: receives args as `argv`, current events as JSONL on
  `stdin`, writes new events as JSONL on `stdout` (one per line, fields: `name`,
  `payload`). The kernel assigns `id`, `seq`, `occurred_at`.
- **projector script**: receives all events as JSONL on `stdin`, writes HTML on
  `stdout`. The kernel persists the output to `SELF_HOME/site/<name>.html` —
  projections don't write to disk, they emit HTML and the kernel decides where it
  goes.

The kernel sets `SELF_HOME` on every script. Capabilities that need intelligence
call `self think` — the brain — instead of making their own HTTP calls. The
kernel is the sole steward of LLM credentials.

## the brain (`self brain` / `self think`)

The brain is a **process the kernel pipes to**, exactly like a command. `self
brain` is the primitive: prompt in (argv), event JSONL out (stdout) — the reply
as `chat.message`, anything it grows as `command.declared` / `projector.declared`.
It's **swappable** via `$SELF_BRAIN`: any program honoring that contract is a
valid brain — the default in-tree LLM, a deterministic script, a swarm, or a
human (see below). The kernel can't tell the difference; intelligence is just
another process.

`self think` is the **call interface capabilities use**: it spawns the configured
brain process and returns `{response, declarations}` JSON, appending nothing (the
caller owns that). So a capability calls `self think` rather than reinventing
HTTP, auth, and prompts, and the brain stays swappable underneath it.

The brain has three powers: **read** (explore the garden — a local brain reads
`site/*.html` and the log directly), **act** (every capability is callable), and
**grow** (declare new capabilities). The projection output *is* the memory, so
conversation and context persist across calls.

**Choosing a brain.** `self init` installs a setup page; first run lands on
`/setup`, where you pick a provider (OpenAI, llama.cpp, Ollama, opencode, or a
custom OpenAI-compatible endpoint) and save — provider/URL/model are recorded as
a `brain.configured` event; the API token is written to `SELF_HOME/.brain-key`,
never to the log. Pick **human** for a no-LLM, zero-dependency mode: `self think`
parks each prompt and you answer it yourself at `/interview` (and can hand-write a
capability there, installed via `self teach`).

## garden-aware compilation

When compiling (at grow time, and at run time via the strange loop), the LLM
gets a read-only `bash` tool with cwd set to `SELF_HOME`. It explores the garden
— `ls capabilities/commands/`, `head events.jsonl`, `cat site/kernel.html` —
before writing the script, so a seed adapts to the receiver. If a finance
projection declares consumption of `finance.expenditure_added` but the log
already has `shopping_bill_uploaded` events, the LLM extends the filter to
consume both. Same seed, different garden, different binary.

The bash tool is sandboxed to a fail-closed allowlist of read-only inspectors:
the LLM can look but not touch.

## on disk

```
SELF_HOME/
  events.jsonl                  the only truth (append-only)
  .secret                       HMAC key — gates install of compiled bytes (0600; never in the log)
  .identity                     ed25519 key — signs verification attestations others can check (0600)
  capabilities/
    commands/<name>             compiled command scripts (any language)
    projectors/<name>           compiled projection scripts (any language)
  site/<name>.html              materialized HTML projections
```

Run `self where` to see all of this for your home, `self ls commands` /
`self ls projectors` for the full file paths, and `self which <name>` for one.
Agents (and you) read `site/` directly — plain files, no API. `self live`
exposes them over HTTP with `/<name>` re-rendered live against current events.

Only the first two files are irreducible. `capabilities/` and `site/` are a
*materialization* of the log: every compiled script lives in the log as a signed
`script.compiled` receipt, and every projection is a pure replay. `self
rehydrate` (run automatically by bare `self` before serving) rebuilds the whole
body from `events.jsonl` + `.secret`, with no LLM and no network — so a home can
be stored, committed, and moved as just those two files. The `.secret` is what
verifies the receipts; without the signing key a log's bytes are inert (you'd
re-grow from its declarations through a brain instead). See `garden/` for a body
stored exactly this way.

## what the kernel is

Five things, irreducible:

1. **event store** — append-only JSONL log (the only truth)
2. **LLM compiler / orchestrator** — at grow time it reads a seed's *intent*,
   explores the garden via a read-only bash tool, and **designs the decomposition**
   (the `command.declared` / `projector.declared` that realize the intent here),
   then compiles each piece with the whole intent woven in; it also compiles
   declarations a running capability emits (the strange loop). Logs every compiled
   script as a `script.compiled` receipt **signed with the home's secret**; install
   verifies the signature, so only kernel-authored code reaches `capabilities/`.
   `self restore` reads those receipts to roll back. The operator can author a
   script directly with `self teach` (kernel-signed) — the same install path, a
   human instead of the LLM, and the no-brain way to add a capability.
3. **the brain** (`self think`) — a process the kernel pipes events to (prompt
   in, event JSONL out), swappable via `$SELF_BRAIN`; the default `self brain`
   wraps the same LLM infrastructure as the compiler. Capabilities call it; it
   reads `site/*.html` for state and produces valid declarations.
4. **pipe orchestrator** — runs commands and projections, moves events,
   persists projection output to `site/`
5. **web server** — `self live` serves the materialized `site/` and re-runs
   projections on demand

The events the kernel acts on carry **data, never code**: `intent.declared` (grow
a genotype into capabilities for this receiver), `command.declared` /
`projector.declared` (compile a spec into a binary), and `restore.requested`
(reinstall an earlier receipt). It writes the receipts itself —
`kernel.initialized`, `script.compiled` (signed compile receipts; anyone may
append one, but only a kernel-signed receipt installs), `script.verified` (signed
conformance attestations), and `seed.planted`. Everything else comes from seeds,
or from capabilities that emit declarations or intents at run time.

## seed format

A seed is a directory — the genotype, not a parts-list:

- **`intent.md`** — the prose intent: what the surface is for, the core
  intuitions, the feel, the anti-goals, and the public surface (the routes and
  command names that are part of how it should feel). No declarations, no code.
- **`invariants.jsonl`** — the fitness function: must-holds in the receiver's own
  vocabulary, one per line (`{name, capability, kind, args|events,
  expect_contains|expect_order}`). Machine-checkable ones run against the grown
  binary; a `brain: true` invariant depends on the brain and is checked live, not
  by static replay (a capability that *thinks* can't be replayed offline).
- **`seed.jsonl`** (optional) — the *maternal deposit*: initial content events
  (e.g. a `self.identity`) the grow lays down once after the decomposition, so the
  surface has something to render from the first moment.

Growing it (`self grow <seed>`): the kernel records the genotype as an
`intent.declared`, the orchestrator explores the garden and declares the
decomposition, each piece is compiled **with the whole intent woven in**, the
maternal deposit is laid, and the phenotype is checked against the invariants —
**re-growing with the failures fed back if it doesn't survive** (development under
selection). Because the invariants are written in the author's vocabulary, a
receiver that grows the seed to a *different* decomposition must still satisfy
them — which turns "the orchestrator realized the intent correctly" into a
property anyone can check. The conformance receipt (`script.verified`) is an
**ed25519-signed attestation** bound to the sha256 of the exact script and
checks, so a *third* node can verify the claim from a public key alone, with no
shared secret and without re-running (`self identity`, `self verify-attestation`;
see `poc/crossnode`).

Growing needs a brain (the orchestrator). The no-LLM authoring path is still
`self teach` — hand-write a script and the kernel signs + installs it (the human
as compiler). Code enters only through that signed install, never the event
stream, preserving both receiver adaptation and the trust model.

## environment

```
SELF_HOME        kernel home (default ~/.self)
SELF_BRAIN       brain process to spawn (overrides everything below; e.g. a script or bridge)
SELF_LLM_URL     LLM API base URL (no /v1 suffix; the kernel appends it)
SELF_LLM_API_KEY LLM API key
SELF_LLM_MODEL   LLM model name
SELF_LLM_STUB    set to "1" to force stub scripts (no LLM, no network)
```

Brain resolution (highest first):

1. `$SELF_BRAIN` — run this program as the brain, whatever it is
2. `SELF_LLM_*` env vars — explicit OpenAI-compatible endpoint
3. the `/setup` page — the saved `brain.configured` provider + `.brain-key` token
4. opencode-go subscription — `~/.local/share/opencode/auth.json`
5. local llama-server — `http://127.0.0.1:8080`, and the automatic fallback on a
   quota / rate-limit error from a remote endpoint
6. stub scripts — `SELF_LLM_STUB=1`, no key, no network

## what's in the repo

- `main.go`, `internal/` — the kernel: event store, LLM compiler, pipe
  orchestrator, web server.
- `seeds/` — intent seeds you grow, each an `intent.md` + `invariants.jsonl` (the
  genotype): `chat` (talk to it and it grows the rest — three-layer memory:
  identity prism, turn-based history, compaction), the `home` board, the `kitchen`
  planner, `plant` (grow capabilities from the browser), plus `artifact` and
  `restore`. Open any `intent.md` to read, in prose, how that surface is meant to
  work. `onboarding` is the one exception — still a verbatim embedded bundle (the
  bootstrap surface: setup, configure, interview, welcome), because it has to work
  *before* a brain is wired, and growing from intent needs the orchestrator.
- `home/` — the demo body a cold `self` brings up (a task board + meal planner),
  stored as just its event log.
- `garden/` — a second example body: one organism's state after a few
  heartbeats, stored as just its log + signing key.
- `brain/` — the human-in-the-loop brain bridge (`bridge.py`) and notes.
- `poc/` — proof-of-concept seeds that test the thesis (emergence, cross-node
  sharing, the build log, …); not needed to use self.
- `ARCHITECTURE.md` — the build log: every validated slice, with its evidence.

## getting started

```sh
go build -o self .
./self                             # first run brings up a demo garden; open http://localhost:7777
```

You land on a living home — a task board and a meal planner you can use right
away. To make it yours, connect an intelligence on `/setup` (OpenAI, Ollama,
llama.cpp, …), then tell it what you need:

```sh
./self run chat "add a habit tracker"   # the capability appears in the garden
```

Prefer growing from the browser? Grow the `plant` seed once and a `/plant` page
appears: paste a spec (a seed's `command.declared` / `projector.declared`), or
click a starter, and the strange loop compiles it into a running capability —
no CLI in the loop after that.

```sh
./self grow seeds/plant                 # then open http://localhost:7777/plant
```

New capabilities are compiled against what already exists, so they fit the
surfaces you have. (No model handy? Pick **human** on `/setup` and answer at
`/interview`, or hand-write a capability with `self teach`.)

## status

Experimental. The thesis: a minimal kernel — event log, signed install, replay —
plus an LLM compiler is enough for a system that grows, tests, and rewrites its
own capabilities while staying local-first and fully inspectable. Every capability
and every byte of state is a plain file or a replayable event.
