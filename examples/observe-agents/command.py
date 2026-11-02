#!/usr/bin/env python3
import hashlib
import json
import os
import pwd
import shutil
import subprocess
import sys

HOME = os.environ.get("SELF_HERDR_HOME") or pwd.getpwuid(os.getuid()).pw_dir
PATH = os.pathsep.join([HOME + "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"])
HC = os.environ.get("SELF_HC_BIN") or shutil.which("hc", path=PATH) or "hc"

if sys.argv[1:]:
    print("usage: self run observe-agents", file=sys.stderr)
    raise SystemExit(2)

events = []
for line in sys.stdin:
    try:
        event = json.loads(line)
    except (TypeError, ValueError):
        continue
    if isinstance(event, dict):
        events.append(event)

goals, removed = {}, set()
for event in events:
    payload = event.get("payload")
    if not isinstance(payload, dict):
        continue
    goal = payload.get("goal")
    name = event.get("name")
    if name == "goal.created" and isinstance(goal, str):
        goals.setdefault(goal, {"status": "active"})
    elif name == "goal.updated" and goal in goals and payload.get("status"):
        goals[goal]["status"] = payload["status"]
    elif name == "goal.removed" and isinstance(goal, str):
        removed.add(goal)

closed_status = {"closed", "done", "completed", "cancelled", "removed"}


def goal_closed(goal):
    return goal in removed or str(goals.get(goal, {}).get("status", "active")).lower() in closed_status


# Closed-goal dispatches are exactly what reclaim targets, so they must be observed
# too: excluding them left the reclaim plan blind precisely where it mattered.
dispatches = {}
reclaimed = set()
authored = set()
for event in events:
    by = event.get("by")
    if isinstance(by, str) and by:
        authored.add(by)
    payload = event.get("payload")
    if not isinstance(payload, dict) or not isinstance(payload.get("agent"), str):
        continue
    if event.get("name") == "agent.started":
        dispatches[payload["agent"]] = payload
        reclaimed.discard(payload["agent"])
    elif event.get("name") == "agent.reclaimed":
        reclaimed.add(payload["agent"])
for agent in reclaimed:
    dispatches.pop(agent, None)

env = dict(os.environ)
env["HOME"] = HOME
env["PATH"] = PATH + os.pathsep + env.get("PATH", "")
# hc (herdr-core) is the terminal roster now; --all keeps finished terminals visible so
# reclaim still sees a worker that exited. A dispatch's hc terminal name is its agent name.
result = subprocess.run([HC, "ls", "--all", "--json"], text=True, capture_output=True, env=env, timeout=30)
if result.returncode != 0:
    print((result.stderr or result.stdout or "hc ls failed").strip(), file=sys.stderr)
    raise SystemExit(1)
try:
    live = json.loads(result.stdout).get("terminals", [])
except (TypeError, ValueError, AttributeError):
    print("observe-agents: hc ls returned invalid JSON", file=sys.stderr)
    raise SystemExit(1)

by_name = {}
for term in live:
    if isinstance(term, dict) and isinstance(term.get("name"), str):
        # Prefer the live terminal when a finished one shares its name.
        if term["name"] not in by_name or term.get("state") == "live":
            by_name[term["name"]] = term


def term_status(term):
    if not term:
        return "not in hc"
    if term.get("state") != "live":
        return term.get("state") or "finished"
    return term.get("status") or "live"


def git(*args, cwd=None):
    return subprocess.run(["/usr/bin/git", *args], text=True, capture_output=True,
                          cwd=cwd, env=env, timeout=60)


def worktree_state(path):
    """Live git state, witnessed here so a pure view can replay it."""
    if not isinstance(path, str) or not path or not os.path.isdir(path):
        return {"present": False}
    porcelain = git("status", "--porcelain", cwd=path)
    if porcelain.returncode != 0:
        return {"present": True, "git": False}
    unpushed = git("rev-list", "--count", "HEAD", "--not", "--remotes", cwd=path)
    count = unpushed.stdout.strip()
    return {
        "present": True,
        "git": True,
        "dirty": len(porcelain.stdout.splitlines()),
        "unpushed": int(count) if count.isdigit() else -1,
        "branch": git("branch", "--show-current", cwd=path).stdout.strip(),
    }


# Absence is derived by the view. Persist only positive observations; the scope
# digest makes a changed dispatch universe observable without storing one repeated
# not_observed row per missing agent.
observed = []
for name, dispatch in sorted(dispatches.items()):
    agent = by_name.get(name)
    path = (agent or {}).get("cwd") or dispatch.get("worktree", "")
    state = worktree_state(path)
    if not agent and not state.get("present"):
        continue
    row = {
        "agent": name,
        "goal": dispatch.get("goal", ""),
        "status": term_status(agent),
        "pane": (agent or {}).get("address") or dispatch.get("terminal") or dispatch.get("pane", ""),
        "worktree": path,
        "goal_closed": goal_closed(dispatch.get("goal", "")),
        "has_authored": name in authored,
    }
    row.update(state)
    observed.append(row)

names = "\n".join(sorted(dispatches))
snapshot = {
    "schema": 3,
    "scope": {
        "active_dispatches": len(dispatches),
        "agent_names_sha256": hashlib.sha256(names.encode()).hexdigest(),
    },
    "agents": observed,
}

latest = None
for event in events:
    if event.get("name") == "agent.observed" and isinstance(event.get("payload"), dict):
        latest = event["payload"]
if latest == snapshot:
    print("no changes", file=sys.stderr)
    raise SystemExit(0)

print(json.dumps({"name": "agent.observed", "payload": snapshot}, ensure_ascii=False))
