#!/usr/bin/env python3
import json
import os
import shutil
import subprocess
import sys

if len(sys.argv) != 5:
    raise SystemExit("usage: dispatch.pane <goal> <agent> <kind> <repo>")

goal, agent, kind, repo = sys.argv[1:]
binary = os.environ["SELF_BINARY"]
herdr = os.environ.get("SELF_HERDR_BIN") or shutil.which("herdr")
fd_text = os.environ.get("SELF_REPOSITORY_RESERVATION_FD", "")
held = False
if fd_text.isdigit():
    fd = int(fd_text)
    held = subprocess.run(
        [binary, "reserve", "held", repo],
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        pass_fds=(fd,),
    ).returncode == 0

if not held:
    result = subprocess.run(
        [binary, "reserve", "dispatch", repo, "--", sys.executable,
         os.path.realpath(__file__), *sys.argv[1:]],
        stdin=sys.stdin.buffer,
    )
    raise SystemExit(result.returncode)

resource_root = os.environ.get("SELF_FIXTURE_RESOURCE_ROOT")
child = None
if resource_root:
    worktree = os.path.join(resource_root, "worktree")
    workspace = os.path.join(resource_root, "workspace")
    pane = os.path.join(resource_root, "pane")
    for path in (worktree, workspace, pane):
        os.makedirs(path, exist_ok=True)
    child = subprocess.Popen(["sleep", "30"])
    child.terminate()
    child.wait(timeout=5)
    shutil.rmtree(resource_root)

if os.environ.get("SELF_FIXTURE_REQUIRE_HERDR") == "1":
    if not herdr:
        raise SystemExit("Herdr unavailable")
    subprocess.run([herdr, "--fixture-probe"], check=True)

base = {"goal": goal, "agent": agent, "kind": kind, "repo": repo,
        "pane": "fixture-pane", "herdr_binary": herdr}
events = [
    {"name": "agent.requested", "payload": base},
    {"name": "agent.failed", "payload": dict(base, writer={
        "state": "stopped",
        "cleanup_confirmed": True,
        "evidence": "fixture child reaped; pane, workspace, and worktree removed",
    })},
]
wire = "".join(json.dumps(event) + "\n" for event in events).encode()
published = subprocess.run(
    [binary, "reserve", "publish", repo],
    input=wire,
    stdout=subprocess.DEVNULL,
    pass_fds=(fd,),
)
raise SystemExit(published.returncode)
