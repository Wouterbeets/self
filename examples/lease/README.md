# Lease: exclusive, expiring ownership of a named resource

`self run lease acquire goal/x --ttl 30m` gives one holder a resource until it
expires. Others are refused with the holder and expiry; `renew` extends it,
`release` frees it, and a lapsed lease (a crashed owner) needs `acquire --steal`,
which records who held it. `self view leases` lists live leases;
`self view leases <resource>` shows one resource's history.

A resource is any string: a goal, a repository branch, a worktree. The kernel
knows nothing about leases. It only offers `"atomic": true` on a command
declaration: the batch commits against the log the command read, or the command
reruns. That is what makes ten concurrent acquires produce one winner. Its
`consumes` narrows that log to lease events, so unrelated appends cause no rerun.
Refusals exit 3, so `self run lease acquire … || …` branches on them.

## Install

```sh
jq -nc '{name:"command.declared",payload:{name:"lease",summary:"Hold exclusive expiring ownership of a named resource",description:"usage: see examples/lease/command.py",atomic:true,consumes:["lease.acquired","lease.renewed","lease.released"]}}
        ,{name:"view.declared",payload:{name:"leases",summary:"Live leases or one resource history",description:"usage: see examples/lease/view.py",consumes:["lease.acquired","lease.renewed","lease.released"]}}' | self hear
jq -nc --rawfile c examples/lease/command.py --rawfile v examples/lease/view.py \
  '{name:"script.authored",payload:{type:"command",name:"lease",script:$c}},{name:"script.authored",payload:{type:"view",name:"leases",script:$v}}' | self hear
```

On a kernel without `atomic`, the declaration field is ignored and concurrent
acquires can all win: `SELF_BIN=<old self> sh examples/lease/test.sh` shows it.

## Test

```sh
sh examples/lease/test.sh
```
