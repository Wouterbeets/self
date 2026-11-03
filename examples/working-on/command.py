#!/usr/bin/env python3
'''working-on - say which goal this conversation works on; it names, files and places its Herdr tab.

usage: self run working-on <goal> [--doing "<one sentence>"] [--area <area>] [--as <2-4 words>]
       self run working-on --doing "<one sentence>"       keep the goal, update the doing line
       self run working-on - <2-4 words> [--area <area>] [--doing "<one sentence>"]
                                                           no goal fits: name it, file it unfiled
       self run working-on --show     print goal/tab/area and the current areas (stderr)
       self run working-on --nudge    print one reminder (stderr) while something is missing
       self run working-on --clear    drop the goal and the doing line, tab back to its number

Call it once you know what the work is, again when it changes. With a goal:
  - the hc terminal gets the label goal=<goal>, which is where the Herdr agents
    list puts it: under that goal in the goal tree, at once;
  - the tab is named after the goal (or --as <words>);
  - the tab moves to the goal's area: --area, else the area its latest dispatch or
    working-on used for this goal or its nearest ancestor, else it stays put.
The goal must be open (self view tree lists them). An unknown or closed goal is
refused with the closest open goals by id and title. Link an existing goal when
one fits; never create a goal just to have one: no fit means `-` (unfiled), which
puts the tab in the list's Unfiled section on top.

--doing is one sentence on what this tab is doing now, the thing a glance should
tell; update it whenever that shifts. Name the work, never people, customers or
prod data. Cut to 200 chars. Kept as the hc label doing=<text>.

--area files the tab under the Herdr space with that name, moving this
conversation's terminal into a new tab there (or creating the space). Areas:
  self      self, Herdr, hc, this machine, agent tooling
  reviews   reviewing someone else's PR
  ops       incidents, deploys, alerts, the away orchestrator
  <project> anything else, e.g. business, mym
A tab holding more than one pane is never moved. An emptied space closes.

Sub-agents an agent starts with hc inherit its goal in the list until they call
working-on themselves.

The caller is found by $HC_TERMINAL_ID (herdr-core). Events:
  goal.worked  {goal|null, terminal, tab, area}   null: declared unfiled
  herdr.named  {tab, pane, terminal, label}
  herdr.doing  {tab, pane, terminal, text}
  herdr.placed {tab, pane, terminal, area, workspace, from_workspace, created}
--nudge reminds, in order: an unnamed tab; a tab with neither a goal nor a
declared `-`; a named tab never placed in an area; a doing line missing since
the last rename or older than 3 hours. --show, --nudge and --clear append
nothing. Outside hc or with no Herdr window everything is a silent no-op, exit 0.

`self run name` is the old form of this command and still works (it never
touches the goal); it prints a reminder to use working-on.
Source: ~/self examples/working-on/command.py.
'''
import json
import os
import re
import secrets
import shutil
import subprocess
import sys
import time
from datetime import datetime, timedelta, timezone

DEFAULT_AREAS = ("self", "reviews", "ops")
PROG = "working-on"
LEGACY = os.environ.get("SELF_LEGACY_NAME") == "1"  # set by the `name` wrapper


def caller_environ():
    try:
        with open("/proc/%d/environ" % os.getppid(), "rb") as handle:
            raw = handle.read().split(b"\0")
    except OSError:
        done = subprocess.run(["/bin/ps", "eww", "-o", "command=", "-p", str(os.getppid())],
                              capture_output=True, text=True)
        found = dict(re.findall(r"(?:^|\s)(HC_[A-Z_]+|HOME|PATH)=(\S+)", done.stdout or ""))
        return dict(os.environ, **found)
    env = {}
    for item in raw:
        if b"=" in item:
            key, value = item.split(b"=", 1)
            env[key.decode("utf-8", "replace")] = value.decode("utf-8", "replace")
    return env or dict(os.environ)


env = caller_environ()
args = sys.argv[1:]
if args and args[0] in ("-h", "--help", "help"):
    print(__doc__.strip(), file=sys.stderr)
    raise SystemExit(0)


def fail(msg, code=1):
    print(PROG + ": " + msg, file=sys.stderr)
    raise SystemExit(code)


# ---- the log: open goals and where each goal's work was filed ----
def moment_key(event):
    stamp = re.match(r"^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?Z$", str(event.get("occurred_at", "")))
    return ("".join(stamp.group(i) for i in range(1, 7)) + (stamp.group(7) or "")[:9].ljust(9, "0") if stamp else "",
            str(event.get("name", "")))


EVENTS = []
if not sys.stdin.isatty():
    for line in sys.stdin:
        if '"goal.' in line or '"herdr.' in line or '"agent.requested"' in line:
            try:
                EVENTS.append(json.loads(line))
            except ValueError:
                pass
EVENTS.sort(key=moment_key)
GOALS, REMOVED, FILED = {}, set(), []
for event in EVENTS:
    item = event.get("payload") if isinstance(event.get("payload"), dict) else {}
    name, target = event.get("name"), item.get("goal")
    if name == "goal.created" and isinstance(target, str):
        GOALS.setdefault(target, dict(item))
    elif name == "goal.updated" and target in GOALS:
        GOALS[target].update({k: item[k] for k in ("status", "owner", "parent") if k in item})
    elif name == "goal.removed":
        REMOVED.add(target)
    elif name == "agent.requested" and (item.get("labels") or {}).get("area"):
        FILED.append((target, item["labels"]["area"]))
    elif name == "goal.worked" and item.get("goal") and item.get("area"):
        FILED.append((item["goal"], item["area"]))


def closed(goal):
    return goal in REMOVED or str(GOALS.get(goal, {}).get("status", "active")).lower() in {
        "closed", "done", "completed", "cancelled", "removed"}


def suggestions(text, limit=3):
    want = set(re.split(r"[^a-z0-9]+", text.lower())) - {""}
    scored = []
    for goal, item in GOALS.items():
        if closed(goal):
            continue
        have = set(re.split(r"[^a-z0-9]+", (goal + " " + str(item.get("title", ""))).lower())) - {""}
        score = len(want & have) + (2 if text.lower() in goal else 0)
        if score:
            scored.append((-score, goal))
    return [g for _, g in sorted(scored)[:limit]]


def goal_area(goal):
    lineage, current = [], goal
    while current in GOALS and current not in lineage:
        lineage.append(current)
        current = GOALS[current].get("parent")
    best = None
    for at, (target, area_name) in enumerate(FILED):
        if target in lineage:
            key = (lineage.index(target), -at)
            if best is None or key < best[0]:
                best = (key, area_name)
    return best[1] if best else None


# ---- argv ----
def take(flag):
    # --flag <words...> up to the next --flag; returns the joined words or None.
    global args
    if flag not in args:
        return None
    at = args.index(flag)
    end = next((i for i in range(at + 1, len(args)) if args[i].startswith("--")), len(args))
    value = " ".join(args[at + 1:end]).strip()
    args = args[:at] + args[end:]
    if not value:
        fail("%s needs a value" % flag, 2)
    return value


def normalise(words, limit):
    text = re.sub(r"[^a-z0-9/-]+", " ", " ".join(words).lower()).split()
    out = ""
    for word in text:
        candidate = (out + " " + word).strip()
        if len(candidate) > limit:
            break
        out = candidate
    return out


doing = take("--doing")
if doing is not None:
    doing = re.sub(r"\s+", " ", doing)[:200]
area = take("--area")
if area is not None:
    area = normalise([area], 16)
    if not area:
        fail("--area has nothing left after normalising", 2)
as_words = take("--as")

command = args[0] if args else ""
USAGE = ("usage: self run working-on <goal> [--doing <sentence>] [--area <area>] [--as <words>] | "
         "--doing <sentence> | - <2-4 words> [--area <area>] [--doing <sentence>] | --show | --nudge | --clear")
goal = None      # a goal id to link
unfiled = False  # `-`: declared no goal
words = []       # tab label words
if LEGACY:
    if command and not command.startswith("-"):
        words = args
    if command not in ("--nudge", "--show", "--clear"):
        print("name: `self run name` is now `self run working-on`: link the goal you work on "
              "(self run working-on <goal> --doing ...), or `-` when none fits.", file=sys.stderr)
elif command in ("--show", "--nudge", "--clear"):
    pass
elif command == "-":
    unfiled, words = True, args[1:]
elif command.startswith("-") or (not args and doing is None and area is None):
    print(USAGE, file=sys.stderr)
    raise SystemExit(2)
elif args:
    goal = command
    if len(args) > 1:
        fail("one goal id, then flags; got %r. Name it differently with --as <words>" % " ".join(args), 2)
    if goal not in GOALS or closed(goal):
        near = suggestions(goal)
        fail("%s is %s. %s" % (goal, "closed" if goal in GOALS else "not a goal",
                               ("Open goals like it: " + ", ".join(near)) if near else "See self view tree")
             + "; none fits: self run working-on - <2-4 words> --area <area>", 2)
    words = as_words.split() if as_words else [goal.replace("-", " ")]
    if area is None:
        area = goal_area(goal)
        if area:
            area = normalise([area], 16) or None

# ---- herdr-core and the window ----
terminal = env.get("HC_TERMINAL_ID", "")
if not terminal:
    raise SystemExit(0)
home = env.get("HOME") or os.path.expanduser("~")
search = os.pathsep.join([os.path.join(home, ".local/bin"), env.get("PATH", ""), "/usr/local/bin:/usr/bin:/bin"])
HC = shutil.which("hc", path=search)
APP = shutil.which("herdr-app", path=search)
if not HC or not APP:
    raise SystemExit(0)


def call(argv, what):
    done = subprocess.run(argv, env=env, capture_output=True, text=True, timeout=20)
    try:
        out = json.loads(done.stdout or "{}")
    except ValueError:
        out = {}
    if done.returncode != 0 or (isinstance(out, dict) and out.get("error")):
        fail("%s: %s" % (what, (out.get("error") if isinstance(out, dict) else "") or (done.stderr or done.stdout).strip()))
    return out


def app(command, **kw):
    argv = [APP, "do", command]
    for key, value in kw.items():
        argv += ["--" + key, value]
    return call(argv, "herdr-app do " + command)


def hc(*argv):
    return call([HC, *argv, "--json"], "hc " + argv[0])


def spaces():
    done = subprocess.run([APP, "ls"], env=env, capture_output=True, text=True, timeout=20)
    if done.returncode != 0:
        raise SystemExit(0)
    return [s for s in json.loads(done.stdout or "{}").get("spaces", []) if not s.get("machine")]


def locate():
    for space in spaces():
        for number, tab in enumerate(space.get("tabs") or [], 1):
            for entry in tab.get("terminals") or []:
                if entry.get("terminal") == terminal:
                    pane = {"pane_id": entry.get("pane"), "tab_id": tab["id"], "workspace_id": space["id"],
                            "terminal_id": terminal}
                    tab = dict(tab, number=number, pane_count=len(tab.get("terminals") or []))
                    return pane, tab, space
    return None


def me():
    found = locate()
    if not found:
        raise SystemExit(0)
    return found


def unnamed(tab):
    label = str(tab.get("name") or "")
    return label == "" or label == str(tab.get("number"))


def labels():
    done = subprocess.run([HC, "get", terminal, "--json"], env=env, capture_output=True, text=True, timeout=10)
    try:
        return json.loads(done.stdout or "{}").get("labels") or {}
    except ValueError:
        return {}


def history(tab):
    # What the log says about this conversation: rename time, doing time, placed, goal declared, areas.
    renamed = latest_doing = doing_text = None
    placed, declared, areas = False, False, []
    for event in EVENTS:
        name = event.get("name") or ""
        payload = event.get("payload") or {}
        if name == "herdr.placed" and payload.get("area"):
            areas.append(payload["area"])
        if not (name.startswith("herdr.") or name == "goal.worked"):
            continue
        mine = payload.get("terminal") == terminal if payload.get("terminal") else payload.get("tab") == tab.get("id")
        if not mine:
            continue
        if name == "herdr.named":
            renamed = event.get("occurred_at")
        elif name == "herdr.doing":
            latest_doing = event.get("occurred_at")
            doing_text = payload.get("text")
        elif name == "herdr.placed":
            placed = True
        elif name == "goal.worked":
            declared = True
    return renamed, latest_doing, doing_text, placed, declared, areas


def current_areas(logged):
    names = {str(s.get("name") or "") for s in spaces()}
    live = [a for a in dict.fromkeys(reversed(logged)) if a in names]
    return live + [a for a in DEFAULT_AREAS if a not in live]


def moment(stamp):
    head, _, frac = stamp.rstrip("Z").partition(".")
    return datetime.fromisoformat(head + "." + (frac + "000000")[:6] + "+00:00")


def stale(renamed, latest):
    try:
        when = moment(latest) if latest else None
        if when is None or (renamed and moment(renamed) > when):
            return True
    except ValueError:
        return True
    return datetime.now(timezone.utc) - when > timedelta(hours=3)


def ids(pane):
    return {"tab": pane["tab_id"], "pane": pane["pane_id"], "terminal": terminal}


def focused():
    done = subprocess.run([APP, "status"], env=env, capture_output=True, text=True, timeout=10)
    try:
        return json.loads(done.stdout or "{}").get("agent") == terminal
    except ValueError:
        return False


def place(pane, tab, label):
    target = next((s for s in spaces() if str(s.get("name") or "") == area), None)
    if target and target["id"] == pane["workspace_id"]:
        return pane, {"name": "herdr.placed", "payload": dict(ids(pane), area=area, workspace=pane["workspace_id"],
                                                              from_workspace=pane["workspace_id"], created=False)}
    if int(tab.get("pane_count") or 1) > 1:
        print("%s: this tab holds %s panes; not moving it to %r" % (PROG, tab.get("pane_count"), area), file=sys.stderr)
        return pane, None
    follow = focused()
    space_id = target["id"] if target else "sp" + secrets.token_hex(16)
    app("terminal.park", terminal=terminal)
    for _ in range(30):
        if not locate():
            break
        time.sleep(0.1)
    hc("label", terminal, "herdr.space=" + space_id, "herdr.tab=tb" + secrets.token_hex(16))
    found = None
    for _ in range(50):
        found = locate()
        if found:
            break
        time.sleep(0.1)
    if not found:
        fail("the window did not adopt %s into %r; hc get %s shows its labels" % (terminal, area, terminal))
    new, _, space = found
    if target is None or space.get("name") != area:
        app("space.rename", id=new["workspace_id"], name=area)
    app("tab.rename", id=new["tab_id"], name=label)
    if follow:
        app("pane.focus", terminal=terminal)
    return new, {"name": "herdr.placed", "payload": dict(ids(new), area=area, workspace=new["workspace_id"],
                                                         from_workspace=pane["workspace_id"],
                                                         created=target is None)}


if command == "--show":
    pane, tab, here = me()
    logged = history(tab)[5]
    print("goal %s" % (labels().get("goal") or "(none)"), file=sys.stderr)
    print("tab  %s: %s%s" % (tab.get("id"), tab.get("name"), " (unnamed)" if unnamed(tab) else ""), file=sys.stderr)
    print("pane %s (terminal %s)" % (pane["pane_id"], terminal), file=sys.stderr)
    print("area %s" % (here.get("name") or "?"), file=sys.stderr)
    print("areas: %s" % ", ".join(current_areas(logged)), file=sys.stderr)
    raise SystemExit(0)

if command == "--nudge":
    pane, tab, _ = me()
    renamed, latest, text, placed, declared, logged = history(tab)
    areas = ", ".join(current_areas(logged))
    have = labels()
    if text and not unnamed(tab) and have.get("doing") != text:
        hc("label", terminal, "doing=" + text)
    if unnamed(tab):
        print("This Herdr tab (%s) is unnamed. Once the work is clear, say which goal it is: "
              "self run working-on <goal> --doing \"<one sentence>\" (open goals: self view tree). "
              "No goal fits: self run working-on - <2-4 words> --area <area> --doing \"<one sentence>\" "
              "(areas now: %s)." % (tab.get("id"), areas), file=sys.stderr)
    elif not have.get("goal") and not declared:
        print("This Herdr tab (%r) has no goal, so it sits in Unfiled. If an open goal fits (self view tree), "
              "link it: self run working-on <goal>. None fits: self run working-on - to say so."
              % tab.get("name"), file=sys.stderr)
    elif not placed:
        print("This Herdr tab (%r) is not filed under an area. File it: self run working-on --area <area> "
              "(areas now: %s; or the project name)." % (tab.get("name"), areas), file=sys.stderr)
    elif stale(renamed, latest):
        print("This Herdr tab (%r) has no current doing line. When the work is clear or shifts, say it in "
              "one sentence, no people or prod data: self run working-on --doing \"<what this tab is doing now>\""
              % tab.get("name"), file=sys.stderr)
    raise SystemExit(0)

if command == "--clear":
    pane, tab, _ = me()
    app("tab.rename", id=tab["id"], name="")
    have = labels()
    if have.get("doing") or have.get("goal"):
        hc("label", terminal, "doing=", "goal=")
    raise SystemExit(0)

events = []
if goal or unfiled:
    hc("label", terminal, "goal=" + (goal or ""))
found = locate()
if not found:
    # A terminal with no tab in the window (a background worker): the goal and doing
    # labels still place it in the agents list; there is nothing to name or move.
    if goal or unfiled:
        events.append({"name": "goal.worked", "payload": {"tab": None, "pane": None, "terminal": terminal,
                                                          "goal": goal, "area": None}})
    if doing is not None:
        hc("label", terminal, "doing=" + doing)
    for event in events:
        print(json.dumps(event))
    raise SystemExit(0)
pane, tab, _ = found
label = None
if words:
    label = normalise(words, 28)
    if not label:
        fail("nothing left of %r after normalising" % " ".join(words), 2)
elif not unnamed(tab):
    label = str(tab.get("name"))
if area is not None:
    if label is None:
        fail("name the tab before filing it: self run working-on - <2-4 words> --area %s" % area, 2)
    pane, placed = place(pane, tab, label)
    if placed:
        events.append(placed)
        pane, tab, _ = me()
if words:
    app("tab.rename", id=pane["tab_id"], name=label)
    named = dict(ids(pane), label=label)
    if int(tab.get("pane_count") or 1) <= 1:
        named["pane"] = None
    events.append({"name": "herdr.named", "payload": named})
if goal or unfiled:
    events.append({"name": "goal.worked", "payload": dict(ids(pane), goal=goal, area=area)})
if doing is not None:
    events.append({"name": "herdr.doing", "payload": dict(ids(pane), text=doing)})
    hc("label", terminal, "doing=" + doing)
for event in events:
    print(json.dumps(event))
