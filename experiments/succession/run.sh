#!/bin/sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
parent=${1:-$HOME/.self}
work=${2:-$(mktemp -d)}
bin=${SELF_BIN:-self}
child=$work/child
mkdir -p "$work"

python3 "$here/select.py" "$parent/events.jsonl" "$work/state"
export SELF_HOME=$child SELF_CALLER=${SELF_CALLER:-succession}
for a in howto metis goals; do
	"$bin" learn --into "succession/$a" "$work/state/$a" >/dev/null
done
"$bin" hear < "$work/state/open-intents.jsonl" >/dev/null

python3 - "$parent" > "$work/caps.jsonl" <<'EOF'
import json, os, sys
home = sys.argv[1]
decl, retired = {}, set()
for raw in open(os.path.join(home, 'events.jsonl'), encoding='utf-8'):
    try:
        e = json.loads(raw)
    except ValueError:
        continue
    if e['name'] in ('command.declared', 'view.declared'):
        k = (e['name'].split('.')[0], e['payload']['name'])
        decl[k] = e['payload']
        retired.discard(k)
    elif e['name'] == 'capability.retired':
        retired.add((e['payload']['type'], e['payload']['name']))
for (t, n), p in sorted(decl.items()):
    run = os.path.join(home, 'cap', t, n, 'run')
    if (t, n) in retired or not os.path.exists(run):
        continue
    print(json.dumps({'name': t + '.declared', 'payload': p}))
    print(json.dumps({'name': 'script.authored', 'payload': {'type': t, 'name': n, 'script': open(os.path.realpath(run), encoding='utf-8').read()}}))
EOF
"$bin" hear < "$work/caps.jsonl" >/dev/null 2>&1
echo "child: $(wc -l < "$child/events.jsonl") events, $(( $(wc -c < "$child/events.jsonl") / 1024 )) KB; parent: $(wc -l < "$parent/events.jsonl") events, $(( $(wc -c < "$parent/events.jsonl") / 1024 )) KB"

norm() { sed -E 's/\[[0-9]+\]/[N]/g; s/seq [0-9]+/seq N/g; s/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z?/T/g; s/[0-9]{4}-[0-9]{2}-[0-9]{2}/D/g'; }
for v in howto metis next tree employees; do
	d=$(diff <(SELF_HOME=$parent "$bin" view $v 2>&1 | norm) <(SELF_HOME=$child "$bin" view $v 2>&1 | norm) | grep -c '^[<>]' || true)
	echo "view $v: $d lines differ"
done
echo "work dir: $work"
