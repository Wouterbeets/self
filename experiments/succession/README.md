# Succession: a child built from a parent's state, not its history

A self that grows unwieldy can hand itself on. This experiment builds the
successor of a live instance and checks what survives. It never touches the
parent: it reads `events.jsonl` and the installed capability scripts, and writes
a fresh child elsewhere.

```sh
experiments/succession/run.sh [parent SELF_HOME] [work dir]
```

`select.py` turns the parent log into state accounts:

- **howto**: every live how-to at its latest revision.
- **metis**: every live card with the confirmations since its latest note.
- **goals**: open goals with their full history, their parents, and the terminal
  status of every goal they depend on, so the child does not read them as
  blocked.
- **open intents**: re-declared by the child as its own outcomes.

The child learns the three accounts (`self learn --into succession/<name>`),
declares the open intents, and re-authors every live capability under its own
key from the parent's script bytes. Nothing runnable travels in an account, so
a successor that wants a capability must adopt it; here the runner does that
for all of them.

## First run, 2026-10-03

Parent: 10,150 events, 18.6 MB. Child: 1,366 events, 2.0 MB, of which about
1 MB is capability scripts.

| view | lines differing after normalising seq and dates |
|---|---|
| howto | 0 |
| tree | 0 |
| employees | 0 |
| metis | 2: the presence line; `presence.*` is not carried |
| next | 14: goals learned through the `goals` account are tagged `[goals]` instead of `[here]` |

What stays behind is telemetry: `prs`, `reviews`, `agents`, `recent`,
`checkpoint` and `evidence` come up near empty in the child. That is the
intended cut. Observations are re-observed (`observe-prs`, `observe-agents`)
or live in subsystems. Peers and identity are per-body configuration and need
a deliberate re-declaration.

`self give <prefix>` exports full history: 1.0 MB of how-to revisions for
0.3 MB of live how-tos, 5.7 MB of goal events for 0.9 MB of open goals. A
succession wants selection by state, which is what `select.py` does outside
the kernel.

The first dry run, without dependency ends, read six goals as blocked whose
dependencies had closed. Carrying the terminal status of every depended-on
goal fixed `tree` and `employees` exactly.
