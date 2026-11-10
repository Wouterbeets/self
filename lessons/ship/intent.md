# ship — follow a change from published to running, and say who it waits on

## purpose

Publishing a change is the middle of the work, not the end. After that it
waits on checks, on a reviewer, on a merge, on a deployment, and each wait is
a place where it is forgotten. Ship follows every change bound to a goal
through those steps, acts where a machine can, and tells a person only when
the change now needs them.

## surface

- `self run ship track <goal> <change>` binds a goal to its published change.
- `self run ship tick` observes each tracked change once and appends its stage
  only when the stage changed.
- `self view ship` lists the tracked changes with their stage.
- `self view ship actions` prints what a driver should do next: announce,
  alert, resume, notify, close.
- `self view ship.flow` shows each change as steps (plan, build, publish,
  checks, review, staging, production), grouped by who it waits on: you, a
  reviewer, a machine, another goal, or a hold.
- A driver on a timer runs `tick`, then carries out the actions.

## constraints

- A stage is recorded when it changes and never otherwise. Its detail text is
  stable: no timestamps, counters or raw error output. A detail that differs
  on every tick turns one situation into a new stage every time.
- An alert fires once for an unbroken run of a stage, however many events
  repeat it.
- Not being able to look is not a finding. When the staging environment cannot
  be reached, the stage says so and nobody is alerted.
- What a change deploys is worked out from the change itself, from the files
  it touched, not from what someone remembered to declare.
- A hold the driver placed for an alert is released by the driver when the
  alert clears. A hold a person placed is never released by a machine.
- A change closed without merging is a decision someone made. It alerts
  nobody.
- A person is told when a change starts needing them: checks failed, changes
  were requested, it is approved and ready, it is running in staging and ready
  for release. Not for progress.
- One driver. Two instances driving the same changes double every message and
  race on every hold.
- Each change says in one line who it waits on and what happens next. The
  list leads with the ones waiting on you.
