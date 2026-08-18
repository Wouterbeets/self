# examples

Mind adapters that are **not part of the kernel**. The kernel holds no model
and no network client; a mind is a process you supply through `SELF_MIND`
(see [the mind](../README.md#the-mind)). These adapters illustrate the
contract's wire shape — copy one, point `SELF_MIND` at it, adapt it to taste.

**The contract the kernel enforces, in one sentence:** the kernel prints an
ask (and keeps the brief at `site/brief.md`); the mind MUST inspect
`SELF_HOME` itself — `site/*.html`, `events.jsonl`, `capabilities/` — with
its own tools. A process without that exploration ability is not a complete
mind. On the default pipe, the mind is a **stdin-filter**: ask on stdin,
events on stdout.

## `mind-claude` / `mind-grok` — argv minds as filters

`claude -p` and `grok --single` take the prompt as an argument, not stdin.
These two-line adapters are the missing filter:

```sh
export SELF_MIND="$PWD/examples/mind-claude"   # or mind-grok
echo hello | self | $SELF_MIND | self
# or, on a tty: just `self`
```

Each reads the ask on stdin, `cd`s to `$SELF_HOME` (so a kernel-repo
`CLAUDE.md` is not loaded as context), and execs the CLI. `mind-claude`
passes `--dangerously-skip-permissions` because a non-interactive explore
will not read the instance without it.

Stateless on purpose: each ask starts cold and orients from the brief and the
rendered state. Resist wiring the harness's session store in as memory
(`--continue`): it accumulates mind state outside the log — unreplayable,
unauditable, absent from every account.

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

A filter (and still a spawn adapter) around `opencode run`. OpenCode prints
its own JSON event stream; this unwraps the assistant text. On the pipe,
stdin is the ask and there is no extra argv. For a non-interactive explore
you almost always need `SELF_OPENCODE_AUTO=1`.

```sh
export SELF_MIND="$PWD/examples/mind-opencode"
export SELF_OPENCODE_AUTO=1
echo hello | self | $SELF_MIND | self
```
