import io
import json
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from server import Jev, Ranker, catalog, run_read
import view


def declaration(kind, name, summary="relevant", **extra):
    return {"id": name, "seq": 1, "name": kind + ".declared",
            "payload": dict(name=name, summary=summary, **extra)}


def installed(events):
    out = []
    for event in events:
        out.append(event)
        if event["name"].endswith(".declared"):
            out.append({"name": "script.installed", "id": "receipt", "seq": 2, "via": "kernel",
                        "payload": {"type": event["name"].split(".")[0], "name": event["payload"]["name"]}})
    return out


class RankingTests(unittest.TestCase):
    def score(self, ask, content):
        return {"score": .9 if "relevant" in content else .1, "truncated": False}

    def test_commands_and_recursive_self_are_never_executed(self):
        reads = []
        events = installed([declaration("command", "danger"), declaration("view", "ask"), declaration("view", "notes")])

        def render(name, args, timeout):
            reads.append((name, args))
            return "relevant `self view notes`\nrelevant `self view ask`\n`self run danger`"

        result = Ranker(self.score, render).rank("relevant", events)
        self.assertEqual(reads, [("notes", [])])
        self.assertIn("command/danger", [c["source"] for c in result["capabilities"]])

    def test_explicit_tsv_navigation_and_depth_budget(self):
        events = installed([declaration("view", "practices", ask={"key_column": 0})])
        reads = []

        def render(name, args, timeout):
            reads.append(args)
            return "sample\trelevant guide\n" if not args else "relevant full procedure"

        result = Ranker(self.score, render).rank("relevant", events)
        self.assertEqual(reads, [[], ["sample"]])
        self.assertTrue(any("full procedure" in n["content"] for n in result["evidence"]))
        self.assertLessEqual(result["budget"]["views"], 4)
        reads.clear()
        Ranker(self.score, render, max_depth=1).rank("relevant", events)
        self.assertEqual(reads, [[]])

    def test_node_and_read_limits(self):
        events = installed([declaration("view", "notes"), declaration("view", "other")])
        calls = []

        def score(ask, content):
            calls.append(content)
            return self.score(ask, content)

        result = Ranker(score, lambda *a: "relevant\n" * 1000, max_nodes=5, max_views=1).rank("relevant", events)
        self.assertEqual(len(calls), 5)
        self.assertEqual(result["budget"]["views"], 1)
        self.assertLessEqual(len(result["evidence"]), 8)

    def test_retirement_and_failed_expansion(self):
        events = installed([declaration("view", "removed"), declaration("view", "notes"),
                  {"id": "retirement", "name": "capability.retired", "payload": {"type": "view", "name": "removed"}}])
        self.assertNotIn("view/removed", catalog(events))

        def render(*args):
            raise subprocess.TimeoutExpired("view", 1)

        result = Ranker(self.score, render).rank("relevant", events)
        self.assertEqual(len(result["errors"]), 1)
        self.assertEqual(len(result["capabilities"]), 1)

    def test_expired_budget_does_not_read(self):
        result = Ranker(self.score, lambda *a: self.fail("read after deadline"), seconds=0).rank(
            "relevant", [declaration("view", "notes")])
        self.assertEqual(result["budget"]["views"], 0)

    def test_pending_views_are_labelled_and_not_expanded(self):
        result = Ranker(self.score, lambda *a: self.fail("pending view executed")).rank(
            "relevant", [declaration("view", "pending"), declaration("view", "../invalid")])
        self.assertEqual(len(result["capabilities"]), 1)
        self.assertEqual(result["capabilities"][0]["availability"], "pending")

    def test_terminal_rows_do_not_hide_navigable_rows(self):
        events = installed([declaration("view", "notes"), declaration("view", "detail")])
        reads = []

        def render(name, args, timeout):
            reads.append((name, args))
            return "relevant terminal\nrelevant other terminal\n`self view detail topic`" if name == "notes" else "body"

        Ranker(self.score, render).rank("relevant", events)
        self.assertIn(("detail", ["topic"]), reads)

    def test_headings_are_not_ranked_as_evidence(self):
        events = installed([declaration("view", "notes")])
        result = Ranker(self.score, lambda *a: "# relevant title\n---\nrelevant evidence").rank("relevant", events)
        self.assertEqual([n["content"] for n in result["evidence"]], ["relevant evidence"])

    def test_jev_response_cache_and_http_failure(self):
        with patch.dict("os.environ", TYPESAFE_API_KEY="test"), patch("server.http.client.HTTPSConnection") as connect:
            response = connect.return_value.getresponse.return_value
            response.status, response.read.return_value = 200, b'{"answers":{"relevance":{"noul":0.75}}}'
            scorer = Jev("jev-1.13.0")
            self.assertEqual(scorer("ask", "item")["score"], .75)
            scorer("ask", "item")
            connect.assert_called_once_with("api.typesafe.ai", timeout=5)
            response.status = 401
            with self.assertRaisesRegex(OSError, "401"): scorer("ask", "other")

    def test_capture_is_bounded_before_buffering(self):
        with tempfile.TemporaryDirectory() as home:
            with self.assertRaises(ValueError):
                run_read(sys.executable, ["-c", "print('x' * 100000)"], home, 2)
            with self.assertRaises(subprocess.TimeoutExpired):
                run_read(sys.executable, ["-c", "import time; time.sleep(5)"], home, .05)


class ClientTests(unittest.TestCase):
    def test_unavailable_service_falls_back_to_input_without_writes(self):
        events = [declaration("command", "work"), declaration("view", "notes")]
        wire = "\n".join(json.dumps(e) for e in events)
        out = io.StringIO()
        with patch.object(sys, "argv", ["ask", "--json", "my task"]), patch.object(sys, "stdin", io.StringIO(wire)), \
             patch.object(sys, "stdout", out), patch.object(sys, "stderr", io.StringIO()), \
             patch("urllib.request.build_opener", side_effect=OSError("offline")):
            view.main()
        result = json.loads(out.getvalue())
        self.assertEqual(result["status"], "fallback")
        self.assertIn("command/work", result["text"])
        self.assertIn("view/notes", result["text"])

    def test_nonlocal_endpoint_is_rejected_before_sending(self):
        with patch.object(sys, "argv", ["ask", "task"]), patch.object(sys, "stdin", io.StringIO("")), \
             patch.object(sys, "stdout", io.StringIO()), patch.object(sys, "stderr", io.StringIO()), \
             patch.dict("os.environ", SELF_ASK_URL="https://example.com"), \
             patch("urllib.request.build_opener") as opener:
            view.main()
        opener.assert_not_called()


if __name__ == "__main__":
    unittest.main()
