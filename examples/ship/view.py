#!/usr/bin/env python3
'''ship - ship pipeline per goal: stage, PR, announce, preprod verification; `actions` prints the driver's TSV to-do list.

usage: ship | ship <goal> | ship actions

After preprod is healthy the driver dispatches a verifier (act `verify`): an
agent that tests the merged change on preprod as Wouter (self run preprod)
and records preprod.verified. pass or skip then notifies prod-ready and
closes the goal; fail alerts (hold + DM); a verifier with no verdict after
VERIFY_HOURS alerts once.
Source: ~/self examples/ship/view.py.
'''
import base64
import json
import re
import sys
from datetime import datetime, timezone

USAGE = "usage: ship [actions | <goal>]"
ALERT = {
    "ci_red": "CI is red",
    "pr_closed": "PR was closed without merging",
    "cd_red": "preprod CD failed",
    "cd_none": "no preprod deploy job ran",
    "stuck": "preprod rollout has not converged",
    "health_unknown": "no preprod workload found to check",
}
SOURCE = "origin/fix/go-dead-code-consolidated"
VERIFY_HOURS = 3

args = sys.argv[1:]
if len(args) > 1:
    print(USAGE, file=sys.stderr)
    raise SystemExit(2)

events = []
for line in sys.stdin:
    try:
        e = json.loads(line)
    except ValueError:
        continue
    if isinstance(e, dict) and isinstance(e.get("payload"), dict):
        events.append(e)

goals, removed = {}, set()
planned, tracked, stages, announced, acted, agents, verdicts = {}, {}, {}, {}, set(), {}, {}
for e in events:
    p, n, seq = e["payload"], e.get("name"), e.get("seq", 0)
    g = p.get("goal")
    if n == "goal.created" and isinstance(g, str):
        goals.setdefault(g, {"status": "active", "parent": p.get("parent", ""), "depends_on": list(p.get("depends_on") or []), "title": p.get("title", "")})
    elif n == "goal.updated" and g in goals:
        for k in ("status", "depends_on"):
            if k in p:
                goals[g][k] = p[k]
    elif n == "goal.removed":
        removed.add(g)
    elif n == "ship.planned":
        planned[g] = dict(p, _seq=seq)
    elif n == "ship.tracked":
        tracked[g] = p
    elif n == "ship.stage":
        stages[g] = dict(p, _seq=seq, _at=e.get("occurred_at", ""))
    elif n in ("ship.announced", "ship.announce_failed"):
        announced[g] = dict(p, _failed=n.endswith("failed"), _seq=seq)
    elif n == "ship.acted":
        acted.add((g, p.get("key")))
    elif n in ("agent.requested", "agent.failed") and g:
        agents.setdefault(g, []).append((seq, n, e.get("occurred_at", "")))
    elif n == "preprod.verified" and g:
        verdicts[g] = dict(p, _seq=seq)


def closed(g):
    return g in removed or str(goals.get(g, {}).get("status", "active")).lower() in {"closed", "done", "completed", "cancelled", "removed"}


def held(g):
    seen = set()
    while g in goals and g not in seen:
        seen.add(g)
        if str(goals[g].get("status", "")).lower() == "on_hold":
            return True
        g = goals[g].get("parent", "")
    return False


def blocked(g):
    return [d for d in goals.get(g, {}).get("depends_on", []) if not closed(d)]


def agent_name(g):
    short = re.sub(r"^go-deadcode-ship-", "", g)
    return ("ship-" + short)[:32]


def prompt(g):
    pl = planned[g]
    lines = [
        "You own goal %s: one slice of the Go dead-code cleanup, shipped as its own PR." % g,
        "Procedure: self view howto mym/ship-deadcode-slice (follow it exactly). Context: self view context %s." % g,
        "Project dir: %s" % pl["project_dir"],
        "Source branch: %s. Bring over, in order:" % SOURCE,
    ]
    lines += ["  - " + c for c in pl["commits"]]
    lines += [
        "Finish with: self run ship track %s <pr-url>, then self run goal progress %s '<evidence; next>'. Then stop." % (g, g),
        "Do NOT post to Slack and do NOT wait for CI or merge: the ship driver announces once CI is green and dispatches the next step after preprod is healthy.",
    ]
    return "\n".join(lines)


def verify_name(g):
    return ("verify-" + re.sub(r"^go-deadcode-ship-", "", g))[:32]


def verify_prompt(g):
    pr = tracked[g]["pr"]
    return "\n".join([
        "You verify goal %s on preprod. Its PR %s is merged and preprod is healthy on that commit." % (g, pr),
        "1. Understand the change: gh pr view %s --comments; gh pr diff %s; self view context %s." % (pr, pr, g),
        "2. Decide the smallest check a user would notice that proves it works on preprod (https://preprod-v2.mym.fans; "
        "creators.preprod-v2, business.preprod-v2, backoffice.preprod-v2 for the other apps). You act as Wouter: "
        "self run preprod login --as fan|creator (account wouter.beets@mym.fans; self brief preprod).",
        "3. Frontend change: self run preprod shot <path> <name> for each screen it touches, then Read every PNG and "
        "judge it. Backend change: drive the UI path or API that runs it, and read what it returned; preprod DB facts "
        "via the monodb/preprod pass entries if needed. Preprod payments use test PSPs only; create no more data than "
        "the test needs and say what you created.",
        "4. Record exactly one verdict: self run preprod verdict %s pass|fail|skip '<what you did, what you saw>' "
        "[--shot <png>]... It posts the evidence on the PR. skip only when nothing user-visible can exercise it "
        "(pure refactor, dead code, infra) and say why. fail when it is broken; do not fix it yourself." % g,
        "Practices for this step, on the files the PR changed: self run practice show --at verify --base <the merge commit>^ "
        "(run it in your worktree after git fetch; the merge commit is gh pr view %s --json mergeCommit). Follow each." % pr,
        "5. self run goal progress %s '<verdict and evidence>', then stop." % g,
    ])


def hours_since(stamp):
    try:
        then = datetime.fromisoformat(stamp.replace("Z", "+00:00"))
    except ValueError:
        return 0
    return (datetime.now(timezone.utc) - then).total_seconds() / 3600


def b64(s):
    return base64.b64encode(s.encode()).decode()


def actions():
    out = []
    for g in sorted(set(planned) | set(tracked)):
        if g not in goals or closed(g):
            continue
        st = stages.get(g, {})
        stage = st.get("stage", "")
        if g in tracked:
            if stage == "ci_green" and g not in announced:
                out.append(("announce", g, "", ""))
            if g in announced and announced[g]["_failed"] and (g, "announce_failed") not in acted:
                out.append(("alert", g, "announce_failed", "%s: Slack announce failed (%s) for %s" % (g, announced[g].get("error", ""), tracked[g]["pr"])))
            if stage in ALERT:
                key = "%s@%d" % (stage, st["_seq"])
                if (g, key) not in acted:
                    out.append(("alert", g, key, "%s: %s. %s %s" % (g, ALERT[stage], st.get("detail", ""), tracked[g]["pr"])))
            if stage == "healthy" and not held(g):
                v = verdicts.get(g)
                if v and v["_seq"] < st["_seq"]:
                    v = None
                if v is None:
                    tries = [t for t in agents.get(g, []) if t[0] > st["_seq"]]
                    if not tries:
                        out.append(("verify", g, verify_name(g), b64(verify_prompt(g))))
                    elif tries[-1][1] == "agent.failed":
                        key = "verify_failed@%d" % tries[-1][0]
                        if (g, key) not in acted:
                            out.append(("alert", g, key, "%s: preprod verifier dispatch failed; see self view agents" % g))
                    elif hours_since(tries[-1][2]) > VERIFY_HOURS:
                        key = "verify_stalled@%d" % tries[-1][0]
                        if (g, key) not in acted:
                            out.append(("alert", g, key, "%s: preprod verifier gave no verdict in %dh. %s" % (g, VERIFY_HOURS, tracked[g]["pr"])))
                elif v["verdict"] == "fail":
                    key = "preprod_fail@%d" % v["_seq"]
                    if (g, key) not in acted:
                        out.append(("alert", g, key, "%s: failed on preprod: %s %s" % (g, v.get("evidence", "")[:300], tracked[g]["pr"])))
                else:
                    word = "verified on preprod" if v["verdict"] == "pass" else "healthy on preprod (verification skipped)"
                    note = "%s: %s (%s)" % (word, v.get("evidence", "")[:300], tracked[g]["pr"])
                    if (g, "prod_ready") not in acted:
                        out.append(("notify", g, "prod_ready", "%s is %s; ready for a prod tag. %s\n%s" % (g, word, tracked[g]["pr"], v.get("evidence", "")[:300])))
                    out.append(("close", g, "", note))
        elif g in planned and not held(g) and not blocked(g):
            tries = [s for s in agents.get(g, []) if s[0] > planned[g]["_seq"]]
            if not tries:
                out.append(("dispatch", g, agent_name(g), b64(prompt(g))))
            elif tries[-1][1] == "agent.failed" and (g, "dispatch_failed@%d" % tries[-1][0]) not in acted:
                out.append(("alert", g, "dispatch_failed@%d" % tries[-1][0], "%s: agent dispatch failed; see self view agents" % g))
    return out


if args == ["actions"]:
    for a in actions():
        print("\t".join(a))
    raise SystemExit(0)

rows = []
for g in sorted(set(planned) | set(tracked)):
    st = stages.get(g, {})
    if closed(g):
        state = "done"
    elif st.get("stage") == "healthy" and g in tracked and not held(g):
        v = verdicts.get(g)
        state = ("verified " + v["verdict"]) if v and v["_seq"] > st["_seq"] else "verifying"
    elif held(g):
        state = "held"
    elif g in tracked:
        state = st.get("stage", "tracked")
    elif blocked(g):
        state = "waiting on " + ", ".join(blocked(g))
    else:
        state = "ready to dispatch"
    ann = "announced" if g in announced and not announced[g]["_failed"] else ("announce failed" if g in announced else "")
    rows.append((g, state, tracked.get(g, {}).get("pr", ""), ann, st.get("detail", "")))

if args:
    g = args[0]
    if g not in planned and g not in tracked:
        print("ship: %s is not planned or tracked" % g, file=sys.stderr)
        raise SystemExit(2)
    print(json.dumps({"goal": g, "planned": planned.get(g), "tracked": tracked.get(g), "stage": stages.get(g), "announced": announced.get(g), "verified": verdicts.get(g)}, indent=2, default=str))
    raise SystemExit(0)

if not rows:
    print("nothing planned or tracked. " + USAGE)
    raise SystemExit(0)
for r in rows:
    print("%-44s %-18s %s %s" % (r[0], r[1], r[2], r[3]))
    if r[4] and r[1] not in ("done",):
        print("%-44s %s" % ("", r[4]))
