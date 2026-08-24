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

goals, order, removed, latest = {}, [], set(), {}
for event in events:
    item, name = payload(event), event.get("name")
    goal = item.get("goal")
    if name == "goal.created" and isinstance(goal, str) and goal not in goals:
        goals[goal] = dict(item)
        order.append(goal)
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
children = {goal: [] for goal in goals}
for goal in order:
    if goals[goal].get("parent") in children:
        children[goals[goal]["parent"]].append(goal)
visible = set(active)
for goal in list(active):
    parent = goals[goal].get("parent")
    while parent in goals and parent not in visible:
        visible.add(parent)
        parent = goals[parent].get("parent")


def blockers(goal):
    return [dependency for dependency in goals[goal].get("depends_on", []) if not closed(dependency)]


lines = []
def render(goal, depth):
    marker = "!" if goal in active and blockers(goal) else "*" if goal in active else "+"
    suffix = f" [blocked: {', '.join(blockers(goal))}]" if blockers(goal) else " [closed ancestor]" if goal not in active else ""
    lines.append(f"{'  ' * depth}{marker} {goal}{suffix} - {goals[goal].get('title', '')}")
    if goal in latest:
        item = payload(latest[goal])
        detail = item.get("report") or item.get("summary") or item.get("note") or item.get("status")
        if detail:
            detail = " ".join(str(detail).split())
            lines.append(f"{'  ' * depth}  [{latest[goal].get('seq', 0)}] {detail[:147] + '...' if len(detail) > 150 else detail}")
    for child in children.get(goal, []):
        if child in visible:
            render(child, depth + 1)


roots = [goal for goal in order if goal in visible and goals[goal].get("parent") not in visible]
print("# Active goal tree")
print("* actionable/active; ! blocked by dependencies; + closed ancestor retained for hierarchy.")
for root in roots:
    render(root, 0)
print("\n".join(lines) if lines else "No active goals.")
print("\nInspect: self view context <goal>")
