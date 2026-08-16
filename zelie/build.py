#!/usr/bin/env python3
"""Build zelie/index.html: one self-contained file, playable from file://.

ES modules do not load from file:// in Chrome, so the game cannot ship as
an importmap + three.module.min.js pair. Instead this script rewrites the
module build's single trailing `export{...};` into `window.THREE = {...}`
and inlines everything into shell.html. Run it from zelie/:

    python3 build.py
"""
import re
import sys
from pathlib import Path

HERE = Path(__file__).parent


def three_as_classic_script() -> str:
    src = (HERE / "vendor" / "three.module.min.js").read_text()
    m = re.search(r"export\{(.*?)\};?\s*$", src, re.S)
    if not m:
        sys.exit("no trailing export{...} found in three.module.min.js")
    pairs = []
    for entry in m.group(1).split(","):
        entry = entry.strip()
        if " as " in entry:
            local, public = [p.strip() for p in entry.split(" as ")]
        else:
            local = public = entry
        pairs.append(f"{public}:{local}")
    # Wrapped in an IIFE: a classic script's top-level const/let land in the
    # shared global lexical scope, and the minifier's short names (it uses `$`)
    # would collide with the game's own bindings.
    body = src[: m.start()] + "window.THREE={" + ",".join(pairs) + "};"
    return "(function(){" + body + "})();"


def main() -> None:
    shell = (HERE / "shell.html").read_text()
    game = (HERE / "game.js").read_text()
    out = shell.replace("{{THREE}}", three_as_classic_script())
    out = out.replace("{{GAME}}", game)
    (HERE / "index.html").write_text(out)
    print(f"index.html written ({len(out):,} bytes)")


if __name__ == "__main__":
    main()
