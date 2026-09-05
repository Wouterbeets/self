#!/usr/bin/env python3
"""Declare a capability and author its script in one hear body.

usage: install.py <SELF_HOME> <command|view> <name> <consumes-csv|-> <script-path> <description>

The declaration and the authoring travel together because a hear body is one
critical section: the kernel resolves declared-ness between its own appends, so
the script installs in the same breath it was declared in.
"""
import json
import os
import subprocess
import sys

home, kind, name, consumes, path, desc = sys.argv[1:7]
decl = {"name": kind + ".declared", "payload": {"name": name, "description": desc}}
if kind == "view":
    decl["payload"]["consumes"] = [c for c in consumes.split(",") if c]
with open(path) as fh:
    script = fh.read()
body = "".join(json.dumps(o) + "\n" for o in (
    decl, {"name": "script.authored", "payload": {"type": kind, "name": name, "script": script}}))
env = dict(os.environ, SELF_HOME=home)
env.setdefault("SELF_CALLER", "install.py")
sys.exit(subprocess.run([env.get("SELF_BIN", "self"), "hear"], input=body, text=True, env=env).returncode)
