#!/usr/bin/env python3
'''practice.backtest - replay the practice checks on what already merged, or on anyone's open PR: what each practice would have caught, and how noisy it is.

usage: self run practice.backtest [--last <n> | --since <YYYY-MM-DD>] [--practice <id>[,<id>…]] [--quiet]
       self run practice.backtest --pr <number>[,<number>…] [--practice <id>[,…]]

Run it from the repo (cwd), e.g. ~/mym. Every live practice declared `--at push`
or `--at pr` that has a --check and whose --remote matches is judged exactly as
pr.push judges it, on its own --when paths.

History (default --last 100): each first-parent commit of the default branch is
one squash-merged PR: base its parent, head the commit, title its subject. The
checks also get SELF_PRACTICE_TIP=<parent>, so ordering checks compare against
master as it was then, not today. A finding here is something that reached
master. All of them calibrate the check: does it fire on code the rule does
not mean? A merged PR whose last pr.push (pr.opened/pr.updated) came after
the practice was first saved went through the gate: if it still has findings
it is an escape, listed with the --override reason pr.checked recorded.
Teammates' PRs and PRs pushed before the rule never count as escapes.

--pr: fetch each open PR (refs/pull/<n>/head), base its merge-base with the
default branch, title the PR's. This is the review insertion point: any
teammate's PR gets the same judgement our agents get at push, before a human
reads it. Practices declared `--at review` are printed as reminders.

Prints per practice: commits it applied to, commits it flagged, findings and a
few samples with PR numbers; with --pr, every finding. History runs append
practice.backtested {repo, range, commits, practices[{practice, applied,
flagged, findings, samples[]}]} so a practice's noise can be followed over time.
--pr runs append nothing.
Source: ~/self examples/practice-backtest/command.py.
'''
import json
import os
import pwd
import re
import shutil
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor

HOME = pwd.getpwuid(os.getuid()).pw_dir
ENV = dict(os.environ, HOME=HOME)
ENV["PATH"] = os.pathsep.join([HOME + "/go/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin",
                               ENV.get("PATH", "")])
SELF = os.environ.get("SELF_BIN") or shutil.which("self", path=ENV["PATH"]) or "self"
GH = os.environ.get("SELF_GH_BIN") or shutil.which("gh", path=ENV["PATH"]) or "gh"
CWD = os.environ.get("SELF_CWD", ".")
GATES = ("push", "pr")
FINDING = re.compile(r"^[^\s:]+:\d+: ")


def fail(msg, code=2):
    print("practice.backtest: " + msg, file=sys.stderr)
    raise SystemExit(code)


def git(*args, ok=False):
    r = subprocess.run(["git", "-C", REPO, *args], capture_output=True, text=True, env=ENV)
    if r.returncode and not ok:
        fail("git %s: %s" % (" ".join(args[:2]), r.stderr.strip()[:300]))
    return r.stdout


def glob_re(pattern):
    i, out = 0, []
    while i < len(pattern):
        if pattern.startswith("**/", i):
            out.append("(?:.*/)?")
            i += 3
        elif pattern.startswith("**", i):
            out.append(".*")
            i += 2
        elif pattern[i] == "*":
            out.append("[^/]*")
            i += 1
        elif pattern[i] == "?":
            out.append("[^/]")
            i += 1
        else:
            out.append(re.escape(pattern[i]))
            i += 1
    return re.compile("^" + "".join(out) + "$")


def matcher(globs):
    inc = [glob_re(g) for g in globs if not g.startswith("!")]
    exc = [glob_re(g[1:]) for g in globs if g.startswith("!")]
    return lambda p: any(r.match(p) for r in inc) and not any(r.match(p) for r in exc)


live, pushed, overridden = {}, {}, {}
for raw in sys.stdin:
    try:
        e = json.loads(raw)
    except ValueError:
        continue
    p = e.get("payload") or {}
    if e.get("name") == "practice.saved" and p.get("practice"):
        since = (live.get(p["practice"]) or {}).get("_since") or e.get("occurred_at", "")[:19]
        live[p["practice"]] = dict(p, _since=since)
    elif e.get("name") == "practice.retired":
        live.pop(p.get("practice"), None)
    elif e.get("name") in ("pr.opened", "pr.updated") and p.get("number"):
        pushed[str(p["number"])] = (e.get("occurred_at", "")[:19], p.get("head"))
    elif e.get("name") == "pr.checked" and p.get("head"):
        overridden[p["head"]] = {x["practice"]: x["override"] for x in p.get("practices", []) if x.get("override")}

args, opts = sys.argv[1:], {}
i = 0
while i < len(args):
    if args[i] in ("--last", "--since", "--practice", "--pr") and i + 1 < len(args):
        opts[args[i]] = args[i + 1]
        i += 2
    elif args[i] == "--quiet":
        opts["--quiet"] = True
        i += 1
    else:
        fail("unknown argument %r (self brief practice.backtest)" % args[i])

REPO = CWD
REPO = git("rev-parse", "--show-toplevel", ok=True).strip()
if not REPO:
    fail("run it inside a git repo")
origin = git("remote", "get-url", "origin", ok=True).strip()
default = git("symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD", ok=True).strip() or "origin/master"
gh_repo = (re.search(r"github\.com[:/]([^/]+/[^/]+?)(?:\.git)?$", origin) or [None, None])[1]

wanted = {x for x in opts.get("--practice", "").split(",") if x}
practices = [p for p in live.values()
             if p.get("check") and set(p.get("at", [])) & set(GATES)
             and (not p.get("remote") or p["remote"] in origin)
             and (not wanted or p["practice"] in wanted)]
if not practices:
    fail("no live practice with a check applies to %s" % origin, 0)
for p in practices:
    p["_ok"] = matcher(p.get("when", []))


def judge(base, head, title, tip):
    paths = git("diff", "--name-only", "--diff-filter=ACMR", base, head, ok=True).split()
    env = dict(ENV, SELF_PR_TITLE=title, SELF_PRACTICE_TIP=tip)
    out = {}
    for p in practices:
        hits = [x for x in paths if p["_ok"](x)]
        if not hits:
            continue
        c = subprocess.run([SELF, "run", p["check"], "--base", base, "--head", head, *hits], cwd=REPO,
                           capture_output=True, text=True, env=env, timeout=300)
        if c.returncode not in (0, 1):
            out[p["practice"]] = {"broke": (c.stderr.strip().splitlines() or ["exit %d" % c.returncode])[-1]}
            continue
        out[p["practice"]] = {"findings": [l for l in c.stderr.splitlines() if FINDING.match(l)]
                              if c.returncode == 1 else []}
    return out


def utc(stamp):
    from datetime import datetime, timezone
    return datetime.fromisoformat(stamp.replace("Z", "+00:00")).astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S")


def pr_number(subject):
    m = re.search(r"\(#(\d+)\)\s*$", subject)
    return m.group(1) if m else None


if opts.get("--pr"):
    numbers = [n.strip().lstrip("#") for n in opts["--pr"].split(",") if n.strip()]
    for n in numbers:
        ref = "refs/self/backtest/pr-" + n
        git("fetch", "-q", "origin", "+refs/pull/%s/head:%s" % (n, ref))
        git("fetch", "-q", "origin", ok=True)
        head = git("rev-parse", ref).strip()
        base = git("merge-base", default, head).strip()
        title, author = "", ""
        if gh_repo:
            r = subprocess.run([GH, "pr", "view", n, "--repo", gh_repo, "--json", "title,author"],
                               capture_output=True, text=True, env=ENV)
            if r.returncode == 0:
                info = json.loads(r.stdout)
                title, author = info.get("title", ""), (info.get("author") or {}).get("login", "")
        title = title or git("log", "-1", "--format=%s", head).strip()
        verdict = judge(base, head, title, default)
        print("#%s %s%s" % (n, title, " (@" + author + ")" if author else ""), file=sys.stderr)
        if not verdict:
            print("  no practice applies to these files", file=sys.stderr)
        for key, v in sorted(verdict.items()):
            if v.get("broke"):
                print("  ? %s: check broke: %s" % (key, v["broke"]), file=sys.stderr)
            elif v["findings"]:
                print("  ✗ %s — %s" % (key, live[key].get("title", "")), file=sys.stderr)
                for f in v["findings"]:
                    print("      " + f, file=sys.stderr)
            else:
                print("  ✓ %s" % key, file=sys.stderr)
        for p in sorted(live.values(), key=lambda p: p["practice"]):
            if "review" in p.get("at", []):
                print("  · review: %s — %s" % (p["practice"], p.get("title", "")), file=sys.stderr)
    raise SystemExit(0)

log_args = ["log", "--first-parent", "--format=%H%x09%P%x09%cI%x09%an%x09%s", default]
if opts.get("--since"):
    log_args.insert(2, "--since=" + opts["--since"])
else:
    log_args.insert(2, "-n%d" % int(opts.get("--last", "100")))
commits = []
for line in git(*log_args).splitlines():
    sha, parents, when, author, subject = line.split("\t", 4)
    parents = parents.split()
    if len(parents) == 1:
        commits.append((sha, parents[0], utc(when), author, subject))
if not commits:
    fail("no single-parent commits on %s in that range" % default, 0)


def one(c):
    sha, parent, _, _, subject = c
    title = re.sub(r"\s*\(#\d+\)\s*$", "", subject)
    return c, judge(parent, sha, title, parent)


with ThreadPoolExecutor(max_workers=6) as pool:
    results = list(pool.map(one, commits))

stats = {p["practice"]: {"practice": p["practice"], "applied": 0, "flagged": 0, "findings": 0, "broke": 0,
                         "samples": [], "authors": {}, "since": live[p["practice"]]["_since"],
                         "after": {"applied": 0, "flagged": 0, "escapes": []}} for p in practices}
for (sha, _, when, author, subject), verdict in results:
    for key, v in verdict.items():
        s = stats[key]
        s["applied"] += 1
        gate = pushed.get(pr_number(subject) or "")
        if gate and gate[0] >= s["since"]:
            s["after"]["applied"] += 1
            if not v.get("broke") and v["findings"]:
                s["after"]["flagged"] += 1
                why = overridden.get(gate[1], {}).get(key)
                s["after"]["escapes"].append("#%s %s%s" % (pr_number(subject), v["findings"][0][:200],
                                                           " [override: %s]" % why[:120] if why else ""))
        a = s["authors"].setdefault(author, [0, 0])
        a[0] += 1
        if v.get("broke"):
            s["broke"] += 1
            s["samples"].append("#%s broke: %s" % (pr_number(subject) or sha[:9], v["broke"][:200]))
            continue
        if v["findings"]:
            s["flagged"] += 1
            a[1] += 1
            s["findings"] += len(v["findings"])
            if len(s["samples"]) < 6:
                s["samples"].append("#%s %s" % (pr_number(subject) or sha[:9], v["findings"][0][:220]))

span = "%s..%s" % (commits[-1][0][:9], commits[0][0][:9])
for s in stats.values():
    s["authors"] = {k: {"applied": n, "flagged": f} for k, (n, f) in s["authors"].items()}
print("backtest of %d merged commits on %s (%s) in %s" % (len(commits), default, span, os.path.basename(REPO)),
      file=sys.stderr)
print("flags calibrate the checks; escapes are gated PRs (pr.push after the practice was saved) that still have findings",
      file=sys.stderr)
print("%-24s %8s %8s %9s %6s  %s" % ("practice", "applied", "flagged", "findings", "rate", "gated: escapes/pushed"),
      file=sys.stderr)
for s in sorted(stats.values(), key=lambda s: -s["flagged"]):
    rate = "%3d%%" % (100 * s["flagged"] // s["applied"]) if s["applied"] else "  - "
    print("%-24s %8d %8d %9d %6s  %d/%d%s" % (s["practice"], s["applied"], s["flagged"], s["findings"], rate,
                                              s["after"]["flagged"], s["after"]["applied"],
                                              "  (%d broke)" % s["broke"] if s["broke"] else ""), file=sys.stderr)
    for x in s["after"]["escapes"]:
        print("    escape " + x, file=sys.stderr)
if not opts.get("--quiet"):
    for s in sorted(stats.values(), key=lambda s: -s["flagged"]):
        if s["samples"]:
            by = sorted(s["authors"].items(), key=lambda kv: -kv[1]["applied"])
            print("\n%s — flagged/applied by author: %s" % (s["practice"], ", ".join(
                "%s %d/%d" % (k, a["flagged"], a["applied"]) for k, a in by if a["applied"] >= 2)), file=sys.stderr)
            for x in s["samples"]:
                print("  " + x, file=sys.stderr)
for s in stats.values():
    s["samples"] = s["samples"][:3]
print(json.dumps({"name": "practice.backtested", "payload": {
    "repo": gh_repo or REPO, "range": span, "commits": len(commits),
    "practices": sorted(stats.values(), key=lambda s: s["practice"])}}, ensure_ascii=False, sort_keys=True))
