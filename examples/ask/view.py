#!/usr/bin/env python3
"""Installed as view/ask. Failure falls back to this exact input snapshot."""
import argparse
import json
import os
import re
import sys
import urllib.request
from urllib.parse import urlparse


def main():
    parser = argparse.ArgumentParser(description="Rank capabilities and read-only evidence for an ask.")
    parser.add_argument("ask", nargs="*")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    events = [json.loads(line) for line in sys.stdin if line.strip()]
    if not args.ask:
        print('usage: self view ask [--json] "what you want to accomplish"')
        print("Uses local Laya estimates to select bounded read-only context; never runs commands.")
        return
    ask = " ".join(args.ask)
    try:
        url = os.environ.get("SELF_ASK_URL", "http://127.0.0.1:8766")
        if urlparse(url).scheme != "http" or urlparse(url).hostname not in ("127.0.0.1", "localhost", "::1"):
            raise ValueError("SELF_ASK_URL must be local")
        request = urllib.request.Request(url.rstrip("/") + "/ask", data=json.dumps({
            "ask": ask, "head": events[-1]["id"] if events else "empty",
        }).encode(), headers={"Content-Type": "application/json"})
        # Never send local work through an HTTP proxy inherited from a shell.
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open(request, timeout=35) as response:
            result = json.load(response)
        print(json.dumps(result, ensure_ascii=False) if args.json else result["text"])
    except (OSError, ValueError, KeyError) as error:
        caps = {}
        for event in events:
            p = event.get("payload")
            if not isinstance(p, dict):
                continue
            name = p.get("name", "")
            if not isinstance(name, str) or not re.fullmatch(r"[a-zA-Z0-9._/-]{1,200}", name) or any(s in ("", ".", "..", "run") or s.startswith(".") or len(s) > 64 for s in name.split("/")):
                continue
            if event["name"] in ("command.declared", "view.declared"):
                caps[event["name"].split(".")[0] + "/" + p["name"]] = p
            elif event["name"] == "capability.retired":
                caps.pop(str(p.get("type")) + "/" + str(p.get("name")), None)
        caps.pop("view/ask", None)
        text = "Local ranking unavailable; unscored capability index.\n" + "\n".join(
            f"- {key}: {str(p.get('summary') or p.get('description', ''))[:110]}"
            for key, p in sorted(caps.items())[:48])
        if len(caps) > 48:
            text += f"\n{len(caps)-48} more; self brief lists the complete index."
        print(json.dumps({"status": "fallback", "ask": ask, "head": events[-1]["id"] if events else "empty", "text": text}) if args.json else text)
        print(f"ask: {error}", file=sys.stderr)


if __name__ == "__main__":
    main()
