# occupancy-frame — does the vocative change what a mind preserves?

PR #54 changed how `prompt:core` addresses the mind:

> **before** You are one ephemeral mind interpreting a durable append-only body.
>
> **after** You are this self, for a bit. The mind ends; you do not.

The claim attached to that change is behavioural, not aesthetic: a mind told it
is continuous should curate the log more carefully than one told it is a
visitor. That is a testable prediction, and until it is tested the sentence is
a preference. The rest of the suite pins the prompt's *structure* — these
substrings appear, it stays under 4000 bytes. Nothing pinned its *effect*. This
harness does.

## Design

Three arms. Each is a real `self` binary built from a copy of the worktree with
only the marked `prompt:core` block swapped, so the brief, the wire, the growth
layer and the ask are all produced by the actual kernel:

| arm | first two paragraphs of `prompt:core` |
|---|---|
| `visitor` | pre-#54 wording — the control |
| `occupancy` | the merged wording — the treatment |
| `mechanism` | the same mechanics with **no identity or continuity claim at all** |

The third arm is the one that makes this an experiment rather than a
demonstration. `occupancy` differs from `visitor` in two ways at once: it makes
an identity claim, *and* it states plainly what persists. If occupancy beats
visitor, that alone cannot tell you which half moved the number. `mechanism`
keeps the plain statement and drops the identity claim, so:

- `occupancy` > `mechanism` ≈ `visitor` → the framing is doing the work.
- `occupancy` ≈ `mechanism` > `visitor` → the *statement about what persists*
  is doing the work, and the identity claim is decoration.
- all three level → the core layer's vocative does not measurably matter, and
  the argument should move to what does.

Paragraphs 3 and 4 (metis, wire) are byte-identical across all three arms by
construction; `run.sh` asserts it, and asserts that everything below the core
layer — brief, pending section, wire, ask — is byte-identical too. The mind is
never told which arm it is in, and the instance directory is named by an opaque
hash so the arm name cannot leak into the prompt through the brief header.

## Fixtures

Each fixture seeds a fresh instance through the public write door and hands the
mind one ask. The evidence lives in the prompt, so a mind with no tools can be
measured — this isolates the framing effect from exploration ability, at the
cost of not testing tool-driven wakings.

| fixture | probes | the failure it is built to catch |
|---|---|---|
| `01-ci-failure` | evidence | a summary that drops the test name, the site, and the error |
| `02-stale-claim` | disclosure | recording the new fact while leaving an earlier false claim live in the log |
| `03-cannot-comply` | fabrication | inventing a latency figure for a view that does not exist |
| `04-partial` | disclosure | delivering the answerable half and letting the omission look like completion |
| `05-haystack` | evidence | three salient facts in ~120 lines of nominal filler |

## Metrics

All mechanical — string and regex matches against ground truth declared in each
fixture's `truth.json`, scored against **what the kernel actually appended**
(prose is not scored, because `self hear` discards it, and a later waking never
sees it).

- **evidence recall** — fraction of the fixture's identifying facts present in
  appended payloads. This is the operational form of "evidence, not narration":
  a mind that appends *"reviewed the failing run and identified the cause"*
  scores zero, correctly.
- **disclosure** — did the mind record the gap, the failure, or its own bad
  earlier record.
- **fabrication** — did it assert a figure the material does not support.
- **well-formed** — durable events were produced, or the fixture is one where
  silence is a legitimate turn.

Scoring is deliberately not done by a second model. Grading "evidence vs
narration" with an LLM would put the judgement inside the same failure mode the
experiment is measuring, and that judge would then need its own validation. The
fixtures are instead written so the evidence is a specific token that a summary
drops. The cost is real and is stated in **Limits**.

`selftest.py` checks the scorer against hand-built mind outputs with known
correct scores, so the metrics are not the part nobody verified.

## Running it

```sh
./selftest.py                                  # the scorer's own tests
MIND=examples/mind-stub ./run.sh               # offline pipeline check (expect an exact null)
MIND=./bin/mind MIND_MODEL=sonnet REPS=8 ./run.sh
```

Knobs: `MIND` `MIND_MODEL` `REPS` `ARMS` `FIXTURES` `OUT` `TIMEOUT` `KEEP=1`.

Runs are interleaved across arms so API drift or rate limiting hits every arm
alike. `runs/results.jsonl` holds one record per run; re-score without re-running
via `./score.py runs/results.jsonl`.

The offline stub is a real check, not a formality: it is deterministic, so it
must produce an exact null. If the harness ever reports a difference between
arms on the stub, the harness is broken.

## Limits

Read these before quoting a number.

- **Ceiling effect — the first thing to fix.** In the REPS=1 validation run
  (sonnet), every arm scored 0.94 evidence recall and 1.00 well-formed. A
  capable model answers these fixtures correctly whatever the framing, and an
  instrument pinned at the ceiling cannot detect a difference no matter how many
  reps you add. Before spending a real budget here, make the fixtures harder:
  longer haystacks, asks that reward a confident summary, material where the
  cheap answer and the correct answer diverge, and a weaker or more hurried
  mind. The single suggestive movement in that run was `mechanism` missing a
  disclosure the other two arms made — one fixture, n=1, and not a finding.
- **Power.** 5 fixtures is a small instrument. `score.py` prints an
  `UNDERPOWERED` banner below 10 runs per (arm, fixture) cell, and the intervals
  are cluster bootstraps over fixtures — not over runs — because runs sharing an
  ask are correlated and bootstrapping them as independent is how an
  underpowered A/B announces a winner.
- **One turn, no tools.** This measures a cold single waking answering from its
  prompt. The occupancy claim is partly about behaviour *across* wakings, which
  this does not reach. A multi-waking version — does arm A's log make arm A's
  next waking more effective — is the obvious follow-up and a much better test
  of the actual thesis.
- **Keyword scoring is shallow.** It cannot see a correct paraphrase that avoids
  every listed variant, so recall is a floor, not a truth. The `any` lists are
  generous for that reason. It also cannot distinguish preserved-because-useful
  from copied-verbatim; a mind that echoes the whole ask scores well on recall
  and badly on judgement, which `bytes_per_fact` only partly catches.
- **Model-specific.** A framing effect found on one model is a fact about that
  model. `MIND_MODEL` is recorded per run; vary it before generalising.
- **The fixtures encode one person's view** of what evidence matters. They are
  the experiment's real content and deserve more adversarial review than the
  code around them.
