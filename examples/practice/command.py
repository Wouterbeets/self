#!/usr/bin/env python3
'''practice - declare how work is done here: a rule, the files it applies to, the moments it is shown or enforced, and the check that enforces it.

usage: self run practice set <id> --title <t> --rule <text> --when <glob>[,<glob>…] --at <point>[,<point>…] [--check <command>] [--remote <substring>]
       self run practice retire <id> <reason…>
       self run practice show [--at <point>] [--dir <path>] [--base <ref>] [--json]

A practice is a named record (practice.saved / practice.retired under a stable
id). `set` revises only on change; `retire` tombstones it. Read them with
`self view practices`.

--when   globs over repo-relative paths; `**` crosses directories, a leading `!`
         excludes. A practice applies when at least one path matches an include
         and no exclude. Example: '**/*.go,!**/*_test.go'.
--at     the insertion points that read it. Known today:
           work  shown when work starts: dispatch prompts, working-on output
           push  enforced by `self run pr.push` before anything leaves the machine
           pr    the pull request itself (title, body); pr.push gates it with push,
                 and checks read the title from SELF_PR_TITLE
         New points (deploy, monitor, review…) need no change here: a caller
         asks `practice show --at <point>` and gets what was declared for it.
--check  a self command that enforces the rule at push. Contract:
           self run <check> --base <sha> --head <sha> <path…>   (cwd: the repo, via SELF_CWD)
           exit 0 clean; exit 1 with one `path:line: problem` per line on stderr;
           only lines the branch adds are judged, so old code never blocks.
--remote applies only in repos whose `origin` URL contains this substring.

`show` appends nothing. It prints, on stderr, the live practices for --at that
apply to the git repo at --dir (default: the caller's directory): with --base,
or at `push`, the paths are the files the branch changed since its merge-base
with --base (default origin/HEAD); otherwise they are the repo's tracked files.
--json prints one JSON object per practice instead, with `paths` (push only).
Outside a git repo it matches nothing and exits 0.

Events: practice.saved {practice, title, rule, when[], at[], check?, remote?}
        practice.retired {practice, reason}
Source: ~/self examples/practice/command.py.
'''
import json
import os
import re
import subprocess
import sys

ENV = dict(os.environ)
ENV["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:" + ENV.get("PATH", "")
ID = re.compile(r"^[a-z0-9][a-z0-9._/-]{0,79}$")
POINT = re.compile(r"^[a-z][a-z0-9-]{0,31}$")
CMD = re.compile(r"^[a-z][a-z0-9._-]{0,79}$")


def fail(msg, code=2):
    print("practice: " + msg, file=sys.stderr)
    raise SystemExit(code)


def live():
    out = {}
    for line in sys.stdin:
        try:
            e = json.loads(line)
        except ValueError:
            continue
        p = e.get("payload") or {}
        key = p.get("practice")
        if e.get("name") == "practice.saved" and key:
            out[key] = p
        elif e.get("name") == "practice.retired" and key:
            out.pop(key, None)
    return out


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


def git(repo, *args):
    r = subprocess.run(["git", "-C", repo, *args], capture_output=True, text=True, env=ENV)
    return r.stdout if r.returncode == 0 else None


def split_csv(values):
    return [v.strip() for raw in values for v in raw.split(",") if v.strip()]


def take(args, flags, multi=()):
    found, rest = {f: [] for f in flags}, []
    i = 0
    while i < len(args):
        a = args[i]
        if a in flags:
            if flags[a]:
                if i + 1 >= len(args):
                    fail(a + " needs a value")
                found[a].append(args[i + 1])
                i += 2
                continue
            found[a].append("1")
        else:
            rest.append(a)
        i += 1
    return found, rest


args = sys.argv[1:]
cmd = args[0] if args else ""

if cmd == "set":
    found, rest = take(args[1:], {"--title": True, "--rule": True, "--when": True, "--at": True,
                                  "--check": True, "--remote": True})
    if len(rest) != 1 or not ID.match(rest[0]):
        fail("set needs exactly one id matching " + ID.pattern)
    for need in ("--title", "--rule", "--when", "--at"):
        if not found[need]:
            fail("set needs " + need)
    item = {"practice": rest[0], "title": found["--title"][-1].strip(), "rule": found["--rule"][-1].strip(),
            "when": split_csv(found["--when"]), "at": split_csv(found["--at"])}
    bad = [p for p in item["at"] if not POINT.match(p)]
    if bad:
        fail("not an insertion point: " + ", ".join(bad))
    if not any(not g.startswith("!") for g in item["when"]):
        fail("--when needs at least one include glob")
    if found["--check"]:
        if not CMD.match(found["--check"][-1]):
            fail("--check must be a self command name")
        item["check"] = found["--check"][-1]
    if "push" in item["at"] and "check" not in item:
        print("practice: note: '%s' is at push without --check; pr.push will only print its rule" % rest[0],
              file=sys.stderr)
    if found["--remote"]:
        item["remote"] = found["--remote"][-1]
    if live().get(rest[0]) != item:
        print(json.dumps({"name": "practice.saved", "payload": item}, ensure_ascii=False, sort_keys=True))

elif cmd == "retire":
    if len(args) < 3:
        fail("usage: retire <id> <reason…>")
    if args[1] not in live():
        fail("no live practice " + args[1])
    print(json.dumps({"name": "practice.retired", "payload": {"practice": args[1], "reason": " ".join(args[2:])}},
                     ensure_ascii=False, sort_keys=True))

elif cmd == "show":
    found, rest = take(args[1:], {"--at": True, "--dir": True, "--base": True, "--json": False})
    if rest:
        fail("show takes no positional arguments: " + " ".join(rest))
    at = found["--at"][-1] if found["--at"] else ""
    where = found["--dir"][-1] if found["--dir"] else os.environ.get("SELF_CWD", ".")
    where = os.path.abspath(os.path.join(os.environ.get("SELF_CWD", "."), where))
    practices = [p for p in live().values() if not at or at in p.get("at", [])]
    top = (git(where, "rev-parse", "--show-toplevel") or "").strip()
    if not top:
        raise SystemExit(0)
    remote = git(top, "remote", "get-url", "origin") or ""
    practices = [p for p in practices if not p.get("remote") or p["remote"] in remote]
    if at == "push" or found["--base"]:
        base = found["--base"][-1] if found["--base"] else "origin/HEAD"
        mb = (git(top, "merge-base", base, "HEAD") or "").strip()
        if not mb:
            fail("no merge-base between %s and HEAD in %s" % (base, top))
        paths = (git(top, "diff", "--name-only", "--diff-filter=ACMR", mb, "HEAD") or "").split()
    else:
        mb, paths = None, (git(top, "ls-files") or "").split("\n")
    shown = []
    for p in sorted(practices, key=lambda p: p["practice"]):
        ok = matcher(p.get("when", []))
        if mb is None:
            if any(x and ok(x) for x in paths):
                shown.append(dict(p))
            continue
        hits = [x for x in paths if ok(x)]
        if hits:
            shown.append(dict(p, paths=hits, base=mb))
    for p in shown:
        if found["--json"]:
            print(json.dumps(p, ensure_ascii=False, sort_keys=True), file=sys.stderr)
        else:
            enforced = " (pr.push checks it: %s)" % p["check"] if p.get("check") else ""
            print("- %s — %s%s\n  %s" % (p["practice"], p["title"], enforced,
                                         p["rule"].replace("\n", "\n  ")), file=sys.stderr)

else:
    fail("usage: practice set|retire|show … (self brief practice)")
