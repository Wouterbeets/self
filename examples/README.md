# examples

Minds that are **not part of the kernel**. The kernel holds no model, no
network client, and spawns nothing: a mind is any process the shell pipes
between two invocations of `self`:

```sh
echo "whats going on today?" | self | <mind> | self
```

**The contract, in one sentence:** a mind reads a situated prompt on stdin
(orientation, recent conversation, pending work, the ask, the answer
contract) and prints event JSONL on stdout — and a *complete* mind also
inspects `SELF_HOME` with its own tools (`site/*.html`, `events.jsonl`,
`capabilities/`), because the prompt is a wake-up card, not a context dump.
`self protocol` prints the full wire contract.

## `claude -p` (and every other agent CLI) — no adapter

Any coding-agent CLI that reads stdin and prints to stdout is already a
mind; there is nothing to install here:

```sh
self learn lessons/journal | claude -p | self
echo "add a mood tracker" | self | claude -p | self
self | claude -p | self       # author pending scripts, or one reflection
```

The same is true of `opencode run`, `gemini -p`, and their kin — the seam is
the pipe, and adapters are what the redesign deleted.

Stateless on purpose: each ask starts cold and orients from the prompt and
the rendered state. Resist wiring a harness session store in as memory
(`claude -p --continue`): it accumulates mind state outside the log —
unreplayable, unauditable, absent from every account. An instance's memory is
its events and projections; the conversation itself is in the log
(`self.asked` / `self.replied`), because the ask face records both sides.

## `mind-stub`

The deterministic offline mind — no LLM, no network, Python standard library
only, and a pure filter. This is what the tests and `demo.sh` pipe in: it
answers every pending `DECLARATION` block with a trivial `script.authored`,
answers a learn prompt (`--- INTENT ---`) with one command + one projection
plus their scripts, and answers anything else with a fixed `self.replied`.
It proves the machinery, not the intelligence.

```sh
./demo.sh          # or: go test ./...
echo "hello" | self | examples/mind-stub | self
```

## A local model as the mind

No adapter is needed for that either — a mind is a filter, so a local model
is one `curl` away from the pipe:

```sh
mind() {
  jq -Rn --rawfile p /dev/stdin \
    '{model:"qwen3", messages:[{role:"user", content:$p}]}' \
  | curl -s http://127.0.0.1:8080/v1/chat/completions -d @- \
  | jq -r '.choices[0].message.content'
}
echo "whats going on today?" | self | mind | self
```

A bare completion endpoint is a *partial* mind: it can converse and emit
events, but it cannot explore `SELF_HOME` or test the scripts it authors.
For real capability growth, use a tool-capable agent CLI, or wrap the model
in your own tool loop — that loop is the mind's concern, never the kernel's.
