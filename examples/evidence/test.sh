#!/bin/sh
# End-to-end check of the evidence command, view and self-check runner against a
# fresh instance, using the self binary in $SELF_BIN (default: build this checkout).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
bin=${SELF_BIN:-}
if [ -z "$bin" ]; then
	(cd "$here/../.." && go build -o "$tmp/self" ./cmd/self 2>/dev/null || go build -o "$tmp/self" .)
	bin=$tmp/self
fi
export SELF_HOME=$tmp/home SELF_BIN=$bin SELF_CALLER=loop XDG_RUNTIME_DIR=$tmp
fail() { echo "FAIL: $*" >&2; exit 1; }
events() { "$bin" view log --all | wc -l; }
check() { python3 "$here/self-check" "$@" 2>>"$tmp/check.err"; }

jq -nc '{name:"command.declared",payload:{name:"evidence",summary:"Record check results against a subject; require them before calling work done",description:"see script"}}
        ,{name:"view.declared",payload:{name:"evidence",summary:"What was checked, what passed, what gates were met",description:"see script",consumes:["evidence.checked","evidence.satisfied"]}}' | "$bin" hear >/dev/null 2>&1
jq -nc --rawfile c "$here/command.py" --rawfile v "$here/view.py" \
	'{name:"script.authored",payload:{type:"command",name:"evidence",script:$c}},{name:"script.authored",payload:{type:"view",name:"evidence",script:$v}}' | "$bin" hear >/dev/null 2>&1

# Nothing recorded: require refuses, names the gap, appends nothing.
n=$(events)
"$bin" run evidence require goal/g1 tests clean 2>"$tmp/err" && fail "require passed with no evidence"
grep -q 'tests: never recorded' "$tmp/err" || fail "gap unclear: $(cat "$tmp/err")"
[ "$(events)" = "$n" ] || fail "refused require appended"

# A passing and a failing check are both recorded; the runner returns the command's status.
mkdir "$tmp/repo"
check goal/g1 tests --dir "$tmp/repo" -- sh -c 'echo ok; pwd' || fail "passing check exited nonzero"
check goal/g1 clean --dir "$tmp/repo" -- sh -c 'echo "dirty: a.go"; exit 1' && fail "failing check exited zero"
"$bin" view evidence goal/g1 | grep -q 'clean: FAIL exit 1' || fail "view: $("$bin" view evidence goal/g1)"
"$bin" view evidence goal/g1 | grep -q 'dirty: a.go' || fail "failure tail not shown"
"$bin" view evidence goal/g1 | grep -q "dir: $tmp/repo" || fail "dir not recorded"
"$bin" run evidence require goal/g1 tests clean 2>"$tmp/err" && fail "require passed with a failing check"
grep -q 'clean: failed (exit 1' "$tmp/err" || fail "failure unclear: $(cat "$tmp/err")"

# A later pass supersedes the failure; the gate is met and cites the results.
check goal/g1 clean -- true || fail "rerun"
"$bin" run evidence require goal/g1 tests clean --note done | grep -q evidence.satisfied || fail "require after passes"
"$bin" view evidence goal/g1 | grep -q 'Earlier runs (1)' || fail "history lost"
"$bin" view evidence | grep -q 'goal/g1 .*passing: clean, tests  satisfied at seq' || fail "summary: $("$bin" view evidence)"

# --max-age accepts a fresh result and refuses an old one (script fed a crafted log).
check goal/g2 tests -- true
"$bin" run evidence require goal/g2 tests --max-age 1h | grep -q evidence.satisfied || fail "fresh result refused"
echo '{"seq":1,"name":"evidence.checked","occurred_at":"2020-01-01T00:00:00Z","by":"loop","payload":{"subject":"goal/g2","check":"tests","exit":0,"passed":true}}' |
	python3 "$here/command.py" require goal/g2 tests --max-age 1h 2>"$tmp/err" && fail "stale result accepted"
grep -q 'older than --max-age' "$tmp/err" || fail "staleness unclear: $(cat "$tmp/err")"

# A timeout kills the whole process group, records 124 and leaves no child behind.
start=$(date +%s)
check goal/g3 slow --timeout 1s -- sh -c 'sleep 37 & sleep 37; echo never' && fail "timeout exited zero"
[ $(( $(date +%s) - start )) -lt 10 ] || fail "timeout did not stop the check"
pgrep -f 'sleep 37' >/dev/null && fail "child survived the timeout"
"$bin" view evidence goal/g3 | grep -q 'slow: TIMEOUT' || fail "timeout not recorded"
"$bin" view evidence goal/g3 | grep -q 'limits: timeout 1s' || fail "limits not recorded"

# --exclusive serialises checks sharing a name.
check goal/g4 a --exclusive heavy -- sh -c 'echo a >>"$0"; sleep 1; echo a >>"$0"' "$tmp/order" &
sleep 0.3
check goal/g4 b --exclusive heavy -- sh -c 'echo b >>"$0"; echo b >>"$0"' "$tmp/order"
wait
[ "$(tr -d '\n' <"$tmp/order")" = "aabb" ] || fail "exclusive overlapped: $(tr -d '\n' <"$tmp/order")"

# --mem and --cpu run the check in a systemd user scope carrying those limits.
if systemd-run --user --scope --quiet --collect true 2>/dev/null; then
	check goal/g5 limits --mem 200M --cpu 50% --tail -- sh -c 'd=/sys/fs/cgroup$(cut -d: -f3 /proc/self/cgroup); cat $d/memory.max $d/cpu.max' || fail "scoped check"
	"$bin" view evidence goal/g5 | grep -q 'limits: mem 200M, cpu 50%' || fail "scope limits not recorded"
	"$bin" run evidence require goal/g5 limits >/dev/null || fail "scoped check not passing"
	grep -q '^209715200$' "$tmp/check.err" || fail "MemoryMax not applied: $(tail -3 "$tmp/check.err")"
	grep -q '^50000 100000$' "$tmp/check.err" || fail "CPUQuota not applied"
fi

# Bad input refuses without appending.
n=$(events)
"$bin" run evidence record goal/x tests 2>/dev/null && fail "record without --exit"
"$bin" run evidence require goal/x 2>/dev/null && fail "require without a check"
python3 "$here/self-check" goal/x t 2>/dev/null && fail "self-check without --"
[ "$(events)" = "$n" ] || fail "refusals appended"
echo "evidence: ok"
