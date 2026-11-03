#!/usr/bin/env python3
# name: the old form of `self run working-on`. Runs the installed working-on script
# in legacy mode: same naming, area and doing behaviour as before, never the goal.
import os
import sys

target = os.path.join(os.environ.get("SELF_HOME", ""), "cap", "command", "working-on", "run")
os.environ["SELF_LEGACY_NAME"] = "1"
os.execv(sys.executable, [sys.executable, target] + sys.argv[1:])
