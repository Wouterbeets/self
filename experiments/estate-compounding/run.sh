#!/usr/bin/env bash
# estate-compounding — does a grown instance amortize?
#
# Two arms, same mind (opencode over the local llama-server), same task
# sequence:
#   bare   each run starts from a genuinely empty SELF_HOME
#   grown  each run starts from a clone of seed-home — a WHOLE-ESTATE
#          treatment: intent, lesson deposit, declarations, signed installs,
#          the complete learned lineage (grown by the deterministic
#          mind-stub — no LLM in the seed)
#
# Each run executes the tasks SEQUENTIALLY in one home, so later tasks can
# lean on earlier state. Per task we drive the full pipeline in two measured
# phases:
#   ask pass:      self "<ask>" | mind | self hear
#   settling loop: self loop -- mind        (converge pending state)
# Ask-pass and loop costs are reported separately plus combined, because a
# mind may finish the task in the ask pass and spend the loop on voluntary
# estate improvement — different costs, both interesting.
#
# Usage:
#   ./run.sh                      # 3 reps x both arms x all tasks
#   REPS=1 ARMS=grown ./run.sh    # smoke
#   TASKS=t1 REPS=1 ./run.sh      # one task
set -u

ROOT="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$ROOT/../.." && pwd)"
SELF="$ROOT/bin/self"
MIND="$ROOT/bin/mind"
STUB="$REPO/examples/mind-stub"
RUNS="$ROOT/runs"
RESULTS="$ROOT/results.jsonl"

REPS="${REPS:-3}"
ARMS="${ARMS:-bare grown}"
TASKS="${TASKS:-t1 t2 t3}"
MAX_PASSES="${MAX_PASSES:-6}"
MIND_LABEL="${MIND_LABEL:-opencode-qwen3.8-27b}"  # names the mind in results; Home Field varies this
ASK_TIMEOUT="${ASK_TIMEOUT:-10m}"    # the whole ask pass
LOOP_TIMEOUT="${LOOP_TIMEOUT:-20m}"  # the whole settling loop
MIND_TIMEOUT="${MIND_TIMEOUT:-8m}"   # one mind process inside the loop
export PATH="$ROOT/bin:$PATH"

# task table: ask + up to two case-insensitive patterns. Both must match the
# SAME domain event appended during the task (second pattern empty = unused).
ask_t1="Record in my journal: watered the tomato plants and moved the lemon tree to the south wall."
chk1_t1="tomato"; chk2_t1=""
ask_t2="Record in my journal: booked a dentist appointment for Friday morning."
chk1_t2="dentist"; chk2_t2=""
ask_t3="Read this instance's journal and append one journal entry listing every plant mentioned so far."
chk1_t3="tomato"; chk2_t3="lemon"

seed() {
  echo "== seeding estate with mind-stub (no LLM) =="
  rm -rf "$ROOT/seed-home"
  mkdir -p "$ROOT/seed-home"
  (
    export SELF_HOME="$ROOT/seed-home"
    "$SELF" learn "$REPO/lessons/journal" | "$STUB" | "$SELF" hear
    "$SELF" loop --max-passes 6 --timeout 2m -- "$STUB"
  ) || { echo "seed loop did not converge" >&2; exit 1; }
  installed=$(grep -c '"script.installed"' "$ROOT/seed-home/events.jsonl" || true)
  [ "$installed" -ge 2 ] || { echo "seed estate incomplete ($installed installs)" >&2; exit 1; }
  echo "== seed ready: $installed scripts installed =="
}

events_count() { [ -f "$1/events.jsonl" ] && wc -l < "$1/events.jsonl" || echo 0; }

# Domain events only: kernel receipts and growth vocabulary are excluded so a
# script body or declaration echoing the ask cannot score. One payload per line.
domain_payloads() { # file, from-line
  tail -n "+$2" "$1" 2>/dev/null | jq -r '
    select(.name | test("^(script|command|view|lesson|intent|capability|account)[.]") | not)
    | .payload | tostring' 2>/dev/null
}

run_task() { # arm rep task home dir
  local arm="$1" rep="$2" task="$3" home="$4" dir="$5"
  local ask chk1 chk2
  eval "ask=\$ask_$task; chk1=\$chk1_$task; chk2=\$chk2_$task"

  local base mid end t0 t1 t2 conv=1
  base=$(events_count "$home")
  : > "$dir/$task.ask.calls"; : > "$dir/$task.loop.calls"

  t0=$(date +%s)
  (
    export SELF_HOME="$home" BENCH_METRICS="$dir/$task.ask.calls"
    "$SELF" "$ask" | timeout "$ASK_TIMEOUT" "$MIND" | "$SELF" hear
  ) >"$dir/$task.ask.stdout" 2>"$dir/$task.ask.stderr"
  t1=$(date +%s)
  mid=$(events_count "$home")

  (
    export SELF_HOME="$home" BENCH_METRICS="$dir/$task.loop.calls"
    exec timeout "$LOOP_TIMEOUT" "$SELF" loop \
      --max-passes "$MAX_PASSES" --timeout "$MIND_TIMEOUT" -- "$MIND"
  ) >"$dir/$task.loop.stdout" 2>"$dir/$task.loop.stderr" && conv=0
  t2=$(date +%s)
  end=$(events_count "$home")

  local success=false
  if [ "$end" -gt "$base" ]; then
    local hits
    hits=$(domain_payloads "$home/events.jsonl" "$((base + 1))" | grep -i "$chk1" || true)
    if [ -n "$hits" ]; then
      if [ -z "$chk2" ] || printf '%s\n' "$hits" | grep -qi "$chk2"; then
        success=true
      fi
    fi
  fi
  local installs=0
  [ "$end" -gt "$base" ] && installs=$(tail -n "+$((base + 1))" "$home/events.jsonl" | grep -c '"script.installed"' || true)

  jq -cn \
    --arg arm "$arm" --arg task "$task" --arg mind "$MIND_LABEL" \
    --argjson rep "$rep" \
    --argjson conv "$([ $conv -eq 0 ] && echo true || echo false)" \
    --argjson ask_wall "$((t1 - t0))" --argjson loop_wall "$((t2 - t1))" \
    --argjson ask_calls "$(wc -l < "$dir/$task.ask.calls")" \
    --argjson loop_calls "$(wc -l < "$dir/$task.loop.calls")" \
    --argjson ask_events "$((mid - base))" --argjson loop_events "$((end - mid))" \
    --argjson installs "$installs" --argjson success "$success" \
    '{mind:$mind, arm:$arm, rep:$rep, task:$task, converged:$conv, success:$success,
      ask:  {wall_s:$ask_wall,  calls:$ask_calls,  events:$ask_events},
      loop: {wall_s:$loop_wall, calls:$loop_calls, events:$loop_events},
      wall_s:($ask_wall+$loop_wall), mind_calls:($ask_calls+$loop_calls),
      events_appended:($ask_events+$loop_events), scripts_installed:$installs}' \
    | tee -a "$RESULTS"
}

main() {
  command -v jq >/dev/null || { echo "jq required" >&2; exit 1; }
  [ -x "$SELF" ] || { echo "missing $SELF (cross-compiled self binary)" >&2; exit 1; }
  seed
  mkdir -p "$RUNS"
  for arm in $ARMS; do
    for rep in $(seq 1 "$REPS"); do
      dir="$RUNS/$arm-r$rep"
      rm -rf "$dir"; mkdir -p "$dir"
      home="$dir/home"
      if [ "$arm" = grown ]; then cp -a "$ROOT/seed-home" "$home"; else mkdir -p "$home"; fi
      echo "== $arm rep $rep =="
      for task in $TASKS; do
        run_task "$arm" "$rep" "$task" "$home" "$dir"
      done
    done
  done
  echo
  echo "== summary (means over reps) =="
  jq -s '
    group_by(.arm + "/" + .task)[] |
    {arm: .[0].arm, task: .[0].task, n: length,
     success:   (map(if .success then 1 else 0 end) | add / length),
     converged: (map(if .converged then 1 else 0 end) | add / length),
     ask_calls:  (map(.ask.calls) | add / length),
     loop_calls: (map(.loop.calls) | add / length),
     ask_wall_s: (map(.ask.wall_s) | add / length),
     loop_wall_s:(map(.loop.wall_s) | add / length),
     wall_s:    (map(.wall_s) | add / length)}' "$RESULTS"
}

main "$@"
