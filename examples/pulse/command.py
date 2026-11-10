#!/usr/bin/env python3
"""pulse - witness this machine's timers and record verdicts on what self view pulse finds.

usage: self run pulse tick
       self run pulse ack <key> <why...>
       self run pulse told <key>

  tick   reads the systemd user timers on this machine and appends pulse.timers
         {host, units[{unit, state, result, since}]} when a state changed. A
         unit is failed when the last run of its service did not succeed,
         inactive when the timer is loaded but not armed, ok otherwise; since
         is when this tick first saw it in that state. The view only calls a
         timer wrong after a day in a bad state, so a peer asleep overnight is
         not a finding. Machines without systemd append nothing.
  ack    appends pulse.acked {key, note}: self view pulse stops listing that
         finding as wrong. Use it after judging one (a deliberate burst, a
         timer you parked), with the reason. A new run of the same loop gets a
         new key and comes back.
  told   appends pulse.told {key}: the driver calls it after the phone push
         went out, so a finding is pushed once.

The findings themselves are computed by the view, which is pure. The driver
~/.local/bin/self-pulse-drive runs tick, then pushes each line of
self view pulse actions through self run notify and calls told.
Source: ~/self examples/pulse/command.py.
"""
import datetime
import json
import os
import re
import shutil
import socket
import subprocess
import sys

USAGE = "usage: self run pulse tick | ack <key> <why...> | told <key>"
KEY = re.compile(r"^[0-9a-f]{8}$")
args = sys.argv[1:]


def fail(message, code=2):
    print(message, file=sys.stderr)
    raise SystemExit(code)


def emit(name, payload):
    print(json.dumps({"name": name, "payload": payload}, ensure_ascii=False))


latest_timers, acked, told = None, {}, set()
for line in sys.stdin:
    try:
        event = json.loads(line)
    except (TypeError, ValueError):
        continue
    payload = event.get("payload") if isinstance(event, dict) else None
    if not isinstance(payload, dict):
        continue
    if event.get("name") == "pulse.timers":
        latest_timers = payload
    elif event.get("name") == "pulse.acked":
        acked[payload.get("key")] = payload.get("note")
    elif event.get("name") == "pulse.told":
        told.add(payload.get("key"))

if args[:1] == ["ack"] and len(args) >= 3 and KEY.match(args[1]):
    note = " ".join(args[2:]).strip()
    if acked.get(args[1]) == note:
        fail("no changes", 0)
    emit("pulse.acked", {"key": args[1], "note": note})
    raise SystemExit(0)

if args[:1] == ["told"] and len(args) == 2 and KEY.match(args[1]):
    if args[1] in told:
        fail("no changes", 0)
    emit("pulse.told", {"key": args[1]})
    raise SystemExit(0)

if args != ["tick"]:
    fail(USAGE)

systemctl = shutil.which("systemctl", path="/usr/bin:/bin:/usr/local/bin")
if not systemctl:
    fail("no changes: no systemd on this machine", 0)

env = dict(os.environ)
env.setdefault("XDG_RUNTIME_DIR", "/run/user/%d" % os.getuid())
env.setdefault("DBUS_SESSION_BUS_ADDRESS", "unix:path=%s/bus" % env["XDG_RUNTIME_DIR"])


def show(*argv):
    done = subprocess.run([systemctl, "--user", "show", *argv], text=True, capture_output=True, env=env, timeout=20)
    if done.returncode != 0:
        raise RuntimeError((done.stderr or done.stdout).strip()[:200])
    blocks = []
    for chunk in done.stdout.split("\n\n"):
        fields = dict(row.split("=", 1) for row in chunk.splitlines() if "=" in row)
        if fields:
            blocks.append(fields)
    return blocks


now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:00Z")
previous = {unit.get("unit"): unit for unit in (latest_timers or {}).get("units", []) if isinstance(unit, dict)}
try:
    timers = show("--type=timer", "--all", "-p", "Id", "-p", "Unit", "-p", "ActiveState", "-p", "LoadState", "*")
    units = []
    for timer in sorted(timers, key=lambda t: t.get("Id", "")):
        if timer.get("LoadState") != "loaded" or not timer.get("Id", "").endswith(".timer"):
            continue
        service = (show("-p", "Result", "-p", "ActiveState", timer.get("Unit") or timer["Id"][:-6] + ".service") or [{}])[0]
        if service.get("ActiveState") == "failed" or service.get("Result", "success") != "success":
            state = "failed"
        elif timer.get("ActiveState") != "active":
            state = "inactive"
        else:
            state = "ok"
        before = previous.get(timer["Id"], {})
        since = before.get("since") if before.get("state") == state and before.get("since") else now
        units.append({"unit": timer["Id"], "state": state, "result": service.get("Result", ""), "since": since})
except (RuntimeError, OSError, subprocess.SubprocessError) as exc:
    fail("pulse: cannot read the user timers: %s" % exc, 1)

snapshot = {"host": socket.gethostname(), "units": units}
if latest_timers == snapshot:
    fail("no changes", 0)
emit("pulse.timers", snapshot)
