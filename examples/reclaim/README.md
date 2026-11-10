# reclaim: remove finished goal workers (hc)

`self run reclaim [--dry-run] [--force] [--fresh-minutes N] [agent...]` ends the
terminal a dispatch started, removes its worktree and local branch, and
tombstones the dispatch. `self view reclaim` is the same plan replayed from the
last `self run observe-agents`, appending nothing. Usage and every guard are in
the docstrings, which `install.sh` installs as the descriptions:
`self brief reclaim`.

Install on every body, from the repo:

```sh
SELF_HOME=~/.self sh examples/reclaim/install.sh
```

It declares and installs four capabilities and prints their hashes, which must
match across bodies: `observe-agents` and the `agents` view from
`examples/observe-agents`, `reclaim` and the `reclaim` view from here.

## What one version had to settle

Two bodies had grown their own variant. This one keeps what each got right.

- **Only the terminal dispatch started is ever ended.** It is the one named
  after the agent that is the address `agent.started` recorded or sits in the
  worktree. Any other live terminal in the worktree blocks the reclaim: it is
  not ended and the folder is not removed from under it.
- **A terminal belongs to a worktree by folder**, launch or current, so a
  session someone moved into the worktree is seen.
- **Kept terminals close.** `hc keep --off` before `hc kill`, or hc respawns a
  shell and `hc rm` never succeeds.
- **Squash merges are not unpushed work.** When the remote branch is gone,
  commits count as safe if the remote default branch already has the same
  patches (`git cherry`) or merging changes nothing (`git merge-tree`). The
  removed HEAD is recorded on `agent.reclaimed`.
- **Other bodies' dispatches are skipped**, not tombstoned as "already gone":
  their paths do not exist here, and a tombstone would sync back and hide a
  worktree that still exists there.
- **`--dry-run`** runs every live guard and removes nothing.
- **The view reads no clock.** It prints when the observation was witnessed
  instead of its age.

Verified 10/11/2026 on Linux against a live roster of 28 terminals and 55
dispatches: the default set matched between `self view reclaim` and
`self run reclaim --dry-run`; a dispatch with a second session in its worktree
was held as "not ours to close"; six squash-merged branches were recognised.
