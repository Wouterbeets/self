# Budget: bounded counters per scope and class

`self run budget spend loop/x/pass/3 dispatches --limit 2` succeeds twice; the
third try records one `budget.exhausted` and later tries are refused with exit 3
and append nothing. `self view budget` shows what each scope used and has left.

A scope is any string (a loop pass, a goal, a day) and a class is whatever is
counted (dispatches, new goals, pushes, changed files). `budget set` records or
changes a limit; raising it reopens an exhausted class. A spend larger than what
is left is refused whole. A caller spent only if the output holds
`budget.spent`:

```sh
self run budget spend "loop/$LOOP/pass/$PASS" dispatches --limit 2 | grep -q budget.spent || exit 0
```

Deciding which class an action falls in (inside the goal, adjacent, expanding
scope) is the caller's judgment; the counter only makes the bound hold. Like
`lease`, it relies on `"atomic": true` so concurrent spends never pass the limit.

## Install

```sh
jq -nc '{name:"command.declared",payload:{name:"budget",summary:"Spend bounded counters per scope and class",description:"usage: see examples/budget/command.py",atomic:true}}
        ,{name:"view.declared",payload:{name:"budget",summary:"Spent and remaining budget per scope",description:"usage: see examples/budget/view.py",consumes:["budget.set","budget.spent","budget.exhausted"]}}' | self hear
jq -nc --rawfile c examples/budget/command.py --rawfile v examples/budget/view.py \
  '{name:"script.authored",payload:{type:"command",name:"budget",script:$c}},{name:"script.authored",payload:{type:"view",name:"budget",script:$v}}' | self hear
```

## Test

```sh
sh examples/budget/test.sh
```
