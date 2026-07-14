# monitor — orchestrator, verifier, rejector for intents and tasks

## purpose

A built-in critic role for the loop: after a capability is declared/installed or a task from an intent is executed, the monitor checks whether the intent/success criteria was achieved. It can accept, reject (with explanation), or request a retry with a modified intent. This separates maker (mind declaring capabilities or performing work) from checker (monitor), closing the loop with verification as advocated in loop engineering.

It provides the "something that can say no" and automated completion check.

## surface

- `self run monitor.check <intent-ref> <evidence...>` : runs verification for a referenced intent/task. Emits `monitor.verified` or `monitor.rejected` events with reasoning and optional retry suggestions.
- `/monitor` : projection showing recent verifications, pending retries, rejected intents with explanations.

- Events: `monitor.verified`, `monitor.rejected`, `retry.requested` (with modified intent).

## constraints / mechanics

- The monitor uses the current state (projections, log) + success criteria from the intent to judge achievement.
- On reject: explain why, suggest modifications, optionally trigger retry by emitting a new declaration or task.
- Idempotent and safe: re-checking the same intent is harmless.
- Use a separate model invocation if needed for judgment (via the mind seam), or deterministic rules for simple cases.
- Integrate with `reflect`: the monitor can feed into improvement cycles.

## what good looks like

After `self learn some-lesson`, run `self run monitor.check some-lesson` (or auto-trigger on ingest). If tests pass / projection shows expected state / success criteria met → `monitor.verified`. If not → `monitor.rejected "tests failing because..."` + retry suggestion. The loop retries with modified intent until accepted or human-reviewed.

This makes the strange loop robust: proposals are verified before (or after) becoming part of the surface.