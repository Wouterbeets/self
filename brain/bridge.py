#!/usr/bin/env python3
"""claude-brain-bridge — a localhost LLM endpoint with a human (well, a Claude)
in the loop.

`self`'s kernel reaches for its brain by POSTing an OpenAI-shaped
`/v1/chat/completions` request to SELF_LLM_URL. This server speaks that
contract — but instead of forwarding to a model API, it *parks* each request on
disk and blocks, waiting for a reply to appear. A Claude Code instance polls the
inbox, reads the parked request, crafts the assistant message by hand, and drops
it in the outbox. The server unwraps it back into a valid completion and the
kernel's thought continues.

So the brain of `self` is, quite literally, whichever Claude is watching the
inbox. Every `self think` / `self heartbeat` round is one request parked here.

Wire protocol (what the kernel sends / expects), see internal/seed/compiler.go:
  POST /v1/chat/completions
    <- {model, messages:[{role,content,tool_calls?}], temperature, tools:[...]}
    -> {choices:[{message:{role,content,tool_calls?}}]}
  A tool_call is {id, type:"function", function:{name, arguments(JSON string)}}.

The brain's side of the contract (what a Claude writes), per request <id>:
  inbox/<id>.json       full request body (machine)
  inbox/<id>.txt        pretty render of system+messages+tools (for a human/Claude)
  outbox/<id>.json      the assistant message to return, ONE of:
                          {"content": "final text answer"}
                          {"tool_calls": [{"name": "...", "arguments": {...}}]}
                        ids/type are filled in automatically; arguments may be a
                        dict (auto-encoded) or an already-encoded JSON string.

Run:  BRAIN_DIR=/path/to/queue python3 bridge.py [port]
"""

import json
import os
import sys
import time
import uuid
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

BRAIN_DIR = os.environ.get("BRAIN_DIR", os.path.join(os.path.dirname(__file__), "queue"))
INBOX = os.path.join(BRAIN_DIR, "inbox")
OUTBOX = os.path.join(BRAIN_DIR, "outbox")
DONE = os.path.join(BRAIN_DIR, "done")
LEDGER = os.path.join(BRAIN_DIR, "pulse.jsonl")
POLL_SECONDS = 0.25
# How long the server will hold a parked request waiting for a Claude to answer.
# Generous: a human-in-the-loop brain thinks across turns, not milliseconds.
WAIT_TIMEOUT = float(os.environ.get("BRAIN_WAIT_TIMEOUT", "3600"))

for d in (INBOX, OUTBOX, DONE):
    os.makedirs(d, exist_ok=True)

_seq_lock = threading.Lock()
_seq = 0


def next_id():
    global _seq
    with _seq_lock:
        _seq += 1
        return f"{_seq:04d}-{uuid.uuid4().hex[:6]}"


def render_pretty(req):
    """A readable transcript of one request, so the brain can answer at a glance."""
    lines = []
    msgs = req.get("messages", [])
    tools = req.get("tools", [])
    tool_names = [t.get("function", {}).get("name", "?") for t in tools]
    lines.append(f"model: {req.get('model')}   temperature: {req.get('temperature')}")
    lines.append(f"tools available: {', '.join(tool_names) or '(none)'}")
    lines.append("=" * 72)
    for m in msgs:
        role = m.get("role", "?")
        lines.append(f"\n[{role.upper()}]")
        content = m.get("content")
        if content:
            lines.append(content if isinstance(content, str) else json.dumps(content))
        for tc in m.get("tool_calls", []) or []:
            fn = tc.get("function", {})
            lines.append(f"  -> tool_call {fn.get('name')}({fn.get('arguments')})")
    lines.append("\n" + "=" * 72)
    lines.append("ANSWER by writing outbox/<id>.json — one of:")
    lines.append('  {"content": "your reply text"}')
    lines.append('  {"tool_calls": [{"name": "bash", "arguments": {"command": "ls site/"}}]}')
    return "\n".join(lines)


def normalize_message(raw):
    """Turn the brain's terse outbox file into a valid assistant message."""
    content = raw.get("content")
    tool_calls = []
    for i, tc in enumerate(raw.get("tool_calls", []) or []):
        args = tc.get("arguments", {})
        if not isinstance(args, str):
            args = json.dumps(args)
        tool_calls.append({
            "id": tc.get("id") or f"call_{i+1}_{uuid.uuid4().hex[:6]}",
            "type": "function",
            "function": {"name": tc["name"], "arguments": args},
        })
    msg = {"role": "assistant", "content": content}
    if tool_calls:
        msg["tool_calls"] = tool_calls
    return msg


def wait_for_answer(req_id):
    out_path = os.path.join(OUTBOX, f"{req_id}.json")
    deadline = time.time() + WAIT_TIMEOUT
    while time.time() < deadline:
        if os.path.exists(out_path):
            # tiny settle delay so we never read a half-written file
            time.sleep(0.05)
            with open(out_path) as f:
                raw = json.load(f)
            os.replace(out_path, os.path.join(DONE, f"{req_id}.answer.json"))
            return raw
        time.sleep(POLL_SECONDS)
    return None


def append_ledger(entry):
    with open(LEDGER, "a") as f:
        f.write(json.dumps(entry) + "\n")


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass  # quiet; the ledger is our record

    def _json(self, code, obj):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path.startswith("/pulse") or self.path == "/":
            pending = sorted(f for f in os.listdir(INBOX) if f.endswith(".json"))
            self._json(200, {"alive": True, "pending": pending, "brain_dir": BRAIN_DIR})
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        if not self.path.startswith("/v1/chat/completions"):
            self.send_response(404)
            self.end_headers()
            return
        length = int(self.headers.get("Content-Length", 0))
        try:
            req = json.loads(self.rfile.read(length) or b"{}")
        except Exception as e:
            self._json(400, {"error": str(e)})
            return

        req_id = next_id()
        with open(os.path.join(INBOX, f"{req_id}.json"), "w") as f:
            json.dump(req, f, indent=2)
        with open(os.path.join(INBOX, f"{req_id}.txt"), "w") as f:
            f.write(render_pretty(req))
        sys.stderr.write(f"\n>>> brain request parked: {req_id} "
                         f"({len(req.get('messages', []))} msgs) — awaiting Claude\n")
        sys.stderr.flush()

        raw = wait_for_answer(req_id)
        # clear the inbox copies now that it's answered (or timed out)
        for ext in (".json", ".txt"):
            p = os.path.join(INBOX, f"{req_id}.{ext.strip('.')}")
            if os.path.exists(p):
                os.remove(p)

        if raw is None:
            self._json(504, {"error": f"no brain answer within {WAIT_TIMEOUT}s"})
            append_ledger({"id": req_id, "status": "timeout"})
            return

        msg = normalize_message(raw)
        append_ledger({
            "id": req_id,
            "status": "answered",
            "had_tool_calls": "tool_calls" in msg,
            "tools": [tc["function"]["name"] for tc in msg.get("tool_calls", [])],
            "preview": (msg.get("content") or "")[:140],
        })
        self._json(200, {
            "id": f"chatcmpl-{req_id}",
            "object": "chat.completion",
            "model": req.get("model", "claude-brain"),
            "choices": [{"index": 0, "message": msg, "finish_reason": "stop"}],
        })


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8088
    srv = ThreadingHTTPServer(("127.0.0.1", port), Handler)
    sys.stderr.write(f"claude-brain-bridge listening on http://127.0.0.1:{port}\n"
                     f"  brain dir: {BRAIN_DIR}\n"
                     f"  inbox:     {INBOX}\n"
                     f"  outbox:    {OUTBOX}\n")
    sys.stderr.flush()
    srv.serve_forever()


if __name__ == "__main__":
    main()
