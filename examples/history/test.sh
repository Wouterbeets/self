#!/bin/sh
# End-to-end check of history against fixture transcripts for Claude Code, pi,
# opencode and Codex, in a scratch index. Reads nothing from the real harnesses.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export SELF_HISTORY_DB=$tmp/index.db SELF_HISTORY_CLAUDE=$tmp/claude \
	SELF_HISTORY_PI=$tmp/pi SELF_HISTORY_OPENCODE=$tmp/opencode.db SELF_HISTORY_CODEX=$tmp/codex \
	CLAUDE_CODE_SESSION_ID= SELF_HISTORY_SESSION=
h() { python3 "$here/self-history" "$@"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

mkdir -p "$tmp/claude/-work-alpha" "$tmp/pi/--work-beta--"
cc=$tmp/claude/-work-alpha/c1.jsonl
big=$(python3 -c 'print("x" * 20000 + " tailmarker")')
jq -nc --arg big "$big" '
	{type:"user",sessionId:"c1",cwd:"/work/alpha",timestamp:"2026-10-01T10:00:00Z",message:{role:"user",content:"pick the zebrafish migration plan"}},
	{type:"assistant",sessionId:"c1",cwd:"/work/alpha",timestamp:"2026-10-01T10:00:01Z",message:{role:"assistant",content:[{type:"tool_use",id:"t1",name:"Bash",input:{command:"cat plan.md"}}]}},
	{type:"user",sessionId:"c1",cwd:"/work/alpha",timestamp:"2026-10-01T10:00:02Z",message:{role:"user",content:[{type:"tool_result",tool_use_id:"t1",content:$big}]}},
	{type:"assistant",sessionId:"c1",cwd:"/work/alpha",timestamp:"2026-10-01T10:00:03Z",message:{role:"assistant",content:[{type:"tool_use",id:"t2",name:"Bash",input:{command:"history search zebrafish"}}]}},
	{type:"user",sessionId:"c1",cwd:"/work/alpha",timestamp:"2026-10-01T10:00:04Z",message:{role:"user",content:[{type:"tool_result",tool_use_id:"t2",content:"echoed zebrafish result"}]}},
	{type:"api-request-blob",message:{role:"user",content:[{type:"text",text:"zebrafish duplicate"}]}}' >"$cc"
jq -nc '
	{type:"session",id:"p1",cwd:"/work/beta",timestamp:"2026-10-02T09:00:00Z"},
	{type:"message",timestamp:"2026-10-02T09:00:01Z",message:{role:"user",content:[{type:"text",text:"why did we reject the okapi approach"}]}},
	{type:"message",timestamp:"2026-10-02T09:00:02Z",message:{role:"assistant",content:[{type:"toolCall",id:"k1",name:"bash",arguments:{command:"git log okapi"}}]}},
	{type:"message",timestamp:"2026-10-02T09:00:03Z",message:{role:"toolResult",toolCallId:"k1",content:[{type:"text",text:"okapi reverted: too slow"}]}}' \
	>"$tmp/pi/--work-beta--/p1.jsonl"
python3 - "$tmp/opencode.db" <<'PY'
import json, sqlite3, sys
db = sqlite3.connect(sys.argv[1])
db.executescript("""
CREATE TABLE session(id TEXT PRIMARY KEY, parent_id TEXT, directory TEXT, title TEXT, time_created INT, time_updated INT);
CREATE TABLE message(id TEXT PRIMARY KEY, session_id TEXT, time_created INT, data TEXT);
CREATE TABLE part(id TEXT PRIMARY KEY, message_id TEXT, session_id TEXT, time_created INT, time_updated INT, data TEXT);
""")
db.execute("INSERT INTO session VALUES ('o1', NULL, '/work/gamma', 'Narwhal rollout', 1791000000000, 1791000003000)")
db.execute("INSERT INTO message VALUES ('m1', 'o1', 1791000001000, ?)", (json.dumps({"role": "user"}),))
db.execute("INSERT INTO message VALUES ('m2', 'o1', 1791000002000, ?)", (json.dumps({"role": "assistant"}),))
db.execute("INSERT INTO part VALUES ('a1', 'm1', 'o1', 1791000001000, 1791000001000, ?)",
           (json.dumps({"type": "text", "text": "ship the narwhal rollout"}),))
db.execute("INSERT INTO part VALUES ('a2', 'm2', 'o1', 1791000002000, 1791000002000, ?)",
           (json.dumps({"type": "tool", "tool": "bash", "state": {"status": "running", "input": {"command": "deploy"}}}),))
db.commit()
PY

mkdir -p "$tmp/codex/2026/10/04"
cx=$tmp/codex/2026/10/04/rollout-2026-10-04T08-00-00-x1.jsonl
jq -nc '
	{timestamp:"2026-10-04T08:00:00Z",type:"session_meta",payload:{id:"x1",cwd:"/work/delta"}},
	{timestamp:"2026-10-04T08:00:01Z",type:"response_item",payload:{type:"message",role:"user",content:[{type:"input_text",text:"<environment_context>pangolin injected</environment_context>"}]}},
	{timestamp:"2026-10-04T08:00:02Z",type:"response_item",payload:{type:"message",role:"user",content:[{type:"input_text",text:"move the axolotl mailbox to the new tenant"}]}},
	{timestamp:"2026-10-04T08:00:03Z",type:"event_msg",payload:{type:"user_message",message:"move the axolotl mailbox to the new tenant"}},
	{timestamp:"2026-10-04T08:00:04Z",type:"response_item",payload:{type:"custom_tool_call",call_id:"q1",name:"exec",input:"dig axolotl.example MX"}},
	{timestamp:"2026-10-04T08:00:05Z",type:"response_item",payload:{type:"custom_tool_call_output",call_id:"q1",output:[{type:"input_text",text:"axolotl.example mail is handled by capybara.example"}]}},
	{timestamp:"2026-10-04T08:00:06Z",type:"response_item",payload:{type:"reasoning",encrypted_content:"tapir"}},
	{timestamp:"2026-10-04T08:00:07Z",type:"response_item",payload:{type:"message",role:"assistant",content:[{type:"output_text",text:"The mailbox now routes through capybara."}]}}' >"$cx"

h search zebrafish | grep -q "user: pick the \[zebrafish\]" || fail "claude prompt not found"
h search axolotl mailbox --role user | grep -q "cx:x1" || fail "codex prompt not found"
[ "$(h search axolotl mailbox --role user | grep -c 'cx:x1')" = 1 ] || fail "codex prompt indexed twice"
h search capybara --role result | grep -q "result:" || fail "codex tool result not found"
h search pangolin | grep -q "no match" || fail "codex injected context indexed"
h search tapir | grep -q "no match" || fail "codex reasoning indexed"
h search capybara --lane /work/delta | grep -q "cx:x1" || fail "codex lane"
id=$(h search capybara --role result | awk -F'\t' 'NR==1{print $1}')
h show "$id" -C 1 | grep -q "dig axolotl.example MX" || fail "codex show is missing the preceding call"
h search zebrafish | grep -q echoed && fail "a history call's result was indexed"
h search duplicate | grep -q "no match" || fail "api-request-blob indexed"
h search okapi too slow | grep -q "result:" || fail "pi tool result not found"
h search narwhal | grep -q "oc:o1" || fail "opencode text not found"
h sessions | grep -q "Narwhal rollout" || fail "opencode title missing"
h search okapi --here | grep -q "no match" || fail "--here should exclude /work/beta"
h search okapi --lane /work/beta | grep -q "pi:p1" || fail "--lane"

# show reads the whole tool output back from the transcript, not the clipped index.
id=$(h search tailmarker | awk -F'\t' 'NR==1{print $1}')
h show "$id" --max 30000 | grep -q tailmarker || fail "show is missing the tail"
h show "$id" -C 1 | grep -q "cat plan.md" || fail "show is missing the preceding call"

# Appended lines are picked up; a finished opencode tool gains its output.
jq -nc '{type:"assistant",sessionId:"c1",cwd:"/work/alpha",timestamp:"2026-10-01T11:00:00Z",message:{role:"assistant",content:[{type:"text",text:"chose the quokka variant"}]}}' >>"$cc"
python3 - "$tmp/opencode.db" <<'PY'
import json, sqlite3, sys
db = sqlite3.connect(sys.argv[1])
db.execute("UPDATE part SET time_updated=1791000009000, data=? WHERE id='a2'",
           (json.dumps({"type": "tool", "tool": "bash", "state": {"status": "completed", "input": {"command": "deploy"}, "output": "walrus deployed"}}),))
db.commit()
PY
h search quokka | grep -q "assistant:" || fail "appended line not indexed"
h search walrus | grep -q "result:" || fail "completed opencode tool not reindexed"
[ "$(h search deploy --role tool | grep -c 'tool:')" = 1 ] || fail "opencode part indexed twice"

# A rewritten (shorter) transcript is reindexed from scratch.
jq -nc '{type:"user",sessionId:"c1",cwd:"/work/alpha",timestamp:"2026-10-03T10:00:00Z",message:{role:"user",content:"only the ibex remains"}}' >"$cc"
h search ibex | grep -q ibex || fail "rewritten file not reindexed"
h search zebrafish | grep -q "no match" || fail "rewritten file kept stale rows"

# The caller's own session is left out.
SELF_HISTORY_SESSION=c1 h search ibex | grep -q "no match" || fail "own session not excluded"

echo "history: all checks passed"
