#!/usr/bin/env python3
"""Make the goal surface correct on a self that has more than one body.

Two bugs, both invisible while there was only one log:

  1. `seq` is assigned per body. A moment learned from the other body today
     lands with a higher seq than everything written here yesterday, so sorting
     by it answers "most recently touched" differently on each body.
  2. Folding "last wins" over stdin order has the same flaw: learned events
     arrive in one block at the point of the learn, not in the order they
     happened, so the newest progress note for a goal is whichever body's copy
     was learned last rather than whichever was written last.

Both are fixed by ordering on `occurred_at`, which travels with the moment,
before anything folds. Printed `[seq]` locators become `[here]` or `[<peer>]`,
which is the honest thing to show a reader who has two logs.
"""
import re
import sys

HELPERS = '''
# This self has more than one body, and `seq` is assigned per body: a moment
# learned from the other one today lands with a higher seq than everything
# written here yesterday. Folding "last wins" over log order, or sorting by
# seq, therefore renders a different answer on each body. occurred_at travels
# with the moment; name and payload break ties the same way on both sides.
import re as _re
_MOMENT = _re.compile(r"^(\\d{4})-(\\d{2})-(\\d{2})T(\\d{2}):(\\d{2}):(\\d{2})(?:\\.(\\d+))?Z$")


def moment(event):
    stamp = _MOMENT.match(str((event or {}).get("occurred_at") or ""))
    return ("".join(stamp.group(i) for i in range(1, 7)) + (stamp.group(7) or "")[:9].ljust(9, "0")
            if stamp else "",
            str((event or {}).get("name") or ""),
            json.dumps((event or {}).get("payload") or {}, sort_keys=True, separators=(",", ":")))


def origin(event):
    """Which body witnessed this. The door is the only honest local fact on a
    deposited event: seq is re-assigned and by/occurred_at belong to the peer."""
    door = str((event or {}).get("via") or "")
    return door[6:] if door.startswith("learn:") else "here"


events.sort(key=moment)
'''

PARSE = 'events = [json.loads(line) for line in sys.stdin if line.strip()]\n'
GOAL_PARSE = '''    if isinstance(event, dict):
        events.append(event)
'''

EDITS = {
    "next": [
        ('print(f"- [{seq(event)}] {goal}{parent}{stamp}")',
         'print(f"- [{origin(event)}] {goal}{parent}{stamp}")'),
        ('key=lambda item: (seq(latest.get(item, {})), item), reverse=True)[:12]',
         'key=lambda item: (moment(latest.get(item, {})), item), reverse=True)[:12]'),
        ('key=lambda item: (seq(latest.get(item, {})), item)) if goal not in freshest',
         'key=lambda item: (moment(latest.get(item, {})), item)) if goal not in freshest'),
        ('key=lambda item: completion[item].get("seq", 0), reverse=True',
         'key=lambda item: moment(completion[item]), reverse=True'),
    ],
    "brief": [
        ('key=lambda goal: completion[goal].get("seq", 0), reverse=True',
         'key=lambda goal: moment(completion[goal]), reverse=True'),
        ('key=lambda goal: (latest.get(goal, {}).get("seq", 0), goal), reverse=True',
         'key=lambda goal: (moment(latest.get(goal, {})), goal), reverse=True'),
        ("""print(f"- [{latest.get(goal, {}).get('seq', 0)}] {goal}:""",
         """print(f"- [{origin(latest.get(goal, {}))}] {goal}:"""),
        ('sorted(stale, key=lambda goal: (latest.get(goal, {}).get("seq", 0), goal))[:3]',
         'sorted(stale, key=lambda goal: (moment(latest.get(goal, {})), goal))[:3]'),
        ("""print(f"- [{completion[goal].get('seq', 0)}] {goal}:""",
         """print(f"- [{origin(completion[goal])}] {goal}:"""),
    ],
    "tree": [
        ("""f" [completion pending: {completion[goal].get('seq', 0)}]\"""",
         """f" [completion pending: {origin(completion[goal])}]\""""),
        ("""[{latest[goal].get('seq', 0)}]""", """[{origin(latest[goal])}]"""),
    ],
    "context": [
        ("""print(f"Completion pending: [{request.get('seq', 0)}]""",
         """print(f"Completion pending: [{origin(request)}]"""),
        ("""print(f"\\n## Compacted baseline [{rows[index].get('seq', 0)}]")""",
         """print(f"\\n## Compacted baseline [{origin(rows[index])}]")"""),
        # The detail view keeps the local seq: there it is a real locator into
        # this body's own log, and the origin says whose testimony it is.
        ("""print(f"- [{event.get('seq', 0)}] {event.get('name')}""",
         """print(f"- [{event.get('seq', 0)} {origin(event)}] {event.get('name')}"""),
    ],
    "goal": [],
}

for name, edits in EDITS.items():
    src = open(name).read()
    anchor = GOAL_PARSE if name == "goal" else PARSE
    if anchor not in src:
        sys.exit("%s: parse anchor not found" % name)
    src = src.replace(anchor, anchor + HELPERS, 1)
    for old, new in edits:
        if old not in src:
            sys.exit("%s: could not find %r" % (name, old[:60]))
        src = src.replace(old, new)
    if re.search(r'get\("seq"|get\(.seq.,\s*0\)', src.replace('event.get(\'seq\', 0)} {origin', 'X')):
        leftover = [l for l in src.splitlines() if 'seq' in l and 'moment' not in l and '_MOMENT' not in l
                    and not l.strip().startswith('#') and 'origin(event)' not in l]
        if leftover:
            print("%s: seq still referenced:" % name)
            for l in leftover:
                print("   ", l.strip()[:100])
    open("patched_" + name, "w").write(src)
    print("patched %s" % name)
