#!/usr/bin/env bash
# preflight — the gate the autonomous heartbeat loop (and you) run before merging.
# It builds, runs the Go tests, and then exercises the real capabilities through
# the projection-as-oracle: rehydrate the shipped board body (no LLM needed) and
# re-run every capability's examples against the installed binaries. Any failure
# exits nonzero, so a beat that broke a contract never merges.
#
#   usage: scripts/preflight.sh
set -euo pipefail
cd "$(dirname "$0")/.."

echo "── build ──────────────────────────────────────────"
go build -o self .

echo "── go test ────────────────────────────────────────"
go test ./...

echo "── capability selftest (projection as oracle) ─────"
# Use the shipped board body so this is deterministic and needs no LLM: rehydrate
# rebuilds the binaries from the log's signed receipts, then selftest runs each
# capability's examples against them.
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
cp home/events.jsonl home/.secret home/.identity "$T/"
SELF_HOME="$T" ./self rehydrate >/dev/null
SELF_HOME="$T" ./self selftest

echo "── preflight OK ───────────────────────────────────"
