#!/usr/bin/env python3
'''check.migrations-ordered - an Atlas migration a branch adds must sort after every migration already on the default branch, and come with its atlas.sum (practice migrations/ordered).

usage: self run check.migrations-ordered [--base <ref>] [--head <ref>] [--tip <ref>] [<path>…]

Atlas applies a directory linearly: a file that sorts before one already
applied fails the deploy at "db migration" with "added out of order"
(pitfall/atlas-migration-out-of-order). That happens when a PR stays open
while another PR merges a newer-dated migration into the same directory.

For each .sql file the branch adds in a directory holding atlas.sum: its
version (the leading digits of the file name) must be greater than every
version in that directory on --tip (default origin/HEAD, as fetched by
pr.push), and the branch must change that atlas.sum. Fix: rename the file to
a version after the newest one on the tip, then `atlas migrate hash` (or the
service's make target) in that directory, and commit both.

Findings on stderr as `path:1: …`. Exit 0 clean, 1 with findings, 2 on a
usage or git error. Appends nothing.
Source: ~/self examples/check-migrations-ordered/command.py.
'''
import os
import re
import subprocess
import sys

ENV = dict(os.environ)
ENV["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:" + ENV.get("PATH", "")
REPO = os.environ.get("SELF_CWD", ".")


def git(*args, ok=False):
    r = subprocess.run(["git", "-C", REPO, *args], capture_output=True, text=True, env=ENV)
    if r.returncode and not ok:
        print("check.migrations-ordered: git %s: %s" % (" ".join(args[:2]), r.stderr.strip()[:300]), file=sys.stderr)
        raise SystemExit(2)
    return r.stdout


args, base, head, tip = sys.argv[1:], None, "HEAD", "origin/HEAD"
i = 0
while i < len(args):
    if args[i] in ("--base", "--head", "--tip") and i + 1 < len(args):
        if args[i] == "--base":
            base = args[i + 1]
        elif args[i] == "--head":
            head = args[i + 1]
        else:
            tip = args[i + 1]
        i += 2
    elif args[i].startswith("--"):
        print("check.migrations-ordered: unknown flag " + args[i], file=sys.stderr)
        raise SystemExit(2)
    else:
        i += 1  # paths are recomputed from the diff: the check needs every added file, not a glob's pick
if base is None:
    base = git("merge-base", tip, head).strip()

added = [p for p in git("diff", "--name-only", "--diff-filter=A", base, head).split() if p.endswith(".sql")]
changed = set(git("diff", "--name-only", base, head).split())
version = lambda name: int(m.group(1)) if (m := re.match(r"^(\d+)", name)) else None

findings = []
for path in sorted(added):
    folder, name = os.path.split(path)
    has_sum = subprocess.run(["git", "-C", REPO, "cat-file", "-e", "%s:%s/atlas.sum" % (head, folder)],
                             capture_output=True, env=ENV).returncode == 0
    if not has_sum:
        continue
    mine = version(name)
    if mine is None:
        continue
    on_tip = [n for n in git("ls-tree", "--name-only", "%s:%s" % (tip, folder), ok=True).split()
              if n.endswith(".sql") and n != name]
    newer = max((v for v in map(version, on_tip) if v is not None), default=None)
    if newer is not None and mine <= newer:
        latest = max(on_tip, key=lambda n: version(n) or 0)
        findings.append("%s:1: out-of-order: version %d does not sort after %s on %s; rename it past %d and rehash atlas.sum"
                        % (path, mine, latest, tip, newer))
    if folder + "/atlas.sum" not in changed:
        findings.append("%s:1: no-sum: %s/atlas.sum is unchanged; run atlas migrate hash there and commit it"
                        % (path, folder))

for f in findings:
    print(f, file=sys.stderr)
raise SystemExit(1 if findings else 0)
