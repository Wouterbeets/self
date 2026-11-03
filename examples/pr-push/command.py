#!/usr/bin/env python3
'''pr.push - carry the current branch out as a pull request: check the practices at push, push, open or update the PR, bind it to the goal. The one way agents push.

usage: self run pr.push [--title <t>] [--body <b> | --body-file <f>] [--draft] [--goal <goal>]
                        [--base <ref>] [--remote <name>] [--force-with-lease]
                        [--override <practice>=<reason>]… [--until <step>] [--skip <step>[,<step>…]]
                        [--dry-run] [--steps]

Run it from the repo (any worktree). Steps run in order; each one reads what
the earlier ones found and may add events. `--steps` lists them:

  context  repo, branch, head, base (merge-base with --base, default origin/HEAD
           after a fetch), the goal (--goal, else a goal/<goal>/… branch, else
           this terminal's latest working-on). Refuses a detached HEAD and the
           default branch; warns about uncommitted tracked changes, which are
           not pushed.
  check    every practice declared `--at push` or `--at pr` whose globs match the
           files the branch changed (self run practice show). Both points are
           gated here, before anything leaves the machine. The PR title is
           resolved first (--title, else the open PR's, else the first commit's
           subject) and handed to checks as SELF_PR_TITLE. Each one with a
           --check runs it on exactly those files and only the lines the branch
           adds. Any finding stops pr.push here, prints the rule and the
           findings, exits 1, and appends nothing: fix, commit, run it again.
           `--override <practice>=<reason>` lets a failing practice through;
           the reason is recorded in pr.checked for the reviewer.
  push     git push -u <remote> <branch> (default origin), --force-with-lease
           only when asked.
  pr       the open PR for this branch is updated (pr.updated; a new --title or
           --body is applied), else one is created with gh under the resolved
           title and --body, else a body from the commits; --draft.
           gh runs as the account named after the repo owner when it holds one
           (SELF_GH_USER overrides), whatever account is active.
  track    with a goal: self run ship track <goal> <url>, so ship ticks CI, CD
           and rollout for it. A refused track warns and does not fail.

--until <step> stops after that step; --skip drops steps; --dry-run is
`--until check` and names what would happen. A failure after something left
the machine (push done, PR call failed) still records what happened plus
pr.push.failed {step, error} and says FAILED on stderr, so the log never
loses an effect; exit status is then 0, as dispatch does.

Events: pr.checked {repo, branch, head, base, title, practices[{practice, at, ok, findings, override?}]}
        pr.pushed {repo, remote, branch, head, forced}
        pr.opened {repo, url, number, branch, head, title, draft} | pr.updated {repo, url, number, head, title}
        pr.push.failed {repo, branch, head, step, error}
Extending: a step is a function step_<name>(ctx) -> [events] registered in
STEPS with whether it acts outside the machine. Deploy, monitor and the like
are next steps in that list; practices for them are declared `--at <step>`.
Source: ~/self examples/pr-push/command.py.
'''
import json
import os
import pwd
import re
import shutil
import subprocess
import sys

HOME = pwd.getpwuid(os.getuid()).pw_dir  # the kernel scrubs HOME
ENV = dict(os.environ, HOME=HOME)
ENV["PATH"] = os.pathsep.join([HOME + "/go/bin", HOME + "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin",
                               "/usr/bin", "/bin", ENV.get("PATH", "")])
SELF = os.environ.get("SELF_BIN") or shutil.which("self", path=ENV["PATH"]) or "self"
GH = os.environ.get("SELF_GH_BIN") or shutil.which("gh", path=ENV["PATH"]) or "gh"
CWD = os.environ.get("SELF_CWD", ".")


class Stop(Exception):
    pass


def say(msg):
    print("pr.push: " + msg, file=sys.stderr)


def fail(msg, code=2):
    say(msg)
    raise SystemExit(code)


def sh(cmd, cwd=None, timeout=300, check=True, env=None):
    r = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, env=env or ENV, timeout=timeout)
    if check and r.returncode:
        raise RuntimeError("%s: %s" % (" ".join(cmd[:3]), (r.stderr or r.stdout).strip()[-800:]))
    return r


def caller_environ():
    '''The kernel scrubs the environment; HC_TERMINAL_ID lives on the calling self process.'''
    r = subprocess.run(["/bin/ps", "eww", "-o", "command=", "-p", str(os.getppid())], capture_output=True, text=True)
    return dict(re.findall(r"(?:^|\s)(HC_[A-Z_]+)=(\S+)", r.stdout or ""))


# ---- the log, as fed on stdin ----
worked = {}
for raw in sys.stdin:
    try:
        e = json.loads(raw)
    except ValueError:
        continue
    if e.get("name") == "goal.worked":
        p = e.get("payload") or {}
        if p.get("terminal"):
            worked[p["terminal"]] = p.get("goal")

# ---- argv ----
VALUED = {"--title", "--body", "--body-file", "--goal", "--base", "--remote", "--override", "--until", "--skip"}
FLAGS = {"--draft", "--force-with-lease", "--dry-run", "--steps"}
opts, overrides = {}, {}
argv = sys.argv[1:]
i = 0
while i < len(argv):
    a = argv[i]
    if a in VALUED and i + 1 < len(argv):
        if a == "--override":
            key, sep, why = argv[i + 1].partition("=")
            if not sep or not why.strip():
                fail("--override needs <practice>=<reason>")
            overrides[key] = why.strip()
        else:
            opts[a] = argv[i + 1]
        i += 2
    elif a in FLAGS:
        opts[a] = True
        i += 1
    else:
        fail("unknown argument %r (self brief pr.push)" % a)


# ---- steps ----
def step_context(ctx):
    top = sh(["git", "rev-parse", "--show-toplevel"], cwd=CWD, check=False).stdout.strip()
    if not top:
        raise Stop("not inside a git repo: " + CWD)
    branch = sh(["git", "symbolic-ref", "-q", "--short", "HEAD"], cwd=top, check=False).stdout.strip()
    if not branch:
        raise Stop("detached HEAD: check out a branch first")
    remote = opts.get("--remote", "origin")
    sh(["git", "fetch", "-q", remote], cwd=top, timeout=180, check=False)
    default = sh(["git", "symbolic-ref", "-q", "--short", "refs/remotes/%s/HEAD" % remote], cwd=top,
                 check=False).stdout.strip() or remote + "/master"
    if branch in (default.split("/", 1)[-1], "master", "main"):
        raise Stop("refusing to push the default branch %s; branch off first (feat/…, fix/…)" % branch)
    base_ref = opts.get("--base", default)
    head = sh(["git", "rev-parse", "HEAD"], cwd=top).stdout.strip()
    base = sh(["git", "merge-base", base_ref, "HEAD"], cwd=top, check=False).stdout.strip()
    if not base:
        raise Stop("no merge-base between %s and HEAD" % base_ref)
    if base == head:
        raise Stop("HEAD has no commits beyond %s: nothing to push" % base_ref)
    if sh(["git", "status", "--porcelain", "--untracked-files=no"], cwd=top).stdout.strip():
        say("warning: uncommitted tracked changes stay local; only commits are pushed")
    url = sh(["git", "remote", "get-url", remote], cwd=top).stdout.strip()
    m = re.search(r"github\.com[:/]([^/]+/[^/]+?)(?:\.git)?$", url)
    goal = opts.get("--goal")
    if not goal and branch.startswith("goal/") and branch.count("/") >= 2:
        goal = branch.split("/")[1]
    if not goal:
        goal = worked.get(caller_environ().get("HC_TERMINAL_ID", ""))
    ctx.update(repo=top, branch=branch, remote=remote, default=default.split("/", 1)[-1], base=base,
               head=head, gh_repo=m.group(1) if m else None, goal=goal)
    say("%s %s at %s, %d commit(s) over %s%s" % (os.path.basename(top), branch, head[:9],
        int(sh(["git", "rev-list", "--count", base + "..HEAD"], cwd=top).stdout.strip() or 0), base_ref,
        ", goal " + goal if goal else ""))
    return []


GATES = ("push", "pr")  # insertion points pr.push gates before anything leaves the machine


def gh_env(owner):
    '''gh holds several accounts and only one is active: use the one named after the repo owner when gh has it
    (SELF_GH_USER overrides), so a work-active gh still opens PRs on a personal repo and the reverse.'''
    user = os.environ.get("SELF_GH_USER") or owner
    r = sh([GH, "auth", "token", "--user", user], check=False, timeout=20)
    token = r.stdout.strip() if r.returncode == 0 else ""
    return dict(ENV, GH_TOKEN=token) if token else ENV


def resolve_pr(ctx):
    '''The open PR for the branch (if any) and the title the PR will carry: --title, else the open PR's,
    else the first commit's subject.'''
    if "title" in ctx:
        return
    ctx["gh_env"], ctx["open_pr"] = ENV, None
    if ctx.get("gh_repo"):
        ctx["gh_env"] = gh_env(ctx["gh_repo"].split("/")[0])
        view = sh([GH, "pr", "list", "--repo", ctx["gh_repo"], "--head", ctx["branch"], "--state", "open",
                   "--json", "url,number,isDraft,title", "--limit", "1"], cwd=ctx["repo"], env=ctx["gh_env"],
                  check=False)
        found = json.loads(view.stdout or "[]") if view.returncode == 0 else []
        ctx["open_pr"] = found[0] if found else None
    subjects = sh(["git", "log", "--reverse", "--format=%s", ctx["base"] + "..HEAD"], cwd=ctx["repo"]).stdout
    subjects = [l for l in subjects.splitlines() if l.strip()]
    ctx["subjects"] = subjects
    ctx["title"] = opts.get("--title") or (ctx["open_pr"] or {}).get("title") or (subjects[0] if subjects else "")


def practices_at(ctx, point):
    r = sh([SELF, "run", "practice", "show", "--at", point, "--dir", ctx["repo"], "--base", ctx["base"], "--json"],
           cwd=ctx["repo"], check=False)
    if r.returncode:
        raise Stop("practice show failed: " + r.stderr.strip()[-400:])
    out = []
    for raw in r.stderr.splitlines():
        try:
            out.append(json.loads(raw))
        except ValueError:
            pass
    return out


def step_check(ctx):
    resolve_pr(ctx)
    env = dict(ENV, SELF_PR_TITLE=ctx["title"])
    seen, results, blocked = set(), [], []
    for point in GATES:
        for p in practices_at(ctx, point):
            if p["practice"] in seen:
                continue
            seen.add(p["practice"])
            res = {"practice": p["practice"], "at": point, "ok": True, "findings": 0}
            if p.get("check"):
                c = sh([SELF, "run", p["check"], "--base", ctx["base"], "--head", ctx["head"], *p.get("paths", [])],
                       cwd=ctx["repo"], check=False, env=env)
                found = [l for l in c.stderr.splitlines() if re.match(r"^[^\s:]+:\d+: ", l)]
                if c.returncode not in (0, 1):
                    raise Stop("check %s broke (exit %d): %s" % (p["check"], c.returncode, c.stderr.strip()[-400:]))
                res["findings"] = len(found)
                if c.returncode == 1:
                    res["ok"] = False
                    if p["practice"] in overrides:
                        res.update(ok=True, override=overrides[p["practice"]])
                        say("override %s (%d finding(s)): %s" % (p["practice"], len(found), overrides[p["practice"]]))
                    else:
                        blocked.append((p, found or c.stderr.strip().splitlines()))
            else:
                say("practice %s has no check; its rule: %s" % (p["practice"], p.get("rule", "")))
            results.append(res)
            if res["ok"] and not res.get("override"):
                say("ok %s (%d file(s))" % (p["practice"], len(p.get("paths", []))))
    if blocked:
        for p, found in blocked:
            print("\n✗ %s — %s\n  %s\n" % (p["practice"], p.get("title", ""), p.get("rule", "").replace("\n", "\n  ")),
                  file=sys.stderr)
            for f in found:
                print("  " + f, file=sys.stderr)
        print("\npr.push: blocked before push. Fix and commit (a title: --title '<type>(<scope>): <subject>'), then "
              "run pr.push again (or --override <practice>=<reason> when the rule truly does not apply).",
              file=sys.stderr)
        raise SystemExit(1)
    return [("pr.checked", {"repo": ctx["repo"], "branch": ctx["branch"], "head": ctx["head"], "base": ctx["base"],
                            "title": ctx["title"], "practices": results})]


def step_push(ctx):
    cmd = ["git", "push", "-u", ctx["remote"], "HEAD:refs/heads/" + ctx["branch"]]
    if opts.get("--force-with-lease"):
        cmd.insert(2, "--force-with-lease")
    sh(cmd, cwd=ctx["repo"], timeout=300)
    ctx["pushed"] = True
    say("pushed %s to %s" % (ctx["branch"], ctx["remote"]))
    return [("pr.pushed", {"repo": ctx["repo"], "remote": ctx["remote"], "branch": ctx["branch"],
                           "head": ctx["head"], "forced": bool(opts.get("--force-with-lease"))})]


def step_pr(ctx):
    if not ctx.get("gh_repo"):
        raise RuntimeError("remote is not on github.com; no PR step for it yet")
    resolve_pr(ctx)
    env, pr = ctx["gh_env"], ctx["open_pr"]
    if pr:
        ctx["url"] = pr["url"]
        edit = []
        if opts.get("--title") and opts["--title"] != pr.get("title"):
            edit += ["--title", opts["--title"]]
        if opts.get("--body-file"):
            edit += ["--body-file", os.path.join(CWD, opts["--body-file"])]
        elif opts.get("--body"):
            edit += ["--body", opts["--body"]]
        if edit:
            sh([GH, "pr", "edit", pr["url"], *edit], cwd=ctx["repo"], env=env)
        say("updated %s" % pr["url"])
        return [("pr.updated", {"repo": ctx["repo"], "url": pr["url"], "number": pr["number"], "head": ctx["head"],
                                "title": ctx["title"]})]
    cmd = [GH, "pr", "create", "--repo", ctx["gh_repo"], "--head", ctx["branch"], "--base", ctx["default"],
           "--title", ctx["title"]]
    if opts.get("--body-file"):
        cmd += ["--body-file", os.path.join(CWD, opts["--body-file"])]
    elif opts.get("--body"):
        cmd += ["--body", opts["--body"]]
    else:
        log = sh(["git", "log", "--reverse", "--format=%s%n%n%b", ctx["base"] + "..HEAD"], cwd=ctx["repo"]).stdout
        cmd += ["--body", log.strip() if len(ctx["subjects"]) == 1 else "\n".join("- " + s for s in ctx["subjects"])]
    if opts.get("--draft"):
        cmd += ["--draft"]
    out = sh(cmd, cwd=ctx["repo"], env=env).stdout.strip().splitlines()
    url = next((l for l in reversed(out) if "/pull/" in l), "")
    if not url:
        raise RuntimeError("gh pr create printed no PR url: " + " ".join(out)[-300:])
    ctx["url"] = url
    say("opened %s" % url)
    return [("pr.opened", {"repo": ctx["repo"], "url": url, "number": int(url.rstrip("/").rsplit("/", 1)[-1]),
                           "branch": ctx["branch"], "head": ctx["head"], "title": ctx["title"],
                           "draft": bool(opts.get("--draft"))})]


def step_track(ctx):
    if not ctx.get("goal") or not ctx.get("url"):
        say("no goal: not tracked by ship (pass --goal <goal> to bind it)" if ctx.get("url") else "no PR to track")
        return []
    r = sh([SELF, "run", "ship", "track", ctx["goal"], ctx["url"]], cwd=ctx["repo"], check=False)
    say(("ship tracks it under goal " + ctx["goal"]) if r.returncode == 0
        else "warning: ship track refused: " + r.stderr.strip()[-300:])
    return []


STEPS = [  # (name, function, acts outside this machine)
    ("context", step_context, False),
    ("check", step_check, False),
    ("push", step_push, True),
    ("pr", step_pr, True),
    ("track", step_track, False),
]

if opts.get("--steps"):
    for name, fn, effect in STEPS:
        print("%-8s %s" % (name, "acts outside" if effect else "local"), file=sys.stderr)
    raise SystemExit(0)

names = [n for n, _, _ in STEPS]
until = "check" if opts.get("--dry-run") else opts.get("--until", names[-1])
skip = {s for s in opts.get("--skip", "").split(",") if s}
for s in skip | {until}:
    if s not in names:
        fail("unknown step %r; steps: %s" % (s, ", ".join(names)))
if "context" in skip:
    fail("context cannot be skipped: every step reads it")

ctx, events, current = {}, [], None
try:
    for name, fn, effect in STEPS:
        if name in skip:
            continue
        current = name
        events += fn(ctx)
        if name == until:
            break
except Stop as exc:
    fail(str(exc), 1)
except SystemExit:
    raise
except Exception as exc:
    if not ctx.get("pushed"):  # nothing has left the machine: fail cleanly, append nothing
        fail("%s failed: %s" % (current, str(exc).strip()), 1)
    events.append(("pr.push.failed", {"repo": ctx.get("repo"), "branch": ctx.get("branch"), "head": ctx.get("head"),
                                      "step": current, "error": str(exc).strip()[-1500:]}))
    say("FAILED at %s after pushing: %s" % (current, str(exc).strip()))

if opts.get("--dry-run"):
    rest = [n for n in names[names.index("check") + 1:] if n not in skip]
    say("dry run: would run %s; nothing appended" % ", ".join(rest))
    raise SystemExit(0)

for name, payload in events:
    print(json.dumps({"name": name, "payload": payload}, ensure_ascii=False, sort_keys=True))
