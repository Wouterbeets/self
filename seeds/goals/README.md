# Goals instance

This is the replacement domain for the work instance. It keeps `self`'s
append-only kernel, but makes goals the primary unit of work.

## Model

- `goal.created`, `goal.updated`, `goal.progress`, `goal.milestone`, and `goal.kpi` are the
  durable goal record.
- `self run goal progress <goal> <report>` records durable progress and the
  final `self.replied` text serves as the loop handoff.
- `project.registered` identifies the repository being changed. A goal may
  reference one project while child goals reference narrower packages.
- Dispatch records `repo`, `worktree`, `branch`, and agent identity separately.
  The default execution space is a new Herdr worktree, never the base clone.
- A top-level goal cannot be completed until its branch and commit are recorded
  as pushed and its worktree has been removed. Completion is a gate, not a
  status label an agent may simply assert.
- `agent.requested`, `agent.started`, `agent.progress`, and `agent.ended`
  connect goals to Herdr-managed coding agents.
- Goals are recursive: `goal.created` may carry `parent`, and every dispatch
  names one goal as its scope.
- `/brief` is the cold-start control room; `/goals`, `/agents`, and `/metrics`
  are focused views.
- The former `~/.self-work` instance is an archive, not a live dependency.

Herdr dispatch is deliberately explicit. A goal can request an agent, but a
process is only considered started after Herdr returns a pane and agent
identity. This keeps desired state separate from observed state.

The initial agent prompt is intentionally small. It names the goal and tells
the agent how to unfold context from `/goals` and the event history, instead
of injecting the entire tree into every prompt. The agent may then create
subgoals and report their identifiers.
