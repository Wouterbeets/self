#!/usr/bin/env python3
import json
import os
import subprocess
import sys

if len(sys.argv) != 2:
    raise SystemExit("usage: reservation.probe <repo>")

repo = sys.argv[1]
binary = os.environ["SELF_BINARY"]
path_result = subprocess.run(
    [binary, "reserve", "path", repo],
    text=True,
    capture_output=True,
    check=True,
)
identity, canonical, path = path_result.stdout.strip().split("\t")
check = subprocess.run(
    [binary, "reserve", "check", repo],
    stdout=subprocess.DEVNULL,
    stderr=subprocess.DEVNULL,
)
print(json.dumps({"name": "reservation.probed", "payload": {
    "identity": identity,
    "repository": canonical,
    "path": path,
    "available": check.returncode == 0,
    "saw_xdg_runtime_dir": bool(os.environ.get("XDG_RUNTIME_DIR")),
    "saw_tmpdir": bool(os.environ.get("TMPDIR")),
}}))
