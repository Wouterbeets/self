# Checkpoint: a human decision on exact actions, each used once

A mind that wants to do something only a human may allow records exactly what
it will do, and waits:

```sh
self run checkpoint propose pr-budget "open PR from self/budget" "push self/budget"
self view checkpoint                                   # what awaits a decision
SELF_CALLER=wouter self run checkpoint approve pr-budget --ttl 24h
SELF_CALLER=wouter self run checkpoint reject pr-budget --reason "not yet"
self run checkpoint use pr-budget "push self/budget" | grep -q checkpoint.used || exit 0
```

An approval covers only the recorded actions, each usable once, until its ttl
runs out; a rejection keeps its reason; an id is never reused. Everything is in
the log, so a restart or replay still shows what is pending. `use` exits 3 and
appends nothing when the action is not covered, so a caller acts only if the
output holds `checkpoint.used`.

Approve and reject belong to `APPROVERS` in the script (changing it is a
`script.authored`, so it stays in history). `use` honours an approval only when
the kernel stamped it with an approver's caller, so a mind cannot approve itself
by appending `checkpoint.approved` under its own name. The caller is a claim,
not authentication: this records and enforces a decision among cooperating
minds; keeping a mind from running a command it was denied is its sandbox's job.

Which actions need a checkpoint (open a PR, touch infrastructure) and which are
never proposed (force-push, merge, production writes) is the caller's policy.
Like `lease` and `budget`, it relies on `"atomic": true`, so two uses of one
action never both succeed.

## Install

```sh
jq -nc '{name:"command.declared",payload:{name:"checkpoint",summary:"Propose exact actions for a human decision; use each approved action once",description:"usage: see examples/checkpoint/command.py",atomic:true,consumes:["checkpoint.proposed","checkpoint.approved","checkpoint.rejected","checkpoint.withdrawn","checkpoint.used"]}}
        ,{name:"view.declared",payload:{name:"checkpoint",summary:"Actions awaiting a human decision, and what approvals still cover",description:"usage: see examples/checkpoint/view.py",consumes:["checkpoint.proposed","checkpoint.approved","checkpoint.rejected","checkpoint.withdrawn","checkpoint.used"]}}' | self hear
jq -nc --rawfile c examples/checkpoint/command.py --rawfile v examples/checkpoint/view.py \
  '{name:"script.authored",payload:{type:"command",name:"checkpoint",script:$c}},{name:"script.authored",payload:{type:"view",name:"checkpoint",script:$v}}' | self hear
```

## Test

```sh
sh examples/checkpoint/test.sh
```
