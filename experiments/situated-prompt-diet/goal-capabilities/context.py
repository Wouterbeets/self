#!/usr/bin/env python3
import json
import sys

if len(sys.argv) > 2:
    print("usage: self view context [goal]", file=sys.stderr)
    raise SystemExit(2)
events = [json.loads(line) for line in sys.stdin if line.strip()]
payload = lambda event: event.get("payload") if isinstance(event.get("payload"), dict) else {}

goals, removed = {}, set()
for event in events:
    item, name = payload(event), event.get("name")
    goal = item.get("goal")
    if name == "goal.created" and isinstance(goal, str):
        goals.setdefault(goal, dict(item))
    elif name == "goal.updated" and goal in goals:
        if item.get("status"):
            goals[goal]["status"] = item["status"]
        if isinstance(item.get("depends_on"), list):
            goals[goal]["depends_on"] = item["depends_on"]
    elif name == "goal.removed" and isinstance(goal, str):
        removed.add(goal)
done = {"closed", "done", "completed", "cancelled", "removed"}
closed = lambda goal: goal in removed or str(goals.get(goal, {}).get("status", "active")).lower() in done

if len(sys.argv) == 1:
    active = {goal for goal in goals if not closed(goal)}
    parents = {goals[goal].get("parent") for goal in active if goals[goal].get("parent") in active}
    leaves = active - parents
    blockers = {goal: [dependency for dependency in goals[goal].get("depends_on", []) if not closed(dependency)] for goal in leaves}
    actionable = sorted(goal for goal in leaves if not blockers[goal])
    waiting = sorted(goal for goal in leaves if blockers[goal])
    other = sorted(active - leaves)
    print("# Goal context guide")
    print("Inspect one bounded goal history: `self view context <goal>`")
    print("Use `self view next` for outcomes and latest state, or `self view tree` for hierarchy.")
    print(f"\n{len(active)} active goals; {len(actionable)} actionable leaves; {len(waiting)} blocked leaves.")
    print("\n## Actionable")
    for goal in actionable[:20]:
        print(f"- {goal}: {goals[goal].get('title', '')}")
    if len(actionable) > 20:
        print(f"- ... {len(actionable) - 20} more; use `self view next`")
    if waiting:
        print("\n## Blocked")
        for goal in waiting[:20]:
            print(f"- {goal} <- {', '.join(blockers[goal])}")
    if other:
        print("\n## Active parents")
        for goal in other[:20]:
            print(f"- {goal}: {goals[goal].get('title', '')}")
    raise SystemExit(0)

target = sys.argv[1]
rows = [event for event in events if payload(event).get("goal") == target]
created = next((event for event in rows if event.get("name") == "goal.created"), None)
if not created:
    print(f"No goal found: {target}")
    raise SystemExit(0)

base = payload(created)
dependencies = goals[target].get("depends_on", [])
blockers = [dependency for dependency in dependencies if not closed(dependency)]
print(f"# {target}")
print(f"Title: {base.get('title', '')}")
print(f"Outcome: {base.get('outcome', '')}")
print(f"Parent: {base.get('parent', '') or '(root)'}")
print(f"Dependencies: {', '.join(dependencies) or '(none)'}")
print(f"Blockers: {', '.join(blockers) or '(none — actionable)'}")
compactions = [index for index, event in enumerate(rows) if event.get("name") == "goal.compacted"]
after = rows
if compactions:
    index = compactions[-1]
    print(f"\n## Compacted baseline [{rows[index].get('seq', 0)}]")
    print(payload(rows[index]).get("summary", ""))
    after = rows[index + 1:]
print("\n## Recent state")
for event in after[-15:]:
    print(f"- [{event.get('seq', 0)}] {event.get('name')}: {json.dumps(payload(event), ensure_ascii=False, sort_keys=True)}")
children = sorted(goal for goal, item in goals.items() if item.get("parent") == target and goal not in removed)
if children:
    print("\n## Child goals")
    for child in children:
        print(f"- {child} (`self view context {child}`)")
print(f"\nWrite: self run goal progress {target} '<change; evidence; blockers; next action>'")
print(f"Compact: self run goal compact {target} '<current distilled state>'")
