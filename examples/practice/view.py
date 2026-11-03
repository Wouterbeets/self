#!/usr/bin/env python3
'''practices - live practices: one per line (id, insertion points, check, title); `practices <id>` prints one in full; `practices --at <point>` filters.

Source: ~/self examples/practice/view.py.
'''
import json
import sys

live, retired = {}, {}
for line in sys.stdin:
    try:
        e = json.loads(line)
    except ValueError:
        continue
    p = e.get("payload") or {}
    key = p.get("practice")
    if not key:
        continue
    if e.get("name") == "practice.saved":
        live[key] = dict(p, _by=e.get("by"), _at=e.get("occurred_at"), _seq=e.get("seq"))
        retired.pop(key, None)
    elif e.get("name") == "practice.retired":
        if key in live:
            retired[key] = dict(live.pop(key), _reason=p.get("reason"))

args = sys.argv[1:]
if args[:1] == ["--at"] and len(args) == 2:
    rows = [p for p in live.values() if args[1] in p.get("at", [])]
elif not args:
    rows = list(live.values())
elif len(args) == 1:
    p = live.get(args[0]) or retired.get(args[0])
    if not p:
        print("no practice %s; live: %s" % (args[0], ", ".join(sorted(live)) or "none"), file=sys.stderr)
        raise SystemExit(1)
    state = "retired: " + str(p.get("_reason")) if args[0] in retired else "live"
    print("# practice/%s — %s (%s)\n" % (p["practice"], p.get("title", ""), state))
    print(p.get("rule", ""))
    print("\nwhen:  " + ", ".join(p.get("when", [])))
    print("at:    " + ", ".join(p.get("at", [])))
    if p.get("check"):
        print("check: self run %s --base <sha> --head <sha> <path…>" % p["check"])
    if p.get("remote"):
        print("repos: origin contains " + p["remote"])
    print("saved: seq %s by %s at %s" % (p.get("_seq"), p.get("_by"), p.get("_at")))
    raise SystemExit(0)
else:
    print("usage: practices [<id> | --at <point>]", file=sys.stderr)
    raise SystemExit(1)

for p in sorted(rows, key=lambda p: p["practice"]):
    print("\t".join([p["practice"], ",".join(p.get("at", [])), p.get("check") or "-", p.get("title", "")]))
if not args and rows:
    print("push with `self run pr.push`: it enforces the practices at push; `self view practices <id>` reads one")
