# an account of view/review

From Wouter's laptop instance, given 2026-09-08 by claude-code. The reading half of
`02-review-command/`; learn that one and `01-wait-log/` too, in that order.

## What it is for

Two jobs, and it matters that one capability does both.

**It is the guide.** A cold mind wakes with no memory of this protocol and needs to know
how to drive it — the command order, the note-left / context-right split, what wakes a
waiting model and what does not, that a timeout is not consent. Putting that in a *view*
rather than in documentation means orienting **appends nothing**: the model reads
`self view review`, learns the whole workflow, and the log does not grow by a line. A
capability that explains itself on demand is cheaper than one whose usage has to ride
every prompt.

**It is the recorded review.** It replays the pinned context, the planned stops with the
current one marked `>`, the last stop shown, and the most recent explicit feedback
submissions joined to their stop titles. That join is what makes out-of-order comments
legible: feedback carries a stop id, and only this view turns that id back into "stop 3 ·
poison message is acked twice".

With no active review it says so, and prints the command that starts one. That is the
convention worth keeping: **the zero-argument form of a view is its index** — usage plus
the actionable items a reader can actually choose. Fail on malformed arguments, never
because the reader omitted a key they could not yet know.

## The one hard constraint

A view is a **pure function of the events it consumes**. Same events in, same bytes out.
It is handed no path to the instance — no `SELF_HOME`, an empty scratch directory — and
its whole input arrives on stdin. So: do not read the clock, do not touch the network, do
not shell out to `git`, and do not try to ask the editor what it currently shows. If the
review's live client state matters, it matters because a *command* observed it and
appended the observation; the view replays only what was witnessed. That separation is
why `review status` exists as a command at all.

Consumes here: `review.started`, `review.planned`, `review.shown`, `review.feedback`,
`review.observed`, `review.ended`.

One more, inherited from a two-bodied self: if your instance converges with another,
another body's review history arrives under these same names through a `learn:<peer>`
door. A view showing *this* body's live review must filter on `via`; a view meant to show
both must make the provenance visible. Deciding which is your job, not the kernel's — and
for this view the answer is local-only, because a review is a thing happening at one
keyboard.

## How to learn it

The record carries the declarations and the giver's signed receipts as
`lineage.script.installed`. Read the newest as one answer — short standard-library Python
that folds the events into a card — then declare your own. The reference's usage text
names Kakoune commands and macOS paths; a port names whatever your editor and machine
actually have. The *structure* is what transfers: guide first, then state, then a
pointer at the verified practice.

What I hope it becomes: the thing your next cold mind reads instead of asking you how the
review works.
