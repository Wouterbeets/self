#!/usr/bin/env python3
import json
import os
import pwd
import shutil
import subprocess
import sys
import time
from datetime import datetime, timezone

ENV = dict(os.environ)
ENV["HOME"] = os.environ.get("SELF_HERDR_HOME") or pwd.getpwuid(os.getuid()).pw_dir
ENV["PATH"] = os.pathsep.join([ENV["HOME"] + "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin",
                               "/usr/bin", "/bin", ENV.get("PATH", "")])
# hc (herdr-core) holds the worker terminals; the herdr 0.9 CLI is retired.
HC = os.environ.get("SELF_HC_BIN") or shutil.which("hc", path=ENV["PATH"]) or "hc"

USAGE = """usage: reclaim [--force] [--fresh-minutes N] [agent...]

  no agents  reclaim the default safe set: dispatches whose goal is closed.
  agent...   reclaim exactly these, whatever their goal state.
  --force    waive ONLY the freshly-dispatched guard. Never waives dirty or unpushed.

Preview before running — the plan is a view, so it appends nothing by construction:
  self view reclaim
  self view reclaim --all --by "$SELF_CALLER"

Names come from that view, through argv (stdin is the event log, so it cannot carry them):
  self run reclaim $(self view reclaim --all --names)"""

argv = sys.argv[1:]
force = False
fresh_minutes = 30.0
names = []
i = 0
while i < len(argv):
    a = argv[i]
    if a == "--force":
        force = True
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

pending = []


def emit(name, payload):
    pending.append({"name": name, "payload": payload})


def git(*args, cwd=None):
    return subprocess.run(["/usr/bin/git", *args], text=True, capture_output=True, cwd=cwd, env=ENV, timeout=60)


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
    # Any event a dispatched agent signed is proof it actually did something.
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
        started[agent] = (item, event.get("occurred_at"), event.get("by"))
        reclaimed.discard(agent)
    elif name == "agent.reclaimed":
        reclaimed.add(agent)

CLOSED = {"closed", "done", "completed", "cancelled", "removed"}


def goal_is_closed(goal):
    return goal in goal_removed or str(goal_status.get(goal, "active")).lower() in CLOSED


def skip(agent, note):
    # A skip is the absence of an action, not something that happened: it is fully
    # derivable from the observation the view replays, so it stays on stderr and
    # never becomes an event.
    plan.append(("skip", agent, note))


live, hc_ok = {}, False
try:
    out = subprocess.run([HC, "ls", "--all", "--json"], text=True, capture_output=True, env=ENV, timeout=30)
    if out.returncode == 0:
        for term in json.loads(out.stdout).get("terminals", []):
            if isinstance(term, dict) and isinstance(term.get("name"), str):
                if term["name"] not in live or term.get("state") == "live":
                    live[term["name"]] = term
        hc_ok = True
except Exception:
    pass


def term_status(term):
    if not term:
        return ""
    if term.get("state") != "live":
        return term.get("state") or "finished"
    return term.get("status") or "live"


open_dispatches = {a: d for a, d in started.items() if a not in reclaimed}
if names:
    unknown = [n for n in names if n not in started]
    for n in unknown:
        print(f"reclaim: no dispatch named {n}", file=sys.stderr)
    already = [n for n in names if n in reclaimed]
    for n in already:
        print(f"reclaim: {n} is already reclaimed", file=sys.stderr)
    selected = [n for n in names if n in open_dispatches]
    explicit = True
else:
    selected = [a for a, (item, _, _) in open_dispatches.items() if goal_is_closed(item.get("goal", ""))]
    explicit = False

plan = []
now = datetime.now(timezone.utc)

for agent in selected:
    dispatch, occurred_at, dispatched_by = open_dispatches[agent]
    worktree = dispatch.get("worktree")
    base = {"agent": agent, "goal": dispatch.get("goal"), "worktree": worktree}
    if not isinstance(worktree, str) or not worktree:
        continue
    if not os.path.isdir(worktree):
        plan.append(("gone", agent, "worktree already absent"))
        emit("agent.reclaimed", dict(base, method="already-gone"))
        continue
    term = live.get(agent)
    status = term_status(term)
    if not hc_ok:
        skip(agent, "hc unreachable")
        continue
    if status in ("working", "blocked"):
        skip(agent, status)
        continue
    # Idle is not finished. A dispatch made minutes ago sits idle with a clean,
    # fully-pushed worktree because it has not started, which is indistinguishable
    # from having finished unless we look at whether it ever wrote anything.
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
    if unpushed.returncode != 0 or unpushed.stdout.strip() != "0":
        skip(agent, f"unpushed ({unpushed.stdout.strip()} commits)")
        continue
    branch = git("branch", "--show-current", cwd=worktree).stdout.strip()
    common = git("rev-parse", "--path-format=absolute", "--git-common-dir", cwd=worktree).stdout.strip()
    mainrepo = os.path.dirname(common) if common.endswith("/.git") else ""
    workspace = dispatch.get("workspace")
    removed = False
    if mainrepo:
        via_git = git("worktree", "remove", worktree, cwd=mainrepo)
        removed = via_git.returncode == 0
        if not removed:
            skip(agent, "removal failed")
            continue
    if not removed:
        skip(agent, "unresolvable")
        continue
    # Close the worker's hc terminal only when it is the one dispatch started: same name
    # AND the address agent.started recorded. Anyone else's terminal is left alone.
    closed_terminal = ""
    if term and dispatch.get("via") == "hc" and term.get("address") == dispatch.get("terminal"):
        if term.get("state") == "live":
            subprocess.run([HC, "kill", term["address"]], text=True, capture_output=True, env=ENV, timeout=30)
        for _ in range(20):   # kill hangs up then escalates; rm succeeds once it has exited
            gone = subprocess.run([HC, "rm", term["address"]], text=True, capture_output=True, env=ENV, timeout=30)
            if gone.returncode == 0:
                closed_terminal = term["address"]
                break
            time.sleep(0.5)
    branch_deleted = False
    if branch and mainrepo:
        branch_deleted = git("branch", "-D", branch, cwd=mainrepo).returncode == 0
    plan.append(("removed", agent, f"{closed_terminal or workspace or '?'} {branch}"))
    emit("agent.reclaimed", dict(base, method="removed", branch=branch, branch_deleted=branch_deleted,
                                 live_status=status or "not in hc", workspace=workspace,
                                 terminal=closed_terminal,
                                 dispatched_by=dispatched_by, selected="named" if explicit else "goal-closed"))

scope = "named" if explicit else "goal-closed"
if not selected:
    extra = "" if explicit else "; nothing has a closed goal. See what naming one would do: self view reclaim --all"
    print(f"reclaim: nothing selected ({scope}){extra}", file=sys.stderr)
for kind, agent, note in plan:
    print(f"{kind:13} {agent}  {note}", file=sys.stderr)
counts = {}
for kind, _, _ in plan:
    counts[kind] = counts.get(kind, 0) + 1
if plan:
    print("reclaim: " + ", ".join(f"{v} {k}" for k, v in sorted(counts.items())), file=sys.stderr)

for event in pending:
    print(json.dumps(event))
