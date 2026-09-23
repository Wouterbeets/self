#!/usr/bin/env python3
"""Optional local adapter. The kernel has no model dependency or scoring policy."""
import argparse
import http.client
from functools import lru_cache
from collections import OrderedDict
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import math
import os
from pathlib import Path
import re
import resource
import shlex
import signal
import subprocess
import tempfile
import time

NAME = re.compile(r"^[a-zA-Z0-9._/-]+$")
QUESTION = {"relevance": {"type": "noul", "instructions":
    "Is the candidate summary directly relevant to the ask? Answer from the ask and candidate only."}}


def words(text):
    stop = set("a an and are as at be been by can do for from has have how i in is it know of on or that the this to want with find show which what who whose their them they our your my me we us its some any all through about into over under when where why would could should please".split())
    return {w[:-1] if len(w) > 3 and w.endswith("s") else w for w in re.findall(r"\w+", text.casefold()) if w not in stop}


def catalog(events):
    caps = {}
    for e in events:
        p = e.get("payload")
        if not isinstance(p, dict) or not isinstance(p.get("name"), str):
            continue
        name = p["name"]
        if not NAME.fullmatch(name) or len(name) > 200 or any(s in ("", ".", "..", "run") or s.startswith(".") or len(s) > 64 for s in name.split("/")):
            continue
        if e["name"] in ("command.declared", "view.declared"):
            key = e["name"].split(".")[0] + "/" + name
            old = caps.get(key, {})
            caps[key] = {"key": key, "decl": p, "seq": e["seq"], "receipt": old.get("receipt", 0)}
        elif e["name"] == "script.installed" and e.get("via") == "kernel":
            key = str(p.get("type")) + "/" + name
            if key in caps:
                caps[key]["receipt"] = e["seq"]
        elif e["name"] == "capability.retired":
            caps.pop(str(p.get("type")) + "/" + name, None)
    return caps


class Scorer:
    def __init__(self, checkpoint, device):
        import laya
        from laya.common import build_sequence
        self.agent = laya.load(checkpoint, device=device)
        self.model = str(Path(checkpoint).resolve())
        self.cache = OrderedDict()
        head, _ = build_sequence(self.agent.tok, "", self.agent._to_internal(QUESTION["relevance"]),
                                 self.agent.cfg.get("max_len", 512), self.agent.cfg.get("head_max_len", 192))
        self.room = self.agent.cfg.get("max_len", 512) - len(head)

    def __call__(self, ask, text):
        key = (ask, text)
        if key in self.cache:
            self.cache.move_to_end(key)
            return self.cache[key]
        tok = self.agent.tok
        task = tok.encode(ask, add_special_tokens=False)
        content = tok.encode(text, add_special_tokens=False)
        # Reserve actual question tokens and JSON framing, not head_max_len.
        budget = self.room - 24
        task_budget = min(96, budget // 2)
        content_budget = budget - min(len(task), task_budget)
        state = {"ask": tok.decode(task[:task_budget]), "candidate": tok.decode(content[:content_budget])}
        while len(tok.encode(json.dumps(state, ensure_ascii=False), add_special_tokens=False)) > self.room:
            content_budget -= 8
            state["candidate"] = tok.decode(content[:max(0, content_budget)])
        answer = self.agent.predict(state, QUESTION)["answers"]["relevance"]
        estimate = float(answer["noul"])
        if not math.isfinite(estimate) or not 0 <= estimate <= 1:
            raise ValueError("invalid relevance estimate")
        value = {"score": estimate, "truncated": len(task) > task_budget or len(content) > content_budget}
        self.cache[key] = value
        if len(self.cache) > 2048:
            self.cache.popitem(last=False)
        return value


class Jev:
    def __init__(self, model):
        self.model, self.key = model, os.environ["TYPESAFE_API_KEY"]

    def predict(self, state, questions):
        conn = http.client.HTTPSConnection("api.typesafe.ai", timeout=5)
        try:
            conn.request("POST", "/v1/systemone", json.dumps(dict(model=self.model, state=state, questions=questions)),
                         {"Authorization": "Bearer " + self.key, "Content-Type": "application/json"})
            response = conn.getresponse()
            if response.status != 200:
                raise OSError(f"Jev HTTP {response.status}")
            return json.loads(response.read(1048576))
        finally:
            conn.close()

    @lru_cache(maxsize=2048)
    def __call__(self, ask, text):
        estimate = float(self.predict({"ask": ask, "candidate": text}, QUESTION)["answers"]["relevance"]["noul"])
        if not math.isfinite(estimate) or not 0 <= estimate <= 1:
            raise ValueError("invalid relevance estimate")
        return {"score": estimate, "truncated": False}


class Ranker:
    def __init__(self, score, render, model="test", max_nodes=96, max_views=4, max_depth=2, seconds=25):
        self.score, self.render, self.model = score, render, model
        self.max_nodes, self.max_views, self.max_depth, self.seconds = max_nodes, max_views, max_depth, seconds

    def rank(self, ask, events):
        caps = catalog(events)
        deadline = time.monotonic() + self.seconds
        scored, evidence, errors, visited = [], [], [], set()
        count = reads = 0

        def assess(items):
            nonlocal count
            result = []
            query = words(ask)
            bags = [words(item["content"]) for item in items]
            weights = {term: math.log(1 + len(items)/(1 + sum(term in bag for bag in bags))) for term in query}
            matches = [sum(weights[t] for t in query & bag) for bag in bags]
            scale = max(matches, default=0) or 1
            for item, match in sorted(zip(items, matches), key=lambda pair: -pair[1]):
                if count >= self.max_nodes or time.monotonic() >= deadline:
                    break
                value = self.score(ask, item["content"])
                count += 1
                result.append(dict(item, **value, match=match/scale, priority=(value["score"] + match/scale)/2))
            return sorted(result, key=lambda x: (-x["priority"], x["source"]))

        for key, c in sorted(caps.items()):
            if key == "view/ask":
                continue
            d = c["decl"]
            scored.append({"source": key, "content": f"{key}: {str(d.get('summary') or d.get('description', ''))[:240]}",
                           "depth": 0, "availability": "installation recorded" if c["receipt"] >= c["seq"] else "pending",
                           "target": [key[5:], []] if key.startswith("view/") and c["receipt"] >= c["seq"] else None})
        scored = assess(scored)

        def expand(node):
            nonlocal reads
            target = node.get("target")
            if not target or node["depth"] >= self.max_depth or reads >= self.max_views or count >= self.max_nodes or time.monotonic() >= deadline:
                return
            name, args = target
            identity = (name, tuple(args))
            if name == "ask" or "view/" + name not in caps or identity in visited:
                return
            visited.add(identity)
            reads += 1
            try:
                body = self.render(name, args, min(3, max(.1, deadline-time.monotonic())))[:32000]
            except (OSError, ValueError, subprocess.SubprocessError) as error:
                errors.append(f"view/{name}: {error}")
                return
            rows = body.splitlines()
            children = []
            navigation = caps["view/" + name]["decl"].get("ask", {})
            if not isinstance(navigation, dict):
                navigation = {}
            for index, row in enumerate(rows):
                if not row.strip() or row.lstrip().startswith("#") or re.fullmatch(r"[\s|:=+-]+", row):
                    continue
                child = None
                if not args and navigation.get("key_column") == 0 and "\t" in row:
                    key = row.split("\t", 1)[0]
                    if NAME.fullmatch(key):
                        child = [name, [key]]
                # Only explicit, concrete view references are executable navigation.
                for ref in re.findall(r"`self view ([^`]+)`", row):
                    try:
                        words = shlex.split(ref)
                    except ValueError:
                        continue
                    if words and all(NAME.fullmatch(w) for w in words):
                        child = [words[0], words[1:]]
                        break
                children.append({"source": f"view/{name} {shlex.join(args)}:{index+1}",
                                 "content": row[:1600], "target": child, "read": [name, args], "depth": node["depth"] + 1})
            # When a view exceeds the node budget, inexpensive term overlap chooses
            # which rows to assess. Omitted rows are not assigned a zero score.
            terms = set(re.findall(r"\w+", ask.casefold()))
            children.sort(key=lambda n: -len(terms & set(re.findall(r"\w+", n["content"].casefold()))))
            children = assess(children[:20])
            evidence.extend(children)
            for child in [c for c in children if c["target"]][:2]:
                expand(child)

        # Preserve room for a second independent view rather than exhaustively
        # following the first branch. Depth and visited identities bound cycles.
        for node in [n for n in scored if n["target"]][:2]:
            expand(node)
        evidence.sort(key=lambda n: (-n["priority"], n["source"]))
        result = {"status": "ranked", "ask": ask, "model": self.model,
                  "head": events[-1]["id"] if events else "empty",
                  "capabilities": scored[:8], "evidence": evidence[:8], "errors": errors,
                  "budget": {"scored": count, "max_nodes": self.max_nodes, "views": reads,
                             "max_views": self.max_views, "max_depth": self.max_depth}}
        lines = [f"# For this ask\n\n{ask}", "Ranking combines model estimates and text matching equally. Scores are uncalibrated; relevance is not permission or proof of capability health."]
        for title, items in [("Capabilities", result["capabilities"]), ("Selected evidence", result["evidence"])]:
            lines.append("\n" + title)
            for index, n in enumerate(items):
                label = "top match" if index < 3 else "possible match"
                excerpt = " ".join(n["content"].split())[:280]
                command = "self brief " + n["source"] if n["depth"] == 0 else "self view " + shlex.join([n["read"][0], *n["read"][1]])
                lines.append(f"- **{label}** · `{command}` · model {n['score']:.2f}, text {n['match']:.2f}\n  {excerpt}" + (" [pending]" if n.get("availability") == "pending" else "") + (" [input clipped]" if n["truncated"] else ""))
        lines.append(f"\nScored {count}/{self.max_nodes} nodes; read {reads}/{self.max_views} views; depth ≤{self.max_depth}. Unscored material remains available via self brief/view.")
        if errors:
            lines.append("\nUnavailable expansions: " + "; ".join(errors)[:500])
        result["text"] = "\n".join(lines)
        return result


def run_read(binary, args, home, timeout):
    """Bound capture before allocation, and kill the entire view group on timeout."""
    with tempfile.TemporaryFile() as output:
        proc = subprocess.Popen([binary, *args], stdout=output, stderr=subprocess.DEVNULL,
                                env=dict(os.environ, SELF_HOME=home), start_new_session=True,
                                preexec_fn=lambda: resource.setrlimit(resource.RLIMIT_FSIZE, (65536, 65536)))
        try:
            proc.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGKILL)
            proc.wait()
            raise
        if proc.returncode:
            raise ValueError(f"read failed or exceeded 64 KiB (exit {proc.returncode})")
        output.seek(0)
        return output.read(65536).decode("utf-8", "replace")


def serve(home, binary, checkpoint, device, port):
    score = Jev(checkpoint) if checkpoint.startswith("jev-") else Scorer(checkpoint, device)

    def snapshot():
        data = (Path(home) / "events.jsonl").read_bytes()
        # A concurrent append may leave an incomplete last line, never a committed record.
        return [json.loads(line) for line in data[:data.rfind(b"\n")+1].splitlines() if line.strip()]

    def render(name, args, timeout):
        start = time.monotonic()
        detail = run_read(binary, ["brief", "view/" + name], home, timeout)
        receipt = re.search(r"declared at seq (\d+) · installed at seq (\d+)", detail)
        if not receipt or int(receipt[1]) > int(receipt[2]):
            raise ValueError("no current installation verified by the kernel")
        return run_read(binary, ["view", name, *args], home, max(.1, timeout-(time.monotonic()-start)))

    ranker = Ranker(score, render, score.model)

    class Handler(BaseHTTPRequestHandler):
        def setup(self):
            super().setup()
            self.connection.settimeout(5)

        def log_message(self, *_):
            pass  # asks and private evidence do not belong in access logs

        def do_GET(self):
            self.reply(200 if self.path == "/health" else 404, {"model": score.model, "status": "ready"})

        def reply(self, status, result):
            body = json.dumps(result, ensure_ascii=False).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            try:
                self.wfile.write(body)
            except (BrokenPipeError, ConnectionResetError):
                pass

        def do_POST(self):
            try:
                size = int(self.headers.get("Content-Length", "0"))
                if self.path != "/ask" or not 0 < size <= 8192:
                    raise ValueError("expected a bounded /ask request")
                request = json.loads(self.rfile.read(size))
                ask = request.get("ask")
                if not isinstance(ask, str) or not 0 < len(ask.strip()) <= 4000:
                    raise ValueError("ask must be 1..4000 characters")
                events = snapshot()
                head = events[-1]["id"] if events else "empty"
                if request.get("head") != head:
                    raise ValueError("instance/head mismatch; use this server's current instance")
                result = ranker.rank(ask, events)
                current = snapshot()
                if (current[-1]["id"] if current else "empty") != head:
                    raise ValueError("log changed during ranking; retry with a current snapshot")
                self.reply(200, result)
            except (OSError, ValueError, KeyError, RuntimeError) as error:
                self.reply(503, {"error": str(error)[:300]})

    print(f"self ask ready: http://127.0.0.1:{port} · {score.model}", flush=True)
    HTTPServer(("127.0.0.1", port), Handler).serve_forever()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--home", default=os.environ.get("SELF_HOME", "."))
    parser.add_argument("--self", dest="binary", required=True)
    parser.add_argument("--checkpoint", required=True, help="local Laya checkpoint or pinned Jev model ID")
    parser.add_argument("--device", default="cuda")
    parser.add_argument("--port", type=int, default=8766)
    args = parser.parse_args()
    os.environ.update(USE_TF="0", HF_HUB_OFFLINE="1", TRANSFORMERS_OFFLINE="1")
    serve(str(Path(args.home).resolve()), str(Path(args.binary).resolve()), args.checkpoint, args.device, args.port)
