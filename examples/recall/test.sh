#!/bin/sh
# End-to-end check of recall: install, hooks, compaction and the view against a
# fresh instance, with a stub mind in place of a model. Uses the self binary in
# $SELF_BIN (default: build this checkout).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
bin=${SELF_BIN:-}
if [ -z "$bin" ]; then
	(cd "$here/../.." && go build -o "$tmp/self" .)
	bin=$tmp/self
fi
fail() { echo "FAIL: $*" >&2; exit 1; }
export SELF_HOME=$tmp/home SELF_BIN=$bin SELF_CALLER=test
export SELF_RECALL_LINE=120 SELF_RECALL_LOW=2000 SELF_RECALL_HIGH=3000
hook=$here/claude-hook
v() { "$bin" view recall --line 120 "$@"; }

# The stub summarizes the first 60 bytes of its input; the first time it sees a
# prompt it answers too long, so every summary also proves the retry.
cat >"$tmp/stub" <<'EOF'
#!/usr/bin/env python3
import re, sys
p = sys.stdin.read()
if 'previous reply' not in p:
    print('x' * 500)
    sys.exit(0)
m = re.search(r'<input>\n(.*?)\n</input>', p, re.S)
print('S[' + ' '.join(m.group(1).split())[:60] + ']')
EOF
chmod +x "$tmp/stub"
export SELF_RECALL_MIND=$tmp/stub

# Install twice: one view, one hook per event.
"$here/install.sh" "$tmp/settings.json" >/dev/null
"$here/install.sh" "$tmp/settings.json" >/dev/null
[ "$(jq '[.hooks[] | length] | add' "$tmp/settings.json")" = 6 ] || fail "hooks not idempotent: $(cat "$tmp/settings.json")"
jq -e '.hooks.PostToolUse[0].matcher == "*"' "$tmp/settings.json" >/dev/null || fail "tool matcher"

# Recording: the injected memory is printed before the prompt is recorded.
send() { jq -nc "$@" | "$hook"; }
out=$(send '{hook_event_name:"SessionStart",source:"startup",session_id:"s1",cwd:"/w"}')
echo "$out" | grep -q 'No earlier conversations' || fail "empty memory not injected: $out"
for i in $(seq 1 40); do
	send --arg p "please do task $i" '{hook_event_name:"UserPromptSubmit",prompt:$p,session_id:"s1",cwd:"/w"}' >"$tmp/injected"
	send --arg o "$(seq 1 $((i * 5)) | tr '\n' ' ')" '{hook_event_name:"PostToolUse",tool_name:"Bash",tool_input:{command:"seq"},tool_response:{stdout:$o},session_id:"s1",cwd:"/w"}' >/dev/null
	SELF_RECALL_MIND=false send --arg r "done with task $i" '{hook_event_name:"Stop",last_assistant_message:$r,session_id:"s1",cwd:"/w"}' >/dev/null
done
grep -q '^<recall>' "$tmp/injected" || fail "prompt hook injected nothing"
grep -q 'please do task 40' "$tmp/injected" && fail "prompt recorded before the memory was injected"
send '{hook_event_name:"PostToolUseFailure",tool_name:"Read",tool_input:{file_path:"/nope"},error:"no such file",session_id:"s1",cwd:"/w"}' >/dev/null
send '{hook_event_name:"SubagentStop",agent_type:"Explore",last_assistant_message:"found it in log.go",session_id:"s1",cwd:"/w"}' >/dev/null
SELF_RECALL_SKIP=1 send '{hook_event_name:"UserPromptSubmit",prompt:"nested",session_id:"s2",cwd:"/w"}' | grep -q . && fail "skipped hook printed"
"$bin" view log --all | grep -c chat.message | grep -qx 123 || fail "expected 123 messages: $("$bin" view log --all | grep -c chat.message)"
"$bin" view log --all | grep -q nested && fail "skipped hook recorded"
v 121+1 | grep -q 'Read /nope ✗ no such file' || fail "failure not recorded: $(v 121+1)"
v 122+1 | grep -q 'subagent' || fail "subagent not recorded"

# Garbage input never fails a hook, so it never blocks a prompt.
echo 'not json' | "$hook" 2>/dev/null || fail "hook failed on bad input"

# Compaction: under budget, every line final, old lines coarse, recent fine.
v >"$tmp/before"
[ "$(wc -c <"$tmp/before")" -gt 3000 ] || fail "test needs a memory over budget"
"$hook" --compact
v >"$tmp/after"
n=$(wc -c <"$tmp/after")
[ "$n" -le 3000 ] || fail "memory is $n bytes after compaction"
grep -q '…$' "$tmp/after" && fail "a line is still a draft: $(grep '…$' "$tmp/after")"
grep -q 'xxxxxxxxxx' "$tmp/after" && fail "an over-long reply was kept"
ids=$(grep -o '^[0-9]*+[0-9]*' "$tmp/after")
next=0
for id in $ids; do
	[ "${id%+*}" = "$next" ] || fail "lines do not tile the messages at $id"
	next=$((${id%+*} + ${id#*+}))
done
[ "$next" = 123 ] || fail "lines end at $next"
first=$(echo "$ids" | head -1) last=$(echo "$ids" | tail -1)
[ "${first#*+}" -gt 1 ] && [ "${last#*+}" = 1 ] || fail "old lines should be coarse and recent ones fine: $first .. $last"

# A second compaction finds nothing to do and appends nothing.
c=$("$bin" view log --all | wc -l)
"$hook" --compact
[ "$("$bin" view log --all | wc -l)" = "$c" ] || fail "idle compaction appended"
v --plan --low 2000 --high 3000 | grep -q . && fail "plan not empty when settled"

# Pure: same log, same bytes.
v | cmp -s - "$tmp/after" || fail "view is not deterministic"

# Zoom: a merged line opens into its halves; a leaf into its whole message.
v "$first" | grep -c "^  [0-9]" | grep -qx 2 || fail "zoom $first: $(v "$first")"
v 1+1 | grep -q 'please do task 1$' || fail "zoom leaf: $(v 1+1)"
v 3+2 2>/dev/null && fail "accepted a misaligned node"
v 1024+1024 2>/dev/null && fail "accepted a node past the end"

# A merge whose summary is missing does not apply.
jq -nc '{name:"recall.merged",payload:{id:"112+8"}}' | "$bin" hear >/dev/null
v | cmp -s - "$tmp/after" || fail "merge without a node applied"

# The memory keeps a sawtooth: more messages grow it back over budget, and the
# next compaction brings it under again.
for i in $(seq 41 60); do
	send --arg o "$(seq 1 $((i * 5)) | tr '\n' ' ')" '{hook_event_name:"PostToolUse",tool_name:"Bash",tool_input:{command:"seq"},tool_response:{stdout:$o},session_id:"s1",cwd:"/w"}' >/dev/null
done
"$hook" --compact
[ "$(v | wc -c)" -le 3000 ] || fail "second batch left $(v | wc -c) bytes"
echo "recall: all checks passed"
