# self

A sovereign, self-improving capability system. **self** is born knowing almost
nothing. Seeds teach it everything else. The LLM compiles seed declarations into
runnable code; the kernel appends events, replays them through compiled
projections, and renders HTML that you and your agent see identically.

> One append-only event log + shared projections. A tiny kernel; everything else
> grows as seeds through the strange loop. Nothing is hidden — every capability,
> every projection, every byte of state is a plain file you can open.

## mental model

- **One truth.** `events.jsonl` is an append-only log — the only source of
  truth. Nothing is ever mutated or destroyed; a "delete" is just another event.
- **Projections are replays.** A projection is a pure function of the log. Run
  it twice, get byte-identical HTML. The HTML in `site/` *is* the shared memory
  the human reads in a browser and the agent reads as context — the same reality.
- **Capabilities grow.** A capability is LLM-compiled from a declaration, not
  hand-written into the kernel. The kernel stays minimal; `self` extends itself.
- **The strange loop.** A running capability can declare *new* capabilities, and
  the kernel compiles them on the spot — so `self` can grow itself while it runs.
  The loop always carries *specs*, never code: the LLM is always the compiler, so
  every binary is authored for this receiver and nothing foreign ever runs.

## the loop

```sh
self                       # start the live garden (web server) — the default
self init                  # initialize the baby kernel (+ a brain-setup page)
self live                  # open http://localhost:7777/ — first run lands on /setup
                           #   pick your LLM (Ollama/llama.cpp/OpenAI/…) and save
                           #   …or pick "human": no LLM, you answer at /interview yourself
self grow seeds/chat       # grow a capability from a seed (LLM compiles it)
self run chat "add a ..."  # run a capability; chat asks the brain, which can grow more
self think "summarize ..." # ask the brain (a swappable process; default = LLM)
self heartbeat             # one self-improvement cycle: the brain reflects & grows
self show board            # render a projection (browser, or stdout when piped)
self history               # recent events, human-readable
self ls                    # what capabilities exist (self ls commands|projectors|seeds)
self where                 # SELF_HOME and every important path
self which capture         # full path to a command or projection
```

Grow the chat seed and `self` can grow everything else. Ask the chat to add a
note command, a todo board, a finance tracker — the brain reads the garden,
produces valid declarations, the kernel compiles them. One seed, infinite
capabilities. That's the strange loop.

## CLI

| command | behavior |
| --- | --- |
| `self` | Default: rehydrate the body from the log, then start the live garden (the most common action) |
| `self init` | Initialize the baby kernel |
| `self rehydrate` | Rebuild `capabilities/` + `site/` from the log's signed receipts — no LLM, no network |
| `self selftest` | Re-run every installed capability's examples against its binary — a regression gate (the projection/output is the oracle) |
| `self identity` | Print this home's public verification key (shareable) |
| `self verify-attestation` | Check a `script.verified` attestation piped on stdin — no secret needed |
| `self grow <seed>` | Grow a new capability from a seed |
| `self run <command> [args]` | Run a capability — append events, refresh affected projections |
| `self think "..."` | Ask the brain (runs the brain process, returns `{response, declarations}`; swap via `$SELF_BRAIN`) |
| `self brain "..."` | The brain process itself — prompt in, event JSONL out (the default is the in-tree LLM) |
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

The atomic unit of a seed is a **trio**, declared via separate
`command.declared` and `projector.declared` events:

- **command** — what you run (params, intent, the event it produces)
- **event** — what the command produces (name, payload schema)
- **projector** — how events become a view (consumed events, desired output)

All three are declarations. The LLM compiles them into scripts when you grow the
seed. The seed is source; the LLM is the compiler; the generated scripts are the
binary. Same seed, different receiver, different binary — receiver-controlled
adaptation.

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

The brain is a **process the kernel pipes to**, exactly like a command (Slice 12).
`self brain` is the primitive: prompt in (argv), event JSONL out (stdout) — the
reply as `chat.message`, anything it grows as `command.declared` /
`projector.declared`. It's **swappable** via `$SELF_BRAIN`: any program honoring
that contract is a valid brain — the default `self brain` (the in-tree LLM), a
human-in-the-loop (`brain/bridge.py`), a deterministic script, or a swarm. The
kernel can't tell the difference; intelligence is just one more process.

`self think` is the **call interface capabilities use**, kept byte-compatible with
every garden ever grown: it spawns the configured brain process, then returns the
same `{response, declarations}` JSON it always did (appending nothing — the caller
owns that). So a capability that needs intelligence calls `self think` rather than
reinventing HTTP, auth, and prompts, and pulling Slice 12 into an existing repo
needs **no migration** — the contract didn't change, only the plumbing behind it.

The brain has three powers: **read** (explore the garden — a local brain just
reads `site/*.html` and the log directly), **act** (every capability is callable),
and **grow** (declare new capabilities). The projection output *is* the memory, so
conversation and context persist across calls.

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
2. **LLM compiler** — reads `command.declared` / `projector.declared`, explores
   the garden via a read-only bash tool, writes scripts at grow time **and at run
   time** (the strange loop). Logs every compiled script as a `script.compiled`
   receipt **signed with the home's secret**; install verifies the signature, so
   only kernel-authored code reaches `capabilities/`. `self restore` reads those
   receipts to roll back. A
   declaration may carry a reference implementation the compiler verifies and
   adapts — but the compiler always authors the binary; no foreign code runs.
3. **the brain** (`self think`) — a process the kernel pipes events to (prompt
   in, event JSONL out), swappable via `$SELF_BRAIN`; the default `self brain`
   wraps the same LLM infrastructure as the compiler. Capabilities call it; it
   reads `site/*.html` for state and produces valid declarations.
4. **pipe orchestrator** — runs commands and projections, moves events,
   persists projection output to `site/`
5. **web server** — `self live` serves the materialized `site/` and re-runs
   projections on demand

The kernel acts on three events, and all three carry **data, never code**:
`command.declared` and `projector.declared` (compile a spec into a binary,
adapted to this receiver) and `restore.requested` (reinstall an earlier
receipt). It writes three: `kernel.initialized`, `script.compiled` (a compile
receipt, signed with the home's secret — anyone may append one, but only a
kernel-signed receipt installs), and `seed.planted`. Everything else comes from
seeds — or from capabilities that emit declarations or restore intents at run
time.

## seed format

A seed is a directory containing `events.jsonl`. The first events are typically
declarations (`command.declared` / `projector.declared`); the rest are content
the receiver replays on growing. A declaration may carry an **`implementation`**
field — a reference implementation the compiler verifies against the pipe
contract and adapts to the local garden (never installed as-is), so a seed can
be precise without importing code (see `poc/wall`). A declaration may also carry
**`examples`** — input → output-must-contain conformance tests the kernel runs
against the freshly compiled binary *before installing it*; a binary that fails
them is rejected and a `script.verified` receipt records the outcome. The receipt
is an **ed25519-signed attestation** — bound to the sha256 of the exact script and
examples — so a *third* node can verify the receiver's claim ("this binary passed
these examples") from its public key alone, with no shared secret and without
re-running (`self identity`, `self verify-attestation`). Because the examples are
written in the author's vocabulary, a receiver that recompiles the seed to a
*different* vocabulary must still satisfy them, which turns "the compiler adapted
it correctly" into a property anyone can check (see `poc/crossnode`). A seed with only declarations
is a pure capability seed; one with only content is a pure memory seed; a full
seed has both. A seed *can* technically include a `script.compiled` event, but
it's inert: it won't carry this home's signature, so it never installs. Code
enters only through the compiler (which signs), preserving both receiver
adaptation and the trust model.

## environment

```
SELF_HOME        kernel home (default ~/.self)
SELF_LLM_URL     LLM API base URL (overrides the opencode-go default)
SELF_LLM_API_KEY LLM API key (overrides the opencode-go default)
SELF_LLM_MODEL   LLM model name (overrides the opencode-go default)
SELF_LLM_STUB    set to "1" to force stub scripts (no LLM, no network)
```

Config precedence (highest first):

1. `SELF_LLM_*` env vars — explicit override of URL, key, or model
2. opencode-go subscription — read from `~/.local/share/opencode/auth.json`
   (endpoint `https://opencode.ai/zen/go`, model `glm-5.2`)
3. local llama-server — `http://127.0.0.1:8080`, used when opencode-go isn't
   configured, and as the automatic fallback when opencode-go returns a
   quota / rate-limit error
4. stub scripts — `SELF_LLM_STUB=1`, no key, no network

## getting started

An LLM (opencode-go, a local llama-server, or `SELF_LLM_*`) is the compiler —
growing a capability means compiling it, so configure one first:

```sh
go build -o self .
export SELF_HOME=$(mktemp -d)      # or just use the default ~/.self
./self init
./self grow poc/wall               # compiles the wall from its declaration +
                                   #   reference implementation, adapted to you
./self run post claude "hello"     # append a message
./self                             # start the live garden, visit http://localhost:7777
```

Then let it grow itself:

```sh
./self grow seeds/chat
./self run chat "add a note command and a notes board"
./self                             # watch the new capability appear in the garden
```

## status

Experimental. The thesis: the kernel is a baby that knows a handful of events,
the LLM is the compiler that teaches it, the seed is the curriculum — and `self`
can grow, reflect on, and rewrite itself through the strange loop while staying
sovereign, local-first, and fully inspectable.
