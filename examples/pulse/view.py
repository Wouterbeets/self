#!/usr/bin/env python3
"""pulse - is self's own machinery behaving: loops, bursts of outward effects, failed timers.

usage: self view pulse [all | actions | --json]

  (bare)   what is wrong now, then the last 24 hours of outward effects, who
           wrote the log, loops that stopped in the last 7 days, and failures.
  all      also shows findings that were acknowledged.
  actions  key<TAB>title<TAB>body for every finding running now that was
           neither acknowledged nor already told: what the pulse driver pushes.
  --json   the findings as data.

An outward effect is an event that records something leaving this machine or
costing money: a Slack DM or message, a phone push, a post in a team channel,
an agent dispatch, a pull request, a git push, a production database query.

  loop     the same effect with the same content (digits and ids masked in
           free text; goals and PR links compared exactly) repeated inside one
           run, a run being occurrences less than six
           hours apart. One alert is one event; the same alert twenty times is
           a machine that cannot tell it already spoke.
  burst    more of one kind of effect inside one clock hour than its limit,
           reported once per stretch of busy hours and only when no loop
           already explains it.
  timer    a systemd user timer that has been failing or unarmed for over a
           day, as witnessed by self run pulse tick. A shorter failure is
           listed under Timers and is not a finding: a peer asleep overnight
           fails its sync and recovers.
  twice    a loop both bodies took part in: two drivers doing one job.

A finding is running while its last event is within two hours of the newest
event in the log, and stopped after that. Acknowledge one with
self run pulse ack <key> <why>; a new run gets a new key.

The view reads no clock: everything is measured against the newest event, so a
machine that is off looks quiet, not broken. It exists because for six days in
November 2026 a driver sent the same alert every five minutes and nothing in
the log said so until a person scrolled their DMs.
Source: ~/self examples/pulse/view.py.
"""
import collections
import datetime
import hashlib
import json
import re
import sys

args = sys.argv[1:]
if args not in ([], ["all"], ["actions"], ["--json"]):
    print("usage: self view pulse [all | actions | --json]", file=sys.stderr)
    raise SystemExit(2)

RUN_GAP_HOURS = 6
RUNNING_HOURS = 2
EFFECTS = {
    "support.dm_sent": ("Slack DM", 6, 3, lambda p: p.get("message"), True),
    "attention.spoken": ("Slack message", 20, 3, lambda p: p.get("text"), True),
    "phone.pushed": ("phone push", 12, 2, lambda p: "%s %s" % (p.get("title"), p.get("body")), True),
    "reminder.sent": ("reminder push", 12, 2, lambda p: p.get("key"), False),
    "ship.announced": ("team channel post", 6, 2, lambda p: p.get("pr"), False),
    "agent.started": ("agent dispatch", 30, 4, lambda p: p.get("goal"), False),
    "pr.opened": ("pull request opened", 10, 2, lambda p: p.get("url") or p.get("branch"), False),
    "pr.pushed": ("git push", 20, None, None, False),
    "dbprod.queried": ("production DB query", 100, None, None, False),
}
FAILURES = {"agent.failed": "agent dispatch failed", "pr.push.failed": "push failed",
            "ship.announce_failed": "review announce failed", "script.rejected": "script rejected"}
STAMP = re.compile(r"^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})")
MASK = re.compile(r"https?://\S+|\b[0-9a-f]{7,}\b|\d+")


def when(event):
    found = STAMP.match(str(event.get("occurred_at") or ""))
    return datetime.datetime(*(int(found.group(i)) for i in range(1, 7))) if found else None


def body_of(event):
    door = str(event.get("via") or "")
    return door[6:] if door.startswith("learn:") else "here"


def show(moment):
    return moment.strftime("%d/%m %H:%MZ") if moment else "?"


events = []
for line in sys.stdin:
    try:
        event = json.loads(line)
    except (TypeError, ValueError):
        continue
    if isinstance(event, dict) and isinstance(event.get("payload"), dict) and when(event):
        events.append(event)
events.sort(key=lambda e: (when(e), e.get("seq", 0)))
tip = when(events[-1]) if events else None
tip_seq = max((e.get("seq", 0) for e in events), default=0)


def hours_before_tip(moment):
    return (tip - moment).total_seconds() / 3600


by_signature = collections.defaultdict(list)
by_hour = collections.defaultdict(list)
acked, told, timers = {}, set(), None
last_day = collections.Counter()
effects_day = collections.Counter()
failures = collections.defaultdict(list)
for event in events:
    name, payload, moment = event.get("name"), event["payload"], when(event)
    age = hours_before_tip(moment)
    if age <= 24:
        last_day[str(event.get("by") or "unsigned")] += 1
    if name == "pulse.acked" and payload.get("key"):
        acked[payload["key"]] = str(payload.get("note") or "")
    elif name == "pulse.told" and payload.get("key"):
        told.add(payload["key"])
    elif name == "pulse.timers":
        timers = event
    elif name in FAILURES and age <= 24 * 7:
        failures[name].append(event)
    elif name == "capability.verified" and payload.get("verdict") == "broken" and age <= 24 * 7:
        failures["capability.verified"].append(event)
    if name not in EFFECTS or age > 24 * 7:
        continue
    if name == "attention.spoken" and not payload.get("channel"):
        continue
    if name == "ship.announced" and (payload.get("found") or payload.get("by_hand")):
        continue
    label, hourly, repeats, content, masked = EFFECTS[name]
    if age <= 24:
        effects_day[label] += 1
    by_hour[(label, moment.strftime("%Y-%m-%dT%H"))].append(event)
    if repeats and content:
        text = " ".join(str(content(payload) or "").split())
        by_signature[(label, MASK.sub("#", text)[:160] if masked else text[:160])].append((event, text))

findings = []


def finding(kind, identity, moments, title, detail, bodies=(), writers=()):
    key = hashlib.sha1(identity.encode()).hexdigest()[:8]
    findings.append({
        "key": key, "kind": kind, "title": title, "detail": detail,
        "first": moments[0], "last": moments[-1], "count": len(moments),
        "running": hours_before_tip(moments[-1]) <= RUNNING_HOURS,
        "bodies": sorted(set(bodies)), "writers": sorted(set(writers)),
        "acked": acked.get(key),
    })


for (label, signature), rows in by_signature.items():
    repeats = next(v[2] for v in EFFECTS.values() if v[0] == label)
    run = []
    for row in rows + [None]:
        if row is not None and (not run or (when(row[0]) - when(run[-1][0])).total_seconds() < RUN_GAP_HOURS * 3600):
            run.append(row)
            continue
        if len(run) >= repeats:
            moments = [when(e) for e, _ in run]
            bodies = [body_of(e) for e, _ in run]
            kind = "twice" if len(set(bodies)) > 1 else "loop"
            finding(kind, "loop|%s|%s|%s" % (label, signature, moments[0].isoformat()), moments,
                    "%s repeated %d times" % (label, len(run)), run[-1][1][:200], bodies,
                    [str(e.get("by") or "unsigned") for e, _ in run])
        run = [row] if row is not None else []

hot = collections.defaultdict(list)
for (label, hour), rows in by_hour.items():
    limit = next(v[1] for v in EFFECTS.values() if v[0] == label)
    if len(rows) >= limit:
        hot[label].append((datetime.datetime.strptime(hour, "%Y-%m-%dT%H"), rows))
explained = [(f["title"].split(" repeated")[0], f["first"], f["last"]) for f in findings]
for label, hours in hot.items():
    limit = next(v[1] for v in EFFECTS.values() if v[0] == label)
    hours.sort(key=lambda pair: pair[0])
    run = []
    for pair in hours + [None]:
        if pair is not None and (not run or (pair[0] - run[-1][0]).total_seconds() < RUN_GAP_HOURS * 3600):
            run.append(pair)
            continue
        rows = [event for _, bucket in run for event in bucket]
        moments = sorted(when(event) for event in rows)
        looping = any(name == label and first <= moments[-1] and last >= moments[0] for name, first, last in explained)
        if not looping:
            peak = max(len(bucket) for _, bucket in run)
            finding("burst", "burst|%s|%s" % (label, run[0][0].isoformat()), moments,
                    "%d %s events in %d busy hour%s" % (len(rows), label, len(run), "" if len(run) == 1 else "s"),
                    "peak %d in one hour, limit %d" % (peak, limit),
                    [body_of(event) for event in rows], [str(event.get("by") or "unsigned") for event in rows])
        run = [pair] if pair is not None else []

TIMER_GRACE_HOURS = 24
bad_timers = []
if timers:
    host = timers["payload"].get("host", "?")
    for unit in timers["payload"].get("units", []):
        if unit.get("state") not in ("failed", "inactive"):
            continue
        since = when({"occurred_at": unit.get("since")}) or when(timers)
        hours = hours_before_tip(since)
        bad_timers.append((unit, since, hours))
        if hours < TIMER_GRACE_HOURS:
            continue
        key = hashlib.sha1(("timer|%s|%s|%s|%s" % (host, unit.get("unit"), unit["state"], since.isoformat())).encode()).hexdigest()[:8]
        findings.append({
            "key": key, "kind": "timer",
            "title": "%s %s for %d days" % (unit.get("unit"), "has failed every run" if unit["state"] == "failed" else "has not been armed", hours // 24),
            "detail": "%s on %s, since %s" % (unit.get("result") or unit["state"], host, show(since)),
            "first": since, "last": tip, "count": 1, "running": True, "bodies": [body_of(timers)], "writers": [],
            "acked": acked.get(key),
        })

findings.sort(key=lambda f: (not f["running"], -f["count"], f["key"]))
loops_covering = {(f["first"].strftime("%Y-%m-%dT%H"), f["title"].split(" repeated")[0]) for f in findings if f["kind"] in ("loop", "twice")}
live = [f for f in findings if f["running"] and (not f["acked"] or args == ["all"])]
stopped = [f for f in findings if not f["running"] and f["kind"] != "burst" and (not f["acked"] or args == ["all"])]
old_bursts = [f for f in findings if not f["running"] and f["kind"] == "burst"]


def line(f):
    where = " on both bodies" if f["kind"] == "twice" else (" on " + f["bodies"][0] if f["bodies"] and f["bodies"] != ["here"] else "")
    who = (" by " + ", ".join(f["writers"][:2])) if f["writers"] else ""
    span = show(f["first"]) if f["count"] == 1 else "%s to %s" % (show(f["first"]), show(f["last"]))
    note = "  [acknowledged: %s]" % f["acked"] if f["acked"] else ""
    return "- [%s] %s%s%s, %s%s\n  %s" % (f["key"], f["title"], where, who, span, note, f["detail"])


if args == ["--json"]:
    plain = [dict(f, first=f["first"].isoformat() + "Z", last=f["last"].isoformat() + "Z") for f in findings]
    print(json.dumps({"tip": tip.isoformat() + "Z" if tip else None, "findings": plain}, indent=1))
    raise SystemExit(0)

if args == ["actions"]:
    for f in live:
        if f["key"] not in told and not f["acked"]:
            title = "self pulse: " + f["title"] + (" on both bodies" if f["kind"] == "twice" else "")
            print("\t".join((f["key"], title, " ".join(f["detail"].split())[:220] or "-")))
    raise SystemExit(0)

print("# Pulse")
if not tip:
    print("The log is empty.")
    raise SystemExit(0)
print("Newest event [%d] %s. %s" % (tip_seq, show(tip), ("1 thing is wrong now." if len(live) == 1 else "%d things are wrong now." % len(live)) if live else "Nothing is looping, bursting or failing right now."))

if live:
    print("\n## Wrong now (%d)" % len(live))
    for f in live:
        print(line(f))
    print("Silence one you have judged: self run pulse ack <key> <why>.")

print("\n## Outward effects, last 24 hours")
if effects_day:
    for label, count in effects_day.most_common():
        print("- %s: %d" % (label, count))
else:
    print("- none")

total = sum(last_day.values())
if total:
    print("\n## Who wrote the last 24 hours (%d events)" % total)
    for writer, count in last_day.most_common(6):
        print("- %s: %d (%d%%)" % (writer, count, round(100 * count / total)))

if timers:
    units = timers["payload"].get("units", [])
    print("\n## Timers on %s, witnessed %s" % (timers["payload"].get("host", "?"), show(when(timers))))
    print("- %d of %d healthy" % (len(units) - len(bad_timers), len(units)))
    for unit, since, hours in bad_timers:
        print("- %s %s since %s (%s)" % (unit.get("unit"), unit["state"], show(since),
                                        "wrong: over a day" if hours >= TIMER_GRACE_HOURS else "within the one-day grace"))
else:
    print("\n## Timers\n- never witnessed: self run pulse tick records them.")

if stopped:
    print("\n## Loops in the last 7 days, now stopped (%d)" % len(stopped))
    for f in stopped[:12]:
        print(line(f))
    if len(stopped) > 12:
        print("- ... %d more" % (len(stopped) - 12))

if old_bursts:
    print("\n## Bursts in the last 7 days, now over (%d)" % len(old_bursts))
    for f in old_bursts[:8]:
        print(line(f))

if failures:
    print("\n## Failures in the last 7 days")
    for name, rows in sorted(failures.items(), key=lambda kv: -len(kv[1])):
        print("- %s: %d, last %s" % (FAILURES.get(name, "capability verified broken"), len(rows), show(when(rows[-1]))))
