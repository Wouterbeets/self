import os
import shlex
import shutil
import subprocess

USAGE = """usage: self run sync.pull <peer> [--full] [--dry-run]

  --full     ignore the watermark and ask for the peer's whole crossable log.
             The repair path, and the right shape for a first sync.
  --dry-run  say what would land, on stderr, and append nothing.

Reads the peer over its declared transport and appends here. The peer is never
written to: a pull is `self view` at the far end, so the read projects and only
this body's log grows. Set SELF_SYNC_BIN if `self` is not on a scrubbed PATH."""

SELF_CANDIDATES = ("/usr/local/bin/self", "/opt/homebrew/bin/self",
                   "/home/wouter/go/bin/self", "/Users/wouterbeets/go/bin/self")


def die(msg):
    sys.stderr.write("sync.pull: %s\n" % msg)
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
    die("cannot find the self binary — a capability runs on a scrubbed PATH, so "
        "export SELF_SYNC_BIN=/path/to/self (it is passed through to scripts)")


def peers_and_marks(events):
    """Live peer records, this body's name, and its own last observation of each."""
    peers, marks, me = {}, {}, None
    for e in events:
        n, p = e.get("name"), e.get("payload") or {}
        if n in ("peer.declared", "peer.updated"):
            key = p.get("peer")
            if key:
                rec = peers.setdefault(key, {})
                rec.update({k: v for k, v in p.items() if v not in (None, "")})
                rec["_live"] = True
        elif n == "peer.retired":
            if p.get("peer") in peers:
                peers[p["peer"]]["_live"] = False
        elif n == "instance.named" and is_local(e) and p.get("instance"):
            me = p["instance"]
        elif n == "sync.observed" and is_local(e):
            # Only this body's own observations: a peer's sync.observed about
            # its own runs arrives through the door like any other testimony.
            if p.get("peer") and p.get("direction") == "pull":
                marks[p["peer"]] = p
    if me and me in peers:
        del peers[me]
    return {k: v for k, v in peers.items() if v.get("_live")}, marks, me


def remote(peer, argv, timeout):
    """Run a `self view` on the peer. A view appends nothing there, ever."""
    inner = "SELF_HOME=%s %s view %s" % (
        shlex.quote(peer.get("self_home") or "~/.self"),
        shlex.quote(peer.get("self_bin") or "self"),
        " ".join(shlex.quote(a) for a in argv))
    if (peer.get("transport") or "ssh") == "local":
        cmd = [peer.get("self_bin") or "self", "view"] + argv
        env = dict(os.environ, SELF_HOME=peer.get("self_home") or "")
    else:
        target = peer.get("ssh")
        if not target:
            die("peer %r has transport ssh but no ssh target" % peer.get("peer"))
        cmd = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
               "-o", "StrictHostKeyChecking=accept-new", target, inner]
        env = dict(os.environ)
    try:
        pr = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            env=env, timeout=timeout)
    except subprocess.TimeoutExpired:
        die("%s did not answer within %ds — nothing was appended" % (peer.get("peer"), timeout))
    except OSError as err:
        die("cannot reach %s: %s" % (peer.get("peer"), err))
    if pr.returncode != 0:
        tail = pr.stderr.decode("utf-8", "replace").strip().splitlines()
        die("%s answered %d: %s (nothing was appended)"
            % (peer.get("peer"), pr.returncode, tail[-1] if tail else "no message"))
    return pr.stdout.decode("utf-8", "replace")


def main():
    args = [a for a in sys.argv[1:]]
    full = "--full" in args
    dry = "--dry-run" in args
    args = [a for a in args if not a.startswith("--")]
    if len(args) != 1:
        sys.stderr.write(USAGE + "\n")
        return 2
    name = args[0]

    home = os.environ.get("SELF_HOME")
    if not home:
        die("no SELF_HOME — a command is given one; this is not a command context")
    timeout = int(os.environ.get("SELF_SYNC_TIMEOUT") or "120")

    events = list(read_log(sys.stdin))
    peers, marks, me = peers_and_marks(events)
    if me and name == me:
        die("%r is this body — a pull converges it with another one" % name)
    if name not in peers:
        die("no live peer named %r — declare one: self run peer add %s ... (have: %s)"
            % (name, name, ", ".join(sorted(peers)) or "none"))
    peer = peers[name]
    peer["peer"] = name
    mark = marks.get(name) or {}

    # What this body already holds, in the peer's own key space.
    mine = set()
    for e in events:
        _, f = shippable(e)
        if f is not None:
            mine.add(f)

    # 1. The cheap probe. A peer whose digest has not moved has nothing to say,
    #    and an unchanged log is the overwhelmingly common tick.
    peer_sha = remote(peer, ["sync.digest", "hash"], timeout).strip()
    if not re.match(r"^[0-9a-f]{64}$", peer_sha):
        die("%s did not print a digest hash — is sync.digest installed there?" % name)
    if not full and peer_sha == mark.get("peer_digest_sha"):
        sys.stderr.write("sync.pull: %s is unchanged (%s…) — nothing to do\n" % (name, peer_sha[:12]))
        return 0

    # 2. The delta. The watermark is an optimisation over a two-body set: a
    #    moment older than it that this log lacks cannot exist, because it would
    #    have had to arrive in an earlier run. --full drops that assumption.
    since = None if full else mark.get("watermark")
    argv = ["sync.export", "all"] if since is None else ["sync.export", "since", since]
    if peer.get("exclude") is not None:
        argv += ["--exclude", peer["exclude"]]
    body = remote(peer, argv, timeout)

    offered = kept = 0
    unshippable = 0
    lines = []
    hi, hi_key = mark.get("watermark"), tskey(mark.get("watermark") or "")
    for line in body.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            rec = json.loads(line)
        except ValueError:
            die("%s printed a record line that is not JSON — nothing was appended" % name)
        offered += 1
        nm, oa = rec.get("name"), rec.get("occurred_at")
        # Defensive: an older export view on the peer could offer a name learn
        # refuses, and learn refuses the WHOLE account for one bad line.
        if not isinstance(nm, str) or nm in REFUSED or not NAME_RE.match(nm) or not isinstance(oa, str):
            unshippable += 1
            continue
        k = tskey(oa)
        if k is not None and (hi_key is None or k > hi_key):
            hi, hi_key = oa, k
        if fingerprint(nm, oa, rec.get("by") or "", rec.get("payload", {})) in mine:
            continue
        kept += 1
        lines.append(json.dumps(rec, sort_keys=True, separators=(",", ":"), ensure_ascii=True))

    sys.stderr.write("sync.pull: %s offered %d, %d new%s\n"
                     % (name, offered, kept,
                        (", %d unshippable" % unshippable) if unshippable else ""))
    if dry:
        return 0

    account = os.path.join(home, "sync", "in", name)
    if kept:
        # The directory basename becomes the door: learn:<peer>.
        if os.path.isdir(account):
            shutil.rmtree(account)
        os.makedirs(account)
        record = "".join(l + "\n" for l in lines).encode("utf-8")
        with open(os.path.join(account, "record.jsonl"), "wb") as fh:
            fh.write(record)
        sha = hashlib.sha256(record).hexdigest()
        with open(os.path.join(account, "manifest.json"), "w") as fh:
            json.dump({"events": kept, "record_sha256": sha}, fh, indent=2)
            fh.write("\n")
        with open(os.path.join(account, "intent.md"), "w") as fh:
            fh.write(INTENT % {
                "peer": name, "n": kept, "offered": offered,
                "role": peer.get("role") or "a peer", "os": peer.get("os") or "unknown",
                "where": peer.get("ssh") or peer.get("self_home") or "local",
                "since": since or "the beginning", "hi": hi or "unknown"})
        env = dict(os.environ, SELF_HOME=home, SELF_CALLER="sync.pull:%s" % name)
        pr = subprocess.run([local_self_bin(), "learn", account],
                            stdout=subprocess.DEVNULL, env=env)
        if pr.returncode != 0:
            die("learn refused the account at %s — nothing was appended" % account)

    # A run that changed nothing and saw nothing new is not news. Staying quiet
    # keeps a 15-minute timer from writing a diary of its own uneventfulness.
    if not kept and not unshippable and peer_sha == mark.get("peer_digest_sha"):
        return 0

    out = {"peer": name, "direction": "pull", "offered": offered, "received": kept,
           "skipped_known": offered - kept - unshippable, "peer_digest_sha": peer_sha,
           "since": since or "all", "transport": peer.get("transport") or "ssh"}
    if unshippable:
        out["unshippable"] = unshippable
    if hi:
        out["watermark"] = hi
    if kept:
        out["account"] = os.path.join("sync", "in", name)
    print(json.dumps({"name": "sync.observed", "payload": out}, sort_keys=True))

    if kept > 20:
        sys.stderr.write(
            "sync.pull: %d moments landed. The mechanical half is done; for the\n"
            "  intelligent half — capabilities that read this evidence HERE, authored\n"
            "  and signed under this key — run:  self learn %s | claude -p | self hear\n"
            % (kept, account))
    return 0


INTENT = """# %(peer)s -> this body: one tick of convergence

Not a gift and not a copy: one delta in a continuous sync between two bodies of
the same self. %(n)s moment(s) of the %(offered)s offered, the rest already held
here under a different `id` and `seq`.

The peer is %(peer)s (%(role)s, %(os)s), reachable at %(where)s. It was read,
not written: a pull is `self view` at the far end.

Reconciled on a fingerprint over (name, occurred_at, by, payload) — the fields
give and learn preserve verbatim. `id` cannot be the key: the kernel mints it
randomly on every append, including on learn, so the same moment on two bodies
carries two ids. Anything already held was dropped before this file was written,
which is why re-learning is a no-op rather than a duplicate.

Their moments and their speakers are theirs and travel untouched. The door reads
learn:%(peer)s, and that is this log's own fact: a view that should show only
what this body witnessed filters on `via`; one that should show both makes the
provenance visible. Deciding which is this mind's job.

Requested everything at or after: %(since)s
Newest moment in this delta:      %(hi)s

Names arriving as lineage.* are the peer's kernel vocabulary and its v1 history,
renamed inert exactly as `self give` renames it. They are reference material —
never re-emit them, and never install from them. If a capability there is worth
having here, read it, test it, and author it under this body's own key.
"""

sys.exit(main())
