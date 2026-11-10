# self

**Install it, then tell your agent to run `self`. That's it.**

Every agent on your machine (Claude Code, opencode, pi, Codex, a local model,
a shell script) now shares one persistent self: memory that survives the
session, crosses harnesses and model providers, and grows its own capabilities.
Insight compounds instead of evaporating when the context window closes.

## It holds no domain

self knows nothing about code, or printers, or anything else. The kernel is an
append-only log, a way to read it, and a way to sign small scripts into it.
Everything specific is grown by your own agents, in your own instance, from
what you ask for:

```sh
intent() { jq -nc --arg n "$1" --arg s "$2" --arg d "$3" \
  '{name:"intent.declared",payload:{name:$n,summary:$s,description:$d}}' | self hear; }

intent spool-fit  "Can the blue spool finish the enclosure?" \
  "Compare remaining filament with the sliced model, with a margin. Record what you measured."
intent club-dues  "Who has not paid this season's subscription?" \
  "Members, what each owes and paid, and a list I can send reminders from."
intent prep-list  "What do we prep before Friday's service?" \
  "From the bookings and the menu: quantities per dish, and what must be ordered by Wednesday."

self loop -- claude -p
```

Three people, three instances that end up with nothing in common: a filament
ledger, a membership roll, a prep sheet. None of it is in this repository, and
none of it should be. What your instance becomes is shaped by you.

## Quick start

```sh
go install github.com/wouterbeets/self@latest   # one static binary
```

Then say this to your agent, in any harness:

```
Run `self` and set it up.
```

That is the whole install. There is no environment variable to export and no
shell to restart: your self lives in `~/.self`. If the agent cannot find
`self`, Go's bin directory is not on your `PATH` yet; say `~/go/bin/self`
instead, and the agent will offer to fix that too.

On an empty log the brief carries a short first-run section. The agent reads
it and offers to make self part of every session: a line in the instruction
file your harness reads, and a session-start hook where the harness has one.
It asks before it changes your files, and the section is gone once the log
holds its first event.

From then on, work as usual. The agent reads a short brief, does your task, and
records what the next mind should know. The first time it needs a new way to
remember or do something, it writes one, and every later session, in any agent,
finds it in the brief.

Prefer to do it by hand? It is one line in `AGENTS.md`, `CLAUDE.md`, or your
system prompt:

```md
Before starting anything, run `self`.
```

and, to get the brief before the first message, a session hook. For Claude
Code, in `.claude/settings.json`:

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

**Solo developer, several harnesses.** Same line in every `AGENTS.md`, and
they all share `~/.self`. Claude Code finds a flaky test's root cause and records it;
tomorrow opencode reads it in the brief before touching the same file.

**One human, many parallel agents.** Agents share one log and grow the
coordination they need: a `recent` view of who touched what, a `howto` store of
verified procedures, a command that pings you when one is blocked. None of it
is built in; every later agent finds it in the brief.

**Hobbyist, household, small business.** Declare what you want to become
possible, as above, and let a mind work on it. It closes the intent with
evidence, or leaves it open with what it still needs from you.

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

Opt in to plain files with `mkdir ~/.self/view ~/.self/bin`. After each
write, `view/<name>` holds what `self view <name>` prints, for `cat`, `grep -r`
and editors; `bin/<name>` runs `self run <name>`, so with
`PATH=~/.self/bin:$PATH` capabilities are ordinary commands.

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

A lesson is an intent a mind turns into capabilities: a page saying what should
become possible and what must hold, and no code. Your model writes the scripts,
so a better model grows a better instance from the same page. Nine ship here.
Six assume nothing about what you do:

- [`lessons/journal`](lessons/journal/intent.md): the smallest possible set,
  one command and one view. `./demo.sh` grows it offline.
- [`lessons/memory`](lessons/memory/intent.md): durable memory for a stateless
  mind: `self run remember …`, `self view memory`.
- [`lessons/goals`](lessons/goals/intent.md): outcomes that outlive the session
  that wanted them, closed with evidence.
- [`lessons/howto`](lessons/howto/intent.md): procedures that worked, kept with
  their proof.
- [`lessons/recall`](lessons/recall/intent.md): search what was said in earlier
  sessions, across every harness.
- [`lessons/pulse`](lessons/pulse/intent.md): notice when your own automation
  loops or goes quiet.

Three come from one software team's instance, as an example of a domain layer
and not as a recommendation:

- [`lessons/workers`](lessons/workers/intent.md): start parallel workers, see
  them, clean up after them.
- [`lessons/practices`](lessons/practices/intent.md): say how work is done
  here, and enforce it where work leaves.
- [`lessons/ship`](lessons/ship/intent.md): follow a change from published to
  running, and say who it waits on.

There are no capability scripts in this repository beyond the stub the demo
uses. The ones an instance grows live in its log, signed under its own key.

`self give <selector> <dir>` writes your own instance's knowledge or
capabilities as an account another instance can `self learn`.

The full contract is [PROTOCOL.md](PROTOCOL.md), which `self help` prints.
`./demo.sh` runs everything offline in about fifteen seconds.

## Setup notes

- The instance is `SELF_HOME` if you set it, else the working directory if it
  already holds `events.jsonl`, else `~/.self`. `SELF_HOME=.` starts an
  instance beside your code.
- **Never commit `.secret`**; it signs your capabilities.
- `self completion zsh|bash|fish` prints shell completion.

## Optional adapters

- `self-serve` / `self-browse`: a local browser view. GET reads views;
  commands require POST.
- [`examples/mind-claude`](examples/mind-claude),
  [`examples/mind-opencode`](examples/mind-opencode): wrap a harness as a mind
  with a live trace on stderr: `self loop -- examples/mind-claude`.
- [`examples/mind-stub`](examples/mind-stub): a mind with no model, for the
  offline demo and for tests.

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
