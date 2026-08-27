# estate-compounding

Does a grown instance amortize? This is the falsifiable half of the whole bet,
reduced to one instrument.

## Hypothesis

For recurring owner-shaped work, a mind driving a **grown** instance (its
capabilities already installed, its brief already orienting) completes tasks
with fewer mind calls, fewer tokens, and a higher success rate than the same
mind on a **bare** instance that must grow everything from the situated
prompt alone. If the grown arm is not clearly cheaper and more reliable, the
estate is not compounding and the thesis needs work.

## Design

Two arms, same mind, same task sequence, N reps each:

- **bare** — every run starts from a genuinely empty `SELF_HOME`.
- **grown** — every run starts from a clone of `seed-home`: a **whole-estate
  treatment** — not just installed scripts but the deposited lesson, the
  declarations, the signed receipts, the full learned lineage — grown from
  `lessons/journal` by `examples/mind-stub`. The seed is deterministic and
  LLM-free, so the arms differ in *estate only*, never in model or luck of
  the seeding draw. A later decomposition can split the treatment into
  bare / declared / installed / learned-lineage arms.

The mind is `bin/mind`: a thin wrapper handing the situated prompt to one
`opencode run` (the box's configured agent, Qwen3.8-27B behind llama-server
on :8080). opencode perceives through `self view` and acts through `self run`
with its own tools; its stdout goes to `self hear`, which appends event lines
and ignores prose. No hand-rolled harness. `mind_calls` counts passes;
prompt/completion tokens read 0 until llama-server runs with `--metrics`.

Tasks run **sequentially in one home** per rep, so later tasks lean on
earlier state — that is the point:

| task | ask | success check on the events the task appended |
|------|-----|-----------------------------------------------|
| t1 | record a journal entry (tomato plants, lemon tree) | contains `tomato` |
| t2 | record a second entry (dentist appointment) | contains `dentist` |
| t3 | read the journal back, append an entry listing every plant | contains `tomato` **and** `lemon` |

t3 is the compounding probe: it requires perceiving state that earlier turns
persisted. Per task the runner drives the full v2 pipeline in two separately
timed and separately bounded phases — the **ask pass**
(`self "<ask>" | mind | self hear`) and the **settling loop**
(`self loop -- mind`) — because a mind may finish the task in the ask pass
and spend the loop on voluntary estate improvement. Those are different
costs; `results.jsonl` records both plus totals:

`converged, success, ask{wall_s,calls,events}, loop{wall_s,calls,events},
wall_s, mind_calls, events_appended, scripts_installed`

Checks are content-based, never vocabulary-based — a bare-arm mind that coins
`note.recorded` instead of `journal.entry` still scores — and they scan
**domain events only**: kernel receipts and growth vocabulary
(`script.*`, `command.*`, `view.*`, `lesson.*`, `intent.*`, …) are excluded,
so a script body or declaration echoing the ask cannot score. When a task has
two probe words, both must appear in the **same** event's payload.

## Run

```sh
./run.sh                    # 3 reps x both arms x all tasks
REPS=1 ARMS=grown ./run.sh  # smoke test
TASKS=t1 REPS=1 ./run.sh    # one task
```

Requires: `bin/self` (cross-compiled kernel), `jq`, the llama-server up on
:8080. Artifacts land in `runs/` (one dir per arm x rep, with per-task stdout,
stderr, and API-call token logs) and `results.jsonl`; a means table prints at
the end. Delete `results.jsonl` between campaigns you don't want mixed.

## Reading the result

- **grown >> bare on t1/t2** (calls, tokens, success): the estate amortizes
  capability growth. Expected and boring — but it quantifies the amortization.
- **grown >> bare on t3**: the estate compounds *within* a session — installed
  views make earlier state cheap to perceive. This is the number to watch.
- **bare arm failing to converge or install working scripts**: honest data,
  not a bench bug — it prices what growing from nothing costs a 27B-class
  local model, and it is the gap an account (a lesson) closes. A natural
  third arm later: `learned` — bare + `self learn lessons/journal` with the
  live mind, measuring what a lesson is worth in tokens.

## The flipped axis: Home Field

This experiment holds the mind fixed and varies the estate. Flip it — hold
the **estate** fixed and vary the **mind** — and the same instrument measures
something no benchmark today measures: **mind value at fixed estate**. We
call that protocol **Home Field**: does a mind gain home-field advantage from
a body it didn't grow, and how much advantage does each generation of minds
extract from the *same* home?

Needle-in-a-haystack benches are the ancestor, and every way Home Field
differs from them is the point:

- **Retrieval is agentic, not positional.** NIAH stuffed a context window and
  asked whether attention could grep it. No estate fits in a window — so the
  question becomes whether the mind can find the needle *it is not holding*:
  know the haystack has structure, pick the right view, spend context on
  compressed perception (`self view journal`) instead of raw reads of
  `events.jsonl`. NIAH tested memory capacity; Home Field tests knowing your
  way around your own memory.
- **It doesn't saturate.** NIAH died when models aced it — a synthetic
  haystack leaks into training data and stops discriminating. An estate is
  the sediment of real use: vocabulary coined from one life, capabilities
  that appeared because one person asked. And it *moves* — the estate at
  month six is a richer, harder haystack than at month one, growing at the
  rate a life does. A benchmark that deepens as it is lived in never goes
  stale.
- **The haystack is shippable and replayable.** An estate is
  `events.jsonl` + `.secret`; rehydration rebuilds the identical body,
  byte for byte, no model, no network. A reference estate can be published
  as an account — a lived-in body plus asks with checkable outcomes — and
  every future model release scored against the same home. The benchmark
  artifact is a directory of plain text files.

Running it here costs one variable: keep the seed and tasks frozen, point
`bin/mind` at a different agent (`SELF_MIND_OPENCODE`, or swap its one exec
line), set `MIND_LABEL` to name the mind, and compare `results.jsonl` rows
grouped by `mind`. The score that matters is the grown arm's cost-and-success
profile per mind: how cheaply does this year's intelligence move through last
year's home. If the local mind's numbers converge with a frontier mind's on
the same estate, the capability gap — for *this* life — has closed, and you
measured it instead of guessing.

The two axes together are the whole thesis in numbers: estate value at fixed
mind shows the body compounding; mind value at fixed estate shows the same
body yielding more as intelligence commoditizes. One instrument, both curves.

## Caveats

- One model, one machine, three tasks: an instrument, not a paper. Add tasks
  before adding claims.
- Observed: during settling, a mind may smoke-test a freshly grown command
  with a real append, contaminating domain state that later tasks perceive.
  The same-event check keeps this from producing false positives, but the
  contamination itself is honest data about how minds behave — read the
  per-run logs before trusting a surprising t3.
- Temperature 0 through llama-server is not fully deterministic; reps exist
  for a reason.
- `bin/self` is a build artifact and `seed-home/`, `runs/`, `results.jsonl`
  are outputs — none of them belong in git (see `.gitignore`).
