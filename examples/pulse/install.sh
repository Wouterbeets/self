#!/bin/sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
: "${SELF_HOME:?set SELF_HOME}"
export SELF_CALLER="${SELF_CALLER:-installer}"

desc() { python3 -c 'import ast,sys; print(ast.get_docstring(ast.parse(open(sys.argv[1]).read())))' "$1"; }

declare() {
  jq -nc --arg t "$1" --arg n "$2" --arg s "$3" --arg d "$(desc "$4")" --argjson c "$5" \
    '{name:($t+".declared"),payload:{name:$n,summary:$s,description:$d,consumes:$c}}' | self hear >/dev/null
  jq -nc --arg t "$1" --arg n "$2" --rawfile s "$4" '{name:"script.authored",payload:{type:$t,name:$n,script:$s}}' | self hear >/dev/null
}

declare view pulse "Is self's own machinery behaving: loops, bursts of outward effects, timers failing for over a day." \
  "$here/view.py" '["*"]'
declare command pulse "Witness this machine's timers (tick) and record verdicts on pulse findings (ack, told)." \
  "$here/command.py" '["pulse.timers","pulse.acked","pulse.told"]'
echo "installed view/pulse and command/pulse; drive.sh is the loop a timer runs"
