# practices — say how work is done here, and enforce it where work leaves

## purpose

Every team has rules that live in someone's head: how errors are wrapped, what
a change's title looks like, which files need a second pair of eyes. Agents
break them politely and at scale. A practice is one such rule written down
with the files it applies to, the moments it should be shown, and, when it can
be checked, the check. The rule is shown when work starts and enforced at the
one door work leaves through.

## surface

- `self run practice set <id> --title <t> --rule <text> --when <globs>
  --at <points> [--check <command>]` appends `practice.saved`.
- `self run practice retire <id> <reason…>` appends `practice.retired`.
- `self run practice show --at <point>` prints the practices that apply to the
  files in play at that point.
- `self view practices` lists the live ones: id, where they apply, the check.
- `self run publish` (name it for your own door) carries a change out: it runs
  every practice declared for that point on the files the change touched, and
  only then lets the change leave.

## constraints

- A practice names its insertion points: when work starts, when it is
  published, when it is verified. A new point needs no change to the store;
  whoever stands at that point asks for what was declared there.
- A check judges only the lines the change adds. Old code never blocks new
  work.
- A check is a command with one contract: given the base, the head and the
  paths, exit clean or print one `path:line: problem` per finding.
- A failing practice stops the change before anything leaves the machine and
  prints the rule beside the findings. An override is allowed, takes a reason,
  and the reason is recorded where a reviewer will see it.
- There is one door. If agents can publish around it, the practices are
  advice.
- Practices that cannot be checked are still shown at their point, marked as
  judged by hand.
