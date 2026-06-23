# ks

A kernel for the Knowledge Seed Protocol. The kernel is born knowing one
event. Seeds teach it everything else. The LLM compiles seed declarations
into runnable code at plant time. The kernel appends events, replays them
through compiled projectors, and renders HTML.

## thesis

A seed is a single event stream (`events.jsonl`). The first events declare
capabilities; the rest use them. There are no two halves — declarations and
content are the same stream.

The kernel is born knowing **two events it acts on**: `command.declared` and
`projector.declared`. When it sees one, it reads the spec from the payload
and hands it to the LLM compiler, which writes the scripts that implement it.
Two provenance events the kernel writes but doesn't interpret:
`kernel.initialized` (at birth) and `seed.planted` (receipt after planting).

Everything else — `note.captured`, `task.created`, `chat.message` — comes
from seeds or from commands that emit declarations at invoke time. A fresh
`ks init` is a baby with no capabilities. Plant seeds, it grows.

## the trio

The atomic unit of a seed is a **trio**, declared via separate
`command.declared` and `projector.declared` events:

- **command** — what the user invokes (params, intent, the event it produces)
- **event** — what the command produces (name, payload schema)
- **projector** — how events become a view (consumed events, desired output)

All three are declarations. The LLM compiles them into scripts at plant time.
The seed is source code; the LLM is the compiler; the generated scripts are
the binary. Same seed, different receivers, different binaries — that's
receiver-controlled adaptation.

## the loop

```
ks init                    # baby kernel born (appends kernel.initialized)
ks plant seeds/chat        # LLM compiles the chat interface — the only seed you need
ks invoke chat "add a..."  # chat calls ks think, brain suggests new capabilities
ks project                 # replay events through projector, emit HTML to site/
ks think "summarize..."    # call the kernel's brain directly (LLM + garden exploration)
ks serve                   # HTTP server: static site/ + /live/<name> + /events
ks log                     # show the event log
ks seeds                   # list planted seeds (from seed.planted events)
```

Plant the chat seed and the kernel can grow everything else. Ask the chat
to add a note command, a todo projector, a finance tracker — the brain
reads the garden, produces valid declarations, the kernel compiles them.
One seed, infinite capabilities. That's the strange loop.

## self-improvement (the strange loop)

`ks invoke` doesn't just append events — it scans them for `command.declared`
and `projector.declared`. If a command emits either, the kernel compiles them
on the spot and writes the scripts to the registry. This means a command can
plant new capabilities, including re-declaring itself.

```
ks plant seeds/chat        # install the chat interface (command + projector)
ks invoke chat "add a summarize command that ..."
# → chat calls ks think, brain reads site/chat.html + garden, suggests declarations
# → chat emits chat.message + command.declared + projector.declared
# → kernel compiles the new command/projector immediately
ks invoke summarize "..."  # the new command works right away
```

The event log keeps every declaration and every compiled script. The
registry holds only the latest. Re-planting from the log is rollback:
find the `script.compiled` event for the capability, restore that exact
script to the registry — no re-compilation, no drift. The chat interface
is the constitution, and it's editable from inside the chat.

## pipe contract

Compiled scripts are standalone executables orchestrated by the kernel via
Unix pipelines. Any language works — Python, bash, node, Perl, anything
`os.Exec` can run:

- **command script**: receives args as `argv`, current events as JSONL on
  `stdin`, writes new events as JSONL on `stdout` (one per line, fields:
  `name`, `payload`). The kernel assigns `id`, `seq`, `occurred_at`.
- **projector script**: receives all events as JSONL on `stdin`, writes
  HTML on `stdout`. The kernel persists the output to
  `KS_HOME/site/<name>.html` — projectors don't write to disk, they just
  emit HTML and the kernel decides where it goes.

The kernel sets `KS_HOME` env var on every script. Commands that need LLM
intelligence call `ks think` — the kernel's brain — instead of making their
own HTTP calls. The kernel is the sole steward of LLM credentials. No helper
module, no language assumptions, no embedded runtime.

## ks think — the kernel's brain

The kernel exposes its LLM as a callable pipe. Commands that need
intelligence call `ks think` instead of reinventing HTTP calls, auth, and
system prompts:

```
echo "add a todo command" | ks think
→ {"response": "I've added a todo command...", "declarations": [...]}
```

The brain is the same LLM infrastructure as the compiler — bash tool,
garden exploration, schema knowledge — with a general-purpose prompt.
It reads `site/*.html` for current state (the projector output IS the
memory: chat projector renders `site/chat.html`, brain reads it before
responding, conversation persists across invocations). When the brain
suggests new capabilities, it produces valid declarations that flow
through the existing strange-loop hook and get compiled.

This collapses the complexity: the chat seed's declaration is ~200 chars
("call ks think, emit the response, forward declarations") instead of
~2000 chars of embedded HTTP/auth/prompt/parsing logic. The brain knows
the schema, so declarations are valid. The brain knows the garden, so
suggestions integrate.

## garden-aware compilation

At plant time (and at invoke time via the strange loop), the compiler gives
the LLM a read-only `bash` tool with cwd set to `KS_HOME`. The LLM explores
the garden — `ls registry/commands/`, `head events.jsonl`, `cat site/kernel.html`
— before writing the script. This is how a seed adapts to the receiver: if
a finance projector declares consumption of `finance.expenditure_added` but
the stream already has `shopping_bill_uploaded` events with `{vendor, amount,
date}`, the LLM extends the projector's filter to consume both, mapping
`vendor→category`. Same seed, different garden, different binary.

The bash tool is sandboxed: restricted bash (`-r`), a denylist of
destructive/network/interpreter commands, no redirection, 10s timeout,
10KB output cap. The LLM can look but not touch.

## on disk

```
KS_HOME/
  events.jsonl               the only truth (append-only)
  registry/
    commands/<name>          compiled command scripts (any language)
    projectors/<name>        compiled projector scripts (any language)
  site/<name>.html           materialized HTML projections (written by ks project)
```

Agents (opencode, grep, anything) read `site/` directly — plain files, no
API. `ks serve` exposes them over HTTP with `/live/<name>` for on-demand
re-projection against current events.

## what the kernel is

Five things, irreducible:

1. **event store** — append-only JSONL log (the only truth)
2. **LLM compiler** — reads `command.declared` and `projector.declared` payloads,
   explores the garden via a read-only bash tool, writes scripts at plant time
   **and at invoke time** (if a command emits declarations). Logs every compiled
   script as a `script.compiled` event for audit-faithful rollback.
3. **LLM brain** (`ks think`) — the same LLM infrastructure as the compiler,
   exposed as a callable pipe. Commands call it for intelligence; it reads
   `site/*.html` for current state and produces valid declarations.
4. **pipe orchestrator** — runs commands and projectors, moves events,
   persists projector output to `site/`
5. **HTTP server** — `ks serve` exposes materialized site/ and re-runs
   projectors on demand at `/live/<name>`

The kernel knows two events: `command.declared` and `projector.declared` (it
compiles them). It writes three: `kernel.initialized`, `script.compiled`,
and `seed.planted`. Everything else comes from seeds — or from commands
that emit declarations at invoke time.

## seed format

A seed is a directory containing `events.jsonl`. That's it. The first events
are typically `trio.declared` (capability declarations). The rest are content
— starter events the receiver replays on planting. A seed with only
`trio.declared` events is a pure capability seed (empty until used). A seed
with only content events is a pure memory seed (the receiver must already have
the capabilities to project them). A full seed has both.

## environment

```
KS_HOME        kernel home (default ~/.ks)
KS_LLM_URL     LLM API base URL (overrides the opencode-go default)
KS_LLM_API_KEY LLM API key (overrides the opencode-go default)
KS_LLM_MODEL   LLM model name (overrides the opencode-go default)
KS_LLM_STUB    set to "1" to force stub scripts
```

Config precedence (highest first):

1. `KS_LLM_*` env vars — explicit override of URL, key, or model
2. opencode-go subscription — read from `~/.local/share/opencode/auth.json`
   (endpoint `https://opencode.ai/zen/go`, model `glm-5.2`)
3. local llama-server — `http://127.0.0.1:8080`, used when opencode-go isn't
   configured, and as the automatic fallback when an opencode-go request is
   refused with a quota-exceeded / rate-limit error (HTTP 429/402, or a quota
   hint in the response body)
4. stub scripts — `KS_LLM_STUB=1`, no key, no network

If you have an opencode-go subscription configured via opencode, `ks plant`
and `ks think` use it automatically — no extra setup. When opencode-go returns
a quota error, ks retries the same call against the local llama-server and
continues. Set `KS_LLM_STUB=1` to force stub scripts without calling the LLM.
Commands don't receive LLM credentials — they call `ks think` for intelligence.

## status

Experimental MVP. The thesis: the kernel is a baby that knows two events,
the LLM is the compiler that teaches it, the seed is the curriculum. This
repo proves the loop with the smallest thing that makes it undeniable —
including the strange loop: a command that plants commands.
