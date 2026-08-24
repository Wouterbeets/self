#!/bin/sh
# Compatibility wrapper for the built-in fixed-point loop.
#
# Prefer argv-safe direct use:
#   self loop -- pi --provider github-copilot --model gpt-5.6-luna --no-session -p
#
# MIND remains a shell command here only for compatibility with older callers.

set -u

SELF="${SELF:-self}"
SELF_LOOP_MIND="${SELF_LOOP_MIND:-${MIND:-claude -p}}"
SELF_LOOP_MAX_PASSES="${SELF_LOOP_MAX_PASSES:-${MAX_PASSES:-12}}"
SELF_LOOP_TIMEOUT="${SELF_LOOP_TIMEOUT:-${PASS_TIMEOUT:-1800}s}"
export SELF_LOOP_MIND SELF_LOOP_MAX_PASSES SELF_LOOP_TIMEOUT

exec "$SELF" loop
