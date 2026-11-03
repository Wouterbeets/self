#!/usr/bin/env python3
'''check.go-errors - judge the Go lines a branch adds against the error-wrapping practice (go/errors).

usage: self run check.go-errors [--base <ref>] [--head <ref>] [<path>…]

Reads `git diff -U0 <base> <head>` in the caller's repo (SELF_CWD) and judges
only added lines of non-test, non-generated .go files (all changed ones when
no path is given). Base defaults to the merge-base with origin/HEAD, head to
HEAD. Findings, one per line on stderr as `path:line: problem`:

  bare-return   `return err` / `return x, err` hands an error up without
                saying what this function was doing
  no-w          fmt.Errorf with err as an argument but no %w: errors.Is/As break
  failed-to     an error message starting "failed to"/"unable to"/"error ": the
                chain already says it failed; name the operation instead

Escape hatch: `//nowrap: <reason>` (or an existing `//nolint:wrapcheck`) on
the line, or on the line above, when
passing the error through really is right (e.g. a one-line forwarder whose
caller already names the operation). The reason is the review's business.

Exit 0 clean, 1 with findings, 2 on a usage or git error. Appends nothing.
This is the `--check` of practice go/errors and follows the practice check
contract (self brief practice). Source: ~/self examples/check-go-errors/command.py.
'''
import os
import re
import subprocess
import sys

ENV = dict(os.environ)
ENV["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:" + ENV.get("PATH", "")
REPO = os.environ.get("SELF_CWD", ".")

BARE = re.compile(r"^\s*return\s+(?:.+,\s*)?(err|[A-Za-z_]\w*Err)\s*(?://.*)?$")
ERRORF = re.compile(r'fmt\.Errorf\(\s*"((?:[^"\\]|\\.)*)"(.*)')
ERR_ARG = re.compile(r",\s*(?:err|[A-Za-z_]\w*Err)\b")
FAILED = re.compile(r'(?:fmt\.Errorf|errors\.New)\(\s*"(?:failed to|unable to|error )', re.I)
BUILT = r"\b%s\s*:?=\s*(?:fmt\.Errorf|errors\.(?:New|Join))\("
LOOKBACK = 12  # lines searched above a bare return for the wrap that built it


def built_above(text, n, ident):
    '''True when ident was assigned from fmt.Errorf/errors.New/Join a few lines up, in the same func.'''
    want = re.compile(BUILT % re.escape(ident))
    for k in range(n - 2, max(-1, n - 2 - LOOKBACK), -1):
        if text[k].startswith("func "):
            return False
        if want.search(text[k]):
            return True
    return False
NOWRAP = re.compile(r"//\s*nowrap\b|//\s*nolint:[\w,]*\bwrapcheck\b")  # ours, or golangci's


def git(*args, ok=False):
    r = subprocess.run(["git", "-C", REPO, *args], capture_output=True, text=True, env=ENV)
    if r.returncode and not ok:
        print("check.go-errors: git %s: %s" % (" ".join(args[:2]), r.stderr.strip()[:300]), file=sys.stderr)
        raise SystemExit(2)
    return r.stdout


args, paths, base, head = sys.argv[1:], [], None, "HEAD"
i = 0
while i < len(args):
    if args[i] in ("--base", "--head") and i + 1 < len(args):
        if args[i] == "--base":
            base = args[i + 1]
        else:
            head = args[i + 1]
        i += 2
    elif args[i].startswith("--"):
        print("check.go-errors: unknown flag " + args[i], file=sys.stderr)
        raise SystemExit(2)
    else:
        paths.append(args[i])
        i += 1
if base is None:
    base = git("merge-base", "origin/HEAD", head).strip()

paths = [p for p in paths if p.endswith(".go") and not p.endswith("_test.go")]
diff = git("diff", "-U0", "--no-color", "--no-ext-diff", base, head, "--", *(paths or ["*.go"]))

added, path, line = {}, None, 0
for raw in diff.splitlines():
    if raw.startswith("+++ "):
        path = raw[6:] if raw.startswith("+++ b/") else None
        if path and (not path.endswith(".go") or path.endswith("_test.go")):
            path = None
        continue
    m = re.match(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@", raw)
    if m:
        line = int(m.group(1))
        continue
    if path and raw.startswith("+") and not raw.startswith("+++"):
        added.setdefault(path, []).append(line)
        line += 1

findings = []
for path in sorted(added):
    text = git("show", "%s:%s" % (head, path), ok=True).splitlines()
    if any("Code generated" in l and "DO NOT EDIT" in l for l in text[:20]):
        continue
    for n in added[path]:
        if n < 1 or n > len(text):
            continue
        cur, prev = text[n - 1], (text[n - 2] if n >= 2 else "")
        if NOWRAP.search(cur) or NOWRAP.match(prev.strip()):
            continue
        bare = BARE.match(cur)
        if bare and not built_above(text, n, bare.group(1)):
            findings.append("%s:%d: bare-return: wrap it, e.g. fmt.Errorf(\"<what this call was doing>: %%w\", err)" % (path, n))
        m = ERRORF.search(cur)
        if m and "%w" not in m.group(1) and ERR_ARG.search(m.group(2)):
            findings.append("%s:%d: no-w: fmt.Errorf takes err without %%w; use %%w so errors.Is/As still see it" % (path, n))
        if FAILED.search(cur):
            findings.append("%s:%d: failed-to: name the operation (\"load order: %%w\"), not that it failed" % (path, n))

for f in findings:
    print(f, file=sys.stderr)
raise SystemExit(1 if findings else 0)
