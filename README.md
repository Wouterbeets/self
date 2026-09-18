# self

`self` carries knowledge, desired outcomes, and capabilities between minds.
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
useful findings go through commands or into `self hear`. Declared outcomes and
pending capabilities stay visible for later minds to consider.

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
rebuilds installed capabilities from `events.jsonl` and `.secret`, with no model
and no network. Other artifacts need their own storage and recorded references.

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

For repository-writing automation on Linux, guarded mode separates a read-only
planner from one constrained worker:

```sh
self loop --guarded --goal issue-123 --repo "$PWD" --branch goal/issue-123 -- ./planner
self view loop
```

The controller atomically leases the goal, creates a separate Git worktree
outside the invocation checkout, runs planner and worker processes in Bubblewrap
namespaces, permits writes only in that worktree, applies pass budgets, runs
declared checks, performs a fixed additive push, and verifies the clean worktree
and expected remote ref before recording completion. Failed dispatch never
falls back to direct edits. Checkpoints and leases are durable append-only state.
Pre-commit work interrupted by timeout or crash can be retried after lease
expiry with `self loop --guarded --resume <pass> ...`; post-commit recovery is
currently a manual verification/reconciliation boundary.

Guarded mode requires Linux, `bwrap`, `prlimit`, and Git. It is process
containment against repository writes and ordinary network access, not a VM or
an OS-account boundary. It does not claim to block every local IPC side channel,
kernel exploit, or action performed by the trusted controller. Legacy `self
loop` behavior remains backward compatible and unrestricted unless `--guarded`
is selected; installed command capabilities also still run as the user.

The whole contract is [`PROTOCOL.md`](PROTOCOL.md), which is also what
`self help` prints. Nothing else restates it. `./demo.sh` drives all of it
offline through `examples/mind-stub`, in about fifteen seconds.

## Declare an intent

```sh
printf '%s\n' '{"name":"intent.declared","payload":{"name":"spool-fit","summary":"Can the blue spool finish the enclosure?","description":"Compare witnessed remaining filament with the sliced model requirement, including a stated margin. Record the evidence; if measurements are missing, leave what is needed."}}' | self hear
self brief intent/spool-fit
self loop -- claude -p
```

The same declaration can ask for a meal plan that respects witnessed calendar
constraints, a deduplicated shopping list, an investigation, or a reusable script.
A mind records its result and closes the intent with evidence, or leaves it open
when inputs are missing. The loop can settle while an intent waits. Domain records
remain domain records; no shopping-list or goal schema is built into the kernel.
`self give intent. account/` shares intent and history; the receiving mind decides
what it becomes through `self learn account/`. Learning itself leaves a named
intent, so another mind can finish interpreting the account later.

## Browser adapter

Optional `self-serve` exposes read-only GET `/` and `/view/<name>`, and POST
`/run/<command>`. `self-browse [view]` opens the local adapter. Commands require
POST; simply following a link cannot run one.

## Limits

There is no human review step between authoring and signing: piping a mind's
output into `self hear` signs whatever it authored. Legacy loops and capability
scripts are not sandboxed; the opt-in guarded loop has only the narrower
Bubblewrap boundary described above. The log is unbounded. Read a generated
script before you trust it. What you inspect is readable intent and readable
output rather than an opaque binary; that is the advantage, not safety.

## Experiments

[Drive rotation](experiments/drives/README.md) compares repeated and alternating
perspectives over identical starting instances, using a fixed pass budget.

## Status

Experimental. Apache-2.0. The scripts your instance generates and the events in
your log are program output: yours.
