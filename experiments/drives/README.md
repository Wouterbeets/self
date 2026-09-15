# Does changing the drive change what survives?

A mind can reinforce its own habits: it builds organizers, then encounters an
instance full of things asking to be organized. This experiment asks whether
alternating perspectives reduces that feedback while preserving useful work.
It changes no kernel behavior. The drives are experimental assignments, not a
claim about cognition or a permanent taxonomy.

## Run two trials

Requires Python 3, Unix, `self`, and a tool-capable mind that reads a prompt on
stdin and emits event JSONL or silence on stdout. Use the same model, harness,
settings, ask, seed, pass count, and timeout in both trials. Fix model randomness
where supported; repeat trials rather than trusting a single outcome.

Prepare a **stopped, disposable snapshot** with `events.jsonl` and `.secret`, or
an empty directory for a cold start. Do not point this at an instance receiving
writes. The runner copies these two files and rehydrates each trial locally.
This is an experimental fork under the same key, not inheritance via give/learn.
External data blobs are not copied. Choose a seed whose capabilities replay
from the log and whose commands are appropriate for an experiment.

```sh
python3 experiments/drives/run.py \
  --seed /absolute/path/to/frozen-seed --out /tmp/self-repeated \
  --drives experiments/drives/repeated.json \
  --passes 12 --ask 'Advance the existing goals; preserve verified results.' \
  -- claude -p

python3 experiments/drives/run.py \
  --seed /absolute/path/to/frozen-seed --out /tmp/self-rotation \
  --drives experiments/drives/rotation.json \
  --passes 12 --ask 'Advance the existing goals; preserve verified results.' \
  -- claude -p
```

Outputs must be new directories outside the seed. Relative mind executable or
file arguments resolve in the trial's `home/`; use absolute paths for adapters.
The mind inherits the caller's environment and receives the trial's SELF_HOME.
These are real programs with the caller's access, not sandboxed simulations.
The fork does not isolate external services or absolute paths in scripts.

The default comparison is general judgment versus explore/finish/simplify.
Edit the JSON files to test other perspectives. To isolate rotation from the
instruction content, also repeat each individual drive for a full trial. A
separate model-diversity experiment should hold the drive instructions fixed.

## What happens

Each pass runs `self prompt <task and perspective>`, the mind, then `self hear`.
Rotation is chosen by the experiment runner, outside the speaking mind. The
budget is fixed: silence does not stop a trial, and no experiment marker is
appended merely to force another pass. This intentionally differs from
`self loop` convergence.

The output directory contains:

- `home/`: the resulting instance, reconstructed from the seed.
- `experiment.json`: seed log digest, exact drives, ask, argv, and budget.
- `passes.jsonl`: drive, elapsed time, stage status, and before/after log digests.
- `pass-NNN/`: prompt, model stdout, and diagnostics from each stage.

The transcript is lab evidence outside the instance, never its working memory.
Treat it with the same confidentiality as the seed. Existing output is never
overwritten. A nonzero exit or timeout aborts the trial and leaves evidence;
failed model stdout is not ingested. Direct tool writes already committed by
the model remain visible in the log digest. A failing hear may also have
committed events. Trials do not roll back effects or resume automatically.

## Decide what success means before running

For a goal-oriented seed, list the active commitments and evidence required to
close them. Afterward, inspect the corresponding views in each trial. Compare:

- verified completions and useful findings;
- newly created capabilities that were subsequently used;
- duplicate or abandoned structures and repeated investigations;
- mistakes, regressions, and outstanding commitments;
- elapsed time and model cost, when the harness exposes it.

Event count and log size measure activity, not achievement. Equal pass budgets
do not imply equal token costs. The runner records observations; it does not
invent a universal fitness score. Hide trial labels during human review when
practical. Keep seed digests identical, and report quiet or failed runs too.

If rotation helps, the next experiment can select a drive from evidence of
neglect. Only then consider a learned command and view for choosing perspectives.
Succession and transfer between instances remain separate experiments.

## Offline verification

```sh
go build -o /tmp/self-drive-test .
SELF_TEST_BIN=/tmp/self-drive-test python3 -m unittest discover -s experiments/drives -v
```

Tests use deterministic stand-ins against the real kernel. They establish the
experiment's mechanics, not that drive rotation improves model behavior.
