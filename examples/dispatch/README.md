# dispatch: a goal worker as a visible hc terminal

`self run dispatch [--area <space>] [--tab <label>] [--pane <doing>] <goal> <name> <kind> <repo> [prompt]`

Starts one agent (`claude`, `opencode`, `pi`, `codex`) on a goal in its own git
worktree (`~/.herdr/worktrees/<repo>/<goal>-<name>`, branch `goal/<goal>/<name>`
off `origin/HEAD`) as a herdr-core terminal:

    hc run --bg --no-parent --outlive-parent --name <name> --cwd <worktree> \
      --label herdr.space=<area space> --label herdr.tab=<new> --label goal=<goal> ... \
      -- env SELF_CALLER=<name> SELF_HOME=<instance> <agent> [args] "<prompt>"

The `herdr.space` label makes the Herdr window show it in the space named
`--area` (default: the repo basename); herdr-app then renames the tab. The
assignment is the program's own first prompt, so nothing is sent to a booting
agent. `agent.started` is recorded only after `hc get` shows `working` (or a
finished turn); `agent.failed` records the stage and anything it created.

This replaces the herdr 0.9 dispatcher, which drove `/opt/homebrew/bin/herdr`
(retired when Herdr moved to herdr-core).

Install: declare `command/dispatch` and author `command.py` (see `self help`).

Verified 02/11/2026 on the live log: seq 14874 agent.requested, 14875
agent.started (terminal t1z, space `self`, observed working), and 14876 a
`goal.progress` written by the worker as `claude-haiku-dispatch-probe` from its worktree.
