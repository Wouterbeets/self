# Subsystem: child instances that hold a stream away from this log

`self run subsystem spawn herdr --owns herdr.` starts a child instance at
`$SELF_HOME/sub/herdr`, with its own log, `view/` and `bin/`. Capabilities that
own a stream write into it with `SELF_HOME=<child> self hear` and read their
history from both logs, so the parent log stays an overview while the child
collects the detail. `self view subsystems --home herdr` prints the path.

Information flows up as summaries, never as pass-through views:
`self run subsystem observe` reads each child log and records
`subsystem.observed {name, head, events, bytes, last_at, since, new}` in the
parent only when the child changed. `self view subsystems` lists the children
and their last observation; `self view subsystems <name>` shows one child's
history. A child's own views stay in the child, browsable as files under
`sub/<name>/view/`.

`self run subsystem retire <name> <reason>` takes a child off the overview;
its directory and log stay. The kernel knows nothing of any of this: a child is
an ordinary instance, and the parent only records where it lives.

## Install

```sh
c='["subsystem.spawned","subsystem.observed","subsystem.retired"]'
jq -nc --argjson c "$c" '{name:"command.declared",payload:{name:"subsystem",summary:"Spawn, observe and retire child instances",description:"usage: see examples/subsystem/command.py",consumes:$c}}
        ,{name:"view.declared",payload:{name:"subsystems",summary:"Child instances and their last observation",description:"usage: see examples/subsystem/view.py",consumes:$c}}' | self hear
jq -nc --rawfile c examples/subsystem/command.py --rawfile v examples/subsystem/view.py \
  '{name:"script.authored",payload:{type:"command",name:"subsystem",script:$c}},{name:"script.authored",payload:{type:"view",name:"subsystems",script:$v}}' | self hear
```

`SELF_BIN` names the `self` binary when it is not at the fallback path;
capability scripts run with a fixed `PATH`. `examples/subsystem/test.sh`
checks it end to end against a fresh instance.
