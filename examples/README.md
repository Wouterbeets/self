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
self grow seeds/chat
```

Stateless on purpose: each ask starts cold and orients from the brief and the
rendered state. Resist wiring the harness's session store in as memory
(`--continue`): it accumulates brain state outside the log — unreplayable,
unauditable, absent from every seed. An instance's memory is its events and
projections; a brain that needs to remember something should write it to the
log through a capability, where `rehydrate` can reach it.

## `brain-stub`

The deterministic offline brain — no LLM, no network, Python standard library
only. This is what the tests and `demo.sh` plug in: `think`/`heartbeat` answer
with fixed prose, `grow` reads the latest `intent.declared` from the
instance's log and declares one command and one projection named in the
intent's backticks, and `compile` answers with a trivial script honoring the
pipe contract. It proves the machinery, not the intelligence — and it goes
through the same seam as every real brain, because the kernel carries no
brain of its own, not even a fake one.

```sh
export SELF_BRAIN="$PWD/examples/brain-stub"
./demo.sh          # or: go test ./...
```

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
