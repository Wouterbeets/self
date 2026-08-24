#!/usr/bin/env python3
import json
import sys


def payload(event):
    return event.get("payload") if isinstance(event.get("payload"), dict) else {}


def seq(event):
    value = event.get("seq")
    return value if isinstance(value, int) and not isinstance(value, bool) else 0


def short(value, limit=220):
    value = " ".join(str(value or "").split())
    return value if len(value) <= limit else value[:limit - 3] + "..."


events = []
for line in sys.stdin:
    try:
        event = json.loads(line)
    except (TypeError, ValueError):
        continue
    if isinstance(event, dict):
        events.append(event)

goals, latest, removed = {}, {}, set()
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

closed_status = {"closed", "done", "completed", "cancelled", "removed"}


def closed(goal):
    return goal in removed or str(goals.get(goal, {}).get("status", "active")).lower() in closed_status


active = {goal for goal in goals if not closed(goal)}
parents = {goals[goal].get("parent") for goal in active if goals[goal].get("parent") in active}
leaves = active - parents
blocked = {goal: [dependency for dependency in goals[goal].get("depends_on", []) if not closed(dependency)] for goal in leaves}
actionable = [goal for goal in leaves if not blocked[goal]]
waiting = [goal for goal in leaves if blocked[goal]]


def render(goal):
    item = goals[goal]
    parent = f" < {item.get('parent')}" if item.get("parent") else ""
    print(f"- [{seq(latest.get(goal, {}))}] {goal}{parent}")
    print(f"  outcome: {short(item.get('outcome'))}")
    if goal in latest:
        data = payload(latest[goal])
        detail = data.get("report") or data.get("status") or data.get("summary") or data.get("note")
        print(f"  latest: {short(detail)}")


print("# Goal work")
print(f"{len(actionable)} actionable leaves; {len(waiting)} blocked leaves. Blocked goals cannot progress or close until dependencies finish.")
print("\n## Actionable")
for goal in sorted(actionable, key=lambda item: (seq(latest.get(item, {})), item), reverse=True)[:12]:
    render(goal)
print("\n## Blocked")
for goal in sorted(waiting):
    render(goal)
    print("  waiting on: " + ", ".join(blocked[goal]))
print("\nAdvance: self run goal progress <goal> '<change; evidence; blockers; next action>'")
