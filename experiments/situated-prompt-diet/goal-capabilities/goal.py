#!/usr/bin/env python3
import json
import sys

USAGE = "usage: goal create <id> <title> <outcome> [project] [parent] [depends_on...] | depends <id> [dependency...] | update|progress|compact|project|complete|milestone|kpi|close|delete ..."
args = sys.argv[1:]
if not args:
    print(USAGE, file=sys.stderr)
    raise SystemExit(2)

events = []
for line in sys.stdin:
    try:
        event = json.loads(line)
    except (TypeError, ValueError):
        continue
    if isinstance(event, dict):
        events.append(event)


def emit(name, payload):
    print(json.dumps({"name": name, "payload": payload}, ensure_ascii=False))


goals = {}
removed = set()
for event in events:
    payload = event.get("payload")
    if not isinstance(payload, dict):
        continue
    goal = payload.get("goal")
    if event.get("name") == "goal.created" and isinstance(goal, str):
        goals.setdefault(goal, dict(payload))
    elif event.get("name") == "goal.updated" and goal in goals:
        if payload.get("status"):
            goals[goal]["status"] = payload["status"]
        if isinstance(payload.get("depends_on"), list):
            goals[goal]["depends_on"] = payload["depends_on"]
    elif event.get("name") == "goal.removed" and isinstance(goal, str):
        removed.add(goal)

closed = {"closed", "done", "completed", "cancelled", "removed"}


def is_closed(goal):
    return goal in removed or str(goals.get(goal, {}).get("status", "active")).lower() in closed


def blockers(goal):
    return [dependency for dependency in goals.get(goal, {}).get("depends_on", []) if not is_closed(dependency)]


def validate_dependencies(goal, dependencies):
    dependencies = list(dict.fromkeys(dependency for dependency in dependencies if dependency))
    missing = [dependency for dependency in dependencies if dependency not in goals or dependency in removed]
    if goal in dependencies:
        print(f"goal {goal} cannot depend on itself", file=sys.stderr)
        raise SystemExit(2)
    if missing:
        print("unknown dependencies: " + ", ".join(missing), file=sys.stderr)
        raise SystemExit(2)
    graph = {name: list(item.get("depends_on", [])) for name, item in goals.items()}
    graph[goal] = dependencies

    def reaches(start, target, seen):
        if start == target:
            return True
        if start in seen:
            return False
        seen.add(start)
        return any(reaches(item, target, seen) for item in graph.get(start, []))

    if any(reaches(dependency, goal, set()) for dependency in dependencies):
        print(f"dependencies would create a cycle through {goal}", file=sys.stderr)
        raise SystemExit(2)
    return dependencies


kind = args[0]
if kind == "create" and len(args) >= 4:
    goal = args[1]
    if goal in goals:
        emit("goal.updated", {"goal": goal, "note": f"duplicate create attempt; title={args[2]} outcome={args[3]}"})
    else:
        dependencies = validate_dependencies(goal, args[6:])
        emit("goal.created", {"goal": goal, "title": args[2], "outcome": args[3], "project": args[4] if len(args) > 4 else "", "parent": args[5] if len(args) > 5 else "", "depends_on": dependencies, "status": "active"})
elif kind == "depends" and len(args) >= 2 and args[1] in goals:
    dependencies = validate_dependencies(args[1], args[2:])
    emit("goal.updated", {"goal": args[1], "depends_on": dependencies, "note": "dependencies replaced"})
elif kind in {"progress", "close"} and len(args) >= 2 and blockers(args[1]):
    print(f"goal {args[1]} is blocked by: {', '.join(blockers(args[1]))}", file=sys.stderr)
    raise SystemExit(2)
elif kind == "update" and len(args) >= 3:
    emit("goal.updated", {"goal": args[1], "note": " ".join(args[2:])})
elif kind == "progress" and len(args) >= 3:
    emit("goal.progress", {"goal": args[1], "report": " ".join(args[2:])})
elif kind == "compact" and len(args) >= 3:
    emit("goal.compacted", {"goal": args[1], "summary": " ".join(args[2:])})
elif kind == "project" and len(args) >= 3:
    if args[1].lower() in {"delete", "remove"}:
        emit("project.removed", {"project": args[2], "reason": " ".join(args[3:]) or "removed"})
    else:
        emit("project.registered", {"project": args[1], "repo": args[2], "default_base": args[3] if len(args) > 3 else ""})
elif kind == "complete" and len(args) >= 4:
    emit("goal.completion.requested", {"goal": args[1], "branch": args[2], "commit": args[3], "worktree_removed": len(args) > 4 and args[4].lower() == "true", "note": " ".join(args[5:])})
elif kind == "milestone" and len(args) >= 4:
    emit("goal.milestone", {"goal": args[1], "milestone": args[2], "status": " ".join(args[3:])})
elif kind == "kpi" and len(args) >= 5:
    emit("goal.kpi", {"goal": args[1], "name": args[2], "value": args[3], "target": " ".join(args[4:])})
elif kind == "close" and len(args) >= 2:
    emit("goal.updated", {"goal": args[1], "status": "closed", "note": " ".join(args[2:]) or "closed"})
elif kind in {"delete", "remove"} and len(args) >= 2:
    emit("goal.removed", {"goal": args[1], "reason": " ".join(args[2:]) or "removed"})
else:
    print(USAGE, file=sys.stderr)
    raise SystemExit(2)
