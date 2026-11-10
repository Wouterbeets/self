#!/bin/bash
set -u
export SELF_HOME="${SELF_HOME:-$HOME/.self}"
export SELF_CALLER="${SELF_CALLER:-pulse-driver}"
export PATH="$HOME/go/bin:$HOME/.local/bin:/usr/bin:/bin"

self run pulse tick || true
self view pulse actions | while IFS=$'\t' read -r key title body; do
  echo "  push $key $title"
  self run notify --key "pulse/$key" --tags pulse,warning "$title" "$body" && self run pulse told "$key"
done
