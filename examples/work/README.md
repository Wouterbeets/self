# General work, three small instances

These fixtures exercise the real kernel and loop with a deterministic Python
mind. They demonstrate the wire and lifecycle, not LLM reasoning quality.
All source observations are fictional fixtures. No external services are called.

From the repository root, build a temporary binary and choose a fixture:

```sh
go build -o /tmp/self-work .
export SELF_HOME="$(mktemp -d)"
/tmp/self-work hear < examples/work/printer.jsonl
/tmp/self-work brief
/tmp/self-work loop -- python3 "$PWD/examples/work/mind.py"
/tmp/self-work brief work/enclosure-fit
/tmp/self-work view log --all
```

Use a fresh `SELF_HOME` for `household.jsonl` or `author.jsonl`. Their work names
are `thursday-dinner` and `sum-filament` respectively. Pointing the loop at a
real tool-capable mind instead uses the same declarations and observations.

| Instance | Open outcome | Result |
|---|---|---|
| Printer hobbyist | Does the spool cover the sliced model plus 15%? | 160 g available; 143.75 g required; an assessment event |
| Household | Dinner within 20 minutes, plus missing groceries | 15-minute tomato pasta; tomatoes on the prepared shopping list |
| Script author | Standalone gram-summing script with verification | Script artifact, preserved source, and verification evidence |

None requires a goal hierarchy or any installed command/view. The family
example reads calendar and pantry observations; planning dinner neither edits
the calendar nor buys groceries. Domain records retain their own vocabulary.
The author example writes an ordinary file; `rehydrate` does not restore it,
though its source remains in the artifact event for a mind to recover.

Remove the spool measurement and the demo mind produces nothing: the loop
settles with the work still open. It needs new evidence, not repeated passes.

To share the unresolved printer work before running the loop:

```sh
/tmp/self-work give work. /tmp/printer-work-account
```

Edit the generated `intent.md` to explain the intended transfer. `work.` selects
only the lifecycle; supporting spool/model observations must be included in a
curated account when the recipient needs them. Learning imports lineage, not
an assignment. The receiver adopts relevant work locally with its own criteria.
