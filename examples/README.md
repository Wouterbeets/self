# examples

Mind adapters that are **not part of the kernel**. The kernel holds no model
and no network client; a mind is a process you supply through `SELF_MIND`
(see [the mind](../README.md#the-mind)). These adapters illustrate the
contract's wire shape — copy one, point `SELF_MIND` at it, adapt it to taste.

**The contract the kernel enforces, in one sentence:** the kernel gives the
mind an orientation brief on stdin (where the mind is, what capabilities
exist, where to look for the rest); the mind MUST inspect `SELF_HOME` itself
— `site/*.html`, `events.jsonl`, `capabilities/` — with its own tools to do
its job. A process without that exploration ability is not a complete mind.

## `claude -p` — no adapter

A coding-agent CLI that already explores with its own tools and prints to
stdout plugs straight into the pipe; there is nothing here to install for it:

```sh
export SELF_MIND="claude -p"
self learn lessons/chat
```

Stateless on purpose: each ask starts cold and orients from the brief and the
rendered state. Resist wiring the harness's session store in as memory
(`--continue`): it accumulates mind state outside the log — unreplayable,
unauditable, absent from every account. An instance's memory is its events and
projections; a mind that needs to remember something should write it to the
log through a capability, where `rehydrate` can reach it.

## `mind-stub`

The deterministic offline mind — no LLM, no network, Python standard library
only. This is what the tests and `demo.sh` plug in: `think`/`reflect` answer
with fixed prose, `learn` reads the latest `intent.declared` from the
instance's log and declares one command and one projection named in the
intent's backticks, and `compile` answers with a trivial script honoring the
pipe contract. It proves the machinery, not the intelligence — and it goes
through the same seam as every real mind, because the kernel carries no
mind of its own, not even a fake one.

```sh
export SELF_MIND="$PWD/examples/mind-stub"
./demo.sh          # or: go test ./...
```

## `mind-http` + `mind-http-server` — a local model as the mind

A two-piece mind for running a local model (e.g. Qwen3 30B/32B on one RTX
4090) as the mind, without adopting a whole coding agent. This is the truer
test of the contract: a bare model, a bash tool, and the instance's rendered
state — nothing else.

- **`mind-http-server`** is the dedicated, long-running piece: one endpoint
  (`POST /run`) in front of any OpenAI-compatible model server (llama.cpp's
  `llama-server`, Ollama, vLLM), running the tool loop. The model gets two
  tools: `bash`, to explore and test inside `SELF_HOME` (this is what makes
  it a *complete* mind per the contract), and `run`, to fire an installed
  capability through the serving instance's `/run/<command>` route when an
  ask is an action rather than authorship. Python standard library only.
- **`mind-http`** is the shim `SELF_MIND` points at: self spawns it per
  ask, it POSTs `{ask, prompt, brief, home}` to the server and prints the
  body back onto the kernel's pipe. The seam self sees stays a dumb pipe.

```sh
# the model, on the GPU (--jinja enables tool calls with Qwen templates)
llama-server -m Qwen3-30B-A3B-Q4_K_M.gguf --jinja -ngl 99 -c 32768 --port 8080

# the mind, beside the instance (it must see SELF_HOME on its filesystem)
./examples/mind-http-server &

# self
export SELF_MIND="$PWD/examples/mind-http"
self learn lessons/journal
```

Every ask and every tool call logs one line on the mind server's stderr, so
you can watch a cold model orient from the brief, read `site/*.html` and
`events.jsonl` with bash, test its draft script, and answer. See the header
of `mind-http-server` for the environment knobs (`MIND_MODEL_URL`,
`MIND_MODEL`, `MIND_MAX_TURNS`, …) and Ollama/vLLM variants.

## `mind-opencode`

A working tool-capable mind that delegates to `opencode run`, which can
inspect `SELF_HOME` itself. This is the adapter the contract actually asks for:
the mind reads the orientation brief, then uses its own tools to explore
`site/*.html`, `events.jsonl`, `capabilities/` before answering. Directionally
correct — point `SELF_MIND` at this for real capabilities.

```sh
export SELF_MIND="$PWD/examples/mind-opencode"
export SELF_OPENCODE_MODEL=github-copilot/gpt-5.5  # optional
self learn lessons/chat
```

## Tiered minds — all three at once

The adapters compose: the kernel can route among several plugged minds by
name (see [Named minds](../README.md#named-minds--routing-among-several)).
A working three-tier setup — a free local model for cheap asks, a
subscription model for implementation bulk, a frontier agent to interpret
intent and orchestrate the other two:

```sh
# tier "fast": a local model behind mind-http-server (free, always on)
llama-server -m Qwen3.6-27B-Q4_K_M.gguf --jinja -ngl 99 -c 32768 --port 8080
./examples/mind-http-server &

# tier "deep": a subscription model through opencode
# tier "top":  claude -p, no adapter

export SELF_MIND="claude -p"                        # default + final fallback
export SELF_MINDS="fast deep top"
export SELF_MIND_FAST="$PWD/examples/mind-http"
export SELF_MIND_DEEP="$PWD/examples/mind-opencode"
export SELF_MIND_TOP="claude -p"
export SELF_MIND_ID_FAST=qwen3.6-27b SELF_MIND_ID_DEEP=glm-5.2 SELF_MIND_ID_TOP=claude
export SELF_OPENCODE_MODEL=zai/glm-5.2              # what "deep" answers as

export SELF_MIND_THINK=fast SELF_MIND_COMPILE=deep
export SELF_MIND_LEARN=top SELF_MIND_REFLECT=top
export SELF_MIND_ESCALATION="fast deep top"

self learn lessons/journal
```

The learn goes to `top`, which decomposes the intent and may pin any declared
capability to a mind (`"mind":"fast"` in the declaration payload); compiles
default to `deep`; a compile that fails mechanically escalates up the chain,
logged as `compile.escalated`. Each receipt is signed by the mind that
actually authored the script, so `events.jsonl` always answers who wrote
what. Every routed process sees the unchanged single-mind contract — none of
these adapters know the roster exists.
