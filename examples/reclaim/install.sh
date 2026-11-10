#!/bin/sh
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
: "${SELF_HOME:?set SELF_HOME}"
export SELF_CALLER="${SELF_CALLER:-installer}"

desc() { python3 -c 'import ast,sys; print(ast.get_docstring(ast.parse(open(sys.argv[1]).read())))' "$1"; }

declare() {
  extra=${6:-}
  [ -n "$extra" ] || extra='{}'
  jq -nc --arg t "$1" --arg n "$2" --arg s "$3" --arg d "$(desc "$4")" --argjson c "$5" --argjson x "$extra" \
    '{name:($t+".declared"),payload:({name:$n,summary:$s,description:$d,consumes:$c}+$x)}' | self hear >/dev/null
  jq -nc --arg t "$1" --arg n "$2" --rawfile s "$4" '{name:"script.authored",payload:{type:$t,name:$n,script:$s}}' | self hear >/dev/null
}

hc='{"requires":[{"executable":"hc","override":"SELF_HC_BIN"}]}'
goals='"goal.created","goal.updated","goal.removed"'

declare command observe-agents "Record changed agent liveness and worktree observations." \
  "$here/observe-agents/command.py" '[]' "$hc"
declare view agents "Goal workers by state, replayed from the last observation." \
  "$here/observe-agents/view.py" "[\"agent.started\",\"agent.failed\",\"agent.observed\",\"agent.reclaimed\",$goals]"
declare command reclaim "Inspect or reclaim finished workers using recorded evidence." \
  "$here/reclaim/command.py" '[]' "$hc"
declare view reclaim "The reclaim plan replayed from the last observation: what would go, what is held and why." \
  "$here/reclaim/view.py" "[\"agent.started\",\"agent.reclaimed\",\"agent.observed\",$goals]"

for f in observe-agents/command.py observe-agents/view.py reclaim/command.py reclaim/view.py; do
  printf '%s  %s\n' "$(shasum -a 256 "$here/$f" 2>/dev/null | cut -c1-12 || sha256sum "$here/$f" | cut -c1-12)" "$f"
done
