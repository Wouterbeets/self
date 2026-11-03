#!/usr/bin/env python3
'''check.pr-title - the PR title must be a Conventional Commit with a lowercase subject (practice pr/title).

usage: self run check.pr-title [--title <title>] [--base <ref>] [--head <ref>] [<path>…]

The title comes from --title, else SELF_PR_TITLE (pr.push sets it to the title
the PR will carry). The monorepo squash-merges, so the title becomes the
commit subject, and .github/workflows/pr-title.yml gates it:
  type ∈ feat|fix|docs|chore|refactor|test|ci|build|perf|revert|security,
  optional (scope), optional !, then ": " and a subject whose first character
  is not an uppercase letter.
A finding on stderr as `(title):1: …`. Exit 0 clean, 1 with a finding, 2 when
no title is given. Base, head and paths are accepted for the check contract and
ignored. Appends nothing. Source: ~/self examples/check-pr-title/command.py.
'''
import os
import re
import sys

TYPES = "feat|fix|docs|chore|refactor|test|ci|build|perf|revert|security"
TITLE = re.compile(r"^(?:%s)(?:\([^()\s]+\))?!?: (?![A-Z])\S" % TYPES)

args, title = sys.argv[1:], os.environ.get("SELF_PR_TITLE")
for i, a in enumerate(args):
    if a == "--title" and i + 1 < len(args):
        title = args[i + 1]
if not title:
    print("check.pr-title: no title (pass --title or set SELF_PR_TITLE)", file=sys.stderr)
    raise SystemExit(2)
if TITLE.match(title):
    raise SystemExit(0)
m = re.match(r"^(\w+)(\([^)]*\))?!?: (.)", title)
if m and m.group(1) in TYPES.split("|") and m.group(3).isupper():
    why = "the subject starts uppercase (%r); lowercase its first character" % m.group(3)
elif m and m.group(1) not in TYPES.split("|"):
    why = "type %r is not one of %s" % (m.group(1), TYPES.replace("|", ", "))
else:
    why = "not <type>(<scope>): <subject>, e.g. 'fix(business-service): map refused payouts to 409'"
print("(title):1: pr-title: %r: %s" % (title, why), file=sys.stderr)
raise SystemExit(1)
