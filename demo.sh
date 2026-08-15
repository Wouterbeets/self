#!/usr/bin/env bash
# demo.sh — see the machinery with no LLM, in about ten seconds.
#
# This shows the whole loop end to end WITHOUT a model. The mind is a shell
# process piped between two selves — here examples/mind-stub, a deterministic
# offline filter plugged through the same seam as any real one:
#
#     self learn <lesson> | mind | self      # intent → declarations → scripts
#     echo "<ask>"        | self | mind | self
#
# A lesson's intent becomes declarations, declarations get authored scripts
# installed under signed receipts, running a command appends an event, a
# projection renders it, and the whole instance rebuilds from events.jsonl +
# .secret alone — byte for byte.
#
# The stub authors trivial scripts; the point here is the machinery, not the
# intelligence. For real capabilities, put a real mind in the pipe:
#     self learn lessons/chat | claude -p | self
set -euo pipefail

root="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mind="$root/examples/mind-stub"
export SELF_MIND_ID="stub (no LLM)"

say() { printf '\n\033[1m== %s\033[0m\n' "$1"; }

say "build"
go build -o "$work/self" "$root"
self="$work/self"
export SELF_HOME="$work/home"

say "the strange loop: learn a lesson through the pipe (deposit, declare, author, install)"
"$self" learn "$root/lessons/journal" | "$mind" | "$self"

say "ask through the pipe (the ask and the reply both land in the log)"
echo "what can you do now?" | "$self" | "$mind" | "$self"

say "the learned command answers to its own name (each run appends one event)"
"$self" entry water the plants
"$self" entry call mum

say "so does the learned view — a pure replay of the log"
"$self" journal

say "memory is a stream: grep it, no query language"
"$self" log journal.entry | grep --color=never plants

say "rebuild a fresh copy from events.jsonl + .secret alone (no LLM, no network)"
mirror="$work/mirror"
mkdir -p "$mirror"
cp "$SELF_HOME/events.jsonl" "$SELF_HOME/.secret" "$mirror/"
SELF_HOME="$mirror" "$self" rehydrate

say "the rebuild is byte-for-byte identical"
if diff -q "$SELF_HOME/site/journal.html" "$mirror/site/journal.html" >/dev/null \
	&& diff -q "$SELF_HOME/capabilities/commands/entry/run" "$mirror/capabilities/commands/entry/run" >/dev/null \
	&& diff -q "$SELF_HOME/capabilities/projectors/journal/run" "$mirror/capabilities/projectors/journal/run" >/dev/null; then
  printf 'OK — the projection and the compiled script reconstructed exactly.\n'
else
  printf 'MISMATCH — reconstruction was not deterministic.\n' >&2
  exit 1
fi

say "the log is the whole truth — this is all that was kept"
cat "$SELF_HOME/events.jsonl"
