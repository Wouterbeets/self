#!/usr/bin/env python3
"""reclaim - the reclaim plan replayed from the last agent.observed snapshot: what self run reclaim would take and why the rest is held.

usage: self view reclaim [--all] [--names|--tsv] [--by S]

  default   candidates only: unreclaimed dispatches whose goal is closed, the
            set self run reclaim takes with no arguments.
  --all     every unreclaimed dispatch, so you can see what naming one would do.
  --names   one name per line, only those that would actually be removed, for
            piping: self view reclaim --names | xargs -r self run reclaim
  --tsv     agent, verdict, detail, goal, dispatcher, worktree
  --by S    substring match on the dispatcher that signed agent.started

Verdicts: remove; already-gone (worktree absent, reclaim tombstones it); held
(wrote nothing yet and the goal is open: idle is not finished); blocked (own
terminal working or blocked, another terminal in the worktree, dirty,
unpushed and not absorbed by the default branch); unknown (not observed);
elsewhere (dispatched on another body).

A view, so it appends nothing and reads no clock: it is exactly as fresh as
the last self run observe-agents, and reclaim re-checks every guard live.
Rehearse with self run reclaim --dry-run.
Source: ~/self examples/reclaim/view.py.
"""
import json
import sys

USAGE = "usage: reclaim [--all] [--names|--tsv] [--by S]   (self brief reclaim for the verdicts)"


def payload(event):
    value = event.get("payload")
    return value if isinstance(value, dict) else {}


def seq(event):
    value = event.get("seq")
    return value if isinstance(value, int) and not isinstance(value, bool) else 0


argv = sys.argv[1:]
mode, show_all, want_by = "text", False, None
i = 0
while i < len(argv):
    a = argv[i]
    if a == "--all":
        show_all = True
    elif a == "--names":
        mode = "names"
    elif a == "--tsv":
        mode = "tsv"
    elif a == "--by":
        i += 1
        if i >= len(argv):
            print(USAGE, file=sys.stderr)
            raise SystemExit(2)
        want_by = argv[i].lower()
    elif a in ("-h", "--help"):
        print(USAGE, file=sys.stderr)
        raise SystemExit(2)
    else:
        print(f"reclaim: unknown argument {a}\n\n{USAGE}", file=sys.stderr)
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

dispatches, reclaimed, goals, removed = {}, set(), {}, set()
observation = None
for event in events:
    item, name = payload(event), event.get("name")
    agent, goal = item.get("agent"), item.get("goal")
    if name == "agent.started" and isinstance(agent, str):
        dispatches[agent] = event
        reclaimed.discard(agent)
    elif name == "agent.reclaimed" and isinstance(agent, str):
        reclaimed.add(agent)
    elif name == "agent.observed":
        observation = event
    elif name == "goal.created" and isinstance(goal, str):
        goals.setdefault(goal, "active")
    elif name == "goal.updated" and isinstance(goal, str) and item.get("status"):
        goals[goal] = str(item["status"])
    elif name == "goal.removed" and isinstance(goal, str):
        removed.add(goal)

CLOSED = {"closed", "done", "completed", "cancelled", "removed"}


def goal_closed(goal):
    return goal in removed or str(goals.get(goal, "active")).lower() in CLOSED


seen = {}
schema = 0
if observation:
    body = payload(observation)
    schema = body.get("schema", 1)
    seen = {r.get("agent"): r for r in body.get("agents", []) if isinstance(r, dict)}


BUSY = {"working", "blocked"}


def verdict(agent, event):
    door = str(event.get("via") or "")
    if door.startswith("learn:"):
        return "elsewhere", f"dispatched on {door[6:]}; reclaim it there"
    row = seen.get(agent)
    if row is None and schema >= 2 and seq(observation) > seq(event):
        return "already-gone", "no worktree and no terminal at the last observation; reclaim tombstones it"
    if row is None:
        return "unknown", "not in the last observation; run self run observe-agents"
    if row.get("present") is False:
        return "already-gone", "worktree absent; reclaim tombstones it"
    if row.get("others"):
        return "blocked", "not ours to close: " + ", ".join(row["others"]) + " in the worktree"
    if row.get("status") in BUSY:
        return "blocked", f"terminal {row.get('pane') or '?'} is {row['status']}"
    if row.get("git") is False:
        return "blocked", "not a usable git worktree"
    if "dirty" not in row:
        return "unknown", "observation predates worktree state; run self run observe-agents"
    if row.get("dirty"):
        return "blocked", f"dirty ({row['dirty']} uncommitted)"
    if row.get("unpushed") and not row.get("absorbed"):
        return "blocked", f"unpushed ({row['unpushed']} commits)"
    if not row.get("has_authored") and not row.get("goal_closed"):
        return "held", "has written nothing yet; idle is not finished"
    ends = f", ends {row['pane']}" if row.get("pane") else ""
    return "remove", f"{row.get('branch') or 'detached'}{ends}"


rows = []
for agent, event in dispatches.items():
    if agent in reclaimed:
        continue
    item = payload(event)
    goal = item.get("goal", "")
    closed = goal_closed(goal)
    if not show_all and not closed:
        continue
    by = event.get("by") or ""
    if want_by is not None and want_by not in by.lower():
        continue
    kind, detail = verdict(agent, event)
    rows.append({
        "seq": seq(event), "agent": agent, "verdict": kind, "detail": detail,
        "goal": goal, "by": by, "goal_closed": closed,
        "worktree": item.get("worktree", ""),
    })
rows.sort(key=lambda r: (r["verdict"] != "remove", -r["seq"]))

if mode == "names":
    for row in rows:
        if row["verdict"] == "remove":
            print(row["agent"])
    raise SystemExit(0)

if mode == "tsv":
    for row in rows:
        print("\t".join(str(row[k]) for k in ("agent", "verdict", "detail", "goal", "by", "worktree")))
    raise SystemExit(0)

scope = "every unreclaimed dispatch" if show_all else "candidates: goal closed"
print(f"# Reclaim plan — {scope}")
if not observation:
    print("No observation yet, so no verdict can be replayed. Run: self run observe-agents")
elif schema < 3:
    print(f"Observation [{seq(observation)}] predates worktree state (schema {schema}); verdicts are incomplete. Run: self run observe-agents")
else:
    print(f"Replaying observation [{seq(observation)}], witnessed {str(observation.get('occurred_at') or '?')[:16]}Z. A preview, not a promise: reclaim re-checks live.")

if not rows:
    print("\nNothing to reclaim." if not show_all else "\nNo unreclaimed dispatches.")
    raise SystemExit(0)

order = ["remove", "already-gone", "held", "blocked", "unknown", "elsewhere"]
labels = {
    "remove": "Would remove", "already-gone": "Would tombstone (worktree already gone)",
    "held": "Held back", "blocked": "Blocked", "unknown": "Unknown",
    "elsewhere": "Dispatched on another body (not reclaimable here)",
}
for kind in order:
    group = [r for r in rows if r["verdict"] == kind]
    if not group:
        continue
    print(f"\n## {labels[kind]} ({len(group)})")
    for row in group[:20]:
        tag = "" if row["goal_closed"] else "  [goal still open — needs naming]"
        by = f" · by {row['by']}" if row["by"] else ""
        print(f"- {row['agent']} -> {row['goal']}{by}")
        print(f"  {row['detail']}{tag}")
    if len(group) > 20:
        print(f"- ... {len(group) - 20} more")

removable = sum(1 for r in rows if r["verdict"] == "remove")
print()
if not removable:
    print("Nothing is currently removable.")
elif not show_all:
    print("Rehearse it live:  self run reclaim --dry-run")
    print("Run it:  self run reclaim")
else:
    scoped = "--by \"$SELF_CALLER\"" if want_by is None else f'--by "{want_by}"'
    print(f"Run it, scoped to you:  self view reclaim --all --names {scoped} | xargs -r self run reclaim")
    print("Anything above signed by another dispatcher is someone else's live work.")
