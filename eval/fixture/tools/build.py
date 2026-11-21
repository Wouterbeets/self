#!/usr/bin/env python3
"""Build the almanac: one output file per published page."""
import configparser, os, pathlib, sys

root = pathlib.Path(__file__).resolve().parent.parent
profile = os.environ.get("ALMANAC_PROFILE")
if not profile:
    sys.exit("build: E17")
cfg = configparser.ConfigParser()
if not cfg.read(root / "profiles" / f"{profile}.ini"):
    sys.exit(f"build: E23 ({profile})")
out = cfg["output"]
font = out.get("font")
if font and not (root / font).exists():
    sys.exit("build: E31")
dest = root / out["dir"]
dest.mkdir(exist_ok=True)
built = 0
for page in sorted((root / "pages").glob("*.md")):
    text = page.read_text()
    head = text.split("---")[1] if text.startswith("---") else ""
    if "draft: true" in head:
        continue
    (dest / (page.stem + out["suffix"])).write_text(text.split("---", 2)[-1].strip() + "\n")
    built += 1
print("ok")
