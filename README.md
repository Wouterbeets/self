# ks

A kernel for the Knowledge Seed Protocol. The kernel is born knowing one
event. Seeds teach it everything else. The LLM compiles seed declarations
into runnable code at plant time. The kernel appends events, replays them
through compiled projectors, and renders HTML.

## thesis

A seed is a single event stream (`events.jsonl`). The first events declare
capabilities; the rest use them. There are no two halves — declarations and
content are the same stream.

The kernel is born knowing **one event it acts on**: `trio.declared`. When it
sees one, it reads the trio spec from the payload and hands it to the LLM
compiler, which writes the scripts that implement it. Two provenance events
the kernel writes but doesn't interpret: `kernel.initialized` (at birth) and
`seed.planted` (receipt after planting).

Everything else — `note.captured`, `task.created`, `meal.planned` — comes from
seeds. A fresh `ks init` is a baby with no capabilities. Plant seeds, it grows.

## the trio

The atomic unit of a seed is a **trio**, declared via a `trio.declared` event:

- **command** — what the user invokes (params, intent)
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
ks project                 # replay events through the projector, emit HTML
ks log                     # show the event log
ks seeds                   # list planted seeds (from seed.planted events)
```

After planting the example notes seed, `ks project` already shows two notes —
the starter events that came with the seed. Then `ks invoke note "buy milk"`
adds a third. One stream, declarations and content together.

## pipe contract

Compiled scripts are standalone executables orchestrated by the kernel via
Unix pipelines:

- **command script**: receives args as `argv`, current events as JSONL on
  `stdin`, writes new events as JSONL on `stdout` (one per line, fields:
  `name`, `payload`). The kernel assigns `id`, `seq`, `occurred_at`.
- **projector script**: receives all events as JSONL on `stdin`, writes
  HTML on `stdout`.

No embedded runtime, no plugin loader. The kernel is a pipe orchestrator.

## what the kernel is

Three things, irreducible:

1. **event store** — append-only JSONL log (the only truth)
2. **LLM compiler** — reads `trio.declared` payloads, writes scripts at plant time
3. **pipe orchestrator** — runs commands and projectors, moves events

The kernel knows one event: `trio.declared`. It writes two: `kernel.initialized`
and `seed.planted`. Everything else comes from seeds.

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
KS_LLM_URL     LLM API base URL (default http://127.0.0.1:8080)
KS_LLM_API_KEY LLM API key (if unset, ks writes stub scripts)
KS_LLM_MODEL   LLM model name (default "local")
```

Without `KS_LLM_API_KEY`, `ks plant` writes trivial stub scripts so the pipe
contract is demonstrable. Set the key and the LLM compiles real
implementations from the declarations.

## status

Experimental MVP. The thesis: the kernel is a baby that knows one event, the
LLM is the compiler that teaches it, the seed is the curriculum. This repo
proves the loop with the smallest thing that makes it undeniable.
