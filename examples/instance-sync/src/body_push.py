import os
import shlex
import shutil
import subprocess

USAGE = """usage: self run sync.push <peer> [--full] [--dry-run]

  --full     ignore the cheap short-circuit and diff the digests outright
  --dry-run  say what would be given, on stderr, and give nothing

The mirror of sync.pull, and the only direction that writes to the other body:
it stages an account here, copies the plain text there, and runs learn there.
Nothing runnable travels and no code is installed at the far end — the peer's
own key remains the only key that can sign a script into its cap/."""

SELF_CANDIDATES = ("/usr/local/bin/self", "/opt/homebrew/bin/self",
                   "/home/wouter/go/bin/self", "/Users/wouterbeets/go/bin/self")


def die(msg):
    sys.stderr.write("sync.push: %s\n" % msg)
    sys.exit(1)


def local_self_bin():
    b = os.environ.get("SELF_SYNC_BIN")
    if b and os.access(b, os.X_OK):
        return b
    b = shutil.which("self")
    if b:
        return b
    for c in SELF_CANDIDATES:
        if os.access(c, os.X_OK):
            return c
    die("cannot find the self binary — export SELF_SYNC_BIN=/path/to/self")


def state(events):
    peers, me, marks = {}, None, {}
    for e in events:
        n, p = e.get("name"), e.get("payload") or {}
        if n in ("peer.declared", "peer.updated") and p.get("peer"):
            peers.setdefault(p["peer"], {}).update({k: v for k, v in p.items() if v not in (None, "")})
            peers[p["peer"]]["_live"] = True
        elif n == "peer.retired" and p.get("peer") in peers:
            peers[p["peer"]]["_live"] = False
        elif n == "instance.named" and is_local(e) and p.get("instance"):
            me = p["instance"]
        elif n == "sync.observed" and is_local(e) and p.get("direction") == "push" and p.get("peer"):
            marks[p["peer"]] = p
    if me and me in peers:
        del peers[me]
    return {k: v for k, v in peers.items() if v.get("_live")}, me, marks


def ssh_argv(peer):
    return ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
            "-o", "StrictHostKeyChecking=accept-new", peer["ssh"]]


def run(argv, timeout, what, stdin=None):
    try:
        pr = subprocess.run(argv, input=stdin, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, timeout=timeout)
    except subprocess.TimeoutExpired:
        die("%s timed out after %ds — nothing was given" % (what, timeout))
    except OSError as err:
        die("%s failed: %s" % (what, err))
    if pr.returncode != 0:
        tail = pr.stderr.decode("utf-8", "replace").strip().splitlines()
        die("%s answered %d: %s" % (what, pr.returncode, tail[-1] if tail else "no message"))
    return pr.stdout


def remote_view(peer, argv, timeout, local_bin):
    inner = "SELF_HOME=%s %s view %s" % (
        shlex.quote(peer.get("self_home") or "~/.self"),
        shlex.quote(peer.get("self_bin") or "self"),
        " ".join(shlex.quote(a) for a in argv))
    if (peer.get("transport") or "ssh") == "local":
        env = dict(os.environ, SELF_HOME=peer.get("self_home") or "")
        pr = subprocess.run([peer.get("self_bin") or local_bin, "view"] + argv,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, timeout=timeout)
        if pr.returncode != 0:
            die("reading %s: %s" % (peer.get("peer"), pr.stderr.decode("utf-8", "replace").strip()))
        return pr.stdout.decode("utf-8", "replace")
    return run(ssh_argv(peer) + [inner], timeout, "reading %s" % peer.get("peer")).decode("utf-8", "replace")


def main():
    args = sys.argv[1:]
    full = "--full" in args
    dry = "--dry-run" in args
    args = [a for a in args if not a.startswith("--")]
    if len(args) != 1:
        sys.stderr.write(USAGE + "\n")
        return 2
    name = args[0]

    home = os.environ.get("SELF_HOME")
    if not home:
        die("no SELF_HOME — this is not a command context")
    timeout = int(os.environ.get("SELF_SYNC_TIMEOUT") or "120")
    local_bin = local_self_bin()

    events = list(read_log(sys.stdin))
    peers, me, marks = state(events)
    if not me:
        die("this body has no name, and the door on the far side is named after it — "
            "name it first:  self run peer here <name> role=...")
    if name == me:
        die("%r is this body — a push gives to another one" % name)
    if name not in peers:
        die("no live peer named %r (have: %s)" % (name, ", ".join(sorted(peers)) or "none"))
    peer = dict(peers[name])
    peer["peer"] = name
    mark = marks.get(name) or {}

    prefixes = [p.strip() for p in (peer.get("exclude") or ",".join(DEFAULT_EXCLUDE)).split(",") if p.strip()]
    records, mine = [], []
    for e in events:
        rec = to_record(e, prefixes)
        if rec is None:
            continue
        records.append(rec)
        mine.append(fingerprint(rec["name"], rec["occurred_at"], rec.get("by") or "", rec["payload"]))
    my_sha = hashlib.sha256("".join(f + "\n" for f in sorted(set(mine))).encode("utf-8")).hexdigest()

    peer_sha = remote_view(peer, ["sync.digest", "hash"], timeout, local_bin).strip()
    if not re.match(r"^[0-9a-f]{64}$", peer_sha):
        die("%s did not print a digest hash — is sync.digest installed there?" % name)
    # Neither side has moved since the last give: there is nothing to say.
    if not full and peer_sha == mark.get("peer_digest_sha") and my_sha == mark.get("mine_digest_sha"):
        sys.stderr.write("sync.push: nothing has moved on either body — nothing to give\n")
        return 0

    theirs = set(remote_view(peer, ["sync.digest", "lines"], timeout, local_bin).split())
    lines, hi, hi_key = [], None, None
    for rec, f in zip(records, mine):
        if f in theirs:
            continue
        lines.append(record_line(rec))
        k = tskey(rec["occurred_at"])
        if k is not None and (hi_key is None or k > hi_key):
            hi, hi_key = rec["occurred_at"], k

    sys.stderr.write("sync.push: %s holds %d, this body offers %d, %d to give\n"
                     % (name, len(theirs), len(set(mine)), len(lines)))
    if dry or not lines:
        if not lines and not dry:
            # Record the pair of digests anyway: it is what makes the next tick free.
            print(json.dumps({"name": "sync.observed", "payload": {
                "peer": name, "direction": "push", "given": 0, "held_by_peer": len(theirs),
                "peer_digest_sha": peer_sha, "mine_digest_sha": my_sha}}, sort_keys=True))
        return 0

    stage = os.path.join(home, "sync", "out", name, me)
    if os.path.isdir(stage):
        shutil.rmtree(stage)
    os.makedirs(stage)
    record = "".join(l + "\n" for l in lines).encode("utf-8")
    with open(os.path.join(stage, "record.jsonl"), "wb") as fh:
        fh.write(record)
    sha = hashlib.sha256(record).hexdigest()
    with open(os.path.join(stage, "manifest.json"), "w") as fh:
        json.dump({"events": len(lines), "record_sha256": sha}, fh, indent=2)
        fh.write("\n")
    with open(os.path.join(stage, "intent.md"), "w") as fh:
        fh.write(INTENT % {"me": me, "peer": name, "n": len(lines), "hi": hi or "unknown",
                           "role": (peers.get(name) or {}).get("role") or "the other body"})

    remote_home = peer.get("self_home") or "~/.self"
    if dry:
        return 0
    if (peer.get("transport") or "ssh") == "local":
        dest = os.path.join(remote_home, "sync", "in", me)
        if os.path.isdir(dest):
            shutil.rmtree(dest)
        shutil.copytree(stage, dest)
        env = dict(os.environ, SELF_HOME=remote_home, SELF_CALLER="sync.push:%s" % me)
        pr = subprocess.run([peer.get("self_bin") or local_bin, "learn", dest],
                            stdout=subprocess.DEVNULL, env=env)
        if pr.returncode != 0:
            die("learn refused the account at %s" % dest)
    else:
        tar = run(["tar", "-C", os.path.dirname(stage), "-cf", "-", me], timeout, "packing the account")
        inbox = "%s/sync/in" % remote_home
        run(ssh_argv(peer) + ["mkdir -p %s && tar -xf - -C %s" % (shlex.quote(inbox), shlex.quote(inbox))],
            timeout, "copying to %s" % name, stdin=tar)
        learn = "SELF_HOME=%s SELF_CALLER=%s %s learn %s >/dev/null" % (
            shlex.quote(remote_home), shlex.quote("sync.push:%s" % me),
            shlex.quote(peer.get("self_bin") or "self"), shlex.quote("%s/%s" % (inbox, me)))
        run(ssh_argv(peer) + [learn], timeout, "learning on %s" % name)

    out = {"peer": name, "direction": "push", "given": len(lines), "held_by_peer": len(theirs),
           "peer_digest_sha": peer_sha, "mine_digest_sha": my_sha,
           "account": os.path.join("sync", "out", name, me)}
    if hi:
        out["watermark"] = hi
    print(json.dumps({"name": "sync.observed", "payload": out}, sort_keys=True))
    return 0


INTENT = """# %(me)s -> %(peer)s: one tick of convergence

Not a gift and not a copy: one delta in a continuous sync between two bodies of
the same self. %(n)s moment(s) that %(peer)s did not already hold, given by
%(me)s, which is %(role)s to it.

Reconciled on a fingerprint over (name, occurred_at, by, payload) — the fields
give and learn preserve verbatim. `id` cannot be the key: the kernel mints it
randomly on every append, including on learn, so one moment on two bodies
carries two ids. Everything already held there was dropped before this file was
written, which is why learning it twice is a no-op rather than a duplicate.

The moments and the speakers are kept; the door will read learn:%(me)s, and that
is the receiving body's own fact. A view there that should show only its own
testimony filters on `via`; one that should show both makes the provenance
visible. That choice belongs to the mind at the far end, not to this one.

Newest moment given: %(hi)s

Names carried as lineage.* are kernel vocabulary and v1 history renamed inert,
exactly as `self give` renames them. Reference material — never re-emit them,
and never install from them. A capability worth having on both bodies is read,
tested and authored again under the local key.
"""

sys.exit(main())
