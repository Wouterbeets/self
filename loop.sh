#!/bin/sh
# loop — the famous self-loop, as a script.
#
#   while ask=$(self); do printf '%s\n' "$ask" | <mind> | self hear; done
#
# That one line converges: bare `self` exits 3 when no declaration is pending
# and no refusal stands, and command substitution is the only place POSIX sh
# can see that code — dash has no pipefail, so `self | mind | self hear`
# swallows the 3 and never stops. (PROTOCOL.md, Exit codes.)
#
# What this script adds is exactly what the kernel refuses to own
# (PROTOCOL.md, "Nothing is timed out"): a pass cap, a stall cap, a per-pass
# timeout, and a log. Every seam is an environment variable; the wire is
# unchanged — the mind's stdout still goes back into `self hear`, nothing else.
#
# Env seams:
#   MIND         the mind: a shell command, stdin = the ask, stdout = the wire.
#                Default: claude -p
#   SELF         the kernel binary. Default: self on PATH
#   SELF_HOME    the instance to work on. Default: $HOME/.self
#   MAX_PASSES   stop after this many mind passes. Default: 12
#   STALL        stop after this many consecutive silent passes (no events
#                heard). Default: 3
#   PASS_TIMEOUT seconds per mind pass. Default: 1800
#   LOG          pass log. Default: $HOME/logs/selfloop.log
#
# Exit: 0 converged or capped cleanly, 1 the loop itself broke.

set -u

SELF="${SELF:-self}"
MIND="${MIND:-claude -p}"
SELF_HOME="${SELF_HOME:-$HOME/.self}"
MAX_PASSES="${MAX_PASSES:-12}"
STALL="${STALL:-3}"
PASS_TIMEOUT="${PASS_TIMEOUT:-1800}"
LOG="${LOG:-$HOME/logs/selfloop.log}"

mkdir -p "$(dirname "$LOG")"
if [ -f "$LOG" ] && [ "$(wc -c < "$LOG")" -gt 1048576 ]; then
    mv "$LOG" "$LOG.1"
fi

# Attribution: events the loop's mind writes carry SELF_CALLER verbatim.
# Default it unless the caller already pinned one.
export SELF_HOME
if [ -z "${SELF_CALLER:-}" ]; then
    export SELF_CALLER="self loop (default mind)"
fi

WORK="$(mktemp -d "${TMPDIR:-/tmp}/selfloop.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT INT TERM

log() {
    printf '%s\n' "── $(date '+%Y-%m-%d %H:%M:%S') — $1" >> "$LOG"
}

log "loop started (max $MAX_PASSES passes, stall after $STALL silent, ${PASS_TIMEOUT}s/pass, mind=$MIND)"

pass=0
silent=0
rc=0
while :; do
    if ask=$("$SELF"); then
        rc=0
    else
        rc=$?
    fi
    if [ $rc -eq 3 ]; then
        # Quiet is success: the kernel has nothing pending and no refusal stands.
        rc=0
        log "converged after $pass passes — nothing pending, nothing refused"
        break
    fi
    if [ $rc -ne 0 ]; then
        log "read face failed (rc=$rc) — stopping"
        rc=1
        break
    fi

    pass=$((pass + 1))
    if [ $pass -gt $MAX_PASSES ]; then
        log "cap reached: $MAX_PASSES passes with work still pending — stopping"
        break
    fi

    ask_bytes=$(printf '%s' "$ask" | wc -c)
    printf '%s\n' "$ask" > "$WORK/ask"
    log "pass $pass: ask $ask_bytes bytes"
    cat "$WORK/ask" >> "$LOG"

    # The mind. timeout is the driver's policy, not the kernel's; its stdin is
    # the ask, its stdout is the wire, and we capture both before hearing.
    if timeout "$PASS_TIMEOUT" sh -c "$MIND" \
        < "$WORK/ask" > "$WORK/mind" 2> "$WORK/mind.err"; then
        mind_rc=0
    else
        mind_rc=$?
    fi
    if [ -s "$WORK/mind.err" ]; then
        {
            printf '%s\n' "── mind stderr (rc=$mind_rc)"
            cat "$WORK/mind.err"
        } >> "$LOG"
    fi

    # Count what is actually wire: lines that parse as {name, payload}.
    # hear will accept those and ignore the rest; a pass with none is silent.
    events=$(python3 -c '
import json, sys
n = 0
for line in open(sys.argv[1], "r", encoding="utf-8", errors="replace"):
    line = line.strip()
    if not line:
        continue
    try:
        obj = json.loads(line)
    except Exception:
        continue
    if isinstance(obj, dict) and obj.get("name") and "payload" in obj:
        n += 1
print(n)' "$WORK/mind" 2>/dev/null || echo 0)
    printf '%s' "$events" > "$WORK/events"
    events=$(cat "$WORK/events")

    # The wire goes back in. hear echoes the lines it ignores and reports what
    # it heard; that report belongs in the log.
    if "$SELF" hear < "$WORK/mind" > "$WORK/hear" 2>&1; then
        hear_rc=0
    else
        hear_rc=$?
    fi
    if [ -s "$WORK/hear" ]; then
        cat "$WORK/hear" >> "$LOG"
    fi

    if [ "$events" -eq 0 ]; then
        silent=$((silent + 1))
        log "pass $pass: silent (mind_rc=$mind_rc, hear_rc=$hear_rc) — $silent/$STALL"
        if [ $silent -ge $STALL ]; then
            log "stalled after $STALL consecutive silent passes — stopping"
            rc=1
            break
        fi
        continue
    fi

    silent=0
    log "pass $pass: $events event(s) heard (mind_rc=$mind_rc, hear_rc=$hear_rc)"
    if [ $hear_rc -ne 0 ]; then
        log "hear failed (rc=$hear_rc) — stopping"
        rc=1
        break
    fi
done

log "loop stopped (rc=$rc)"
exit "$rc"
