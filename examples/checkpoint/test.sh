#!/bin/sh
# End-to-end check of the checkpoint command and view against a fresh instance,
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
export SELF_HOME=$tmp/home
loop() { SELF_CALLER=loop "$bin" "$@"; }
wouter() { SELF_CALLER=wouter "$bin" "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }
events() { loop view log --all | wc -l; }

jq -nc '{name:"command.declared",payload:{name:"checkpoint",summary:"Propose exact actions for a human decision; use each approved action once",description:"see script",atomic:true}}
        ,{name:"view.declared",payload:{name:"checkpoint",summary:"Actions awaiting a human decision, and what approvals still cover",description:"see script",consumes:["checkpoint.proposed","checkpoint.approved","checkpoint.rejected","checkpoint.withdrawn","checkpoint.used"]}}' | loop hear >/dev/null 2>&1
jq -nc --rawfile c "$here/command.py" --rawfile v "$here/view.py" \
	'{name:"script.authored",payload:{type:"command",name:"checkpoint",script:$c}},{name:"script.authored",payload:{type:"view",name:"checkpoint",script:$v}}' | loop hear >/dev/null 2>&1

# A proposal is pending and actionable; the proposer cannot approve or use it.
loop run checkpoint propose cp1 "open PR from self/budget" "push self/budget" --note "pass 5" | grep -q checkpoint.proposed || fail propose
loop run checkpoint propose cp1 "open PR from self/budget" "push self/budget" | grep -q . && fail "identical re-propose appended"
loop run checkpoint propose cp1 "merge" 2>/dev/null && fail "conflicting re-propose accepted"
loop view checkpoint | grep -q 'decide: self run checkpoint approve cp1' || fail "view pending: $(loop view checkpoint)"
loop run checkpoint approve cp1 2>/dev/null && fail "loop approved"
loop run checkpoint use cp1 "push self/budget" 2>/dev/null && fail "used before approval"

# A forged approval appended under the loop's own name does not count.
jq -nc '{name:"checkpoint.approved",payload:{id:"cp1",actions:["push self/budget"],expires:"2999-01-01T00:00:00Z"}}' | loop hear >/dev/null
loop run checkpoint use cp1 "push self/budget" 2>/dev/null && fail "forged approval honoured"
loop view checkpoint cp1 | grep -q 'ignored: not an approver' || fail "forgery not flagged: $(loop view checkpoint cp1)"

# The approver's approval covers only the recorded actions, each once, even under concurrency.
wouter run checkpoint approve cp1 --ttl 2h | grep -q checkpoint.approved || fail approve
loop run checkpoint use cp1 "merge" 2>"$tmp/err" && fail "unrecorded action used"
grep -q 'not among the approved actions' "$tmp/err" || fail "refusal unclear: $(cat "$tmp/err")"
for i in 0 1 2 3 4 5 6 7 8 9; do
	(loop run checkpoint use cp1 "push self/budget" >"$tmp/out.$i" 2>/dev/null || true) &
done
wait
used=$(cat "$tmp"/out.* | grep -c checkpoint.used || true)
[ "$used" = 1 ] || fail "expected 1 use of the action, got $used"
n=$(events)
loop run checkpoint use cp1 "push self/budget" 2>/dev/null && fail "second use"
[ "$(events)" = "$n" ] || fail "refused use appended"
loop view checkpoint | grep -q -- '- open PR from self/budget' || fail "view approved: $(loop view checkpoint)"
loop run checkpoint use cp1 "open PR from self/budget" | grep -q '"left":0' || fail "last use"
loop view checkpoint | grep -q 'no open checkpoints' || fail "spent still open: $(loop view checkpoint)"
loop view checkpoint --all | grep -q 'cp1  \[spent\]' || fail "spent not closed: $(loop view checkpoint --all)"

# Rejection needs a reason, keeps it, and blocks use; an approver cannot then approve.
loop run checkpoint propose cp2 "comment on PR 12" | grep -q proposed || fail propose2
wouter run checkpoint reject cp2 2>/dev/null && fail "reject without reason"
wouter run checkpoint reject cp2 --reason "no GitHub comments from loops" | grep -q rejected || fail reject
wouter run checkpoint approve cp2 2>/dev/null && fail "approved after rejection"
loop run checkpoint use cp2 "comment on PR 12" 2>/dev/null && fail "rejected used"
loop view checkpoint cp2 | grep -q 'reason: no GitHub comments from loops' || fail "reason lost"
loop run checkpoint propose cp2 "comment on PR 12" 2>/dev/null && fail "reused id after rejection"

# An expired approval cannot be used.
loop run checkpoint propose cp3 "rerun deploy" >/dev/null
jq -nc '{name:"checkpoint.approved",payload:{id:"cp3",actions:["rerun deploy"],expires:"2000-01-01T00:00:00Z"}}' | wouter hear >/dev/null
loop run checkpoint use cp3 "rerun deploy" 2>"$tmp/err" && fail "expired approval used"
grep -q expired "$tmp/err" || fail "expiry unclear: $(cat "$tmp/err")"

# Withdrawal: the proposer or an approver, never a third caller.
loop run checkpoint propose cp4 "delete branch x" >/dev/null
SELF_CALLER=other "$bin" run checkpoint withdraw cp4 2>/dev/null && fail "third caller withdrew"
loop run checkpoint withdraw cp4 --reason "not needed" | grep -q withdrawn || fail withdraw
wouter run checkpoint approve cp4 2>/dev/null && fail "approved after withdrawal"

# Malformed arguments fail without appending.
n=$(events)
loop run checkpoint propose cp5 2>/dev/null && fail "empty proposal"
loop run checkpoint propose cp5 a a 2>/dev/null && fail "duplicate actions"
loop run checkpoint use nope x 2>/dev/null && fail "unknown id"
loop run checkpoint bogus cp1 2>/dev/null && fail "unknown verb"
wouter run checkpoint approve cp1 --ttl 0h 2>/dev/null && fail "bad ttl"
[ "$(events)" = "$n" ] || fail "refusals appended"
echo "checkpoint: all checks passed"
