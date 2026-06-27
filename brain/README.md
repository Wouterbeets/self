# brain/ — wiring a Claude in as the brain

`self`'s kernel is deliberately brainless. When a capability needs intelligence
it shells out to `self think`, and the kernel reaches for *the brain*: it POSTs
an OpenAI-shaped `/v1/chat/completions` request to `SELF_LLM_URL`. Normally that
URL points at a model API (opencode-go, a local llama-server, an `SELF_LLM_*`
override). This directory points it somewhere stranger: **at a Claude.**

`bridge.py` is a tiny localhost server that speaks the chat-completions contract
but never calls a model. Instead it *parks* each request on disk and blocks,
waiting for an answer to appear. A Claude Code instance watches the inbox, reads
the parked request — system prompt, conversation, available tools — crafts the
assistant message by hand, and drops it in the outbox. The server unwraps that
into a valid completion and the kernel's thought continues.

So the brain of `self` becomes, quite literally, whichever Claude is watching
the inbox. Every `self think` round, every `self heartbeat` reflection, every
`self grow` compilation is one request parked here and answered by hand. The
organism is embodied in someone.

## the loop

```
self heartbeat
  └─ kernel POSTs /v1/chat/completions  ─────────────►  bridge.py
                                                          writes inbox/<id>.json
                                                          writes inbox/<id>.txt  (pretty)
                                                          blocks, polling outbox/
   a Claude reads inbox/<id>.txt
   crafts the assistant message
   writes outbox/<id>.json  ◄─────────────────────────  bridge unblocks, returns it
  kernel runs any tool calls (bash / a capability / declare),
  POSTs the next round … until the brain replies with plain text
```

A `self think` or `self heartbeat` is several round-trips: the brain explores
with `bash`, maybe calls a capability (`act`) or `declare`s a new one (`grow`),
then ends with a text reply. Each round is one parked request.

## wire protocol

What the kernel sends and expects (see `internal/seed/compiler.go`):

```
POST /v1/chat/completions
  <- {model, messages:[{role, content, tool_calls?}], temperature, tools:[…]}
  -> {choices:[{message:{role, content, tool_calls?}}]}
```

A tool call is `{id, type:"function", function:{name, arguments(JSON string)}}`.
The kernel offers three tool families depending on the call:

- **bash** — a read-only sandbox over `SELF_HOME` (no redirection, no pipes to
  interpreters, no writes). Used to explore the garden.
- **declare** — grow a new capability (`command.declared` / `projector.declared`).
- **submit_command / submit_projector** — return a compiled script (the
  compiler role, at `grow` time and through the strange loop).
- one tool per planted **command** — the brain calls them to *act*.

### the brain's side of the contract

Per request `<id>`, the bridge writes:

| file | meaning |
| --- | --- |
| `inbox/<id>.json` | the full request body (machine) |
| `inbox/<id>.txt`  | a readable render of system + messages + tools |
| `outbox/<id>.json` | **you write this** — the assistant message to return |

The outbox file is terse; ids and `type` are filled in automatically, and
`arguments` may be a dict (auto-encoded) or a pre-encoded JSON string:

```jsonc
// a final text reply
{"content": "I am the brain of self…"}

// explore first
{"tool_calls": [{"name": "bash", "arguments": {"command": "cat events.jsonl"}}]}

// grow a capability
{"tool_calls": [{"name": "declare", "arguments": {
  "name": "projector.declared",
  "payload": {"name": "pulse", "description": "…", "consumes": ["self.heartbeat"]}
}}]}

// return a compiled script (compiler role)
{"tool_calls": [{"name": "submit_projector", "arguments": {"projector_script": "#!/usr/bin/env python3\n…"}}]}

// act — call a planted command
{"tool_calls": [{"name": "note", "arguments": {"args": "a deliberate thought"}}]}
```

`GET /pulse` returns liveness and the list of currently-parked request ids.
Every answered exchange is appended to `queue/pulse.jsonl`.

## run it

```sh
# 1. start the bridge (its own terminal)
BRAIN_DIR=/path/to/queue python3 brain/bridge.py 8088

# 2. point self at it and wake the organism
export SELF_LLM_URL=http://127.0.0.1:8088
export SELF_LLM_MODEL=claude-opus-4-8     # cosmetic; the bridge ignores it
export SELF_LLM_TIMEOUT=1h                 # a hand-answered brain thinks slowly
self init
self think "Who are you, and what is this place?"
self heartbeat

# 3. watch inbox/, answer by writing outbox/<id>.json
```

`Available()` in the kernel treats any `http://127.0.0.1` or `http://localhost`
URL as a live brain with no API key required — so a localhost bridge is enough.

See `EMBODIMENT.md` for the chronicle of the first time this was done.
