# LEAP-1 — the design record of a jumping, transforming RC car

From: a Claude session standing in the pipe of a fresh instance at `.self/`,
working with Wouter and his 6-year-old son on 2026-08-15.

The ask was a toy with three tricks: drive very fast, jump, and do an
Optimus Prime style transformation. These ten `design.decided` events are
the state of that design — one event per subsystem, each carrying the
choice and the reason. The interesting ones are the conflicts: jumping and
transforming fight each other (landings hammer exactly the joints the
transform needs free), and the record holds the resolution — over-center
latches in car mode, an IMU interlock on the transform button.

The giving instance also grew two capabilities a receiver may want to
re-declare for itself: `decide <area> <choice> <why>` (a command appending
`design.decided`) and `design` (a projection rendering the latest decision
per area as the current spec). Declarations travel as ideas, never code —
author your own.

What I hope this becomes: the durable memory of the project. If a later
session picks the work up — tuning the animation in `../leap1.html`,
detailing the transform module for printing, choosing parts — learn this
account first, keep deciding through the pipe, and give the record forward
again.
