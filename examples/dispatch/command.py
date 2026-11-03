#!/usr/bin/env python3
"""dispatch: start one goal worker as a visible hc (herdr-core) terminal in its own git worktree.

Replaces the herdr 0.9 dispatcher: no /opt/homebrew/bin/herdr call remains. The worker is
started with `hc run --bg --no-parent --outlive-parent`, labelled herdr.space/herdr.tab so the
Herdr window shows it under its area, and recorded as agent.requested then agent.started
(or agent.failed) with its hc address, worktree, branch and delivery proof."""
import json, os, pwd, re, secrets, shlex, shutil, subprocess, sys, time

# Scripts run on a scrubbed environment: only SELF_* pass through. hc and git need HOME
# and a PATH that reaches ~/.local/bin, Homebrew and the agent binaries.
HOME = os.environ.get("SELF_HERDR_HOME") or pwd.getpwuid(os.getuid()).pw_dir
ENV = dict(os.environ)
ENV["HOME"] = HOME
ENV["PATH"] = os.pathsep.join([HOME + "/.local/bin", HOME + "/go/bin", HOME + "/.opencode/bin", HOME + "/.bun/bin",
                               "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin", ENV.get("PATH", "")])
HC = os.environ.get("SELF_HC_BIN") or shutil.which("hc", path=ENV["PATH"]) or "hc"
APP = os.environ.get("SELF_HERDR_APP_BIN") or shutil.which("herdr-app", path=ENV["PATH"]) or "herdr-app"
INSTANCE = os.environ.get("SELF_HOME") or (HOME + "/.self")
WORKTREES = os.environ.get("SELF_WORKTREES") or (HOME + "/.herdr/worktrees")
AGENT_ARGS_RAW = os.environ.get("SELF_DISPATCH_AGENT_ARGS") or os.environ.get("SELF_HERDR_AGENT_ARGS", "")

USAGE = """usage: dispatch [--area <space>] [--tab <label>] [--pane <doing>] [--no-preflight] <goal> <name> <kind> <repo> [prompt]
       dispatch --auto <goal> [prompt]

  --dry-run  print what would be started (stderr) and start nothing.

  --auto   infer the rest, for one-click starts (the Herdr goal-agents list): kind
           claude; repo and area from the latest dispatch on this goal or its
           nearest ancestor (else the latest dispatch anywhere); name from the goal
           id, numbered past names already used on it; prompt from the goal title.

  <name> becomes the agent's speaker identity and its hc terminal name, so name it
  <kind>-<model>-<task> (e.g. claude-opus-payment-bug-fixer).
  <kind> is the agent program: claude, opencode, pi or codex.

  --area   the Herdr space it shows in (default: the repo's basename, e.g. mym)
           --workspace is accepted as an alias.
  --tab    the tab name (default: the agent name)
  --pane   what it is doing, kept as its hc `doing` label
  Labels are cosmetic: reclaim and observe-agents never key on them."""


def without_prompt(payload):
    slim = dict(payload)
    slim["prompt_chars"] = len(str(slim.pop("prompt", "")))
    return slim


    slim["prompt_chars"] = len(str(slim.pop("prompt", "")))
    return slim


# ---- argv: optional labels anywhere, then the four positionals and the prompt ----
raw = sys.argv[1:]
labels, preflight, positional, auto, dry = {}, True, [], False, False
i = 0
while i < len(raw):
    tok = raw[i]
    if tok in ("--workspace", "--area", "--tab", "--pane"):
        if i + 1 >= len(raw):
            print(USAGE, file=sys.stderr)
            raise SystemExit(2)
        labels[tok[2:]] = raw[i + 1]
        i += 2
        continue
    if tok.startswith(("--workspace=", "--area=")) or tok.startswith("--tab=") or tok.startswith("--pane="):
        key, value = tok[2:].split("=", 1)
        labels[key] = value
        i += 1
        continue
    if tok == "--no-preflight":
        preflight = False
        i += 1
        continue
    if tok == "--dry-run":
        dry = True
        i += 1
        continue
    if tok == "--auto":
        auto = True
        i += 1
        continue
    positional.append(tok)
    i += 1
if len(positional) < (1 if auto else 4):
    print(USAGE, file=sys.stderr)
    raise SystemExit(2)
if auto:
    goal, name, kind, repo = positional[0], "", "claude", ""
    positional = [goal, "", "", ""] + positional[1:]
else:
    goal, name, kind, repo = positional[:4]

# ---- fold the log: the goal must be open, unheld, unblocked ----
events = [json.loads(line) for line in sys.stdin if line.strip()]
def event_key(e):
    stamp = re.match(r"^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?Z$", str(e.get("occurred_at", "")))
    return ("".join(stamp.group(i) for i in range(1, 7)) + (stamp.group(7) or "")[:9].ljust(9, "0") if stamp else "", e.get("name", ""), json.dumps(e.get("payload"), sort_keys=True))
goals, removed, requested = {}, set(), []
for event in sorted(events, key=event_key):
    item = event.get("payload")
    if not isinstance(item, dict):
        continue
    target = item.get("goal")
    if event.get("name") == "goal.created" and isinstance(target, str):
        goals.setdefault(target, dict(item))
    elif event.get("name") == "goal.updated" and target in goals:
        goals[target].update({k: item[k] for k in ("status", "depends_on") if k in item})
    elif event.get("name") == "goal.removed":
        removed.add(target)
    elif event.get("name") == "agent.requested":
        requested.append(item)
def closed(target):
    return target in removed or str(goals.get(target, {}).get("status", "active")).lower() in {"closed", "done", "completed", "cancelled", "removed"}
if goal not in goals or closed(goal):
    sys.exit("dispatch: goal is missing or closed: " + goal)
current, visited = goal, set()
while current in goals and current not in visited:
    visited.add(current)
    if str(goals[current].get("status", "")).lower() == "on_hold":
        sys.exit("dispatch: goal is on hold through " + current)
    current = goals[current].get("parent")
blocked = [g for g in goals[goal].get("depends_on", []) if not closed(g)]
if blocked:
    sys.exit("dispatch: goal is blocked by " + ", ".join(blocked))
if auto:
    lineage, current = [], goal
    while current in goals and current not in lineage:
        lineage.append(current)
        current = goals[current].get("parent")
    near = lambda item: lineage.index(item["goal"]) if item.get("goal") in lineage else len(lineage)
    # Another body's dispatches name paths that do not exist here (/home/wouter/...).
    prior = [item for item in requested if isinstance(item.get("repo"), str) and os.path.isabs(item["repo"])
             and os.path.isdir(item["repo"])]
    if not prior:
        sys.exit("dispatch --auto: no earlier dispatch to take a repo from; dispatch it once by hand")
    best = min(range(len(prior)), key=lambda i: (near(prior[i]), -i))
    repo = prior[best]["repo"]
    if near(prior[best]) < len(lineage) and (prior[best].get("labels") or {}).get("area"):
        labels.setdefault("area", prior[best]["labels"]["area"])
    words = [w for w in re.split(r"[^a-z0-9]+", goal.lower()) if w]
    stem = ""
    for word in reversed(words[-3:]):
        if len(word) + len(stem) + 1 > 24 and stem:
            break
        stem = (word + "-" + stem).strip("-")
    stem = stem[:24] or "agent"
    used = {item.get("agent") for item in requested if item.get("goal") == goal}
    name, n = stem, 1
    while name in used:
        n += 1
        name = "%s-%d" % (stem, n)
    labels.setdefault("tab", " ".join(words[-4:]))
    if len(positional) <= 4:
        positional.append("Move goal %s toward its outcome: %s. Read the outcome and history in self view context first."
                          % (goal, goals[goal].get("title") or goal))
if not os.path.isabs(repo):
    sys.exit("dispatch: repository must be an absolute path")

# ---- names ----
requested_name = name
def slug(value, limit=32):
    value = re.sub(r"[^a-zA-Z0-9_-]+", "-", value).strip("-").lower()
    return value[:limit] or "agent"
def label_text(value, limit=60):
    value = re.sub(r"\s+", " ", str(value)).strip()
    return value[:limit]
name = slug(name)
branch = "goal/" + slug(goal, 48) + "/" + name
cwd = repo

agent_args = shlex.split(AGENT_ARGS_RAW)
def model_id():
    value = ""
    for j, t in enumerate(agent_args):
        if t in ("--model", "-m"):
            if j + 1 >= len(agent_args) or not agent_args[j + 1] or agent_args[j + 1].startswith("-"):
                sys.exit("dispatch: missing model argument")
            value = agent_args[j + 1]
        elif t.startswith(("--model=", "-m=")):
            value = t.split("=", 1)[1]
            if not value:
                sys.exit("dispatch: missing model argument")
    return value
def model_hint(value):
    # "amazon-bedrock/global.openai.gpt-5.6-sol" -> "gpt-5.6-sol"; "opus" -> "opus".
    parts = value.rsplit("/", 1)[-1].split(".")
    while len(parts) > 1 and parts[0].isalpha():   # drop provider prefixes like global.openai.
        parts.pop(0)
    return ".".join(parts)
model_full = model_id()
model = model_hint(model_full)

# Herdr sidebar: the workspace is the stream of work, the tab the goal it falls under,
# the pane what is running. The caller may name all three; these are the fallbacks.
# One area is one Herdr space (as in `self run name --area`); default: the project.
area = slug(labels.get("area") or labels.get("workspace") or os.path.basename(repo.rstrip("/")), 16)
workspace_label = area
tab_label = label_text(labels.get("tab") or name)
pane_label = label_text(labels.get("pane") or " ".join(x for x in (name, kind, model) if x))

# ---- prompt: the exact text delivered is what gets recorded ----
assignment = " ".join(positional[4:]) or "Inspect the goal context and continue the assigned work."
prompt = ("You are %s, one worker in a relay on goal %s; the shared event log is the only memory that outlives you. SELF_CALLER and SELF_HOME are already exported in your shell: leave them.\n"
          "Start with: self view context %s\n"
          "Report with: self run goal progress %s '<change; evidence; blockers; next action>'\n"
          "Use the other self tools freely: self brief lists them, and they compose well with the rest of unix, so pipe them into grep, jq, and friends.\n"
          "Stop at the first natural checkpoint: humans steer only between runs. Work beyond your outcome becomes subgoals you report, not work you do.\n"
          "Worktree: __WORKTREE__ on branch %s of %s.\n\n"
          "Assignment:\n" % (name, goal, goal, goal, branch, repo)) + assignment

base = {"goal": goal, "agent": name, "requested_agent": requested_name, "kind": kind, "model": model_full,
        "repo": repo, "branch": branch,
        "labels": {"area": area, "tab": tab_label, "pane": pane_label},
        "prompt": prompt}
if agent_args:
    base["agent_args"] = AGENT_ARGS_RAW
if dry:
    print(json.dumps(dict(without_prompt(base), assignment=" ".join(positional[4:])), indent=1), file=sys.stderr)
    raise SystemExit(0)


def opencode_binary():
    override = os.environ.get("SELF_OPENCODE_BIN")
    if override:
        return override
    home = ENV["HOME"]
    for path in (home + "/.opencode/bin/opencode", home + "/.local/bin/opencode", "/opt/homebrew/bin/opencode", "/usr/local/bin/opencode"):
        if os.path.isfile(path) and os.access(path, os.X_OK):
            return path
    path = shutil.which("opencode", path=ENV.get("PATH"))
    if not path:
        raise RuntimeError("opencode binary not found; set SELF_OPENCODE_BIN")
    return path


def prepare_opencode_launch(args, directory):
    binary = opencode_binary()
    env = dict(ENV, SELF_CALLER=name, SELF_HOME=INSTANCE)

    def call(*argv):
        proc = subprocess.run([binary, *argv], cwd=directory, env=env, text=True,
                              capture_output=True, timeout=30)
        if proc.returncode:
            raise RuntimeError("opencode launch configuration failed: " + (proc.stderr or proc.stdout).strip()[-1000:])
        return proc.stdout

    help_text = re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", call("--help"))
    if re.search(r"(?m)^\s*(?:-m,\s*)?--model(?:[\s,=]|$)", help_text):
        return list(args), [], {"mode": "native-model-flag", "model": model_full}

    launch, connection, session = [], [], None
    i = 0
    while i < len(args):
        arg = args[i]
        if arg in ("--model", "-m"):
            if i + 1 >= len(args) or args[i + 1].startswith("-"):
                raise RuntimeError("missing OpenCode model argument")
            i += 2
            continue
        if arg.startswith(("--model=", "-m=")):
            i += 1
            continue
        if arg in ("--continue", "-c", "--standalone"):
            raise RuntimeError("OpenCode V2 model dispatch requires a shared-server session; omit " + arg)
        if arg in ("--session", "-s", "--server"):
            if i + 1 >= len(args) or args[i + 1].startswith("-"):
                raise RuntimeError("missing OpenCode argument for " + arg)
            value = args[i + 1]
            launch.extend([arg, value])
            if arg == "--server":
                connection.extend([arg, value])
            else:
                session = value
            i += 2
            continue
        if arg.startswith(("--session=", "--server=")):
            launch.append(arg)
            key, value = arg.split("=", 1)
            if key == "--server":
                connection.extend([key, value])
            else:
                session = value
            i += 1
            continue
        launch.append(arg)
        i += 1

    selector, separator, variant = model_full.partition("#")
    provider, slash, model_name = selector.partition("/")
    if not slash or not provider or not model_name or (separator and not variant):
        raise RuntimeError("OpenCode V2 requires provider/model[#variant], got " + model_full)
    selection = {"providerID": provider, "id": model_name}
    if separator:
        selection["variant"] = variant

    def api(method, path, data=None):
        argv = ["api", *connection, method, path]
        if data is not None:
            argv.extend(["--data", json.dumps(data)])
        output = call(*argv)
        return json.loads(output) if output.strip() else {}

    if session:
        if not re.fullmatch(r"ses_[A-Za-z0-9]+", session):
            raise RuntimeError("invalid OpenCode session id")
        info = api("get", "/api/session/" + session)["data"]
        if os.path.realpath(info.get("location", {}).get("directory", "")) != os.path.realpath(directory):
            raise RuntimeError("OpenCode session belongs to a different directory")
        api("post", "/api/session/" + session + "/model", {"model": selection})
    else:
        info = api("post", "/api/session", {"title": tab_label, "model": selection,
                                           "location": {"directory": directory}})["data"]
        session = info["id"]
        if not re.fullmatch(r"ses_[A-Za-z0-9]+", session):
            raise RuntimeError("OpenCode returned an invalid session id")
        launch.extend(["--session", session])

    base["launch_configuration"] = {"mode": "session-model-api", "session": session, "model": model_full}
    info = api("get", "/api/session/" + session)["data"]


def run(argv, timeout=30, check=True, cwd=None):
    proc = subprocess.run(argv, text=True, capture_output=True, env=ENV, timeout=timeout, cwd=cwd)
    if check and proc.returncode:
        raise RuntimeError("%s failed: %s" % (" ".join(argv[:3]), (proc.stderr or proc.stdout).strip()[-800:]))
    return proc


def hc_json(*argv, timeout=30, check=True):
    proc = run([HC, argv[0], "--json", *argv[1:]], timeout=timeout, check=check)
    try:
        return json.loads(proc.stdout or "{}")
    except ValueError:
        return {}


def screen(term, lines=25):
    try:
        return run([HC, "read", "--lines", str(lines), term], timeout=20, check=False).stdout.strip()[-2000:]
    except Exception as exc:
        return "hc read failed: %s" % exc


def spaces():
    try:
        return [s for s in json.loads(run([APP, "ls"], timeout=20).stdout or "{}").get("spaces", []) if not s.get("machine")]
    except Exception:
        return []


def locate(term):
    for space in spaces():
        for tab in space.get("tabs", []):
            for t in tab.get("terminals", []):
                if t.get("terminal") == term:
                    return space, tab
    return None, None


# ---- roster: refuse a live duplicate name ----
try:
    roster = hc_json("ls", timeout=15).get("terminals")
    if not isinstance(roster, list):
        raise RuntimeError("hc ls returned no terminal roster")
    if any(t.get("name") == name and t.get("state") == "live" for t in roster if isinstance(t, dict)):
        raise RuntimeError("Agent name already exists: " + name + "; choose another name")
    if run([HC, "get", name, "--json"], timeout=15, check=False).returncode == 0:
        raise RuntimeError("an hc terminal named " + name + " already exists; choose another name")
except Exception as exc:
    print("dispatch: " + str(exc).strip(), file=sys.stderr)
    raise SystemExit(1)

stage, term, space_id, tab_id = "worktree", None, None, None
try:
    # ---- worktree: a fresh branch off the remote default branch, outside the repo ----
    cwd = os.path.join(WORKTREES, os.path.basename(repo.rstrip("/")), slug(goal, 48) + "-" + name)
    if not os.path.isdir(cwd):
        run(["git", "-C", repo, "fetch", "-q", "origin"], timeout=120, check=False)
        head = run(["git", "-C", repo, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"], check=False).stdout.strip()
        os.makedirs(os.path.dirname(cwd), exist_ok=True)
        made = run(["git", "-C", repo, "worktree", "add", "-q", "-b", branch, cwd, head or "HEAD"], timeout=120, check=False)
        if made.returncode:
            run(["git", "-C", repo, "worktree", "add", "-q", cwd, branch], timeout=120)
    prompt = prompt.replace("__WORKTREE__", cwd)   # str.replace so a '%' in the assignment cannot break it
    base["prompt"] = prompt
    base["worktree"] = cwd

    launch_args = list(agent_args)
    if kind == "opencode" and model_full:
        stage = "launch configuration"
        launch_args, run_args, launch_config = prepare_opencode_launch(agent_args, cwd)
        base["launch_args"] = launch_args
        base["launch_configuration"] = launch_config

    # An inaccessible model parks the agent silently (seq 5011/5012). One cheap non-interactive
    # call before the TUI exists turns that into agent.failed with the reason.
    stage = "preflight"
    preflight_result = None
    if preflight and kind == "opencode" and model_full:
        env = dict(ENV, SELF_CALLER=name, SELF_HOME=INSTANCE)
        done = subprocess.run([opencode_binary(), "run", *run_args, "--model", model_full, "Reply with the single word ok."],
                              cwd=cwd, env=env, text=True, capture_output=True, timeout=120)
        tail = (done.stdout + done.stderr)[-2000:]
        failed = done.returncode != 0 or re.search(r"(?i)\b(error|denied|forbidden|invalid model|not found)\b", tail)
        preflight_result = {"model": model_full, "exit": done.returncode, "ok": not failed}
        if failed:
            raise RuntimeError("model preflight failed for %s (exit %s): %s" % (model_full, done.returncode, tail[-600:]))

    # ---- the agent argv: the assignment travels as the program's own first prompt ----
    stage = "agent start"
    program = {"opencode": opencode_binary() if kind == "opencode" else "opencode"}.get(kind, kind)
    if kind == "opencode":
        argv = [program, *launch_args, "--prompt", prompt]
    else:
        argv = [program, *launch_args, prompt]
    target = next((s for s in spaces() if str(s.get("name") or "") == area), None)
    space_id = target["id"] if target else "sp" + secrets.token_hex(16)
    tab_id = "tb" + secrets.token_hex(16)
    # Attribution belongs to the process tree: exported before the agent starts, every tool
    # call and subprocess inherits SELF_CALLER/SELF_HOME.
    started = hc_json("run", "--bg", "--no-parent", "--outlive-parent", "--name", name, "--cwd", cwd,
                      "--label", "goal=" + goal, "--label", "self.agent=" + name,
                      "--label", "herdr.space=" + space_id, "--label", "herdr.tab=" + tab_id,
                      "--label", "doing=" + pane_label,
                      "--", "env", "SELF_CALLER=" + name, "SELF_HOME=" + INSTANCE, *argv, timeout=60)
    term = started.get("address") or started.get("terminal") or started.get("id")
    if not term:
        raise RuntimeError("hc run returned no address: " + json.dumps(started)[:500])

    # ---- delivery proof: require observed `working` (seq 5033-5036: TUIs that came up idle) ----
    stage = "prompt"
    status, trusted, deadline = None, False, time.time() + 90
    while time.time() < deadline:
        got = hc_json("get", term, timeout=15, check=False)
        status = got.get("status")
        status = status.get("state") if isinstance(status, dict) else status   # hc get: {app, state}
        if status == "working" or (status in ("idle", "done") and int(got.get("turn") or 0) >= 1):
            break   # a fast turn can start and finish between two polls; turn>=1 proves it ran
        if got.get("state") not in (None, "live"):
            raise RuntimeError("agent exited before working (%s, exit %s): %s" % (got.get("state"), got.get("exit_code"), screen(term)[-600:]))
        shown_now = screen(term, 0)
        if not trusted and re.search(r"(?i)trust this folder|trust the files", shown_now):
            # First visit to a new worktree. The cursor starts on "No, exit": move to "Yes" first.
            keys = ["enter"] if re.search(r"❯\s*(\d\.\s*)?Yes", shown_now) else ["down", "enter"]
            run([HC, "keys", term, *keys], timeout=15, check=False)
            trusted = True
        time.sleep(1)
    if status not in ("working", "idle", "done") or (status != "working" and int(got.get("turn") or 0) < 1):
        raise RuntimeError("prompt_undelivered: no observed working within 90s (status %s); screen: %s" % (status, screen(term)[-600:]))
    delivery = {"attempts": 1, "observed": status, "turn": got.get("turn"), "via": "argv", "trust_accepted": trusted}

    # ---- cosmetic: name the tab (and a new space) in the window; never fails the dispatch ----
    stage = "labels"
    space, tab = None, None
    for _ in range(30):
        space, tab = locate(term)
        if tab:
            break
        time.sleep(0.2)
    if tab:
        run([APP, "do", "tab.rename", "--id", tab["id"], "--name", tab_label], timeout=15, check=False)
        if space and space.get("name") != area:
            run([APP, "do", "space.rename", "--id", space["id"], "--name", area], timeout=15, check=False)

    shown = {"space": (space or {}).get("id") or space_id, "tab": (tab or {}).get("id") or tab_id, "visible": bool(tab)}
    payload = dict(base, via="hc", terminal=term, area=area, herdr=shown, delivery=delivery)
    if preflight_result:
        payload["preflight"] = preflight_result
    print(json.dumps({"name": "agent.requested", "payload": dict(base, via="hc")}))
    print(json.dumps({"name": "agent.started", "payload": without_prompt(payload)}))
except Exception as exc:
    failed = dict(base, via="hc", error=str(getattr(exc, "stderr", None) or exc).strip()[-1500:], stage=stage)
    failed["prompt"] = str(failed.get("prompt", "")).replace("__WORKTREE__", failed.get("worktree", ""))
    if term:
        failed["terminal"] = term
    if space_id:
        failed["herdr"] = {"space": space_id, "tab": tab_id}
    print(json.dumps({"name": "agent.requested", "payload": failed}))
    print(json.dumps({"name": "agent.failed", "payload": without_prompt(failed)}))
