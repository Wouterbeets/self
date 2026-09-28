#!/bin/sh
# End-to-end check of the lease command and leases view against a fresh
# instance, using the self binary in $SELF_BIN (default: build this checkout).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
bin=${SELF_BIN:-}
if [ -z "$bin" ]; then
	(cd "$here/../.." && go build -o "$tmp/self" ./cmd/self 2>/dev/null || go build -o "$tmp/self" .)
	bin=$tmp/self
fi
export SELF_HOME=$tmp/home
s() { "$bin" "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

jq -nc '{name:"command.declared",payload:{name:"lease",summary:"Hold exclusive expiring ownership of a named resource",description:"see script",atomic:true}}
        ,{name:"view.declared",payload:{name:"leases",summary:"Live leases or one resource history",description:"see script",consumes:["lease.acquired","lease.renewed","lease.released"]}}' | s hear >/dev/null 2>&1
jq -nc --rawfile c "$here/command.py" --rawfile v "$here/view.py" \
	'{name:"script.authored",payload:{type:"command",name:"lease",script:$c}},{name:"script.authored",payload:{type:"view",name:"leases",script:$v}}' | s hear >/dev/null 2>&1

# Concurrent acquisition by ten holders: exactly one winner.
for i in 0 1 2 3 4 5 6 7 8 9; do
	(SELF_CALLER=w$i s run lease acquire goal/x --ttl 10m >/dev/null 2>&1 && echo won >"$tmp/won.$i" || true) &
done
wait
won=$(ls "$tmp" | grep -c '^won\.' || true)
[ "$won" = 1 ] || fail "expected one winner, got $won"
winner=$(ls "$tmp" | grep '^won\.' | sed 's/won\./w/')
acq=$(s view leases goal/x | grep -c ' acquired ' || true)
[ "$acq" = 1 ] || fail "expected one lease.acquired, got $acq"

# Others are refused with the holder named; the holder renews and releases.
SELF_CALLER=other s run lease acquire goal/x 2>"$tmp/err" && fail "second holder acquired"
grep -q "held by $winner" "$tmp/err" || fail "refusal does not name holder: $(cat "$tmp/err")"
SELF_CALLER=other s run lease renew goal/x 2>/dev/null && fail "non-holder renewed"
SELF_CALLER=$winner s run lease renew goal/x --ttl 1h | grep -q lease.renewed || fail "renew"
s view leases | grep -q "^goal/x	$winner	until" || fail "view does not list live lease"
SELF_CALLER=other s run lease release goal/x 2>/dev/null && fail "non-holder released"
SELF_CALLER=$winner s run lease release goal/x | grep -q lease.released || fail "release"
s view leases | grep -q 'no live leases' || fail "released lease still listed"
SELF_CALLER=other s run lease acquire goal/x --ttl 1s --note "short" | grep -q lease.acquired || fail "reacquire after release"

# Expiry: a crashed holder's lease lapses, and only --steal takes it.
sleep 2
SELF_CALLER=$winner s run lease renew goal/x 2>/dev/null && fail "renewed someone else's lease"
SELF_CALLER=third s run lease acquire goal/x 2>"$tmp/err" && fail "took expired lease without --steal"
grep -q 'pass --steal' "$tmp/err" || fail "expired refusal unclear"
SELF_CALLER=third s run lease acquire goal/x --steal | grep -q stolen_from || fail "steal"
s view leases goal/x | grep -q 'acquired third.*stolen_from=other' || fail "history lacks steal"

# Malformed arguments fail without appending.
n=$(s view log --all | wc -l)
SELF_CALLER=a s run lease acquire goal/y --ttl 0m 2>/dev/null && fail "zero ttl accepted"
SELF_CALLER=a s run lease bogus goal/y 2>/dev/null && fail "unknown verb accepted"
s run lease acquire goal/y --holder '' 2>/dev/null && fail "empty holder accepted"
[ "$(s view log --all | wc -l)" = "$n" ] || fail "refusals appended"
echo "lease: all checks passed (winner $winner)"
