# observe-agents: witness hc terminals and worktrees for every goal dispatch

`self run observe-agents` reads `hc ls --all --json` and, for each live
terminal, `hc get` for its current folder. For every unreclaimed dispatch made
on this body it records the status of the dispatch's own terminal, the other
terminals living in its worktree, and the worktree's git state, as one
`agent.observed` snapshot (schema 4, `runtime: hc`). An unchanged snapshot
appends nothing.

`self view agents` (`view.py`) replays that snapshot: workers by state, with
`--by`, `--goal`, `--status`, `--names`, `--tsv`. `self view reclaim` and
`self run reclaim` (in `examples/reclaim`) read the same rows.

Install with `examples/reclaim/install.sh`, which installs all four together.
States and row fields are in the docstrings: `self brief observe-agents`,
`self brief agents`.

Verified 10/11/2026 on Linux: 55 dispatches and 28 live terminals observed in
under five seconds; two worktrees reported a second session (`others`).
