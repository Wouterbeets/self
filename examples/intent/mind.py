#!/usr/bin/env python3
"""Deterministic demo mind for the three fixtures, not an LLM or a scheduler.

Each invocation discovers open work from the prompt, reads the authoritative
log, and performs one fixture's bounded calculation. Missing inputs yield silence.
"""
import json
import os
from pathlib import Path
import subprocess
import sys

prompt = sys.stdin.read()
home = Path(os.environ["SELF_HOME"])
events = [json.loads(line) for line in (home / "events.jsonl").read_text().splitlines()]
work = {}
for event in events:
    if event["name"] == "intent.declared":
        work[event["payload"]["name"]] = event["payload"]
    elif event["name"] == "intent.closed":
        work.pop(event["payload"]["name"], None)


def latest(name):
    return next((e["payload"] for e in reversed(events) if e["name"] == name), None)


def emit(name, payload):
    print(json.dumps({"name": name, "payload": payload}))


for name in work:
    if f"- intent/{name} —" not in prompt:
        continue
    if name == "enclosure-fit":
        spool, model = latest("spool.weighed"), latest("model.sliced")
        if not spool or not model:
            continue
        remaining = spool["gross_g"] - spool["tare_g"]
        required = model["required_g"] * 1.15
        emit("print.assessed", {"model": model["model"], "spool": spool["spool"],
                               "remaining_g": remaining, "required_with_margin_g": required,
                               "fits": remaining >= required})
        reason = f"spool.weighed minus tare gives {remaining} g; model.sliced with 15% margin needs {required:.2f} g. Fits: {remaining >= required}. See print.assessed."
    elif name == "thursday-dinner":
        calendar, pantry = latest("calendar.recorded"), latest("pantry.counted")
        if not calendar or not pantry:
            continue
        recipe = next((e["payload"] for e in events if e["name"] == "recipe.saved"
                       and e["payload"]["minutes"] <= calendar["available_cooking_minutes"]), None)
        if not recipe:
            continue
        missing = sorted(set(recipe["ingredients"]) - set(pantry["items"]))
        emit("meal.planned", {"day": calendar["day"], "recipe": recipe["name"]})
        emit("shopping.prepared", {"items": missing, "for": calendar["day"]})
        reason = f"{recipe['name']} takes {recipe['minutes']} minutes within the recorded {calendar['available_cooking_minutes']}-minute window. Missing ingredients: {', '.join(missing)}. See meal.planned and shopping.prepared."
    elif name == "sum-filament":
        source = '''#!/usr/bin/env python3
import sys
from decimal import Decimal
values = [Decimal(arg) for arg in sys.argv[1:]]
if any(not value.is_finite() or value < 0 for value in values):
    sys.exit("amounts must be finite and nonnegative")
print(sum(values, Decimal(0)))
'''
        artifact = home / "artifacts" / "sum-filament.py"
        artifact.parent.mkdir(exist_ok=True)
        artifact.write_text(source)
        good = subprocess.run([sys.executable, str(artifact), "12.5", "7.5"], capture_output=True, text=True)
        bad = subprocess.run([sys.executable, str(artifact), "-1"], capture_output=True, text=True)
        assert good.returncode == 0 and good.stdout.strip() == "20.0"
        assert bad.returncode != 0
        emit("artifact.written", {"path": str(artifact), "source": source,
                                  "verification": "12.5 + 7.5 = 20.0; negative input rejected"})
        reason = f"Verified {artifact}: 12.5 + 7.5 = 20.0; negative input rejected. Source preserved in artifact.written."
    else:
        continue
    emit("intent.closed", {"name": name, "outcome": "completed", "reason": reason})
    break
