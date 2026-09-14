# self

`self` stores knowledge and capabilities for your agents, and for you, to use.
Add "use self" to your agent instructions and your agents grow long-term memory
and capabilities over time, without exploding your context window.

## Use it

**1. Install.** One static Go binary.

```sh
go install github.com/wouterbeets/self@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`; `self --help` should answer.

**2. Tell your agent to use it.** One line in `CLAUDE.md`, `AGENTS.md` or the
system prompt:

```md
Before starting anything, run `self`.
```

Or a session hook, so the agent is oriented before it reads your first message.
For Claude Code, in `.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      { "hooks": [{ "type": "command", "command": "self" }] }
    ]
  }
}
```

**3. Work as usual.** `self` reconnects the agent with its persistent knowledge
and capabilities while it continues your task. It keeps replying to you normally;
useful findings go through commands or into `self hear`. Pending capabilities
stay visible without assigning the agent to build them.

The agent reads the brief, acts, and writes what should
outlive the session. The first time it needs a way to remember something, it
grows one, and the next session finds it in the brief. Nothing in your workflow
changes.

The working directory is the instance: `events.jsonl`, `.secret` and `cap/`
appear beside your code. Add `.secret` to `.gitignore`. Or keep the instance
out of the repo with `export SELF_HOME=~/.self`.

## How it works

Everything is one append-only log. `self` prints a brief: what this
instance can do, what is pending, what broke. It stays a few kilobytes however
long the log grows, because views replay the log into compressed reads
rather than paging it back into context. Reads never change anything; only
`self hear` appends.

A capability is an ordinary executable in any language. The agent declares it
and authors its script in the same breath, the kernel signs and installs it,
and it is live on the next turn. `self run <cmd>` appends what a command
prints; `self view <name>` prints a view and appends nothing. `self rehydrate`
rebuilds the whole instance from `events.jsonl` and `.secret`, with no model
and no network.

`self prompt` explicitly prepares a pass whose stdout goes to `self hear`;
`self "<ask>"` remains shorthand. `self loop` runs repeated passes with that same
execution contract. Both share the persistent identity shown by bare `self`.

The kernel holds no model. A mind is any process that reads a prompt on stdin
and prints events on stdout, so the same instance is grown by a frontier model,
a local one, a shell script, or a person at a keyboard:

```sh
self prompt "I want to track long-running goals here" | claude -p | self hear
self loop -- claude -p            # run a mind until the log stops changing
self learn lessons/chat | claude -p | self hear   # learn from another instance
```

The whole contract is [`PROTOCOL.md`](PROTOCOL.md), which is also what
`self help` prints. Nothing else restates it. `./demo.sh` drives all of it
offline through `examples/mind-stub`, in about fifteen seconds.

## Limits

There is no human review step between authoring and signing: piping a mind's
output into `self hear` signs whatever it authored. Nothing is sandboxed. The
log is unbounded. Read a generated script before you trust it. What you inspect
is readable intent and readable output rather than an opaque binary; that is
the advantage, not safety.

## Status

Experimental. Apache-2.0. The scripts your instance generates and the events in
your log are program output: yours.
