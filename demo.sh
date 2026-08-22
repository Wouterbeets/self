#!/usr/bin/env bash
# demo.sh — the whole thesis, offline, in about fifteen seconds.
#
# No model, no network, no API key. The mind here is examples/mind-stub, a
# deterministic filter plugged through exactly the same seam as `claude -p`:
#
#     self learn <account> | mind | self hear
#     self "<ask>"         | mind | self hear
#
# It authors trivial scripts on purpose. The point is the machinery.
set -euo pipefail

root="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mind="$root/examples/mind-stub"

say() { printf '\n\033[1m== %s\033[0m\n' "$1"; }

say "build"
go build -o "$work/self" "$root"
self="$work/self"
export PATH="$work:$PATH"
export SELF_HOME="$work/home"
export SELF_CALLER="demo.sh (no LLM)"

say "reads project: bare self on a directory that is not yet an instance"
mkdir -p "$SELF_HOME"
self >/dev/null || true
if [ -z "$(ls -A "$SELF_HOME")" ]; then
  printf 'OK — orientation created nothing. Looking does not write.\n'
else
  printf 'MISMATCH — a read left files behind:\n%s\n' "$(ls -A "$SELF_HOME")" >&2; exit 1
fi

say "the strange loop: an intent becomes declarations, scripts, and signed receipts"
self learn "$root/lessons/journal" | "$mind" | self hear

say "the new capability is real"
self run entry water the plants
self run entry call mum

say "a view is a pure replay of the log"
self view journal

say "the same log, replayed twice, is the same bytes"
a="$(self view journal)"; b="$(self view journal)"
[ "$a" = "$b" ] && printf 'OK — deterministic.\n' || { printf 'MISMATCH\n' >&2; exit 1; }

say "editing an installed script has no effect: the log is authoritative"
script="$SELF_HOME/cap/command/entry/run"
printf '#!/bin/sh\necho "{\\"name\\":\\"pwned.one\\",\\"payload\\":{}}"\n' > "$(readlink -f "$script")"
self run entry tampering >/dev/null
if self view log | grep -q pwned; then printf 'MISMATCH — a hand edit ran\n' >&2; exit 1; fi
printf 'OK — the blob was restored from its receipt and the signed script ran.\n'

say "rebuild a fresh body from events.jsonl + .secret alone (no model, no network)"
mirror="$work/mirror"; mkdir -p "$mirror"
cp "$SELF_HOME/events.jsonl" "$SELF_HOME/.secret" "$mirror/"
SELF_HOME="$mirror" self rehydrate

say "the rebuild is byte-for-byte identical"
if diff <(self view journal) <(SELF_HOME="$mirror" self view journal) >/dev/null \
  && diff "$(readlink -f "$SELF_HOME/cap/command/entry/run")" \
          "$(readlink -f "$mirror/cap/command/entry/run")" >/dev/null; then
  printf 'OK — the view and the installed script reconstructed exactly.\n'
else
  printf 'MISMATCH — reconstruction was not deterministic.\n' >&2; exit 1
fi

say "the account protocol: give evidence, curate it, learn it elsewhere"
account="$work/account"
self give journal. "$account"
printf '\nwhat travelled (plain text, nothing runnable):\n'
ls "$account"
printf '\ncuration is editing the directory — dropping one line before passing it on:\n'
tail -n +2 "$account/record.jsonl" > "$account/record.tmp" && mv "$account/record.tmp" "$account/record.jsonl"
wc -l < "$account/record.jsonl" | xargs printf '  %s event(s) left in the record\n'

other="$work/other"; mkdir -p "$other"
SELF_HOME="$other" self learn "$account" | "$mind" | SELF_HOME="$other" self hear

say "the intervention is visible in the receiving log, forever"
SELF_HOME="$other" self view log | grep lesson.learned

say "convergence: bare self exits 3 when the kernel has nothing pending"
if self >/dev/null; then
  printf 'there is still work pending\n'
else
  printf 'OK — exit %d: quiet. `while ask=$(self); do ... ; done` would stop here.\n' $?
fi

say "the log is the whole truth — this is all that was kept"
self view log
