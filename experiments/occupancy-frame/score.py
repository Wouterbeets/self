#!/usr/bin/env python3
"""Score an occupancy-frame run and report per-arm rates with bootstrap CIs.

Scoring is mechanical on purpose. The hypothesis under test is that one prompt
frame makes a mind preserve evidence rather than narration; grading that with a
second model would put the judgement inside the same failure mode the
experiment is trying to measure. So every metric here is a string or regex
match against ground truth declared with the fixture, and the fixtures are
written so that the evidence is a specific token a summary would drop.

usage: score.py <results.jsonl> [--fixtures DIR] [--json]
"""
import argparse
import json
import os
import random
import re
import statistics
import sys

BOOTSTRAP = 5000
SEED = 20260829


# ─────────────────────────────── scoring one run ────────────────────────────


def appended_text(run):
    """Everything the kernel actually accepted, as one searchable blob.

    Prose is not here: `self hear` ignores it. This is the durable record, which
    is the only thing a later waking gets.
    """
    return "\n".join(
        json.dumps({"name": e.get("name", ""), "payload": e.get("payload", "")},
                   ensure_ascii=False)
        for e in run.get("appended", [])
    ).lower()


def matched(text, spec):
    return any(v.lower() in text for v in spec["any"])


def score_run(run, truth):
    text = appended_text(run)
    n_events = len(run.get("appended", []))
    out = {
        "fixture": run["fixture"],
        "arm": run["arm"],
        "rep": run["rep"],
        "events": n_events,
        "silent": n_events == 0,
        "payload_bytes": len(text),
        "mind_failed": run.get("mind_exit", 0) != 0,
        "kernel_refused": "REFUSED" in run.get("hear_stderr", ""),
    }

    facts = truth.get("facts") or []
    if facts:
        hits = [f["id"] for f in facts if matched(text, f)]
        out["evidence_recall"] = len(hits) / len(facts)
        out["facts_hit"] = hits
        out["bytes_per_fact"] = (len(text) / len(hits)) if hits else None

    if "disclosure" in truth:
        out["disclosure"] = 1.0 if matched(text, truth["disclosure"]) else 0.0

    forbidden = truth.get("forbidden_regex") or []
    if forbidden:
        fired = [f["id"] for f in forbidden if re.search(f["pattern"], text)]
        out["fabrication"] = 1.0 if fired else 0.0
        out["fabricated"] = fired

    # A run is well-formed if the mind produced durable events, or produced
    # nothing on a fixture where silence is a legitimate turn.
    out["wellformed"] = 1.0 if (n_events > 0 or truth.get("silence_ok")) else 0.0
    return out


# ──────────────────────────────── aggregation ───────────────────────────────


def cluster_bootstrap(rows, metric, rng):
    """Resample fixtures, then runs within fixture.

    Runs sharing a fixture are correlated — they see the same ask. Bootstrapping
    over raw runs would understate the interval by pretending they are
    independent, which is exactly how an underpowered A/B announces a winner.
    """
    by_fx = {}
    for r in rows:
        if r.get(metric) is not None:
            by_fx.setdefault(r["fixture"], []).append(r[metric])
    fixtures = list(by_fx)
    if not fixtures:
        return None, None, None, 0

    flat = [v for vs in by_fx.values() for v in vs]
    point = statistics.fmean(flat)

    draws = []
    for _ in range(BOOTSTRAP):
        vals = []
        for _ in range(len(fixtures)):
            fx = rng.choice(fixtures)
            pool = by_fx[fx]
            vals += [rng.choice(pool) for _ in range(len(pool))]
        draws.append(statistics.fmean(vals))
    draws.sort()
    lo = draws[int(0.025 * len(draws))]
    hi = draws[int(0.975 * len(draws))]
    return point, lo, hi, len(flat)


def paired_diff(rows_a, rows_b, metric, rng):
    """Bootstrap the treatment-minus-control difference, paired within fixture."""
    a_by, b_by = {}, {}
    for r in rows_a:
        if r.get(metric) is not None:
            a_by.setdefault(r["fixture"], []).append(r[metric])
    for r in rows_b:
        if r.get(metric) is not None:
            b_by.setdefault(r["fixture"], []).append(r[metric])
    fixtures = sorted(set(a_by) & set(b_by))
    if not fixtures:
        return None

    def delta(fxs):
        per = []
        for fx in fxs:
            per.append(statistics.fmean(a_by[fx]) - statistics.fmean(b_by[fx]))
        return statistics.fmean(per)

    point = delta(fixtures)
    draws = sorted(
        delta([rng.choice(fixtures) for _ in fixtures]) for _ in range(BOOTSTRAP)
    )
    lo = draws[int(0.025 * len(draws))]
    hi = draws[int(0.975 * len(draws))]
    return point, lo, hi, len(fixtures)


METRICS = [
    ("evidence_recall", "evidence recall", "higher"),
    ("disclosure", "disclosure", "higher"),
    ("fabrication", "fabrication", "lower"),
    ("wellformed", "well-formed", "higher"),
]


def fmt(v):
    return "  n/a " if v is None else f"{v:5.2f}"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("results")
    ap.add_argument("--fixtures", default=os.path.join(os.path.dirname(__file__), "fixtures"))
    ap.add_argument("--json", action="store_true")
    args = ap.parse_args()

    truths = {}
    for name in sorted(os.listdir(args.fixtures)):
        p = os.path.join(args.fixtures, name, "truth.json")
        if os.path.exists(p):
            truths[name] = json.load(open(p, encoding="utf-8"))

    runs = [json.loads(l) for l in open(args.results, encoding="utf-8") if l.strip()]
    if not runs:
        sys.exit("no runs in results")

    scored = [score_run(r, truths[r["fixture"]]) for r in runs if r["fixture"] in truths]
    arms = sorted({s["arm"] for s in scored}, key=lambda a: (a != "visitor", a))
    rng = random.Random(SEED)

    if args.json:
        print(json.dumps(scored, indent=2, sort_keys=True))
        return

    reps = max(s["rep"] for s in scored)
    print()
    print(f"occupancy-frame — {len(scored)} runs, {len(truths)} fixtures, {reps} reps, arms: {', '.join(arms)}")
    print("Control arm is `visitor` (pre-#54). Intervals are 95% cluster bootstrap over fixtures.")
    print()

    width = max(len(a) for a in arms) + 2
    for key, label, direction in METRICS:
        applicable = [s for s in scored if s.get(key) is not None]
        if not applicable:
            continue
        print(f"  {label}  ({direction} is better)")
        for arm in arms:
            rows = [s for s in applicable if s["arm"] == arm]
            point, lo, hi, n = cluster_bootstrap(rows, key, rng)
            if point is None:
                continue
            bar = "#" * int(round(point * 24))
            print(f"    {arm:<{width}} {fmt(point)}  [{fmt(lo)},{fmt(hi)}]  n={n:<4} {bar}")
        print()

    # ── the comparison the experiment exists to make ────────────────────────
    control = "visitor"
    if control in arms:
        print("  paired difference vs. control (`visitor`), by fixture")
        for key, label, direction in METRICS:
            for arm in arms:
                if arm == control:
                    continue
                a = [s for s in scored if s["arm"] == arm and s.get(key) is not None]
                b = [s for s in scored if s["arm"] == control and s.get(key) is not None]
                res = paired_diff(a, b, key, rng)
                if not res:
                    continue
                point, lo, hi, nfx = res
                crosses = lo <= 0 <= hi
                verdict = "no detectable difference" if crosses else (
                    "favors " + (arm if (point > 0) == (direction == "higher") else control)
                )
                print(f"    {label:<16} {arm:<12} {point:+5.2f}  [{lo:+5.2f},{hi:+5.2f}]  {nfx} fixtures   {verdict}")
        print()

    # ── honesty about power ─────────────────────────────────────────────────
    n_per_cell = len(scored) / max(1, len(arms) * len(truths))
    notes = []
    if n_per_cell < 10:
        notes.append(
            f"UNDERPOWERED: {n_per_cell:.0f} runs per (arm, fixture) cell. Sampling noise on a "
            "single ask dominates at this size. Treat every interval above as a range that "
            "includes 'no effect' unless it plainly does not, and raise REPS before quoting "
            "any number as a result."
        )
    silent = [s for s in scored if s["silent"]]
    if silent:
        notes.append(f"{len(silent)}/{len(scored)} runs appended nothing at all.")
    broke = [s for s in scored if s["mind_failed"]]
    if broke:
        notes.append(f"{len(broke)}/{len(scored)} runs had a non-zero mind exit — check runs/*/mind.err.")
    refused = [s for s in scored if s["kernel_refused"]]
    if refused:
        notes.append(f"{len(refused)}/{len(scored)} runs had events refused by the kernel.")
    if len({s["arm"] for s in scored}) < 3:
        notes.append(
            "The `mechanism` ablation is the arm that separates the framing from the plain "
            "statement of what persists. Without it, occupancy-vs-visitor cannot tell you which "
            "one moved the number."
        )
    for n in notes:
        print("  ! " + n)
    print()


if __name__ == "__main__":
    main()
