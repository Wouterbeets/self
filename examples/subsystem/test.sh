#!/bin/sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
bin=${SELF_BIN:-}
if [ -z "$bin" ]; then
	(cd "$here/../.." && go build -o "$tmp/self" .)
	bin=$tmp/self
fi
export SELF_HOME=$tmp/home SELF_BIN=$bin SELF_CALLER=t
s() { "$bin" "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }
ev='["subsystem.spawned","subsystem.observed","subsystem.retired"]'

jq -nc --argjson c "$ev" '{name:"command.declared",payload:{name:"subsystem",summary:"Spawn, observe and retire child instances",description:"see script",consumes:$c}}
        ,{name:"view.declared",payload:{name:"subsystems",summary:"Child instances and their last observation",description:"see script",consumes:$c}}' | s hear >/dev/null 2>&1
jq -nc --rawfile c "$here/command.py" --rawfile v "$here/view.py" \
	'{name:"script.authored",payload:{type:"command",name:"subsystem",script:$c}},{name:"script.authored",payload:{type:"view",name:"subsystems",script:$v}}' | s hear >/dev/null 2>&1

s view subsystems | grep -q 'no subsystems' || fail "empty view"
s run subsystem spawn ops --owns herdr.,attention. --note "voice and tabs" | grep -q subsystem.spawned || fail "spawn"
child=$SELF_HOME/sub/ops
[ -s "$child/events.jsonl" ] && [ -d "$child/view" ] && [ -d "$child/bin" ] || fail "child not initialised"
[ "$(s view subsystems --home ops)" = "$child" ] || fail "--home"
code=0; s run subsystem spawn ops 2>/dev/null || code=$?
[ "$code" = 3 ] || fail "respawn: exit $code, want 3"

n=$(s view log --all | wc -l)
printf '%s\n' '{"name":"herdr.doing","payload":{"text":"a"}}' '{"name":"herdr.doing","payload":{"text":"b"}}' '{"name":"attention.spoken","payload":{"text":"hi"}}' | SELF_HOME=$child s hear >/dev/null
[ "$(s view log --all | wc -l)" = "$n" ] || fail "child write touched the parent"
s run subsystem observe | grep -q '"herdr.doing":2' || fail "observe counts"
[ -z "$(s run subsystem observe)" ] || fail "unchanged child observed again"
s view subsystems | grep -q '^ops	4 events' || fail "overview: $(s view subsystems)"
s view subsystems ops | grep -q 'herdr.doing×2' || fail "detail"

s run subsystem retire ops 2>/dev/null && fail "retire without reason"
s run subsystem retire ops "folded back" | grep -q subsystem.retired || fail "retire"
s view subsystems | grep -q 'ops	retired' || fail "retired listing"
s view subsystems --home ops >/dev/null && fail "--home of retired"
n=$(s view log --all | wc -l)
s run subsystem spawn 'Bad Name' 2>/dev/null && fail "bad name"
s run subsystem bogus 2>/dev/null && fail "bad verb"
[ "$(s view log --all | wc -l)" = "$n" ] || fail "refusals appended"
echo "subsystem: all checks passed"
