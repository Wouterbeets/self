# Audit: what one run of work did

A loop pass, an agent session or a worker leaves events scattered across the
log. `self view audit` gathers one caller's run and shows, with seq citations:
intents declared or closed, goals touched, capabilities declared, installed,
rejected or verified, workers dispatched on its behalf, evidence and gates,
leases/budgets/checkpoints, pull requests and reviews, commits, branches and PR
URLs named in payloads, what it said aloud, and the human replies to that speech.

```sh
self view audit                          # recent runs of every caller
self view audit loop-opus-intents        # its latest run in full
self view audit loop-opus-intents --list # its runs, one line each
self view audit loop-opus-intents --seq 8490-8511
```

A run is a caller's events with no quiet gap longer than `--gap` minutes
(default 20) and no `loop.settled` inside it. Kernel receipts count for the
caller whose declaration they answer; events whose payload names the caller as
`dispatched_by` or `requested_by` count for it too. Speech counts when it is on
a Herdr tab the run named or reported doing on; a reply counts when its `after`
matches that speech.

Limits: the log records no pass boundary, so a loop's back-to-back passes read
as one run unless `--seq` pins one. The caller is the kernel-stamped
`SELF_CALLER`, a claim rather than authentication. The view shows what was
recorded, not whether the work was right. It needs no kernel change.

## Install

```sh
jq -nc '{name:"view.declared",payload:{name:"audit",summary:"What one caller run did: intents, goals, capabilities, workers, evidence, PRs, speech, replies",description:"usage: see examples/audit/view.py",consumes:["*"]}}' | self hear
jq -nc --rawfile v examples/audit/view.py '{name:"script.authored",payload:{type:"view",name:"audit",script:$v}}' | self hear
```

## Test

```sh
sh examples/audit/test.sh
```
