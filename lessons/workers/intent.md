# workers — start parallel workers, see them, clean up after them

## purpose

One person with many agents needs three things the agents will not do for
themselves: start a worker on a goal in its own copy of the code, know at a
glance which workers are busy, waiting on a person or finished, and remove
what a finished worker leaves behind without destroying work that exists
nowhere else.

## surface

- `self run dispatch <goal> <name> <program> <repo> [task…]` starts one worker
  on a goal, in its own checkout on its own branch, and appends what was asked
  and what started (or why it failed).
- `self run observe-agents` witnesses every worker this instance started: is
  its terminal working, waiting or gone, and is its checkout clean and
  published.
- `self view agents` renders that observation by state.
- `self view reclaim` shows what cleanup would remove and why the rest is held.
- `self run reclaim [--dry-run] [name…]` removes finished workers.

## constraints

- The worker's first message says who it is, which goal it serves, where to
  read the context and how to report. The exact text delivered is recorded.
- A worker is only started for a goal that is open, not on hold and not
  blocked, and a dispatch is only recorded as started once the worker was seen
  working. A worker that came up idle did not receive its task.
- Observing is a command that records evidence; the views only replay it. A
  view says how old its observation is and never claims more.
- Idle is not finished. A worker dispatched minutes ago with a clean checkout
  has not started; one whose goal is closed has nothing left to do.
- Cleanup never removes a checkout with uncommitted changes, or commits that
  exist on no remote. A branch whose changes are already in the default branch
  under other commit ids (a squash merge) counts as published.
- Cleanup ends only the terminal it started. Anyone else's session inside a
  worker's checkout blocks the cleanup; it is not closed and the folder is not
  removed from under it.
- Without arguments cleanup takes only the safe set: workers whose goal is
  closed. Anything else is named explicitly, and there is a rehearsal that
  runs every check and removes nothing.
- Workers started by another instance are that instance's to clean up.
