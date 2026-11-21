#!/usr/bin/env python3
"""Score trials written by eval/run.sh.

    eval/score.py <runs-dir>

<runs-dir> holds <variant>/<model>/<rep>/ directories. Prints one row per
variant and model, each figure a count of trials out of the trials run:

  ran      the first session ran `self` at all
  kept     the instance holds at least one event after the first session
  made     it holds at least one installed capability after the first session
  reused   the second session ran or read something the first one left
  recalled the third session ran or read something an earlier one left
  knew     the log already held the answer to the third session's second
           question (why the print profile fails) before that session began
  shown    that answer was in what `self` printed when the third session arrived
  stopped  sessions the model's safety classifier ended early, of all sessions
  refused  scripts the kernel refused, summed over the trials
  caps     installed capabilities at the end, averaged
  turns    agent turns in sessions two and three, averaged (work not redone)
  cost     dollars per trial, all three sessions
"""
import json
import pathlib
import re
import sys

SELF = re.compile(r"(?:^|[\s;&|(/])self(?:\s|$)")
ANSWER = re.compile(r"\.pfb|font|E31", re.I)
USE = re.compile(r"(?:^|[\s;&|(/])self\s+(run|view|brief)\s+([\w./-]+)")


def session(path):
    cmds, turns, cost, stopped = [], 0, 0.0, False
    if not path.exists():
        return cmds, turns, cost, stopped
    for raw in path.read_text().splitlines():
        try:
            m = json.loads(raw)
        except ValueError:
            continue
        if m.get("type") == "assistant":
            for c in m["message"].get("content", []):
                if c.get("type") == "tool_use" and isinstance(c.get("input"), dict):
                    cmd = c["input"].get("command")
                    if isinstance(cmd, str):
                        cmds.append(cmd)
        elif m.get("type") == "result":
            turns, cost = m.get("num_turns", 0), m.get("total_cost_usd", 0.0)
            stopped = m.get("stop_reason") == "refusal"
    return cmds, turns, cost, stopped


def log(path):
    events = []
    if path.exists():
        for raw in path.read_text().splitlines():
            try:
                events.append(json.loads(raw))
            except ValueError:
                pass
    return events


def installed(events):
    return {(e["payload"].get("type"), e["payload"].get("name")) for e in events if e["name"] == "script.installed"}


def used(cmds, names):
    """Did a session run or read a capability that already existed?"""
    for cmd in cmds:
        for _, name in USE.findall(cmd):
            if name in names:
                return True
    return False


def trial(d):
    s = [session(d / f"s{n}.jsonl") for n in (1, 2, 3)]
    logs = [log(d / f"events.s{n}.jsonl") for n in (1, 2, 3)]
    caps = [installed(ev) for ev in logs]
    names = [{n for _, n in c} for c in caps]
    return {
        "ran": any(SELF.search(c) for c in s[0][0]),
        "kept": len(logs[0]) > 0,
        "made": len(caps[0]) > 0,
        "reused": used(s[1][0], names[0]),
        "recalled": used(s[2][0], names[1]),
        "knew": any(ANSWER.search(json.dumps(e.get("payload"))) for e in logs[1]),
        "shown": (d / "arrive.s3.txt").exists() and bool(ANSWER.search((d / "arrive.s3.txt").read_text())),
        "stopped": sum(1 for x in s if x[3]),
        "refused": sum(1 for e in logs[2] if e["name"] == "script.rejected"),
        "caps": len(caps[2]),
        "turns": s[1][1] + s[2][1],
        "cost": sum(x[2] for x in s),
    }


def main(root):
    root = pathlib.Path(root)
    head = f"{'variant':<10}{'model':<8}{'n':>3}{'ran':>6}{'kept':>6}{'made':>6}{'reused':>8}{'recalled':>10}{'knew':>6}{'shown':>7}{'stopped':>9}{'refused':>9}{'caps':>6}{'turns':>7}{'cost':>8}"
    print(head)
    for variant in sorted(p for p in root.iterdir() if p.is_dir()):
        for model in sorted(p for p in variant.iterdir() if p.is_dir()):
            rows = [trial(d) for d in sorted(model.iterdir()) if (d / "s3.jsonl").exists()]
            if not rows:
                continue
            n = len(rows)
            count = lambda k: f"{sum(1 for r in rows if r[k])}/{n}"
            mean = lambda k: sum(r[k] for r in rows) / n
            print(f"{variant.name:<10}{model.name:<8}{n:>3}{count('ran'):>6}{count('kept'):>6}{count('made'):>6}"
                  f"{count('reused'):>8}{count('recalled'):>10}{count('knew'):>6}{count('shown'):>7}{str(sum(r['stopped'] for r in rows)) + '/' + str(3 * n):>9}{sum(r['refused'] for r in rows):>9}"
                  f"{mean('caps'):>6.1f}{mean('turns'):>7.1f}{mean('cost'):>8.2f}")


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "runs")
