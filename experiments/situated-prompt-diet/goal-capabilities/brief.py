#!/usr/bin/env python3
import json
import sys


def payload(event):
    return event.get("payload") if isinstance(event.get("payload"), dict) else {}


events = []
for line in sys.stdin:
    try:
        event = json.loads(line)
    except (TypeError, ValueError):
        continue
    if isinstance(event, dict):
        events.append(event)

goals, removed, latest = {}, set(), {}
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
        latest[goal] = event
    elif name in {"goal.progress", "goal.milestone", "goal.compacted"} and goal in goals:
        latest[goal] = event
    elif name == "goal.removed" and isinstance(goal, str):
        removed.add(goal)

done = {"closed", "done", "completed", "cancelled", "removed"}
closed = lambda goal: goal in removed or str(goals.get(goal, {}).get("status", "active")).lower() in done
active = {goal for goal in goals if not closed(goal)}
parents = {goals[goal].get("parent") for goal in active if goals[goal].get("parent") in active}
leaves = active - parents
blocked = {goal: [dependency for dependency in goals[goal].get("depends_on", []) if not closed(dependency)] for goal in leaves}
actionable = sorted((goal for goal in leaves if not blocked[goal]), key=lambda goal: (latest.get(goal, {}).get("seq", 0), goal), reverse=True)
waiting = sorted(goal for goal in leaves if blocked[goal])

print("# Goals control room")
print(f"{len(active)} active goals; {len(actionable)} actionable leaves; {len(waiting)} blocked leaves.")
if actionable:
    print("\n## Act now")
    for goal in actionable[:10]:
        print(f"- [{latest.get(goal, {}).get('seq', 0)}] {goal}: {goals[goal].get('title', '')}")
if waiting:
    print("\n## Waiting")
    for goal in waiting[:10]:
        print(f"- {goal} <- {', '.join(blocked[goal])}")
print("\nViews: self view next | self view tree | self view context <goal>")
