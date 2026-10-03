#!/bin/sh
# End-to-end check of the budget command and view against a fresh instance,
# using the self binary in $SELF_BIN (default: build this checkout).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
bin=${SELF_BIN:-}
if [ -z "$bin" ]; then
	(cd "$here/../.." && go build -o "$tmp/self" ./cmd/self 2>/dev/null || go build -o "$tmp/self" .)
	bin=$tmp/self
fi
export SELF_HOME=$tmp/home SELF_CALLER=loop
s() { "$bin" "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

jq -nc '{name:"command.declared",payload:{name:"budget",summary:"Spend bounded counters per scope and class",description:"see script",atomic:true,consumes:["budget.set","budget.spent","budget.exhausted"]}}
        ,{name:"view.declared",payload:{name:"budget",summary:"Spent and remaining budget per scope",description:"see script",consumes:["budget.set","budget.spent","budget.exhausted"]}}' | s hear >/dev/null 2>&1
jq -nc --rawfile c "$here/command.py" --rawfile v "$here/view.py" \
	'{name:"script.authored",payload:{type:"command",name:"budget",script:$c}},{name:"script.authored",payload:{type:"view",name:"budget",script:$v}}' | s hear >/dev/null 2>&1

# Ten concurrent spends against a limit of 3: exactly three succeed, one
# exhaustion is recorded, the rest append nothing.
s run budget set pass/1 dispatches 3 | grep -q budget.set || fail "set"
for i in 0 1 2 3 4 5 6 7 8 9; do
	(s run budget spend pass/1 dispatches --note "w$i" >"$tmp/out.$i" 2>/dev/null || true) &
done
wait
spent=$(cat "$tmp"/out.* | grep -c budget.spent || true)
exh=$(cat "$tmp"/out.* | grep -c budget.exhausted || true)
[ "$spent" = 3 ] || fail "expected 3 spends, got $spent"
[ "$exh" = 1 ] || fail "expected 1 exhaustion, got $exh"
s view budget pass/1 | grep -q 'dispatches	3/3 used	0 left  EXHAUSTED' || fail "view: $(s view budget pass/1)"

# Once exhausted, spends exit 3 and append nothing; other classes and scopes are independent.
n=$(s view log --all | wc -l)
s run budget spend pass/1 dispatches 2>"$tmp/err" && fail "spent past exhaustion"
grep -q 'exhausted: 3 of 3 used' "$tmp/err" || fail "refusal unclear: $(cat "$tmp/err")"
[ "$(s view log --all | wc -l)" = "$n" ] || fail "repeat refusal appended"
s run budget spend pass/1 files --n 4 --limit 10 | grep -q budget.spent || fail "inline limit"
s run budget spend pass/1 files --n 6 | grep -q budget.spent || fail "spend to the limit"
s run budget spend pass/1 files --limit 12 2>/dev/null && fail "conflicting inline limit accepted"
s run budget spend pass/2 dispatches --limit 1 | grep -q budget.spent || fail "new scope"

# A spend larger than what is left is refused whole.
s run budget spend pass/2 goals --limit 2 --n 3 2>/dev/null | grep -q budget.exhausted || fail "oversized spend not refused"

# Raising the limit reopens the class; an unchanged set appends nothing.
s run budget set pass/1 dispatches 4 | grep -q budget.set || fail "raise"
s run budget spend pass/1 dispatches | grep -q budget.spent || fail "spend after raise"
n=$(s view log --all | wc -l)
s run budget set pass/1 dispatches 4 >/dev/null
[ "$(s view log --all | wc -l)" = "$n" ] || fail "unchanged set appended"
s view budget | head -1 | grep -q '^pass/1$' || fail "overview order: $(s view budget)"

# Malformed arguments fail without appending.
s run budget spend pass/3 x 2>/dev/null && fail "spend without limit accepted"
s run budget spend pass/3 x --limit 1 --n 0 2>/dev/null && fail "zero spend accepted"
s run budget set pass/3 x -1 2>/dev/null && fail "negative limit accepted"
s run budget bogus pass/3 x 2>/dev/null && fail "unknown verb accepted"
[ "$(s view log --all | wc -l)" = "$n" ] || fail "refusals appended"
echo "budget: all checks passed"
