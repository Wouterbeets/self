#!/usr/bin/env python3
import json
import os
import shutil
import subprocess
import sys
import time

if len(sys.argv) != 5:
    raise SystemExit("usage: dispatch <goal> <agent> <kind> <repo>")

goal, agent, kind, repo = sys.argv[1:]
binary = os.environ["SELF_BINARY"]
herdr = os.environ.get("SELF_HERDR_BIN") or shutil.which("herdr")
fd_text = os.environ.get("SELF_REPOSITORY_RESERVATION_FD", "")
held = False
if fd_text.isdigit():
    fd = int(fd_text)
    check = subprocess.run(
        [binary, "reserve", "held", repo],
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        pass_fds=(fd,),
    )
    held = check.returncode == 0

if not held:
    result = subprocess.run(
        [binary, "reserve", "dispatch", repo, "--", sys.executable,
         os.path.realpath(__file__), *sys.argv[1:]],
        stdin=sys.stdin.buffer,
    )
    raise SystemExit(result.returncode)

marker = os.environ.get("SELF_FIXTURE_DISPATCH_MARKER")
if marker:
    open(marker, "w", encoding="utf-8").close()
delay = float(os.environ.get("SELF_FIXTURE_DISPATCH_DELAY", "0"))
if delay:
    time.sleep(delay)

base = {
    "goal": goal,
    "agent": agent,
    "kind": kind,
    "repo": repo,
    "herdr_binary": herdr,
    "reservation_validated": True,
}
if os.environ.get("SELF_FIXTURE_REQUIRE_HERDR") == "1":
    if not herdr:
        raise SystemExit("Herdr unavailable")
    subprocess.run([herdr, "--fixture-probe"], check=True)
mode = os.environ.get("SELF_FIXTURE_DISPATCH_MODE", "started")
events = [{"name": "agent.requested", "payload": base}]
if mode == "started":
    events.append({"name": "agent.started", "payload": base})
elif mode == "failed-clean":
    resource_root = os.environ.get("SELF_FIXTURE_RESOURCE_ROOT")
    if resource_root:
        worktree = os.path.join(resource_root, "worktree")
        os.makedirs(worktree, exist_ok=True)
        child = subprocess.Popen(["sleep", "30"])
        child.terminate()
        child.wait(timeout=5)
        shutil.rmtree(resource_root)
    failed = dict(base, writer={
        "state": "stopped",
        "cleanup_confirmed": True,
        "evidence": "fixture child reaped and fixture worktree removed",
    })
    events.append({"name": "agent.failed", "payload": failed})
elif mode == "failed-unsafe":
    events.append({"name": "agent.failed", "payload": base})
else:
    raise SystemExit("unknown fixture mode")

wire = "".join(json.dumps(event) + "\n" for event in events).encode()
published = subprocess.run(
    [binary, "reserve", "publish", repo],
    input=wire,
    stdout=subprocess.DEVNULL,
    pass_fds=(fd,),
)
raise SystemExit(published.returncode)
