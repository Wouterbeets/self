#!/usr/bin/env python3
'''feedback - carry reviewers' comments on a goal's PR to the agent that owns the goal, and let recurring ones become practices.

usage: self run feedback tick [--dry-run] [--goal <goal>]
       self run feedback status

tick: for every goal bound to an open PR (ship.tracked), read the PR's
review comments, reviews with a body and conversation comments (gh api).
Bots and the PR's own author (the account agents push and reply as) are
skipped. Each new one is appended as feedback.received. The first time a PR
is seen, the comments already on it are recorded as baseline and not
delivered: they predate the loop.

New comments are delivered once, grouped per goal:
  send   the goal's agent terminal is alive (latest agent.started or
         working-on for the goal, hc get): the comments are pasted into it.
  spawn  otherwise a worker is dispatched on the goal (self run dispatch),
         told to work on the PR's own branch.
The message asks the agent to fix and push with pr.push, answer each thread
on GitHub, and, when a comment names something that will come back on other
PRs, declare it as a practice (self run practice set) citing the comment.
A failed delivery is recorded and retried at the next tick.

status: per goal, comments received, delivered and pending.

The ship driver (self-ship-drive, every 5 min) runs tick after ship tick.
Events: feedback.received {goal, pr, id, kind, author, path?, line?, url, body, created_at, baseline}
        feedback.delivered {goal, pr, ids[], how, terminal?}
        feedback.failed {goal, pr, ids[], error}
Source: ~/self examples/feedback/command.py.
'''
import json
import os
import pwd
import re
import shutil
import subprocess
import sys

HOME = pwd.getpwuid(os.getuid()).pw_dir
ENV = dict(os.environ, HOME=HOME)
ENV["PATH"] = os.pathsep.join([HOME + "/go/bin", HOME + "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin",
                               "/usr/bin", "/bin", ENV.get("PATH", "")])
SELF = os.environ.get("SELF_BIN") or shutil.which("self", path=ENV["PATH"]) or "self"
GH = os.environ.get("SELF_GH_BIN") or shutil.which("gh", path=ENV["PATH"]) or "gh"
HC = os.environ.get("SELF_HC_BIN") or shutil.which("hc", path=ENV["PATH"]) or "hc"
REPO = os.environ.get("SELF_FEEDBACK_REPO") or HOME + "/mym"
ALIVE = {"idle", "working", "blocked", "waiting"}


def say(msg):
    print("feedback: " + msg, file=sys.stderr)


def emit(name, payload):
    print(json.dumps({"name": name, "payload": payload}, ensure_ascii=False, sort_keys=True))
    sys.stdout.flush()


def run(cmd, timeout=60, env=None):
    tries = 3 if cmd[:1] == [GH] else 1
    for n in range(tries):
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout, env=env or ENV)
        if r.returncode == 0:
            break
    return r


tracked, terminals, received, delivered, closed = {}, {}, {}, set(), set()
for raw in sys.stdin:
    try:
        e = json.loads(raw)
    except ValueError:
        continue
    n, p = e.get("name"), e.get("payload") or {}
    if n == "ship.tracked" and p.get("goal") and p.get("pr"):
        tracked[p["goal"]] = p["pr"]
    elif n == "agent.started" and p.get("goal") and p.get("terminal"):
        terminals[p["goal"]] = p["terminal"]
    elif n == "goal.worked" and p.get("goal") and p.get("terminal"):
        terminals[p["goal"]] = p["terminal"]
    elif n == "feedback.received" and p.get("id"):
        received[str(p["id"])] = p
    elif n == "feedback.delivered":
        delivered.update(str(i) for i in p.get("ids", []))
    elif n == "goal.updated" and p.get("goal") and p.get("status"):
        (closed.add if p["status"] == "closed" else closed.discard)(p["goal"])
    elif n == "goal.removed" and p.get("goal"):
        closed.add(p["goal"])

args = sys.argv[1:]
cmd = args[0] if args else ""

if cmd == "status":
    rows = {}
    for c in received.values():
        r = rows.setdefault(c["goal"], [c["pr"], 0, 0, 0])
        if c.get("baseline"):
            continue
        r[1] += 1
        r[2 if str(c["id"]) in delivered else 3] += 1
    if not rows:
        print("no feedback received yet", file=sys.stderr)
    for goal, (pr, got, done, pending) in sorted(rows.items()):
        print("%s\t%s\treceived %d\tdelivered %d\tpending %d" % (goal, pr, got, done, pending), file=sys.stderr)
    raise SystemExit(0)

if cmd != "tick":
    say("usage: feedback tick [--dry-run] [--goal <goal>] | feedback status")
    raise SystemExit(2)

dry = "--dry-run" in args
only = args[args.index("--goal") + 1] if "--goal" in args and args.index("--goal") + 1 < len(args) else None


def gh_env(owner):
    r = run([GH, "auth", "token", "--user", os.environ.get("SELF_GH_USER") or owner], timeout=20)
    return dict(ENV, GH_TOKEN=r.stdout.strip()) if r.returncode == 0 and r.stdout.strip() else ENV


def api(path, env):
    r = run([GH, "api", "--paginate", path], env=env)
    if r.returncode:
        raise RuntimeError("gh api %s: %s" % (path, r.stderr.strip()[-300:]))
    out = []
    for chunk in re.split(r"(?<=\])\s*(?=\[)", r.stdout.strip() or "[]"):
        out.extend(json.loads(chunk))
    return out


def comments_of(owner_repo, number, env):
    pr = json.loads(run([GH, "api", "repos/%s/pulls/%s" % (owner_repo, number)], env=env).stdout or "{}")
    if not pr:
        raise RuntimeError("cannot read PR %s#%s" % (owner_repo, number))
    author = (pr.get("user") or {}).get("login")
    found = []
    for c in api("repos/%s/pulls/%s/comments" % (owner_repo, number), env):
        found.append(("inline", c, c.get("path"), c.get("line") or c.get("original_line")))
    for c in api("repos/%s/pulls/%s/reviews" % (owner_repo, number), env):
        if (c.get("body") or "").strip():
            found.append(("review:" + (c.get("state") or "").lower(), c, None, None))
    for c in api("repos/%s/issues/%s/comments" % (owner_repo, number), env):
        found.append(("conversation", c, None, None))
    out = []
    for kind, c, path, line in found:
        user = c.get("user") or {}
        if user.get("type") == "Bot" or user.get("login") == author or user.get("login", "").endswith("[bot]"):
            continue
        item = {"id": str(c["id"]), "kind": kind, "author": user.get("login"), "url": c.get("html_url"),
                "body": (c.get("body") or "")[:2000], "created_at": c.get("created_at") or c.get("submitted_at")}
        if path:
            item.update(path=path, line=line)
        if kind == "inline" and c.get("in_reply_to_id"):
            item["reply_to"] = str(c["in_reply_to_id"])
        out.append(item)
    return pr, out


def message(goal, url, branch, items):
    lines = ["Review feedback arrived on %s (goal %s, branch %s):" % (url, goal, branch), ""]
    for c in items:
        where = "%s:%s " % (c["path"], c["line"]) if c.get("path") else ""
        body = re.sub(r"\s+", " ", c["body"]).strip()
        lines.append("- @%s %s[%s] %s (%s, comment id %s)" % (c["author"], where, c["kind"], body[:600], c["url"], c["id"]))
    lines += ["",
              "For each comment: fix it in a commit and push with self run pr.push, or decide it should not change "
              "and say why. Then answer it on GitHub as one short reply naming what changed (inline: gh api "
              "repos/<owner>/<repo>/pulls/<n>/comments/<id>/replies -f body=...; otherwise gh pr comment). "
              "If a comment names something reviewers will ask for again on other PRs, make it a practice so no "
              "agent repeats it: self run practice set <area>/<name> --title ... --rule ... --when <globs> --at work "
              "(self brief practice), citing this comment's URL in the rule. Report with self run goal progress %s." % goal]
    return "\n".join(lines)


def alive(term):
    if not term:
        return False
    r = run([HC, "get", term], timeout=15)
    if r.returncode:
        return False
    try:
        return (json.loads(r.stdout).get("status") or "") in ALIVE
    except ValueError:
        return False


for goal, url in sorted(tracked.items()):
    if goal in closed or (only and goal != only):
        continue
    m = re.search(r"github\.com/([^/]+/[^/]+)/pull/(\d+)", url)
    if not m:
        continue
    owner_repo, number = m.group(1), m.group(2)
    env = gh_env(owner_repo.split("/")[0])
    try:
        pr, found = comments_of(owner_repo, number, env)
    except (RuntimeError, ValueError, subprocess.TimeoutExpired) as err:
        say("%s: %s" % (goal, err))
        continue
    if pr.get("state") != "open":
        continue
    first_sight = not any(c.get("pr") == url for c in received.values())
    fresh = []
    for c in found:
        if c["id"] in received:
            continue
        c.update(goal=goal, pr=url, baseline=first_sight)
        received[c["id"]] = c
        if not dry:
            emit("feedback.received", c)
        if not first_sight:
            fresh.append(c)
    pending = [c for c in received.values() if c.get("pr") == url and not c.get("baseline")
               and c["id"] not in delivered]
    if first_sight and found:
        say("%s: %d existing comment(s) on %s recorded as baseline" % (goal, len(found), url))
    if not pending:
        continue
    branch = (pr.get("head") or {}).get("ref", "")
    text = message(goal, url, branch, sorted(pending, key=lambda c: c.get("created_at") or ""))
    ids = [c["id"] for c in pending]
    term = terminals.get(goal)
    if dry:
        say("%s: would deliver %d comment(s) to %s\n%s" % (goal, len(ids), term if alive(term) else "a new worker", text))
        continue
    if alive(term):
        r = run([HC, "send", term, text], timeout=60)
        if r.returncode == 0:
            emit("feedback.delivered", {"goal": goal, "pr": url, "ids": ids, "how": "send", "terminal": term})
            say("%s: %d comment(s) sent to %s" % (goal, len(ids), term))
            delivered.update(ids)
            continue
        say("%s: hc send %s failed, dispatching instead: %s" % (goal, term, r.stderr.strip()[-200:]))
    prompt = (text + "\n\nYou start in a fresh worktree on its own branch, but the PR lives on %s. First: "
              "git fetch origin %s && git switch %s (git switch -c %s --track origin/%s if it is not local). If "
              "git says another worktree holds it, cd to that worktree (git worktree list) and work there. "
              "pr.push from that branch updates the PR." % (branch, branch, branch, branch, branch))
    name = re.sub(r"[^a-z0-9-]", "-", goal.lower())[:26].strip("-") + "-fb"
    r = run([SELF, "run", "dispatch", "--area", os.environ.get("SELF_SHIP_AREA", "mym"), "--pane",
             "feedback on #" + number, goal, name, "claude", REPO, prompt], timeout=300,
            env=dict(ENV, SELF_DISPATCH_AGENT_ARGS="--model " + os.environ.get("SELF_SHIP_MODEL", "opus")))
    if r.returncode == 0 and "agent.started" in (r.stdout + r.stderr):
        emit("feedback.delivered", {"goal": goal, "pr": url, "ids": ids, "how": "spawn", "worker": name})
        say("%s: %d comment(s) given to new worker %s" % (goal, len(ids), name))
    else:
        emit("feedback.failed", {"goal": goal, "pr": url, "ids": ids,
                                 "error": (r.stderr or r.stdout).strip()[-400:]})
        say("%s: delivery failed: %s" % (goal, (r.stderr or r.stdout).strip()[-300:]))
