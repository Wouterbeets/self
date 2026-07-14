# monitor — orchestrator, verifier, rejector for intents and tasks

## purpose

A built-in critic role for the loop: after a capability is declared/installed or a task from an intent is executed, the monitor checks whether the intent/success criteria was achieved. It can accept, reject (with explanation), or request a retry with a modified intent. This separates maker (mind declaring capabilities or performing work) from checker (monitor), closing the loop with verification as advocated in loop engineering.

It provides the "something that can say no" and automated completion check.

## surface

- `self run monitor.check <intent-ref> <evidence...>` : runs verification for a referenced intent/task. Emits `monitor.verified` or `monitor.rejected` events with reasoning and optional retry suggestions.
- `/monitor` : projection showing recent verifications, pending retries, rejected intents with explanations. It should consume the kernel's own no-vocabulary alongside the monitor's — `monitor.verified`, `monitor.rejected`, `retry.requested`, plus `mind.refused`, `review.rejected`, and `compile.escalated` — so this one page shows every no in the system: who declined what, and why.

- Events: `monitor.verified`, `monitor.rejected`, `retry.requested` (with modified intent).

## constraints / mechanics

- The monitor uses the current state (projections, log) + success criteria from the intent to judge achievement.
- On reject: explain why, suggest modifications, optionally trigger retry by emitting a new declaration or task.
- Idempotent and safe: re-checking the same intent is harmless.
- When judgment needs a model, the command shells out to `self think "<judgment prompt>"` and reads the JSON reply — with named minds plugged, that ask routes through `SELF_MIND_THINK`, so the monitor's judgment rides the cheap tier by default and the operator can re-route it without touching the script. Deterministic rules stay preferable for simple cases.
- The monitor is the post-hoc layer of a three-layer no: a maker can refuse an ask outright (`mind.refused`, kernel), a checker can reject a script before it installs (`SELF_MIND_REVIEW` → `review.rejected`, kernel), and the monitor judges outcomes after they land (this lesson, user space). It reads the first two layers' events as evidence rather than duplicating them.
- Integrate with `reflect`: the monitor can feed into improvement cycles.

## what good looks like

After `self learn some-lesson`, run `self run monitor.check some-lesson` (or auto-trigger on ingest). If tests pass / projection shows expected state / success criteria met → `monitor.verified`. If not → `monitor.rejected "tests failing because..."` + retry suggestion. The loop retries with modified intent until accepted or human-reviewed.

This makes the strange loop robust: proposals are verified before (or after) becoming part of the surface.