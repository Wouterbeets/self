#!/usr/bin/env python3
'''check.no-comments - judge the comment lines a branch adds to Go, PHP, TypeScript and JavaScript (practice code/no-comments).

usage: self run check.no-comments [--base <ref>] [--head <ref>] [<path>…]

Reads `git diff -U0 <base> <head>` in the caller's repo (SELF_CWD). An added
line is a finding when it is, or ends in, a prose comment: `// …`, `/* … */`,
` * …` inside a block, or PHP `# …`. Not findings:
  - directives and tool markers: //go:…, // +build, //nolint, //nowrap,
    //lint:, // Code generated, @ts-…, eslint-…, prettier-ignore,
    istanbul, phpcs:, @phpstan-/@psalm-, #[…] attributes, #! shebangs
  - docblock lines that only carry annotations (@var, @param, @return,
    @template, @throws, @extends, @implements, @method, @property, @mixin,
    @deprecated, @internal, @ORM…, @see) and bare /** or */ delimiters
  - a comment line that the same diff removes from the same file (moved, not added)
  - generated files
Findings are `path:line: comment: <text>` on stderr. Exit 0 clean, 1 with
findings, 2 on a usage or git error. Appends nothing.
Source: ~/self examples/check-no-comments/command.py.
'''
import os
import re
import subprocess
import sys

ENV = dict(os.environ)
ENV["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:" + ENV.get("PATH", "")
REPO = os.environ.get("SELF_CWD", ".")
EXT = (".go", ".php", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs")
MARKER = re.compile(r"^(?://|/\*+|\*|#)\s*(?:go:|\+build|nolint|nowrap|lint:|Code generated|@ts-|eslint|prettier-ignore|"
                    r"istanbul|phpcs:|@phpstan-|@psalm-|webpackChunkName|c8 |biome-ignore|#region|#endregion|region\b|endregion\b)")
ANNOT = re.compile(r"^\*\s*@(?:var|param|return|template|throws|extends|implements|method|property|mixin|deprecated|"
                   r"internal|ORM|Assert|Route|Groups|SerializedName|Ignore|see|psalm|phpstan|covers|dataProvider|"
                   r"test|group|uses|codeCoverageIgnore|inheritDoc|api|readonly|immutable|template-\w+|no-named-arguments)\b")
DELIM = re.compile(r"^(?:/\*\*?|\*/|\*)\s*$")
STRINGS = re.compile(r'"(?:[^"\\]|\\.)*"|\'(?:[^\'\\]|\\.)*\'|`[^`]*`')


def git(*args, ok=False):
    r = subprocess.run(["git", "-C", REPO, *args], capture_output=True, text=True, env=ENV)
    if r.returncode and not ok:
        print("check.no-comments: git %s: %s" % (" ".join(args[:2]), r.stderr.strip()[:300]), file=sys.stderr)
        raise SystemExit(2)
    return r.stdout


def comment_of(line, php):
    '''The comment text a line carries, or None. Whole-line comments first, then a trailing //.'''
    s = line.strip()
    if not s:
        return None
    if s.startswith("#!") or (php and s.startswith("#[")):
        return None
    if s.startswith(("//", "/*", "*")) or (php and s.startswith("#")):
        if MARKER.match(s) or DELIM.match(s) or ANNOT.match(s) or re.match(r"^/\*\*\s*@", s):
            return None
        if s.startswith("*") and not s.startswith("*/") and len(s) > 1 and s[1] not in " \t/":
            return None  # `*ptr = x`, `**kwargs`: code, not a block comment line
        return s
    bare = STRINGS.sub('""', line)
    m = re.search(r"\s//\s?(\S.*)$", bare)
    if m and not MARKER.match("//" + m.group(1)):
        return "// " + m.group(1)
    return None


args, paths, base, head = sys.argv[1:], [], None, "HEAD"
i = 0
while i < len(args):
    if args[i] in ("--base", "--head") and i + 1 < len(args):
        base, head = (args[i + 1], head) if args[i] == "--base" else (base, args[i + 1])
        i += 2
    elif args[i].startswith("--"):
        print("check.no-comments: unknown flag " + args[i], file=sys.stderr)
        raise SystemExit(2)
    else:
        paths.append(args[i])
        i += 1
if base is None:
    base = git("merge-base", "origin/HEAD", head).strip()
paths = [p for p in paths if p.endswith(EXT)]
diff = git("diff", "-U0", "--no-color", "--no-ext-diff", base, head, "--",
           *(paths or ["*" + e for e in EXT]))

added, removed, path, line = {}, {}, None, 0
for raw in diff.splitlines():
    if raw.startswith("--- "):
        continue
    if raw.startswith("+++ "):
        path = raw[6:] if raw.startswith("+++ b/") else None
        if path and not path.endswith(EXT):
            path = None
        continue
    m = re.match(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@", raw)
    if m:
        line = int(m.group(1))
        continue
    if not path:
        continue
    if raw.startswith("+"):
        added.setdefault(path, []).append((line, raw[1:]))
        line += 1
    elif raw.startswith("-"):
        removed.setdefault(path, set()).add(raw[1:].strip())

findings = []
for path in sorted(added):
    head_text = git("show", "%s:%s" % (head, path), ok=True)
    if re.search(r"Code generated .* DO NOT EDIT|@generated", head_text[:2000]):
        continue
    php = path.endswith(".php")
    for n, text in added[path]:
        if text.strip() in removed.get(path, ()):
            continue
        c = comment_of(text, php)
        if c:
            findings.append("%s:%d: comment: %s" % (path, n, c[:120]))

for f in findings:
    print(f, file=sys.stderr)
raise SystemExit(1 if findings else 0)
