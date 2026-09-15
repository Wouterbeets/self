"""Offline integration tests against the real self binary; no model calls."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest

spec = importlib.util.spec_from_file_location("drive_run", Path(__file__).with_name("run.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class ExperimentTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.seed = self.root / "seed"
        self.seed.mkdir()
        self.binary = os.environ["SELF_TEST_BIN"]
        subprocess.run([self.binary, "hear"], input=b'{"name":"test.seed","payload":{"text":"keep"}}\n',
                       env=dict(os.environ, SELF_HOME=str(self.seed)), check=True, capture_output=True)
        self.original = {p.name: p.read_bytes() for p in self.seed.iterdir() if p.is_file()}
        self.drives = self.root / "drives.json"
        self.drives.write_text(json.dumps([
            {"name": "explore", "instruction": "Find evidence."},
            {"name": "finish", "instruction": "Finish existing work."}]))

    def args(self, script, **changes):
        args = SimpleNamespace(seed=self.seed, out=self.root / "trial", drives=self.drives,
                               passes=3, timeout=5, ask="Test the experiment.",
                               self_bin=self.binary, mind=[sys.executable, "-c", script])
        for key, value in changes.items():
            setattr(args, key, value)
        return args

    def records(self, out):
        return [json.loads(s) for s in (out / "passes.jsonl").read_text().splitlines()]

    def test_rotation_keeps_audit_outside_log_and_seed_untouched(self):
        script = ('import json,sys; p=sys.stdin.read(); '
                  'assert "Your stdout is event JSONL or silence" in p; '
                  'print(json.dumps({"name":"test.observed","payload":{"finish":'
                  '"Perspective for this pass: finish." in p}}))')
        out = runner.experiment(self.args(script))
        records = self.records(out)
        self.assertEqual([r["drive"] for r in records], ["explore", "finish", "explore"])
        self.assertTrue(all(r["status"] == "ok" for r in records))
        events = [json.loads(s) for s in (out / "home/events.jsonl").read_text().splitlines()]
        self.assertEqual([e["name"] for e in events], ["test.seed"] + ["test.observed"] * 3)
        self.assertEqual(self.original, {p.name: p.read_bytes() for p in self.seed.iterdir() if p.is_file()})
        self.assertEqual(records[0]["before"], json.loads((out / "experiment.json").read_text())["seed"])

    def test_signed_capability_is_rebuilt_in_trial(self):
        body = "\n".join(json.dumps(e) for e in [
            {"name": "view.declared", "payload": {"name": "proof", "description": "proof"}},
            {"name": "script.authored", "payload": {"type": "view", "name": "proof",
             "script": "#!/bin/sh\ncat >/dev/null\nprintf 'rebuilt'\n"}}])
        subprocess.run([self.binary, "hear"], input=body.encode(),
                       env=dict(os.environ, SELF_HOME=str(self.seed)), check=True, capture_output=True)
        out = runner.experiment(self.args("pass", passes=1))
        result = subprocess.run([self.binary, "view", "proof"],
                                env=dict(os.environ, SELF_HOME=str(out / "home")),
                                check=True, capture_output=True)
        self.assertEqual(result.stdout, b"rebuilt")

    def test_hear_failure_keeps_partial_commit_and_stops(self):
        script = ('import json; '
                  'print(json.dumps({"name":"test.before_refusal","payload":{}})); '
                  'print(json.dumps({"name":"script.authored","payload":'
                  '{"type":"view","name":"missing","script":"#!/bin/sh"}}))')
        args = self.args(script)
        with self.assertRaisesRegex(RuntimeError, "hear exited"):
            runner.experiment(args)
        records = self.records(args.out)
        self.assertEqual(len(records), 1)
        self.assertEqual(records[0]["status"], "hear")
        self.assertNotEqual(records[0]["before"], records[0]["after"])

    def test_silence_does_not_end_fixed_budget_or_change_log(self):
        out = runner.experiment(self.args("import sys; sys.stdin.read()"))
        records = self.records(out)
        self.assertEqual(len(records), 3)
        self.assertTrue(all(r["before"] == r["after"] for r in records))

    def test_failed_mind_stdout_is_not_ingested(self):
        args = self.args('print(\'{"name":"test.bad","payload":{}}\'); raise SystemExit(7)')
        with self.assertRaisesRegex(RuntimeError, "mind exited 7"):
            runner.experiment(args)
        records = self.records(args.out)
        self.assertEqual(len(records), 1)
        self.assertEqual(records[0]["before"], records[0]["after"])
        self.assertEqual(records[0]["status"], "mind")

    def test_direct_tool_write_is_visible_even_on_failure(self):
        script = ('import subprocess,sys; '
                  'subprocess.run([sys.argv[1],"hear"], '
                  'input=b\'{"name":"test.direct","payload":{}}\\n\', check=True); '
                  'raise SystemExit(7)')
        args = self.args(script)
        args.mind.append(self.binary)
        with self.assertRaises(RuntimeError):
            runner.experiment(args)
        record = self.records(args.out)[0]
        self.assertNotEqual(record["before"], record["after"])

    def test_timeout_is_recorded(self):
        args = self.args("import time; time.sleep(30)", timeout=0.2)
        with self.assertRaisesRegex(RuntimeError, "mind exited 124"):
            runner.experiment(args)
        self.assertEqual(self.records(args.out)[0]["status"], "mind")
        self.assertIn("timed out", (args.out / "pass-001/mind.stderr").read_text())

    def test_existing_output_and_nested_output_are_refused(self):
        args = self.args("pass")
        args.out.mkdir()
        with self.assertRaises(FileExistsError):
            runner.experiment(args)
        args.out = self.seed / "nested"
        with self.assertRaises(ValueError):
            runner.experiment(args)
        self.assertFalse(args.out.exists())

    def test_invalid_drives_do_not_create_output(self):
        self.drives.write_text('[{"name":"missing instruction"}]')
        args = self.args("pass")
        with self.assertRaises(ValueError):
            runner.experiment(args)
        self.assertFalse(args.out.exists())


if __name__ == "__main__":
    unittest.main()
