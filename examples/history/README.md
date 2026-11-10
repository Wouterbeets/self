# History: search every earlier agent conversation, with no model calls

```
$ self-history search Customer Team fallback
5645  2026-10-21 15:49  cc:15eb6a5a-abe  ~/mym  tool: …for the [Customer] [Team] [fallback] the card links to the support page…
5610  2026-10-21 15:33  cc:15eb6a5a-abe  ~/mym/frontends/business/src  tool: …Tom suggested a [customer]-[team] or support [fallback]…
$ self-history show 5645 -C 2      # that turn in full, two turns either side
$ self-history sessions --here     # recent sessions in this directory and below
```

Claude Code (`~/.claude/projects/**/*.jsonl`, subagents included), pi
(`~/.pi/agent/sessions`), Codex (`~/.codex/sessions/**/rollout-*.jsonl`, source
`cx`) and opencode (`opencode.db`) already keep every turn. From a Codex
rollout the user and assistant messages, tool calls and tool outputs are
indexed; injected context blocks, reasoning and the duplicate `event_msg`
copies are not.
`self-history` reads them into an SQLite FTS5 index at
`~/.cache/self-history/index.db` and searches it. Every call first catches the
index up: a JSONL file is read from where the last pass stopped, opencode from
the last `time_updated`. The index is derived; delete it and it rebuilds.

On this laptop (26/10): 184k messages, 1.3 GB of transcripts, a 515 MB index
built in 51 s; a catch-up pass with nothing new takes 0.17 s.

## What it does not do, and why

This replaced `examples/recall` (since removed), which hooked every turn into a self child
instance and kept a summary tree compacted by `claude -p --model haiku`. That
spent a whole five-hour subscription window in two minutes: up to eight
concurrent Haiku processes, each given the entire rendered memory, retried up
to five times to fit a 200-byte line. The summaries also lost the details
people later asked for. So:

- **No model calls.** Nothing summarizes. The agent that asked the question
  reads the hits and draws its own conclusions.
- **No hooks, nothing recorded.** The harnesses already write transcripts, so
  `self-history` reads them. Nothing runs on the hot path of a turn.
- **Pull, not push.** Nothing is injected but the three-line pointer in the
  self brief (`view.sh`). The agent searches when it has a question.
- **Exact text.** Identifiers (`biz-qa2110-am-sync`, PR numbers, error
  strings) match as phrases. `show` reads the message back from the source
  transcript in full; only tool output in the index is clipped to its head and
  tail.

A hit is what was said at the time, by whoever said it: an agent claiming
"deployed" is a claim. Current decisions and work state live in self (goals,
howtos); `self-history` is for finding why and when.

Calls to `self-history` itself, and their results, are not indexed, or every search
would find the earlier searches. The caller's own session is left out of
results (`CLAUDE_CODE_SESSION_ID`, or set `SELF_HISTORY_SESSION`).

## Install

```sh
ln -s "$PWD/examples/history/self-history" ~/go/bin/   # any directory on PATH
self-history index                                          # first build, about a minute

jq -nc '{name:"view.declared",payload:{name:"history",summary:"Search earlier sessions: self-history search <words>",description:"usage: self view history — no arguments; prints how to search earlier Claude Code, opencode and pi sessions with self-history (examples/history). Not `history`: zsh owns that name. A brief section, so every arrival sees it.",consumes:["history.never"],brief:{order:20,bytes:600}}}' | self hear
jq -nc --rawfile v examples/history/view.sh '{name:"script.authored",payload:{type:"view",name:"history",script:$v}}' | self hear
```

The view consumes an event name nothing writes. `consumes: []` would mean
every event, and the whole log would be piped into a view that prints static
text.

Paths are overridable with `SELF_HISTORY_DB`, `SELF_HISTORY_CLAUDE`,
`SELF_HISTORY_PI` and `SELF_HISTORY_OPENCODE`.

## Test

```sh
sh examples/history/test.sh
```

Fixture transcripts for all three harnesses in a scratch directory; it reads
nothing real.
