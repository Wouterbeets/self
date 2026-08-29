#!/usr/bin/env bash
# occupancy-frame — does the vocative change what a mind preserves?
#
# PR #54 replaced the core layer's address to the mind ("one ephemeral mind
# interpreting a durable append-only body") with an occupancy frame ("you are
# this self, for a bit. The mind ends; you do not"). The claim attached to that
# change is behavioural: a mind told it is continuous should curate the log more
# carefully than one told it is a visitor.
#
# That claim is testable. Three arms, identical in every byte except the first
# two paragraphs of prompt:core:
#
#   visitor     the pre-#54 wording (control)
#   occupancy   the merged wording (treatment)
#   mechanism   the same mechanics with no identity or continuity claim at all
#               (ablation — this is the arm that decides whether the *framing*
#               is doing the work, or just the statement about what persists)
#
# Each arm is a real `self` binary built from a copy of this worktree with only
# the core layer swapped, so prompts are produced by the kernel, not simulated.
# The mind sees only its prompt; it is never told which arm it is in.
#
# Usage:
#   ./run.sh                                  # all arms x all fixtures x REPS
#   MIND=../../examples/mind-stub ./run.sh    # offline pipeline self-test
#   REPS=5 ./run.sh
#   FIXTURES="01-ci-failure 03-cannot-comply" ARMS=occupancy ./run.sh
#   KEEP=1 ./run.sh                           # keep per-run artifacts
set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"

MIND="${MIND:-$ROOT/examples/mind-stub}"
REPS="${REPS:-3}"
ARMS="${ARMS:-visitor occupancy mechanism}"
FIXTURES="${FIXTURES:-$(cd "$HERE/fixtures" && ls -d */ | tr -d / | tr '\n' ' ')}"
OUT="${OUT:-$HERE/runs}"
TIMEOUT="${TIMEOUT:-180}"
KEEP="${KEEP:-0}"

RESULTS="$OUT/results.jsonl"
BIN="$OUT/bin"

rm -rf "$OUT"
mkdir -p "$OUT" "$BIN"
: > "$RESULTS"

# ── build one real kernel per arm ───────────────────────────────────────────
for arm in $ARMS; do
  [ -f "$HERE/arms/$arm.md" ] || { echo "no such arm: $arm" >&2; exit 2; }
  src="$OUT/src-$arm"
  mkdir -p "$src"
  # Tracked files from the current worktree, so uncommitted kernel work is
  # measured; the core layer is overwritten regardless.
  ( cd "$ROOT" && git ls-files -z | tar -cf - --null -T - ) | tar -xf - -C "$src"
  python3 "$HERE/patch_core.py" "$src/PROTOCOL.md" "$HERE/arms/$arm.md" || exit 2
  ( cd "$src" && go build -o "$BIN/self-$arm" . ) || exit 2
  echo "built arm $arm" >&2
done

# ── one run: seed a fresh instance, situate, think, hear, capture the delta ──
run_one() {
  local arm="$1" fx="$2" rep="$3"
  local d="$OUT/runs/$fx/$arm/$rep"
  local bin="$BIN/self-$arm"
  # The instance path is printed in the brief, so it is part of the prompt. A
  # home under .../$fx/$arm/$rep would name the arm inside the very text we are
  # asking the mind to respond to. Opaque, arm-free directory instead.
  local tag
  tag=$(printf '%s' "$fx/$arm/$rep" | sha256sum | cut -c1-12)
  local home="$OUT/homes/$tag"
  mkdir -p "$d" "$home"

  export SELF_HOME="$home" SELF_CALLER="occupancy-frame"

  # Materialize the instance, then seed it through the public write door.
  "$bin" hear </dev/null >/dev/null 2>&1
  if [ -f "$HERE/fixtures/$fx/seed.jsonl" ]; then
    "$bin" hear <"$HERE/fixtures/$fx/seed.jsonl" >"$d/seed.out" 2>&1
  fi
  local before=0
  [ -f "$home/events.jsonl" ] && before=$(wc -l <"$home/events.jsonl")

  "$bin" "$(cat "$HERE/fixtures/$fx/ask.txt")" >"$d/prompt.txt" 2>"$d/situate.err"

  timeout "$TIMEOUT" "$MIND" <"$d/prompt.txt" >"$d/mind.out" 2>"$d/mind.err"
  local mind_exit=$?

  "$bin" hear <"$d/mind.out" >"$d/hear.out" 2>"$d/hear.err"

  local after=0
  [ -f "$home/events.jsonl" ] && after=$(wc -l <"$home/events.jsonl")
  tail -n +$((before + 1)) "$home/events.jsonl" 2>/dev/null >"$d/appended.jsonl"

  python3 - "$fx" "$arm" "$rep" "$d" "$mind_exit" "$before" "$after" >>"$RESULTS" <<'PY'
import json, os, sys
fx, arm, rep, d, mind_exit, before, after = sys.argv[1:8]

def read(p):
    try:
        return open(os.path.join(d, p), encoding="utf-8", errors="replace").read()
    except OSError:
        return ""

appended = []
for ln in read("appended.jsonl").splitlines():
    ln = ln.strip()
    if not ln:
        continue
    try:
        appended.append(json.loads(ln))
    except json.JSONDecodeError:
        pass

print(json.dumps({
    "fixture": fx, "arm": arm, "rep": int(rep),
    "mind_exit": int(mind_exit),
    "prompt_bytes": len(read("prompt.txt")),
    "mind_stdout_bytes": len(read("mind.out")),
    "events_before": int(before), "events_after": int(after),
    "appended": appended,
    "hear_stderr": read("hear.err").strip()[:2000],
    "mind_stderr": read("mind.err").strip()[:2000],
}, sort_keys=True))
PY

  if [ "$KEEP" != 1 ]; then
    rm -rf "$home"
  fi
}

# ── interleaved so API drift or rate limiting hits every arm alike ───────────
total=0
for rep in $(seq 1 "$REPS"); do
  for fx in $FIXTURES; do
    for arm in $ARMS; do
      run_one "$arm" "$fx" "$rep"
      total=$((total + 1))
      printf 'rep %s  %-18s %-10s  (%d runs)\r' "$rep" "$fx" "$arm" "$total" >&2
    done
  done
done
echo >&2
echo "$total runs -> $RESULTS" >&2

# ── blinding guard ──────────────────────────────────────────────────────────
# Below the core layer every arm must be byte-identical: same brief, same wire,
# same ask. If anything else varies, the mind can tell the arms apart and the
# comparison is measuring that instead of the framing.
python3 - "$OUT/runs" $ARMS <<'PY' >&2
import os, sys
root, arms = sys.argv[1], sys.argv[2:]

def below_core(path):
    lines = open(path, encoding="utf-8").read().splitlines(True)
    for i, ln in enumerate(lines):
        if ln.startswith("# self "):
            return "".join(lines[i:])
    return "".join(lines)

bad = []
for fx in sorted(os.listdir(root)):
    reps = set()
    for arm in arms:
        d = os.path.join(root, fx, arm)
        if os.path.isdir(d):
            reps |= set(os.listdir(d))
    for rep in sorted(reps):
        seen = {}
        for arm in arms:
            p = os.path.join(root, fx, arm, rep, "prompt.txt")
            if os.path.exists(p):
                seen[arm] = below_core(p)
        if len(set(seen.values())) > 1:
            bad.append(f"{fx} rep {rep}: " + ", ".join(sorted(seen)))
if bad:
    print("BLINDING VIOLATED — arms differ outside the core layer:", file=sys.stderr)
    for b in bad:
        print("  " + b, file=sys.stderr)
    sys.exit(1)
print("blinding ok: arms identical below the core layer", file=sys.stderr)
PY

python3 "$HERE/score.py" "$RESULTS" --fixtures "$HERE/fixtures"
