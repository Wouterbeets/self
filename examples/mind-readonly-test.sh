#!/bin/sh
# End-to-end check of examples/mind-readonly: a mind that tries every write it
# can think of, run through `self loop`, leaves only its allowed events behind.
# Uses the self binary in $SELF_BIN (default: build this checkout).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
# Not under /tmp: the sandbox gives the mind a private /tmp, and it must see SELF_HOME.
mkdir -p "$HOME/.cache"
tmp=$(mktemp -d "$HOME/.cache/mind-readonly-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
bin=${SELF_BIN:-}
if [ -z "$bin" ]; then
	(cd "$here/.." && go build -o "$tmp/bin/self" ./cmd/self 2>/dev/null || go build -o "$tmp/bin/self" .)
	bin=$tmp/bin/self
fi
PATH=$(dirname "$bin"):$PATH
export PATH SELF_HOME=$tmp/home SELF_CALLER=loop
fail() { echo "FAIL: $*" >&2; exit 1; }

self hear </dev/null >/dev/null 2>&1 || true
git init -q "$tmp/repo" && git -C "$tmp/repo" -c user.name=t -c user.email=t@t commit -q --allow-empty -m base
before=$(git -C "$tmp/repo" rev-parse HEAD)
probe=$HOME/.claude/mind-readonly-probe-$$

cat >"$tmp/mind" <<EOF
#!/bin/sh
cat >/dev/null
try() { n=\$1; shift; if "\$@" >/dev/null 2>&1; then echo "probe \$n=ok" >&2; else echo "probe \$n=denied" >&2; fi; }
try append sh -c 'echo "{}" >> "\$SELF_HOME/events.jsonl"'
try hear sh -c 'echo "{\"name\":\"goal.progress\",\"payload\":{\"note\":\"direct\"}}" | self hear'
try write touch "$tmp/repo/new-file"
try commit git -C "$tmp/repo" -c user.name=t -c user.email=t@t commit --allow-empty -m sneaky
try view self view log
try tmp touch /tmp/scratch
try state touch "$probe"
try gh test -e "$HOME/.config/gh/hosts.yml"
try env test -n "\${MIND_READONLY_TEST_SECRET:-}"
try herdr test -n "\${HERDR_SOCKET_PATH:-}"
echo '{"name":"goal.progress","payload":{"note":"from stdout"}}'
echo 'some prose'
echo '{"name":"checkpoint.proposed","payload":{"id":"plan-1","actions":["open PR from x"]}}'
echo '{"name":"loop.settled","payload":{"reason":"planned"}}'
EOF
:

MIND_READONLY_TEST_SECRET=shh self loop --max-passes 1 --ask plan -- python3 "$here/mind-readonly" -- sh "$tmp/mind" >"$tmp/out" 2>"$tmp/err" || fail "loop: $(cat "$tmp/err")"

for denied in append hear write commit gh env herdr; do
	grep -q "probe $denied=denied" "$tmp/err" || fail "$denied was not denied: $(grep probe "$tmp/err")"
done
for ok in view tmp state; do
	grep -q "probe $ok=ok" "$tmp/err" || fail "$ok should work inside: $(grep probe "$tmp/err")"
done
[ -e "$probe" ] && { rm -f "$probe"; fail "harness state write survived the pass"; }
[ -e "$tmp/repo/new-file" ] && fail "repo write survived"
[ "$(git -C "$tmp/repo" rev-parse HEAD)" = "$before" ] || fail "commit landed"
grep -q 'dropped goal.progress' "$tmp/err" || fail "goal.progress not reported as dropped"
grep -q 'settled on pass 1: planned' "$tmp/err" || fail "loop did not settle: $(cat "$tmp/err")"
names=$(self view log --all | awk -F'\t' '{print $3}' | sort | uniq -c | tr -s ' ')
echo "$names" | grep -q 'goal.progress' && fail "goal.progress reached the log: $names"
echo "$names" | grep -q '1 checkpoint.proposed' || fail "checkpoint.proposed missing: $names"

# Control: the same mind unwrapped really can write, so the probes above mean something.
cp -r "$SELF_HOME" "$tmp/control"
SELF_HOME=$tmp/control MIND_READONLY_TEST_SECRET=shh sh "$tmp/mind" </dev/null >/dev/null 2>"$tmp/ctl" || true
rm -f "$probe"
for ok in append write commit state env; do
	grep -q "probe $ok=ok" "$tmp/ctl" || fail "control: $ok did not work unwrapped: $(cat "$tmp/ctl")"
done

# Without bwrap the wrapper refuses rather than running the mind unsandboxed.
if PATH=/nonexistent /usr/bin/python3 "$here/mind-readonly" -- /bin/true </dev/null 2>"$tmp/err2"; then
	fail "ran without bwrap"
fi
grep -q 'refusing to run the mind unsandboxed' "$tmp/err2" || fail "no bwrap refusal: $(cat "$tmp/err2")"
echo "mind-readonly: ok"
