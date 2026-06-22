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
ks plant examples/notes    # LLM compiles trio.declared, replays starter events
ks invoke note "buy milk"  # run the compiled command, event appended
ks project                 # replay events through projector, emit HTML + persist to site/
ks serve                   # HTTP server: static site/ + /live/<name> + /events
ks log                     # show the event log
ks seeds                   # list planted seeds (from seed.planted events)
```

After planting the example notes seed, `ks project` already shows two notes —
the starter events that came with the seed — and writes `~/.ks/site/note.html`.
Then `ks invoke note "buy milk"` adds a third. `ks serve` exposes the
materialized projection at `http://localhost:7777/` and a live re-projection
at `/live/note`. One stream, declarations and content together.

## self-improvement (the strange loop)

`ks invoke` doesn't just append events — it scans them for `command.declared`
and `projector.declared`. If a command emits either, the kernel compiles them
on the spot and writes the scripts to the registry. This means a command can
plant new capabilities, including re-declaring itself.

```
ks plant seeds/chat        # install the chat interface (command + projector)
ks invoke chat "add a summarize command that ..."
# → chat emits chat.message + command.declared + projector.declared
# → kernel compiles the new command/projector immediately
ks invoke summarize "..."  # the new command works right away
```

The event log keeps every declaration — old and new. The registry holds only
the latest. Re-planting an older `command.declared` from the log is rollback.
The chat interface is the constitution, and it's editable from inside the
chat.

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

The kernel sets `KS_HOME` env var on every script. Commands that need
persistent state between calls can write to `$KS_HOME/artefacts/<name>.json`.
The kernel also passes `KS_LLM_URL`, `KS_LLM_API_KEY`, and `KS_LLM_MODEL` to
command scripts — so commands like `chat` can call the LLM directly.
No helper module, no language assumptions, no embedded runtime.

## artefacts on disk

```
KS_HOME/
  events.jsonl               the only truth (append-only)
  registry/
    commands/<name>          compiled command scripts (any language)
    projectors/<name>        compiled projector scripts (any language)
  site/<name>.html           materialized HTML projections (written by ks project)
  artefacts/<name>.json      structured state (written by commands via $KS_HOME)
```

Agents (opencode, grep, anything) read `site/` and `artefacts/` directly —
plain files, no API. `ks serve` exposes them over HTTP with `/live/<name>`
for on-demand re-projection against current events.

## what the kernel is

Four things, irreducible:

1. **event store** — append-only JSONL log (the only truth)
2. **LLM compiler** — reads `trio.declared` payloads, writes scripts at plant
   time **and at invoke time** (if a command emits declarations)
3. **pipe orchestrator** — runs commands and projectors, moves events,
   persists projector output to `site/`
4. **HTTP server** — `ks serve` exposes materialized site/ and re-runs
   projectors on demand at `/live/<name>`

The kernel knows two events: `command.declared` and `projector.declared` (it
compiles them). It writes two: `kernel.initialized` and `seed.planted`.
Everything else comes from seeds — or from commands that emit declarations
at invoke time.

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
KS_LLM_URL     LLM API base URL (auto-detected from opencode-go)
KS_LLM_API_KEY LLM API key (auto-detected from opencode-go)
KS_LLM_MODEL   LLM model name (auto-detected from opencode-go)
KS_LLM_STUB    set to "1" to force stub scripts
```

Config precedence (highest first):

1. `KS_LLM_*` env vars — explicit override of URL, key, or model
2. opencode-go subscription — read from `~/.local/share/opencode/auth.json`
   (endpoint `https://opencode.ai/zen/go`, model `glm-5.2`)
3. stub scripts — no key, no network

If you have an opencode-go subscription configured via opencode, `ks plant`
uses it automatically — no extra setup. Set `KS_LLM_STUB=1` to force stub
scripts without calling the LLM.

## status

Experimental MVP. The thesis: the kernel is a baby that knows two events,
the LLM is the compiler that teaches it, the seed is the curriculum. This
repo proves the loop with the smallest thing that makes it undeniable —
including the strange loop: a command that plants commands.
