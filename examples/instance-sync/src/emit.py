#!/usr/bin/env python3
"""Emit a hear body that declares capabilities and authors their scripts.

usage: emit.py <spec.json>

The spec is a list of {kind, name, consumes?, script, description}. Declaration
and authoring go out together because one hear body is one critical section:
the kernel resolves declared-ness between its own appends, so the bytes install
in the same breath they were declared in.
"""
import json
import sys

for c in json.load(open(sys.argv[1])):
    decl = {"name": c["kind"] + ".declared",
            "payload": {"name": c["name"], "description": c["description"]}}
    if c["kind"] == "view":
        decl["payload"]["consumes"] = c.get("consumes") or ["*"]
    print(json.dumps(decl))
    print(json.dumps({"name": "script.authored", "payload": {
        "type": c["kind"], "name": c["name"], "script": open(c["script"]).read()}}))
