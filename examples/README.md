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
export SELF_BRAIN="claude -p"
self learn lessons/chat
```

Stateless on purpose: each ask starts cold and orients from the brief and the
rendered state. Resist wiring the harness's session store in as memory
(`--continue`): it accumulates brain state outside the log — unreplayable,
unauditable, absent from every account. An instance's memory is its events and
projections; a brain that needs to remember something should write it to the
log through a capability, where `rehydrate` can reach it.

## `brain-stub`

The deterministic offline brain — no LLM, no network, Python standard library
only. This is what the tests and `demo.sh` plug in: `think`/`reflect` answer
with fixed prose, `learn` reads the latest `intent.declared` from the
instance's log and declares one command and one projection named in the
intent's backticks, and `compile` answers with a trivial script honoring the
pipe contract. It proves the machinery, not the intelligence — and it goes
through the same seam as every real brain, because the kernel carries no
brain of its own, not even a fake one.

```sh
export SELF_BRAIN="$PWD/examples/brain-stub"
./demo.sh          # or: go test ./...
```

## `brain-http` + `brain-http-server` — a local model as the mind

A two-piece brain for running a local model (e.g. Qwen3 30B/32B on one RTX
4090) as the mind, without adopting a whole coding agent. This is the truer
test of the contract: a bare model, a bash tool, and the instance's rendered
state — nothing else.

- **`brain-http-server`** is the dedicated, long-running piece: one endpoint
  (`POST /run`) in front of any OpenAI-compatible model server (llama.cpp's
  `llama-server`, Ollama, vLLM), running the tool loop. The model gets two
  tools: `bash`, to explore and test inside `SELF_HOME` (this is what makes
  it a *complete* brain per the contract), and `run`, to fire an installed
  capability through the serving instance's `/run/<command>` route when an
  ask is an action rather than authorship. Python standard library only.
- **`brain-http`** is the shim `SELF_BRAIN` points at: self spawns it per
  ask, it POSTs `{ask, prompt, brief, home}` to the server and prints the
  body back onto the kernel's pipe. The seam self sees stays a dumb pipe.

```sh
# the model, on the GPU (--jinja enables tool calls with Qwen templates)
llama-server -m Qwen3-30B-A3B-Q4_K_M.gguf --jinja -ngl 99 -c 32768 --port 8080

# the brain, beside the instance (it must see SELF_HOME on its filesystem)
./examples/brain-http-server &

# self
export SELF_BRAIN="$PWD/examples/brain-http"
self learn lessons/journal
```

Every ask and every tool call logs one line on the brain server's stderr, so
you can watch a cold model orient from the brief, read `site/*.html` and
`events.jsonl` with bash, test its draft script, and answer. See the header
of `brain-http-server` for the environment knobs (`BRAIN_MODEL_URL`,
`BRAIN_MODEL`, `BRAIN_MAX_TURNS`, …) and Ollama/vLLM variants.

## `brain-opencode`

A working tool-capable brain that delegates to `opencode run`, which can
inspect `SELF_HOME` itself. This is the adapter the contract actually asks for:
the brain reads the orientation brief, then uses its own tools to explore
`site/*.html`, `events.jsonl`, `capabilities/` before answering. Directionally
correct — point `SELF_BRAIN` at this for real capabilities.

```sh
export SELF_BRAIN="$PWD/examples/brain-opencode"
export SELF_OPENCODE_MODEL=github-copilot/gpt-5.5  # optional
self learn lessons/chat
```
