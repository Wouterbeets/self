#!/usr/bin/env bash
# demo.sh — see the machinery with no LLM, in about ten seconds.
#
# This shows the kernel loop end to end WITHOUT a model: declarations compile
# into scripts (here via examples/mind-stub, a deterministic offline mind
# plugged through the same seam as any real one), running a command appends an
# event, a projection renders it, and the whole instance rebuilds from
# events.jsonl + .secret alone — byte for byte.
#
# The stub authors trivial scripts; the point here is the machinery, not the
# intelligence. For real, LLM-generated capabilities, plug a real mind and
# use `self grow` (see the README).
set -euo pipefail

root="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
export SELF_MIND="$root/examples/mind-stub"
export SELF_MIND_ID="stub (no LLM)"

say() { printf '\n\033[1m== %s\033[0m\n' "$1"; }

say "build"
go build -o "$work/self" "$root"
self="$work/self"
export SELF_HOME="$work/home"

say "declare two capabilities (the stub mind authors, the kernel signs)"
# A seed is event JSONL. `adopt` re-declares it, and the kernel compiles it
# locally under a receipt signed by THIS instance's key.
printf '{"name":"command.declared","payload":{"name":"entry","description":"record an entry","params":{"text":"string"},"event":{"name":"journal.entry","fields":{"title":"string"}}}}\n' > "$work/cmd.jsonl"
printf '{"name":"projector.declared","payload":{"name":"journal","description":"all entries","consumes":["journal.entry"]}}\n' > "$work/proj.jsonl"
"$self" adopt "$work/cmd.jsonl"
"$self" adopt "$work/proj.jsonl"

say "run the command a couple of times (each appends one event)"
"$self" run entry water the plants
"$self" run entry call mum

say "the projection is a pure replay of the log"
"$self" show journal

say "rebuild a fresh copy from events.jsonl + .secret alone (no LLM, no network)"
mirror="$work/mirror"
mkdir -p "$mirror"
cp "$SELF_HOME/events.jsonl" "$SELF_HOME/.secret" "$mirror/"
SELF_HOME="$mirror" "$self" rehydrate

say "the rebuild is byte-for-byte identical"
if diff -q "$SELF_HOME/site/journal.html" "$mirror/site/journal.html" >/dev/null \
   && diff -q "$SELF_HOME/capabilities/commands/entry" "$mirror/capabilities/commands/entry" >/dev/null; then
  printf 'OK — the projection and the compiled script reconstructed exactly.\n'
else
  printf 'MISMATCH — reconstruction was not deterministic.\n' >&2
  exit 1
fi

say "the log is the whole truth — this is all that was kept"
cat "$SELF_HOME/events.jsonl"
