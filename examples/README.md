# examples

Brain adapters that are **not part of the kernel**. The kernel holds no model
and no network client; a brain is a process you supply through `SELF_BRAIN`
(see [the brain](../README.md#the-brain)). These adapters illustrate the
contract's wire shape — copy one, point `SELF_BRAIN` at it, adapt it to taste.

**The contract the kernel enforces, in one sentence:** the kernel gives the
brain an orientation brief on stdin (where the brain is, what capabilities
exist, where to look for the rest); the brain MUST inspect `SELF_HOME` itself
— `site/*.html`, `events.jsonl`, `capabilities/` — with its own tools to do
its job. A process without that exploration ability is not a complete brain.

## `claude -p` — no adapter

A coding-agent CLI that already explores with its own tools and prints to
stdout plugs straight into the pipe; there is nothing here to install for it:

```sh
export SELF_BRAIN="claude -p --continue"
self grow seeds/chat
```

`--continue` matters: the kernel runs the brain with `SELF_HOME` as its
working directory, so each ask resumes the previous ask's conversation for
that instance — compiles inherit the orchestrating ask's exploration instead
of re-reading the instance cold, and an interactive session you ran in the
instance directory becomes the conversation the kernel continues.

## `brain-openai`

A reference adapter that illustrates the wire shape for any OpenAI-compatible
chat endpoint, using only the Python standard library (no packages to install).

```sh
export SELF_BRAIN="$PWD/examples/brain-openai"
export SELF_LLM_URL=http://127.0.0.1:8080     # any /v1/chat/completions server
export SELF_LLM_API_KEY=...                   # if the endpoint needs one
export SELF_LLM_MODEL=local                   # the model name to request
self grow seeds/chat
```

It honors the brain's pipe contract: `$SELF_ASK` selects the kind (`think` /
`heartbeat` / `grow` / `compile`), the prompt arrives as the last argument, the
orientation brief arrives as plain text on stdin, and it answers in event JSONL
(with `script.authored` for a compile) plus bare prose for the reply.

It is intentionally incomplete by spec: it is single-shot — one model call per
ask, no tool use, no file inspection — so a real brain needs a tool loop to
read `SELF_HOME` and verify scripts by running them. Use it to understand the
seam; for real capabilities, plug a tool-capable coding agent like
`brain-opencode` (below) or `claude -p`. Read a generated script before you
trust it, the same as you would with any brain.

## `brain-opencode`

A working tool-capable brain that delegates to `opencode run`, which can
inspect `SELF_HOME` itself. This is the adapter the contract actually asks for:
the brain reads the orientation brief, then uses its own tools to explore
`site/*.html`, `events.jsonl`, `capabilities/` before answering. Directionally
correct — point `SELF_BRAIN` at this for real capabilities.

```sh
export SELF_BRAIN="$PWD/examples/brain-opencode"
export SELF_OPENCODE_MODEL=github-copilot/gpt-5.5  # optional
self grow seeds/chat
```
