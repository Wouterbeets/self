#!/usr/bin/env python3
"""agents - goal workers by state, replayed from the last agent.observed snapshot.

usage: self view agents [--names|--tsv] [--by S] [--goal S] [--status S] [--reclaimable]

  --names        one agent name per line and nothing else, for piping.
  --tsv          seq, agent, status, goal, dispatcher, terminal, worktree
  --by S         substring match on the dispatcher that signed agent.started
  --goal S       substring match on the goal
  --status S     one of the states below
  --reclaimable  goal_closed only: the set self run reclaim takes with no arguments

States: working, blocked (waiting on a person), running (a live terminal with
no agent status), error, idle, done, finished (the terminal exited), absent
(no terminal left, worktree still on disk), failed (dispatch failed),
not_checked (dispatched after the last observation), not_observed (seen
neither as terminal nor as worktree), unknown, goal_closed, elsewhere
(dispatched on another body, which is the one that can see it).

Without --by a reader cannot tell whose workers these are, and cleanup gets
aimed at another session's live work. A view: as fresh as the last
self run observe-agents.
Source: ~/self examples/observe-agents/view.py.
"""
import json
import sys

USAGE = "usage: agents [--names|--tsv] [--by S] [--goal S] [--status S] [--reclaimable]   (self brief agents for the states)"


def seq(event):
    value = event.get("seq")
    return value if isinstance(value, int) and not isinstance(value, bool) else 0


def payload(event):
    value = event.get("payload")
    return value if isinstance(value, dict) else {}


argv = sys.argv[1:]
mode = "text"
want_by = want_goal = want_status = None
i = 0
while i < len(argv):
    a = argv[i]
    if a == "--names":
        mode = "names"
    elif a == "--tsv":
        mode = "tsv"
    elif a == "--reclaimable":
        want_status = "goal_closed"
    elif a in ("--by", "--goal", "--status"):
        i += 1
        if i >= len(argv):
            print(USAGE, file=sys.stderr)
            raise SystemExit(2)
        if a == "--by":
            want_by = argv[i].lower()
        elif a == "--goal":
            want_goal = argv[i].lower()
        else:
            want_status = argv[i].lower()
    elif a in ("-h", "--help"):
        print(USAGE, file=sys.stderr)
        raise SystemExit(2)
    else:
        print(f"agents: unknown argument {a}\n\n{USAGE}", file=sys.stderr)
        raise SystemExit(2)
    i += 1

events = []
for line in sys.stdin:
    try:
        event = json.loads(line)
    except (TypeError, ValueError):
        continue
    if isinstance(event, dict):
        events.append(event)

dispatches, failed, reclaimed = {}, set(), set()
observation = None
goals, removed = {}, set()
for event in events:
    item, name = payload(event), event.get("name")
    agent, goal = item.get("agent"), item.get("goal")
    if name == "agent.started" and isinstance(agent, str):
        dispatches[agent] = event
        failed.discard(agent)
        reclaimed.discard(agent)
    elif name == "agent.reclaimed" and isinstance(agent, str):
        reclaimed.add(agent)
    elif name == "agent.failed" and isinstance(agent, str):
        failed.add(agent)
    elif name == "agent.observed":
        observation = event
    elif name == "goal.created" and isinstance(goal, str):
        goals.setdefault(goal, {"status": "active"})
    elif name == "goal.updated" and goal in goals and item.get("status"):
        goals[goal]["status"] = item["status"]
    elif name == "goal.removed" and isinstance(goal, str):
        removed.add(goal)

closed_status = {"closed", "done", "completed", "cancelled", "removed"}


def goal_closed(goal):
    return goal in removed or str(goals.get(goal, {}).get("status", "active")).lower() in closed_status


observed = {}
if observation:
    observed = {
        item.get("agent"): item
        for item in payload(observation).get("agents", [])
        if isinstance(item, dict) and isinstance(item.get("agent"), str)
    }

KNOWN = {"working", "blocked", "running", "error", "idle", "done", "finished", "absent"}
rows = []
reclaimed_count = sum(1 for agent in dispatches if agent in reclaimed)
observation_seq = seq(observation or {})
for agent, event in dispatches.items():
    if agent in reclaimed:
        continue
    item = payload(event)
    live = observed.get(agent, {})
    door = str(event.get("via") or "")
    if door.startswith("learn:"):
        status = "elsewhere"
    elif goal_closed(item.get("goal", "")):
        status = "goal_closed"
    elif agent in failed:
        status = "failed"
    elif agent in observed:
        status = live.get("status", "unknown")
        if status not in KNOWN:
            status = "unknown"
    elif observation_seq > seq(event):
        status = "not_observed"
    else:
        status = "not_checked"
    rows.append({
        "seq": seq(event), "agent": agent, "status": status,
        "goal": item.get("goal") or "", "by": event.get("by") or "",
        "pane": live.get("pane") or item.get("pane") or "?",
        "worktree": live.get("worktree") or item.get("worktree") or item.get("branch") or "",
    })

kept = []
for row in rows:
    if want_by is not None and want_by not in row["by"].lower():
        continue
    if want_goal is not None and want_goal not in row["goal"].lower():
        continue
    if want_status is not None and want_status != row["status"]:
        continue
    kept.append(row)
kept.sort(key=lambda r: r["seq"], reverse=True)

if mode == "names":
    for row in kept:
        print(row["agent"])
    raise SystemExit(0)

if mode == "tsv":
    for row in kept:
        print("\t".join(str(row[k]) for k in ("seq", "agent", "status", "goal", "by", "pane", "worktree")))
    raise SystemExit(0)

filters = []
if want_by is not None:
    filters.append(f"by~{want_by}")
if want_goal is not None:
    filters.append(f"goal~{want_goal}")
if want_status is not None:
    filters.append(f"status={want_status}")

print("# Goal agents" + (f" ({', '.join(filters)}: {len(kept)} of {len(rows)})" if filters else ""))
if reclaimed_count and not filters:
    print(f"{reclaimed_count} reclaimed dispatches hidden (worktrees removed; see agent.reclaimed events).")
if observation:
    schema = payload(observation).get("schema", 1)
    if payload(observation).get("runtime") == "hc":
        scope = payload(observation).get("scope", {})
        print(f"Observation [{observation_seq}] {str(observation.get('occurred_at') or '')[:16]}Z: {len(observed)} worktrees or terminals seen of {scope.get('active_dispatches', '?')} unreclaimed dispatches made here, {scope.get('live_terminals', '?')} live hc terminals. Refresh: self run observe-agents")
    else:
        print(f"Observation [{observation_seq}] predates hc (schema {schema}); statuses below are stale. Refresh: self run observe-agents")
else:
    print("No live observation yet. Run: self run observe-agents")

labels = {
    "working": "Working", "blocked": "Blocked, waiting on a person", "running": "Running a plain program",
    "error": "Errored", "idle": "Idle", "done": "Done", "finished": "Terminal exited",
    "absent": "No terminal left, worktree still on disk",
    "failed": "Failed", "not_observed": "Checked And Absent",
    "not_checked": "Not Checked Since Dispatch", "unknown": "Unknown",
    "elsewhere": "Dispatched on another body",
    "goal_closed": "Goal closed: self view reclaim says which can go",
}
order = ("working", "blocked", "running", "error", "idle", "done", "finished", "absent", "failed", "not_checked",
         "not_observed", "unknown", "goal_closed", "elsewhere")
for status in order:
    group = [r for r in kept if r["status"] == status]
    if not group:
        continue
    print(f"\n## {labels[status]} ({len(group)})")
    shown = group if status not in ("not_observed", "goal_closed", "elsewhere") else group[:10]
    for row in shown:
        by = f" · by {row['by']}" if row["by"] else ""
        print(f"- [{row['seq']}] {row['agent']} -> {row['goal']} ({row['pane']}){by}")
        if status != "goal_closed" and row["worktree"]:
            print(f"  {row['worktree']}")
    if len(shown) < len(group):
        print(f"- ... {len(group) - len(shown)} older omitted")

if not kept:
    print("\nNothing matches.")
else:
    print("\nRehearse the default safe set (closed goals):  self run reclaim --dry-run")
    print("Rehearse exactly what you see:  self view agents --names" +
          (" " + " ".join(
              (["--by", want_by] if want_by else []) +
              (["--goal", want_goal] if want_goal else []) +
              (["--status", want_status] if want_status else [])) if filters else "") +
          " | xargs -r self run reclaim --dry-run")
