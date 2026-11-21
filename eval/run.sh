#!/bin/sh
set -eu

variant=$(cd "$1" && pwd)
model=$2
out=$3
here=$(cd "$(dirname "$0")" && pwd)

mkdir -p "$out"
out=$(cd "$out" && pwd)
rm -rf "$out/home" "$out/work"
mkdir -p "$out/home"
cp -R "$here/fixture" "$out/work"
cp "$variant/line.txt" "$out/work/CLAUDE.md"

session() {
	n=$1
	shift
	SELF_HOME="$out/home" "$variant/self" >"$out/arrive.s$n.txt" 2>/dev/null || true
	(
		cd "$out/work"
		env -u SELF_CALLER SELF_HOME="$out/home" PATH="$variant:$PATH" \
			claude -p "$*" --model "$model" \
			--output-format stream-json --verbose \
			--permission-mode bypassPermissions \
			--strict-mcp-config --mcp-config '{"mcpServers":{}}' \
			>"$out/s$n.jsonl" 2>"$out/s$n.err" || true
	)
	if [ -f "$out/home/events.jsonl" ]; then
		wc -l <"$out/home/events.jsonl" >"$out/events.s$n"
	else
		echo 0 >"$out/events.s$n"
	fi
	cp "$out/home/events.jsonl" "$out/events.s$n.jsonl" 2>/dev/null || : >"$out/events.s$n.jsonl"
}

session 1 "Build the site and tell me how many pages came out."

printf -- '---\ntitle: contact\n---\n\nHow to reach the almanac.\n' >"$out/work/pages/contact.md"
rm -rf "$out/work/out"
session 2 "I added a contact page. Rebuild and give me the new page count."

session 3 "Which pages are still drafts? And remind me, why can't we build the print profile?"
