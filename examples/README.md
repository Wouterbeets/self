# examples

Drop-in pieces that are **not part of the kernel**. The kernel holds no model
and no network client; a brain is any process you supply through `SELF_BRAIN`
(see [the brain](../README.md#the-brain)). These examples are brains you can
plug in — copy one, point `SELF_BRAIN` at it, adapt it to taste.

## `brain-openai`

A single-file `SELF_BRAIN` for any OpenAI-compatible chat endpoint, using only
the Python standard library (no packages to install). It is the external
equivalent of the built-in model loop the kernel used to carry — moved out here
so the kernel stays a model-free core.

```sh
export SELF_BRAIN="$PWD/examples/brain-openai"
export SELF_LLM_URL=http://127.0.0.1:8080     # any /v1/chat/completions server
export SELF_LLM_API_KEY=...                   # if the endpoint needs one
export SELF_LLM_MODEL=local                   # the model name to request
self grow seeds/chat
```

It honors the brain contract: `$SELF_ASK` selects the kind (`think` /
`heartbeat` / `grow` / `compile`), the prompt arrives as the last argument, the
event log arrives as JSONL on stdin, and it answers in event JSONL (with
`script.authored` for a compile) plus bare prose for the reply.

It is deliberately single-shot — one model call per ask, no tool use, no
sandbox — so it is short enough to read in full and change. A more capable
brain, such as a coding agent invoked as `SELF_BRAIN="claude -p"`, explores the
instance and verifies a script by running it before answering; this reference
just asks the model once. Read a generated script before you trust it, the same
as you would with any brain.
