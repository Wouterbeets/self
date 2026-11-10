#!/usr/bin/env python3
"""reclaim - remove finished goal workers: end the terminal dispatch started, remove the worktree and local branch, tombstone the dispatch.

usage: self run reclaim [--dry-run] [--force] [--fresh-minutes N] [agent...]

  no agents  the default safe set: dispatches made on this body whose goal is closed.
  agent...   exactly these, whatever their goal state.
  --dry-run  run every live check, print the plan, remove and append nothing.
  --force    waive ONLY the freshly-dispatched guard.

Preview from the last observation with the pure view (self view reclaim, or
--all), then rehearse live with --dry-run. Name agents by piping, never with
$(...): an empty expansion is indistinguishable from no arguments and would
run the default set.
  self view reclaim --all --names --by "$SELF_CALLER" | xargs -r self run reclaim

Guards, re-checked live and never waived:
  - hc is reachable.
  - no terminal the dispatch did not start lives in the worktree. Someone's
    shell or session there is never ended and never has the folder removed
    from under it: close it yourself first.
  - the dispatch's own terminal is not working or blocked on a person.
  - the worktree has no uncommitted paths.
  - every commit is on a remote, or already in the remote default branch
    (same patches, or merging it changes nothing): a squash merge deletes the
    remote branch, which would otherwise make every merged worker look
    unpushed forever.
Only --force waives the last one: a dispatch younger than --fresh-minutes
(default 30) that never signed an event is not finished, it has not started.

The dispatch's own terminal is one named after the agent that is either the
address agent.started recorded or sits in the worktree. It is ended with
hc keep --off, hc kill, hc rm (a kept terminal would otherwise respawn a
shell). Then git worktree remove, then git branch -D. agent.reclaimed records
the removed HEAD, so the commits stay findable.

Dispatches learned from another body are skipped: their worktrees live there.
A skip prints on stderr and appends nothing; only removals and tombstones
(worktree already gone) become events.
Source: ~/self examples/reclaim/command.py.
"""
import json
import os
import pwd
import shutil
import subprocess
import sys
from datetime import datetime, timezone

HOME = os.environ.get("SELF_HERDR_HOME") or pwd.getpwuid(os.getuid()).pw_dir
ENV = dict(os.environ)
ENV["HOME"] = HOME
ENV["PATH"] = os.pathsep.join([HOME + "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin",
                               ENV.get("PATH", "")])
HC = os.environ.get("SELF_HC_BIN") or shutil.which("hc", path=ENV["PATH"])
GIT = shutil.which("git", path=ENV["PATH"]) or "/usr/bin/git"
BUSY = {"working", "blocked"}
USAGE = "usage: reclaim [--dry-run] [--force] [--fresh-minutes N] [agent...]   (self brief reclaim for the guards)"

argv = sys.argv[1:]
force, dry = False, False
fresh_minutes = 30.0
names = []
i = 0
while i < len(argv):
    a = argv[i]
    if a == "--force":
        force = True
    elif a == "--dry-run":
        dry = True
    elif a == "--fresh-minutes":
        i += 1
        if i >= len(argv):
            print(USAGE, file=sys.stderr)
            raise SystemExit(2)
        try:
            fresh_minutes = float(argv[i])
        except ValueError:
            print(USAGE, file=sys.stderr)
            raise SystemExit(2)
    elif a in ("-h", "--help"):
        print(USAGE, file=sys.stderr)
        raise SystemExit(2)
    elif a.startswith("-"):
        print(f"reclaim: unknown option {a}\n\n{USAGE}", file=sys.stderr)
        raise SystemExit(2)
    else:
        names.append(a)
    i += 1

pending, plan = [], []


def emit(name, payload):
    pending.append({"name": name, "payload": payload})


def git(*args, cwd=None):
    return subprocess.run([GIT, *args], text=True, capture_output=True, cwd=cwd, env=ENV, timeout=120)


def hc(*args, timeout=30):
    return subprocess.run([HC, *args], text=True, capture_output=True, env=ENV, timeout=timeout)


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


def when(value):
    if not isinstance(value, str):
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


started, reclaimed = {}, set()
goal_status, goal_removed, authored = {}, set(), {}
for line in sys.stdin:
    try:
        event = json.loads(line)
    except (TypeError, ValueError):
        continue
    if not isinstance(event, dict):
        continue
    name = event.get("name")
    item = event.get("payload") if isinstance(event.get("payload"), dict) else {}
    by = event.get("by")
    if isinstance(by, str) and by:
        authored[by] = authored.get(by, 0) + 1
    goal = item.get("goal")
    if name == "goal.created" and isinstance(goal, str):
        goal_status.setdefault(goal, "active")
    elif name == "goal.updated" and isinstance(goal, str) and item.get("status"):
        goal_status[goal] = str(item["status"])
    elif name == "goal.removed" and isinstance(goal, str):
        goal_removed.add(goal)
    agent = item.get("agent")
    if not isinstance(agent, str):
        continue
    if name == "agent.started":
        door = str(event.get("via") or "")
        started[agent] = (item, event.get("occurred_at"), event.get("by"), door[6:] if door.startswith("learn:") else "")
        reclaimed.discard(agent)
    elif name == "agent.reclaimed":
        reclaimed.add(agent)

CLOSED = {"closed", "done", "completed", "cancelled", "removed"}


def goal_is_closed(goal):
    return goal in goal_removed or str(goal_status.get(goal, "active")).lower() in CLOSED


def skip(agent, note):
    plan.append(("skip", agent, note))


terminals, hc_ok = [], False
if HC:
    try:
        listed = hc("ls", "--all", "--json")
        roster = json.loads(listed.stdout).get("terminals") if listed.returncode == 0 else None
        if isinstance(roster, list):
            for terminal in roster:
                if not isinstance(terminal, dict) or not terminal.get("address"):
                    continue
                live = terminal.get("state") == "live"
                folders = {terminal.get("cwd")}
                if live:
                    detail = hc("get", terminal["address"], "--json")
                    if detail.returncode != 0:
                        raise RuntimeError("hc get failed for " + terminal["address"])
                    folders.add((json.loads(detail.stdout).get("foreground") or {}).get("cwd"))
                terminals.append({
                    "address": terminal["address"],
                    "name": terminal.get("name") or "",
                    "live": live,
                    "status": (terminal.get("status") or "running") if live else "finished",
                    "folders": {os.path.realpath(f) for f in folders if isinstance(f, str) and f},
                })
            hc_ok = True
    except Exception:
        terminals, hc_ok = [], False


def within(terminal, path):
    root = os.path.realpath(path)
    return any(f == root or f.startswith(root + os.sep) for f in terminal["folders"])


def end_terminal(terminal):
    address = terminal["address"]
    if terminal["live"]:
        hc("keep", address, "--off", "--json")
        hc("kill", address, "--json")
    hc("rm", address, "--json")
    return hc("get", address, "--json").returncode != 0


open_dispatches = {a: d for a, d in started.items() if a not in reclaimed}
if names:
    for n in [n for n in names if n not in started]:
        print(f"reclaim: no dispatch named {n}", file=sys.stderr)
    for n in [n for n in names if n in reclaimed]:
        print(f"reclaim: {n} is already reclaimed", file=sys.stderr)
    selected = [n for n in names if n in open_dispatches]
    explicit = True
else:
    selected = [a for a, (item, _, _, body) in open_dispatches.items()
                if not body and goal_is_closed(item.get("goal", ""))]
    explicit = False

now = datetime.now(timezone.utc)

for agent in selected:
    dispatch, occurred_at, dispatched_by, body = open_dispatches[agent]
    worktree = dispatch.get("worktree")
    base = {"agent": agent, "goal": dispatch.get("goal"), "worktree": worktree}
    if body:
        skip(agent, f"dispatched on {body}; reclaim it there")
        continue
    if not isinstance(worktree, str) or not worktree:
        continue
    present = os.path.isdir(worktree)
    own = [t for t in terminals if t["name"] == agent
           and (t["address"] == dispatch.get("terminal") or (present and within(t, worktree)))]
    if not present:
        if dry:
            plan.append(("would-tombstone", agent, "worktree already absent"))
            continue
        for terminal in own:
            if not terminal["live"]:
                end_terminal(terminal)
        plan.append(("gone", agent, "worktree already absent"))
        emit("agent.reclaimed", dict(base, method="already-gone"))
        continue
    if not hc_ok:
        skip(agent, "hc unreachable")
        continue
    others = [t for t in terminals if t["live"] and t not in own and within(t, worktree)]
    if others:
        skip(agent, "not ours to close: " + ", ".join(f"{t['address']} ({t['name'] or t['status']})" for t in others)
             + " in the worktree")
        continue
    busy = [t for t in own if t["status"] in BUSY]
    if busy:
        skip(agent, ", ".join(f"{t['status']} in {t['address']}" for t in busy))
        continue
    age_minutes = None
    stamp = when(occurred_at)
    if stamp is not None:
        age_minutes = (now - stamp).total_seconds() / 60.0
    if (not force and age_minutes is not None and age_minutes < fresh_minutes
            and not authored.get(agent)):
        skip(agent, f"fresh ({age_minutes:.0f}m, no events)")
        continue
    porcelain = git("status", "--porcelain", cwd=worktree)
    if porcelain.returncode != 0:
        skip(agent, "not a git worktree")
        continue
    if porcelain.stdout.strip():
        skip(agent, f"dirty ({len(porcelain.stdout.splitlines())} paths)")
        continue
    unpushed = git("rev-list", "--count", "HEAD", "--not", "--remotes", cwd=worktree)
    merged_into = ""
    if unpushed.returncode != 0 or unpushed.stdout.strip() != "0":
        merged_into = absorbed(worktree) if unpushed.returncode == 0 else ""
        if not merged_into:
            skip(agent, f"unpushed ({unpushed.stdout.strip()} commits)")
            continue
    head = git("rev-parse", "HEAD", cwd=worktree).stdout.strip()
    branch = git("branch", "--show-current", cwd=worktree).stdout.strip()
    common = git("rev-parse", "--path-format=absolute", "--git-common-dir", cwd=worktree).stdout.strip()
    mainrepo = os.path.dirname(common) if common.endswith("/.git") else ""
    if not mainrepo:
        skip(agent, "unresolvable: no main repository")
        continue
    addresses = sorted(t["address"] for t in own)
    detail = (f"{branch or 'detached'}" + (f", ends {', '.join(addresses)}" if addresses else "")
              + (f", {unpushed.stdout.strip()} local commits already in {merged_into}" if merged_into else ""))
    if dry:
        plan.append(("would-remove", agent, detail))
        continue
    stuck = [t["address"] for t in own if not end_terminal(t)]
    if stuck:
        skip(agent, "could not end " + ", ".join(stuck))
        continue
    if git("worktree", "remove", worktree, cwd=mainrepo).returncode != 0:
        skip(agent, "worktree removal failed" + (f" after ending {', '.join(addresses)}" if addresses else ""))
        continue
    branch_deleted = bool(branch) and git("branch", "-D", branch, cwd=mainrepo).returncode == 0
    plan.append(("removed", agent, detail))
    emit("agent.reclaimed", dict(base, method="removed", branch=branch, branch_deleted=branch_deleted, head=head,
                                 absorbed_by=merged_into,
                                 live_status=", ".join(t["status"] for t in own) or "absent",
                                 terminals=addresses, runtime="hc", dispatched_by=dispatched_by,
                                 selected="named" if explicit else "goal-closed"))

scope = "named" if explicit else "goal-closed"
if not selected:
    extra = "" if explicit else "; nothing made here has a closed goal. See what naming one would do: self view reclaim --all"
    print(f"reclaim: nothing selected ({scope}){extra}", file=sys.stderr)
for kind, agent, note in plan:
    print(f"{kind:15} {agent}  {note}", file=sys.stderr)
counts = {}
for kind, _, _ in plan:
    counts[kind] = counts.get(kind, 0) + 1
if plan:
    print("reclaim" + (" (dry run)" if dry else "") + ": " + ", ".join(f"{v} {k}" for k, v in sorted(counts.items())),
          file=sys.stderr)

if not dry:
    for event in pending:
        print(json.dumps(event))
