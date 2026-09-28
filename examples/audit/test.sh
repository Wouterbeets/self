#!/bin/sh
# End-to-end check of the audit view: installed on a fresh instance through the
# kernel (attribution of callers and kernel receipts), and fed a synthetic log
# directly (run splitting by quiet gaps and loop.settled, since the kernel
# stamps real time). Uses the self binary in $SELF_BIN (default: build this checkout).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
bin=${SELF_BIN:-}
if [ -z "$bin" ]; then
	(cd "$here/../.." && go build -o "$tmp/self" ./cmd/self 2>/dev/null || go build -o "$tmp/self" .)
	bin=$tmp/self
fi
export SELF_HOME=$tmp/home XDG_RUNTIME_DIR=$tmp
fail() { echo "FAIL: $*" >&2; exit 1; }
hear() { SELF_CALLER=$1 "$bin" hear >/dev/null 2>&1; }

jq -nc '{name:"view.declared",payload:{name:"audit",summary:"What one caller run did",description:"see script",consumes:["*"]}}' | hear setup
jq -nc --rawfile v "$here/view.py" '{name:"script.authored",payload:{type:"view",name:"audit",script:$v}}' | hear setup
"$bin" view audit >/dev/null || fail "audit not installed: $("$bin" view log)"

# A loop pass: an intent, a goal, a capability the kernel installs, evidence, a
# spoken line with a reply, a worker dispatched on its behalf, and a settle.
jq -nc '{name:"intent.declared",payload:{name:"demo/x",summary:"x",description:"Built on branch self/demo, commit abc1234."}}
       ,{name:"goal.created",payload:{goal:"g1",title:"Do the thing",status:"active"}}
       ,{name:"command.declared",payload:{name:"hello",summary:"hi",description:"usage: hello"}}' | hear loop-a
jq -nc '{name:"script.authored",payload:{type:"command",name:"hello",script:"#!/bin/sh\necho\n"}}' | hear loop-a
jq -nc '{name:"herdr.doing",payload:{tab:"w1:t1",text:"Doing the thing"}}
       ,{name:"evidence.checked",payload:{subject:"goal/g1",check:"tests",passed:true,exit:0}}' | hear loop-a
jq -nc '{name:"agent.started",payload:{agent:"worker-1",goal:"g1",dispatched_by:"loop-a"}}
       ,{name:"attention.spoken",payload:{tab:"w1:t1",text:"The thing is ready for review"}}' | hear wouter
jq -nc '{name:"goal.progress",payload:{goal:"other",report:"unrelated work"}}' | hear loop-b
jq -nc '{name:"attention.heard",payload:{text:"Ship it",after:"The thing is ready for review"}}
       ,{name:"attention.heard",payload:{text:"Not for you",after:"Some other tab said this"}}' | hear wouter
jq -nc '{name:"loop.settled",payload:{reason:"Thing done, waits on review"}}' | hear loop-a
jq -nc '{name:"goal.progress",payload:{goal:"g1",report:"next run"}}' | hear loop-a

out=$("$bin" view audit loop-a --run 2)
echo "$out" | grep -q '^# Audit: loop-a, run 2 of 2' || fail "settle did not split runs: $out"
echo "$out" | grep -q 'declared demo/x' || fail "intent missing: $out"
echo "$out" | grep -q 'created g1 Do the thing' || fail "goal missing: $out"
echo "$out" | grep -q 'script.installed command/hello' || fail "kernel receipt not attributed: $out"
echo "$out" | grep -q 'checked goal/g1 tests pass' || fail "evidence missing: $out"
echo "$out" | grep -q 'agent.started worker-1' || fail "dispatched worker not attributed: $out"
echo "$out" | grep -q 'spoke: The thing is ready' || fail "outward speech (by tab) missing: $out"
echo "$out" | grep -q 'wouter replied: Ship it' || fail "human reply missing: $out"
echo "$out" | grep -q 'Not for you' && fail "reply to another tab attributed: $out"
echo "$out" | grep -q 'unrelated work' && fail "another caller's event attributed: $out"
echo "$out" | grep -q 'branch self/demo' || fail "branch ref missing: $out"
echo "$out" | grep -q 'commit abc1234' || fail "commit ref missing: $out"
echo "$out" | grep -q 'Thing done, waits on review' || fail "settle reason missing: $out"
"$bin" view audit loop-a | grep -q 'progress g1 next run' || fail "latest run wrong: $("$bin" view audit loop-a)"
"$bin" view audit loop-a --list | grep -q '2 runs of loop-a' || fail "list"
"$bin" view audit | grep -q '^- loop-b ' || fail "overview misses loop-b: $("$bin" view audit)"
"$bin" view audit --run 2 >/dev/null 2>&1 && fail "--run without caller accepted"
"$bin" view audit nobody | grep -q 'No events by nobody' || fail "unknown caller"

# Quiet gaps split runs; --gap widens them; --seq pins a range.
cat >"$tmp/log.jsonl" <<'EOF'
{"seq":1,"name":"goal.progress","by":"m","occurred_at":"2026-01-01T10:00:00Z","payload":{"goal":"a","report":"one"}}
{"seq":2,"name":"goal.progress","by":"m","occurred_at":"2026-01-01T10:10:00Z","payload":{"goal":"a","report":"two"}}
{"seq":3,"name":"goal.progress","by":"m","occurred_at":"2026-01-01T11:00:00Z","payload":{"goal":"a","report":"three"}}
EOF
v() { python3 "$here/view.py" "$@" <"$tmp/log.jsonl"; }
v m --list | grep -q '2 runs of m' || fail "gap split: $(v m --list)"
v m --list --gap 60 | grep -q '1 runs of m' || fail "--gap: $(v m --list --gap 60)"
v m --seq 2-2 | grep -q 'two' || fail "--seq"
v m --seq 2-2 | grep -q 'one' && fail "--seq leaked"
echo "audit: all checks passed"
