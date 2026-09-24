# self

**Put `use self` in your `AGENTS.md`. That's it.**

Every agent on your machine (Claude Code, opencode, pi, Codex, a local model,
a shell script) now shares one persistent self: memory that survives the
session, crosses harnesses and model providers, and grows its own capabilities.
Insight compounds instead of evaporating when the context window closes.

## Quick start

```sh
go install github.com/wouterbeets/self@latest   # one static binary
export SELF_HOME=~/.self                         # one self for every project
```

Then add one line to `AGENTS.md`, `CLAUDE.md`, or your system prompt:

```md
Before starting anything, run `self`.
```

Work as usual. The agent reads a short brief, does your task, and records what
the next mind should know. The first time it needs a new way to remember or do
something, it writes one, and every later session, in any agent, finds it in
the brief.

Want it before the first message? A Claude Code session hook in
`.claude/settings.json`:

```json
{ "hooks": { "SessionStart": [{ "hooks": [{ "type": "command", "command": "self" }] }] } }
```

## What you get

- **Cross-session memory.** Knowledge, goals and verified how-tos outlive the
  conversation that found them.
- **Cross-agent, cross-provider.** One log, read by any harness and any model.
  Claude can pick up where opencode left off.
- **Capabilities that grow.** Agents author small scripts (views to read,
  commands to act). The kernel signs and installs them; they're live next turn.
- **Bounded context.** The brief grows with what the instance can do, not
  with how long the log is. Views compress history instead of paging it back in.
- **Many agents at once.** Every write is one locked batch, so parallel agents
  can share a log; `self hear --after <head>` commits only if nothing changed.
- **Rebuildable.** `events.jsonl` plus `.secret` is the whole self. Hand-edited
  scripts are ignored; the signed bytes from the log run instead.

## Profiles

**Solo developer, several harnesses.** Same `SELF_HOME`, same line in every
`AGENTS.md`. Claude Code finds a flaky test's root cause and records it;
tomorrow opencode reads it in the brief before touching the same file.

**One human, many parallel agents.** Agents share one log and grow the
coordination they need: a `recent` view of who touched what, a `howto` store of
verified procedures, a command that pings you when one is blocked. None of it
is built in; every later agent finds it in the brief.

**Hobbyist or household.** Declare what you want to become possible and let a
mind work on it:

```sh
echo '{"name":"intent.declared","payload":{"name":"spool-fit","summary":"Can the blue spool finish the enclosure?","description":"Compare remaining filament with the sliced model, with a margin. If measurements are missing, record what is needed."}}' | self hear
self loop -- claude -p
```

The mind closes the intent with evidence, or leaves it open with what it needs.

**Unattended.** Any process that reads a prompt and prints events is a mind,
so a cron job with a local model grows the same instance:

```sh
self prompt "summarize today's notes into memories" | ollama run qwen3 | self hear
```

## How it works

Everything is one append-only log, `events.jsonl`. Reads (`self`, `view`,
`brief`, `prompt`) never change it; `hear`, `run` and `learn` append.

```sh
self                      # reconnect: identity, capabilities, open intents
self view <name>          # a compressed read; appends nothing
self run <command>        # act; what it prints is appended
self brief <name>         # one capability or intent in full
self hear < batch.jsonl   # append events or authored scripts
```

A capability is an ordinary executable in any language. `self rehydrate`
rebuilds all of them from `events.jsonl` and `.secret`, with no model and no
network: the log is the whole self.

The kernel holds no model. A mind is anything that reads a prompt on stdin and
prints events on stdout:

```sh
self prompt "track my long-running goals" | claude -p | self hear
self loop -- claude -p                           # run until the log settles
self learn lessons/memory | claude -p | self hear # grow a lesson's capabilities
```

## Lessons

A lesson is an intent a mind turns into capabilities. Two ship here:

- [`lessons/journal`](lessons/journal/intent.md): the smallest possible set,
  one command and one view. `./demo.sh` grows it offline.
- [`lessons/memory`](lessons/memory/intent.md): durable memory for a stateless
  mind: `self run remember …`, `self view memory`.

`self give <selector> <dir>` writes your own instance's knowledge or
capabilities as an account another instance can `self learn`.

The full contract is [PROTOCOL.md](PROTOCOL.md), which `self help` prints.
`./demo.sh` runs everything offline in about fifteen seconds.

## Setup notes

- Without `SELF_HOME`, the working directory is the instance: `events.jsonl`,
  `.secret` and `cap/` appear beside your code.
- **Never commit `.secret`**; it signs your capabilities.
- `self completion zsh|bash|fish` prints shell completion.

## Optional adapters

- `self-serve` / `self-browse`: a local browser view. GET reads views;
  commands require POST.
- [`examples/mind-claude`](examples/mind-claude),
  [`examples/mind-opencode`](examples/mind-opencode): wrap a harness as a mind
  with a live trace on stderr: `self loop -- examples/mind-claude`.
- [ask](examples/ask/README.md): ranks capabilities and views for a task with a
  local model; falls back to an unscored index.

## Limits

- Piping a mind's output into `self hear` signs whatever it authored. There is
  no review step; hold the output in a file first if you want a pause.
- Anyone who can run `self` against your `SELF_HOME` can install capabilities.
- Nothing is sandboxed and capability scripts are not timed out. They run as you.
- The log is unbounded; the kernel does not compact it.
- Batches are locked, but a command is not a transaction: external effects are
  not rolled back.

Read a generated script before you trust it. What you inspect is readable
intent and readable output, not an opaque binary. That is the advantage, not
safety. Full list: [PROTOCOL.md § Limits](PROTOCOL.md#limits).

## Status

Experimental. Apache-2.0. The scripts your instance generates and the events in
your log are program output: yours.
