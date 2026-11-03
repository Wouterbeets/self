#!/usr/bin/env python3
'''check.workflow-scope - stop a push that touches .github/workflows/ when the GitHub token cannot push workflow files (practice github/workflow-scope).

usage: self run check.workflow-scope [--base <ref>] [--head <ref>] [<path>…]

GitHub refuses a push whose commits create or change .github/workflows/*
unless the token has the `workflow` scope ("refusing to allow an OAuth App to
create or update workflow"); pitfalls github-workflow-push-scope and
app-definition-workflow-scope were both found that way, after the push.

Passes when no such file changed, or when gh's token for the account named
after the origin owner (SELF_GH_USER overrides; else the active one) has the
workflow scope. Otherwise one finding per workflow file on stderr as
`path:1: …`. Ways out: `gh auth refresh -h github.com -s workflow` (Wouter,
interactive), or hand the workflow change to devops in its own PR.
Exit 0 clean, 1 with findings, 2 on a usage or git error. Appends nothing.
Source: ~/self examples/check-workflow-scope/command.py.
'''
import os
import pwd
import re
import subprocess
import sys

HOME = pwd.getpwuid(os.getuid()).pw_dir
ENV = dict(os.environ, HOME=HOME)
ENV["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:" + ENV.get("PATH", "")
REPO = os.environ.get("SELF_CWD", ".")
GH = os.environ.get("SELF_GH_BIN") or "gh"


def git(*args):
    r = subprocess.run(["git", "-C", REPO, *args], capture_output=True, text=True, env=ENV)
    if r.returncode:
        print("check.workflow-scope: git %s: %s" % (" ".join(args[:2]), r.stderr.strip()[:300]), file=sys.stderr)
        raise SystemExit(2)
    return r.stdout


args, base, head = sys.argv[1:], None, "HEAD"
i = 0
while i < len(args):
    if args[i] in ("--base", "--head") and i + 1 < len(args):
        base, head = (args[i + 1], head) if args[i] == "--base" else (base, args[i + 1])
        i += 2
    else:
        i += 1
if base is None:
    base = git("merge-base", "origin/HEAD", head).strip()

touched = [p for p in git("diff", "--name-only", base, head).split() if p.startswith(".github/workflows/")]
if not touched:
    raise SystemExit(0)

owner = ""
m = re.search(r"github\.com[:/]([^/]+)/", git("remote", "get-url", "origin"))
if m:
    owner = m.group(1)
user = os.environ.get("SELF_GH_USER") or owner
token = subprocess.run([GH, "auth", "token", "--user", user], capture_output=True, text=True, env=ENV)
env = dict(ENV, GH_TOKEN=token.stdout.strip()) if token.returncode == 0 and token.stdout.strip() else ENV
probe = subprocess.run([GH, "api", "-i", "user"], capture_output=True, text=True, env=env, timeout=30)
scopes = re.search(r"(?im)^x-oauth-scopes:\s*(.*)$", probe.stdout or "")
have = {s.strip() for s in (scopes.group(1) if scopes else "").split(",")}
if "workflow" in have:
    raise SystemExit(0)

who = user if env is not ENV else "the active gh account"
for p in touched:
    print("%s:1: workflow-scope: %s's token has no `workflow` scope (has: %s), so GitHub will refuse this push; "
          "gh auth refresh -h github.com -s workflow, or move the workflow change to its own PR for devops"
          % (p, who, ", ".join(sorted(s for s in have if s)) or "unknown"), file=sys.stderr)
raise SystemExit(1)
