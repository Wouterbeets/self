#!/bin/sh
# Reproducible prompt A/B harness. The control arm is HEAD; treatment is the
# current worktree, so uncommitted prompt changes can be measured safely.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
MIND=${MIND:-"$ROOT/examples/mind-stub"}
KEEP=${KEEP:-0}
CONTROL_REF=${CONTROL_REF:-4410bcd}
TMP=$(mktemp -d "${TMPDIR:-/tmp}/self-prompt-ab.XXXXXX")
cleanup() {
  if [ "$KEEP" = 1 ]; then
    printf '%s\n' "artifacts kept at $TMP" >&2
  else
    rm -rf "$TMP"
  fi
}
trap cleanup EXIT INT TERM

mkdir -p "$TMP/control" "$TMP/treatment"
# Build the two arms from isolated source trees. The treatment build includes
# the caller's current worktree; the control build is the committed baseline.
( cd "$ROOT" && go build -o "$TMP/treatment/self" . )
git -C "$ROOT" archive "$CONTROL_REF" | tar -x -C "$TMP/control"
( cd "$TMP/control" && go build -o self . )

run_arm() {
  arm=$1
  bin=$2
  home="$TMP/$arm/home"
  out="$TMP/$arm"
  mkdir -p "$home"
  # Seed both equivalent homes through the public write door, not by editing
  # events.jsonl. A declaration deliberately leaves one capability pending.
  printf '%s\n' '{"name":"command.declared","payload":{"name":"entry","description":"append an entry"}}' |
    SELF_HOME="$home" SELF_CALLER="ab-harness" "$bin" hear >"$out/seed.out" 2>"$out/seed.err"

  pass=1
  SELF_HOME="$home" SELF_CALLER="ab-harness" "$bin" "record one" >"$out/prompt-$pass.txt"
  wc -c <"$out/prompt-$pass.txt" | tr -d ' ' >"$out/prompt-$pass.bytes"
  if "$MIND" <"$out/prompt-$pass.txt" >"$out/mind-$pass.jsonl" 2>"$out/mind-$pass.err"; then echo 0 >"$out/mind-$pass.status"; else echo $? >"$out/mind-$pass.status"; fi
  if SELF_HOME="$home" SELF_CALLER="ab-harness" "$bin" hear <"$out/mind-$pass.jsonl" >"$out/hear-$pass.out" 2>"$out/hear-$pass.err"; then echo 0 >"$out/hear-$pass.status"; else echo $? >"$out/hear-$pass.status"; fi

  pass=2
  SELF_HOME="$home" SELF_CALLER="ab-harness" "$bin" "record one" >"$out/prompt-$pass.txt"
  wc -c <"$out/prompt-$pass.txt" | tr -d ' ' >"$out/prompt-$pass.bytes"
  if "$MIND" <"$out/prompt-$pass.txt" >"$out/mind-$pass.jsonl" 2>"$out/mind-$pass.err"; then echo 0 >"$out/mind-$pass.status"; else echo $? >"$out/mind-$pass.status"; fi
  if SELF_HOME="$home" SELF_CALLER="ab-harness" "$bin" hear <"$out/mind-$pass.jsonl" >"$out/hear-$pass.out" 2>"$out/hear-$pass.err"; then echo 0 >"$out/hear-$pass.status"; else echo $? >"$out/hear-$pass.status"; fi

  SELF_HOME="$home" "$bin" view log >"$out/log.txt"
  awk 'END { print NR+0 }' "$home/events.jsonl" >"$out/events.lines"
}

run_arm control "$TMP/control/self"
run_arm treatment "$TMP/treatment/self"

for arm in control treatment; do
  test "$(cat "$TMP/$arm/events.lines")" = 2
  test "$(awk 'END { print NR+0 }' "$TMP/$arm/mind-1.jsonl")" = 1
  test "$(awk 'END { print NR+0 }' "$TMP/$arm/mind-2.jsonl")" = 0
  test "$(cat "$TMP/$arm/mind-1.status")" = 0
  test "$(cat "$TMP/$arm/mind-2.status")" = 0
  test "$(cat "$TMP/$arm/hear-1.status")" = 0
  test "$(cat "$TMP/$arm/hear-2.status")" = 0
done

# Structural goal dependency fixture. Install the experiment's goal command and
# next view locally, then prove blocked work cannot progress until its dependency
# closes. This is domain capability behavior, not kernel vocabulary.
dependency_home="$TMP/dependencies/home"
dependency_out="$TMP/dependencies"
mkdir -p "$dependency_home"
jq -nc '{name:"command.declared",payload:{name:"goal",description:"dependency fixture"}}, {name:"script.authored",payload:{type:"command",name:"goal",script:$script}}' \
  --rawfile script "$ROOT/experiments/situated-prompt-diet/goal-capabilities/goal.py" |
  SELF_HOME="$dependency_home" SELF_CALLER="ab-harness" "$TMP/treatment/self" hear >"$dependency_out/install-goal.out"
jq -nc '{name:"view.declared",payload:{name:"next",description:"dependency fixture",consumes:["goal.created","goal.updated","goal.removed","goal.progress","goal.milestone","goal.compacted"]}}, {name:"script.authored",payload:{type:"view",name:"next",script:$script}}' \
  --rawfile script "$ROOT/experiments/situated-prompt-diet/goal-capabilities/next.py" |
  SELF_HOME="$dependency_home" SELF_CALLER="ab-harness" "$TMP/treatment/self" hear >"$dependency_out/install-next.out"
jq -nc '{name:"view.declared",payload:{name:"context",description:"dependency fixture",consumes:["goal.created","goal.updated","goal.removed","goal.progress","goal.milestone","goal.kpi","goal.compacted"]}}, {name:"script.authored",payload:{type:"view",name:"context",script:$script}}' \
  --rawfile script "$ROOT/experiments/situated-prompt-diet/goal-capabilities/context.py" |
  SELF_HOME="$dependency_home" SELF_CALLER="ab-harness" "$TMP/treatment/self" hear >"$dependency_out/install-context.out"
SELF_HOME="$dependency_home" SELF_CALLER="ab-harness" "$TMP/treatment/self" run goal create build "Build" "built" >"$dependency_out/create-build.out"
SELF_HOME="$dependency_home" SELF_CALLER="ab-harness" "$TMP/treatment/self" run goal create qa "QA" "verified" "" "" build >"$dependency_out/create-qa.out"
SELF_HOME="$dependency_home" "$TMP/treatment/self" view next >"$dependency_out/before.txt"
SELF_HOME="$dependency_home" "$TMP/treatment/self" view context >"$dependency_out/context-guide.txt"
if SELF_HOME="$dependency_home" SELF_CALLER="ab-harness" "$TMP/treatment/self" run goal depends build missing >"$dependency_out/unknown.out" 2>"$dependency_out/unknown.err"; then
  echo "unknown dependency unexpectedly accepted" >&2
  exit 1
fi
if SELF_HOME="$dependency_home" SELF_CALLER="ab-harness" "$TMP/treatment/self" run goal depends build qa >"$dependency_out/cycle.out" 2>"$dependency_out/cycle.err"; then
  echo "dependency cycle unexpectedly accepted" >&2
  exit 1
fi
if SELF_HOME="$dependency_home" SELF_CALLER="ab-harness" "$TMP/treatment/self" run goal progress qa "too early" >"$dependency_out/blocked.out" 2>"$dependency_out/blocked.err"; then
  echo "blocked goal unexpectedly accepted progress" >&2
  exit 1
fi
if SELF_HOME="$dependency_home" SELF_CALLER="ab-harness" "$TMP/treatment/self" run goal close qa "too early" >"$dependency_out/blocked-close.out" 2>"$dependency_out/blocked-close.err"; then
  echo "blocked goal unexpectedly accepted close" >&2
  exit 1
fi
SELF_HOME="$dependency_home" SELF_CALLER="ab-harness" "$TMP/treatment/self" run goal close build "done" >"$dependency_out/close-build.out"
SELF_HOME="$dependency_home" "$TMP/treatment/self" view next >"$dependency_out/after.txt"
grep -q '## Blocked' "$dependency_out/before.txt"
grep -q 'qa' "$dependency_out/before.txt"
grep -q 'waiting on: build' "$dependency_out/before.txt"
grep -q '# Goal context guide' "$dependency_out/context-guide.txt"
grep -q 'qa <- build' "$dependency_out/context-guide.txt"
grep -q 'build: Build' "$dependency_out/context-guide.txt"
grep -q 'blocked by: build' "$dependency_out/blocked.err"
grep -q 'blocked by: build' "$dependency_out/blocked-close.err"
grep -q 'unknown dependencies: missing' "$dependency_out/unknown.err"
grep -q 'create a cycle' "$dependency_out/cycle.err"
grep -A4 '## Actionable' "$dependency_out/after.txt" | grep -q 'qa'

result="$ROOT/experiments/situated-prompt-diet/result.txt"
{
  echo "Situated prompt diet A/B (deterministic pending-capability fixture)"
  echo "Generated by: experiments/situated-prompt-diet/run.sh"
  echo "Control: $CONTROL_REF prompt; treatment: current worktree prompt"
  echo "Mind: $MIND"
  echo
  for arm in control treatment; do
    echo "$arm"
    echo "  pass 1 prompt bytes: $(cat "$TMP/$arm/prompt-1.bytes")"
    echo "  pass 2 prompt bytes: $(cat "$TMP/$arm/prompt-2.bytes")"
    echo "  appended event lines: $(cat "$TMP/$arm/events.lines")"
    echo "  pass 1 mind output lines: $(awk 'END { print NR+0 }' "$TMP/$arm/mind-1.jsonl")"
    echo "  pass 2 mind output lines: $(awk 'END { print NR+0 }' "$TMP/$arm/mind-2.jsonl")"
    echo "  mind exit statuses: pass 1=$(cat "$TMP/$arm/mind-1.status"), pass 2=$(cat "$TMP/$arm/mind-2.status")"
    echo "  hear exit statuses: pass 1=$(cat "$TMP/$arm/hear-1.status"), pass 2=$(cat "$TMP/$arm/hear-2.status")"
    echo "  passes: 2 (pending authoring, then unchanged-state check)"
    echo "  artifacts: prompt-1.txt prompt-2.txt log.txt"
  done
  echo
  echo "The first prompt is the pending path; after the stub authors the command,"
  echo "the second prompt is the ordinary path. Exact prompt bytes are retained"
  echo "only with KEEP=1 to keep this checked-in result deterministic."
  echo "Structural dependency fixture: blocked progress rejected; qa became actionable after build closed."
} >"$result"
cat "$result"
