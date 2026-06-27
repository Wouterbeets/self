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
self init                  # initialize the baby kernel
self grow seeds/chat       # grow a capability from a seed (LLM compiles it)
self run chat "add a ..."  # run a capability; chat asks the brain, which can grow more
self think "summarize ..." # ask the brain directly (LLM + garden exploration)
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
| `self` | Default: start the web server / live garden (the most common action) |
| `self init` | Initialize the baby kernel |
| `self grow <seed>` | Grow a new capability from a seed |
| `self run <command> [args]` | Run a capability — append events, refresh affected projections |
| `self think "..."` | Ask the brain (LLM + garden exploration) |
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
attack surface). A capability cannot install a binary: `script.compiled` is a
**kernel-only** receipt, ignored if a seed or command emits one.

Two consequences worth naming:

- **Precision without code injection.** A seed that wants exact, complex behavior
  ships a *reference implementation* — an `implementation` field on a declaration.
  The compiler verifies it against the pipe contract and adapts it to the local
  garden; it is never installed as-is. Near-identical power to handing over code,
  but coherent with receiver adaptation and with zero new attack surface (see
  `poc/wall`).
- **Rollback splits cleanly into trigger and install.** Every compile is logged
  as a kernel-only `script.compiled` receipt. *Installing* an earlier one is the
  kernel's job — same privilege as compile. But *triggering* a rollback is just a
  data-only `restore.requested {name, seq}` event, which anything may emit. So
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

## the brain (`self think`)

The kernel exposes its LLM as a callable pipe. Capabilities that need
intelligence call `self think` instead of reinventing HTTP, auth, and prompts.
The brain has three powers: **read** (a sandboxed bash tool to explore the
garden), **act** (every capability is a callable tool), and **grow** (declare
new capabilities). It reads `site/*.html` for current state — the projection
output *is* the memory — so conversation and context persist across calls.

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
  capabilities/
    commands/<name>             compiled command scripts (any language)
    projectors/<name>           compiled projection scripts (any language)
  site/<name>.html              materialized HTML projections
```

Run `self where` to see all of this for your home, `self ls commands` /
`self ls projectors` for the full file paths, and `self which <name>` for one.
Agents (and you) read `site/` directly — plain files, no API. `self live`
exposes them over HTTP with `/<name>` re-rendered live against current events.

## what the kernel is

Five things, irreducible:

1. **event store** — append-only JSONL log (the only truth)
2. **LLM compiler** — reads `command.declared` / `projector.declared`, explores
   the garden via a read-only bash tool, writes scripts at grow time **and at run
   time** (the strange loop). Logs every compiled script as a kernel-only
   `script.compiled` receipt; `self restore` reads those receipts to roll back. A
   declaration may carry a reference implementation the compiler verifies and
   adapts — but the compiler always authors the binary; no foreign code runs.
3. **the brain** (`self think`) — the same LLM infrastructure as the compiler,
   exposed as a callable pipe. Capabilities call it; it reads `site/*.html` for
   state and produces valid declarations.
4. **pipe orchestrator** — runs commands and projections, moves events,
   persists projection output to `site/`
5. **web server** — `self live` serves the materialized `site/` and re-runs
   projections on demand

The kernel acts on three events, and all three carry **data, never code**:
`command.declared` and `projector.declared` (compile a spec into a binary,
adapted to this receiver) and `restore.requested` (reinstall an earlier
receipt). It writes three: `kernel.initialized`, `script.compiled` (a compile
receipt — kernel-only, never accepted from a seed or command), and
`seed.planted`. Everything else comes from seeds — or from capabilities that
emit declarations or restore intents at run time.

## seed format

A seed is a directory containing `events.jsonl`. The first events are typically
declarations (`command.declared` / `projector.declared`); the rest are content
the receiver replays on growing. A declaration may carry an **`implementation`**
field — a reference implementation the compiler verifies against the pipe
contract and adapts to the local garden (never installed as-is), so a seed can
be precise without importing code (see `poc/wall`). A seed with only declarations
is a pure capability seed; one with only content is a pure memory seed; a full
seed has both. A seed may **not** carry a `script.compiled` event — that's a
kernel-only receipt, and accepting code from a seed would break both receiver
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
