#!/usr/bin/env python3
"""Run a fixed-budget drive experiment outside the self kernel (Unix)."""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import shutil
import signal
import subprocess
import time


def run(argv, cwd, env, stdin, timeout):
    with subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.PIPE,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          start_new_session=True) as process:
        try:
            stdout, stderr = process.communicate(stdin, timeout=timeout)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            stdout, stderr = process.communicate()
            return 124, stdout, stderr + b"\nexperiment: process timed out\n"
        except BaseException:
            os.killpg(process.pid, signal.SIGKILL)
            process.communicate()
            raise
        return process.returncode, stdout, stderr


def revision(home):
    path = home / "events.jsonl"
    data = path.read_bytes() if path.exists() else b""
    return {"sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data)}


def experiment(args):
    drives = json.loads(args.drives.read_text())
    if not isinstance(drives, list) or not drives or any(
        not isinstance(d, dict) or set(d) != {"name", "instruction"}
        or not all(isinstance(v, str) and v.strip() for v in d.values())
        for d in drives
    ):
        raise ValueError("drives must be a nonempty list of {name, instruction} strings")
    if args.passes < 1 or not math.isfinite(args.timeout) or args.timeout <= 0:
        raise ValueError("passes and timeout must be positive")
    mind = args.mind[1:] if args.mind[:1] == ["--"] else args.mind
    if not mind:
        raise ValueError("pass a mind command after --")
    binary = shutil.which(args.self_bin)
    if binary is None:
        raise ValueError("self executable not found")
    binary = str(Path(binary).resolve())
    seed, out = args.seed.resolve(), args.out.resolve()
    if not seed.is_dir() or seed == out or seed in out.parents:
        raise ValueError("seed must be a directory; output must be outside it")
    if (seed / "events.jsonl").exists() and not (seed / ".secret").is_file():
        raise ValueError("a seed log requires its .secret for faithful replay")
    out.mkdir(mode=0o700, parents=True, exist_ok=False)
    home = out / "home"
    home.mkdir()
    for name in ("events.jsonl", ".secret"):
        if (seed / name).exists():
            shutil.copy2(seed / name, home / name)
    env = dict(os.environ, SELF_HOME=str(home))
    manifest = {"seed": revision(home), "drives": drives, "passes": args.passes,
                "timeout": args.timeout, "ask": args.ask, "mind": mind,
                "self": binary, "self_sha256": hashlib.sha256(Path(binary).read_bytes()).hexdigest()}
    (out / "experiment.json").write_text(json.dumps(manifest, indent=2) + "\n")
    code, stdout, stderr = run([binary, "rehydrate"], home, env, b"", args.timeout)
    (out / "rehydrate.stderr").write_bytes(stderr)
    if code:
        raise RuntimeError("seed could not rehydrate; see rehydrate.stderr")
    for index in range(args.passes):
        drive = drives[index % len(drives)]
        folder = out / f"pass-{index + 1:03d}"
        folder.mkdir()
        ask = (f"{args.ask}\n\nPerspective for this pass: {drive['name']}.\n"
               f"{drive['instruction']}\n"
               "This perspective serves the task. Inspect existing evidence before acting. "
               "Do not invent work to satisfy it. Silence is valid.")
        before = revision(home)
        started = time.monotonic()
        status = "prompt"
        try:
            code, prompt, stderr = run([binary, "prompt", ask], home, env, b"", args.timeout)
            (folder / "prompt.txt").write_bytes(prompt)
            (folder / "prompt.stderr").write_bytes(stderr)
            if code:
                raise RuntimeError("prompt failed")
            status = "mind"
            code, answer, stderr = run(mind, home, env, prompt, args.timeout)
            (folder / "answer.jsonl").write_bytes(answer)
            (folder / "mind.stderr").write_bytes(stderr)
            if code:
                raise RuntimeError(f"mind exited {code}; stdout was not ingested")
            status = "hear"
            code, stdout, stderr = run([binary, "hear"], home, env, answer, args.timeout)
            (folder / "hear.stdout").write_bytes(stdout)
            (folder / "hear.stderr").write_bytes(stderr)
            if code:
                raise RuntimeError(f"hear exited {code}; inspect committed state before continuing")
            status = "ok"
        finally:
            record = {"pass": index + 1, "drive": drive["name"], "status": status,
                      "seconds": time.monotonic() - started,
                      "before": before, "after": revision(home)}
            with (out / "passes.jsonl").open("a") as log:
                log.write(json.dumps(record) + "\n")
    return out


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--seed", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--drives", type=Path, required=True)
    parser.add_argument("--ask", required=True)
    parser.add_argument("--passes", type=int, default=6)
    parser.add_argument("--timeout", type=float, default=1800)
    parser.add_argument("--self-bin", default="self")
    parser.add_argument("mind", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    try:
        print(experiment(args))
    except (OSError, ValueError, RuntimeError) as error:
        parser.exit(1, f"experiment: {error}\n")


if __name__ == "__main__":
    main()
