#!/bin/sh
set -e
B="$(dirname "$0")"
mkdir -p "$B/out"
for pair in digest:sync.digest export:sync.export pull:sync.pull; do
  body="${pair%%:*}"; name="${pair##*:}"
  { printf '#!/usr/bin/env python3\n'; cat "$B/prelude.py" "$B/body_$body.py"; } > "$B/out/$name"
  chmod +x "$B/out/$name"
done
ls -l "$B/out"
for pair in peer:peer peers:peers push:sync.push ask:ask.peer answer:answer.peer inbox:inbox; do
  body="${pair%%:*}"; name="${pair##*:}"
  { printf '#!/usr/bin/env python3\n'; cat "$B/prelude.py" "$B/body_$body.py"; } > "$B/out/$name"
  chmod +x "$B/out/$name"
done
