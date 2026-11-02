# reclaim: remove finished goal workers (hc)

`self run reclaim [--force] [--fresh-minutes N] [agent...]` — same guards as
before (not working/blocked, clean, fully pushed, fresh-dispatch guard), but the
roster is `hc ls --all --json` (herdr-core) instead of the retired `herdr agent list`,
and the worktree is removed with `git worktree remove`. After removal it closes the
worker's hc terminal (`hc kill` then `hc rm`) only when the live terminal with the
agent's name has the address `agent.started` recorded (`via: hc`): terminals that
self did not start are never touched. `agent.reclaimed` gains `terminal`.

Verified 02/11/2026: dispatched claude-haiku-reclaim-probe (14882/14883, t21),
it wrote 14884 and finished; `self run reclaim claude-haiku-reclaim-probe` ->
14887 agent.reclaimed; t21, its worktree and branch were gone afterwards.
