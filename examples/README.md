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
