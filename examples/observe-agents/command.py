#!/usr/bin/env python3
"""observe-agents - witness the hc terminals and the worktree of every goal dispatch made on this body.

usage: self run observe-agents

For each unreclaimed dispatch this body made, records one row: the status of
the dispatch's own hc terminal, the other terminals living in its worktree,
and the worktree's git state. Appends one agent.observed snapshot, and nothing
when the snapshot is unchanged.

  own terminal   a terminal named after the agent that is either the address
                 agent.started recorded or sits in the worktree. Status is its
                 agent status (working, blocked, error, idle, done), running
                 for a live terminal with no agent status, finished once it
                 exited, absent when there is none.
  others         live terminals in the worktree that the dispatch did not
                 start: someone's shell or session. Reclaim never touches
                 them, so it has to know they are there.
  worktree       present, git, dirty (uncommitted paths), unpushed (commits on
                 no remote), branch, and absorbed: the remote default branch
                 when it already holds every local commit (same patches, or
                 merging changes nothing), which is what a squash merge that
                 deleted the remote branch looks like.

A terminal is in a worktree when its launch folder or its current folder is
the worktree or inside it. Only dispatches with a worktree or a terminal left
are persisted; absence is derived by the views. Dispatches learned from
another body are left to that body: their paths mean nothing here.

This is the witnessing step that lets self view agents and self view reclaim
be pure replays.
Source: ~/self examples/observe-agents/command.py.
"""
import hashlib
import json
import os
import pwd
import shutil
import subprocess
import sys

HOME = os.environ.get("SELF_HERDR_HOME") or pwd.getpwuid(os.getuid()).pw_dir
ENV = dict(os.environ)
ENV["HOME"] = HOME
ENV["PATH"] = os.pathsep.join([HOME + "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin",
                               ENV.get("PATH", "")])
HC = os.environ.get("SELF_HC_BIN") or shutil.which("hc", path=ENV["PATH"])
GIT = shutil.which("git", path=ENV["PATH"]) or "/usr/bin/git"
BUSIEST = ("working", "blocked", "running", "error", "idle", "done", "finished")

if sys.argv[1:]:
    print("usage: self run observe-agents", file=sys.stderr)
    raise SystemExit(2)
if not HC:
    print("observe-agents: hc unavailable; set SELF_HC_BIN or put hc on PATH", file=sys.stderr)
    raise SystemExit(1)

events = []
for line in sys.stdin:
    try:
        event = json.loads(line)
    except (TypeError, ValueError):
        continue
    if isinstance(event, dict):
        events.append(event)

goals, removed = {}, set()
dispatches, authored = {}, set()
for event in events:
    by = event.get("by")
    if isinstance(by, str) and by:
        authored.add(by)
    payload = event.get("payload")
    if not isinstance(payload, dict):
        continue
    goal, name, agent = payload.get("goal"), event.get("name"), payload.get("agent")
    if name == "goal.created" and isinstance(goal, str):
        goals.setdefault(goal, {"status": "active"})
    elif name == "goal.updated" and goal in goals and payload.get("status"):
        goals[goal]["status"] = payload["status"]
    elif name == "goal.removed" and isinstance(goal, str):
        removed.add(goal)
    elif name == "agent.started" and isinstance(agent, str):
        if str(event.get("via") or "").startswith("learn:"):
            dispatches.pop(agent, None)
        else:
            dispatches[agent] = payload
    elif name == "agent.reclaimed" and isinstance(agent, str):
        dispatches.pop(agent, None)

closed_status = {"closed", "done", "completed", "cancelled", "removed"}


def goal_closed(goal):
    return goal in removed or str(goals.get(goal, {}).get("status", "active")).lower() in closed_status


def hc(*argv, timeout=30):
    return subprocess.run([HC, *argv], text=True, capture_output=True, env=ENV, timeout=timeout)


def roster():
    listed = hc("ls", "--all", "--json")
    if listed.returncode != 0:
        raise RuntimeError((listed.stderr or listed.stdout or "hc ls failed").strip())
    terminals = json.loads(listed.stdout).get("terminals")
    if not isinstance(terminals, list):
        raise RuntimeError("hc ls returned no terminal roster")
    found = []
    for terminal in terminals:
        if not isinstance(terminal, dict) or not terminal.get("address"):
            continue
        live = terminal.get("state") == "live"
        folders = {terminal.get("cwd")}
        if live:
            detail = hc("get", terminal["address"], "--json")
            if detail.returncode == 0:
                try:
                    folders.add((json.loads(detail.stdout).get("foreground") or {}).get("cwd"))
                except (TypeError, ValueError):
                    pass
        found.append({
            "address": terminal["address"],
            "name": terminal.get("name") or "",
            "live": live,
            "status": (terminal.get("status") or "running") if live else "finished",
            "folders": {os.path.realpath(folder) for folder in folders if isinstance(folder, str) and folder},
        })
    return found


try:
    terminals = roster()
except (RuntimeError, ValueError, OSError, subprocess.SubprocessError) as exc:
    print(f"observe-agents: {exc}", file=sys.stderr)
    raise SystemExit(1)


def within(terminal, path):
    root = os.path.realpath(path)
    return any(folder == root or folder.startswith(root + os.sep) for folder in terminal["folders"])


def git(*args, cwd=None):
    return subprocess.run([GIT, *args], text=True, capture_output=True, cwd=cwd, env=ENV, timeout=120)


def absorbed(path):
    default = git("symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD", cwd=path).stdout.strip()
    if not default:
        return ""
    cherry = git("cherry", default, "HEAD", cwd=path)
    marks = [mark for mark in cherry.stdout.split("\n") if mark] if cherry.returncode == 0 else []
    if marks and all(mark.startswith("-") for mark in marks):
        return default
    merged = git("merge-tree", "--write-tree", default, "HEAD", cwd=path)
    tree = git("rev-parse", default + "^{tree}", cwd=path).stdout.strip()
    if merged.returncode == 0 and tree and merged.stdout.split("\n")[0].strip() == tree:
        return default
    return ""


def worktree_state(path):
    if not isinstance(path, str) or not path or not os.path.isdir(path):
        return {"present": False}
    porcelain = git("status", "--porcelain", cwd=path)
    if porcelain.returncode != 0:
        return {"present": True, "git": False}
    unpushed = git("rev-list", "--count", "HEAD", "--not", "--remotes", cwd=path)
    count = unpushed.stdout.strip()
    state = {
        "present": True,
        "git": True,
        "dirty": len(porcelain.stdout.splitlines()),
        "unpushed": int(count) if count.isdigit() else -1,
        "branch": git("branch", "--show-current", cwd=path).stdout.strip(),
    }
    if state["unpushed"]:
        state["absorbed"] = absorbed(path)
    return state


observed = []
for name, dispatch in sorted(dispatches.items()):
    path = dispatch.get("worktree", "")
    state = worktree_state(path)
    present = bool(state.get("present"))
    own = [t for t in terminals if t["name"] == name
           and (t["address"] == dispatch.get("terminal") or (present and within(t, path)))]
    others = [t for t in terminals if t["live"] and t not in own and present and within(t, path)]
    if not present and not own:
        continue
    statuses = [t["status"] for t in own]
    row = {
        "agent": name,
        "goal": dispatch.get("goal", ""),
        "status": min(statuses, key=BUSIEST.index) if statuses else "absent",
        "terminals": sorted(t["address"] for t in own),
        "others": sorted(t["address"] for t in others),
        "pane": ", ".join(sorted(t["address"] for t in own)),
        "worktree": path,
        "goal_closed": goal_closed(dispatch.get("goal", "")),
        "has_authored": name in authored,
    }
    row.update(state)
    observed.append(row)

names = "\n".join(sorted(dispatches))
snapshot = {
    "schema": 4,
    "runtime": "hc",
    "scope": {
        "active_dispatches": len(dispatches),
        "live_terminals": sum(1 for t in terminals if t["live"]),
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
