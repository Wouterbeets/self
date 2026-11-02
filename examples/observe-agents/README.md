# observe-agents: witness hc terminals and worktrees for every goal dispatch

`self run observe-agents` reads `hc ls --all --json` (herdr-core) instead of the
retired `herdr agent list`. A dispatch's hc terminal name is its agent name;
`status` is the terminal's agent status (`working`, `idle`, `done`, `blocked`),
its state when not live (`finished`), or `not in hc`; `pane` is the hc address.
Worktree state (dirty, unpushed, branch) is unchanged. Install: author
`command.py` as `command/observe-agents` (declared at seq 2517).

Verified 02/11/2026: seq 14881 agent.observed, ship-04-live seen `done` at t1r;
`self view reclaim` replays it.
