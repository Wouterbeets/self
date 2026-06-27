#!/usr/bin/env python3
"""
brainbridge — let a human or agent be `self`'s brain.

`self`'s brain/compiler is just an OpenAI-compatible POST /v1/chat/completions
to SELF_LLM_URL. This server parks each request as a file and blocks until you
answer it, so the "model" is you.

Run:
    BRIDGE_DIR=./bridge python3 tools/brainbridge.py 7800

Point self at it (a long timeout — you're answering by hand):
    export SELF_LLM_URL=http://127.0.0.1:7800
    export SELF_LLM_TIMEOUT=1h
    self think "..."        # or grow / heartbeat / run chat — all use the brain

Protocol (per round of the tool-use loop):
    bridge/inbox.json   {"id", "request": <the chat-completions request>}   <- written by the bridge
    bridge/outbox.json  {"id", "message": {"role":"assistant","content": "...",
                                            "tool_calls": [...optional...]}}  <- you write this
The id in outbox must match inbox. tool_calls follow the OpenAI shape:
    {"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"ls\"}"}}
A reply with no tool_calls ends the loop (its content becomes the answer).
"""
import json, os, time, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

DIR = os.environ.get("BRIDGE_DIR", "bridge")
INBOX, OUTBOX = os.path.join(DIR, "inbox.json"), os.path.join(DIR, "outbox.json")
TIMEOUT = float(os.environ.get("BRIDGE_TIMEOUT", "3600"))


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0))).decode()
        rid = "%.6f" % time.time()
        json.dump({"id": rid, "request": json.loads(body)}, open(INBOX, "w"), indent=2)
        sys.stderr.write("brainbridge: parked request %s (awaiting brain)\n" % rid)
        sys.stderr.flush()
        deadline, msg = time.time() + TIMEOUT, None
        while time.time() < deadline:
            if os.path.exists(OUTBOX):
                try:
                    out = json.load(open(OUTBOX))
                except Exception:
                    time.sleep(0.2)
                    continue
                if out.get("id") == rid:
                    msg = out.get("message", {})
                    os.remove(OUTBOX)
                    if os.path.exists(INBOX):
                        os.remove(INBOX)
                    break
            time.sleep(0.3)
        if msg is None:
            self.send_response(504)
            self.end_headers()
            return
        data = json.dumps({"choices": [{"message": msg}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


if __name__ == "__main__":
    os.makedirs(DIR, exist_ok=True)
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 7800
    sys.stderr.write("brainbridge: http://127.0.0.1:%d  (BRIDGE_DIR=%s)\n" % (port, DIR))
    HTTPServer(("127.0.0.1", port), H).serve_forever()
