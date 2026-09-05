

USAGE = """usage: self view peers [<name>|--names]

  (bare)   this body, its siblings, and how converged each one is
  <name>   one peer in full, with the lines that reach it
  --names  one live peer name per line and nothing else, so the roster composes
           into argv: for p in $(self view peers --names); do ... done

This self has more than one body. They converge by exchanging accounts, so what
another body knows arrives here as testimony through a learn:<peer> door, never
as code — the local key is still the only key that installs."""


def main():
    args = sys.argv[1:]
    if len(args) > 1:
        sys.stderr.write(USAGE + "\n")
        return 2

    peers, me, mine = {}, None, {}
    obs, doors = {}, {}
    for e in read_log(sys.stdin):
        n, p = e.get("name"), e.get("payload") or {}
        if n in ("peer.declared", "peer.updated") and p.get("peer"):
            peers.setdefault(p["peer"], {}).update(p)
            peers[p["peer"]]["_live"] = True
        elif n == "peer.retired" and p.get("peer") in peers:
            peers[p["peer"]]["_live"] = False
            peers[p["peer"]]["_reason"] = p.get("reason")
        elif n == "instance.named" and p.get("instance"):
            if is_local(e):
                me, mine = p["instance"], p
        elif n == "sync.observed" and is_local(e) and p.get("peer"):
            obs.setdefault(p["peer"], {})[p.get("direction") or "pull"] = p
        via = str(e.get("via") or "")
        if via.startswith("learn:"):
            doors[via[len("learn:"):]] = doors.get(via[len("learn:"):], 0) + 1

    # A synced roster carries the other body's record of THIS one. Naming
    # yourself as your own sibling is how a driver ends up syncing with itself.
    if me and me in peers:
        del peers[me]

    if args and args[0] == "--names":
        for name in sorted(k for k, v in peers.items() if v.get("_live")):
            print(name)
        return 0

    if args:
        name = args[0]
        if name not in peers:
            sys.stderr.write("no peer named %r (have: %s)\n" % (name, ", ".join(sorted(peers)) or "none"))
            return 1
        p = peers[name]
        print("# %s%s" % (name, "" if p.get("_live") else "  (retired: %s)" % (p.get("_reason") or "")))
        print()
        for k in ("role", "os", "transport", "ssh", "self_home", "self_bin", "exclude", "note"):
            if p.get(k):
                print("%-10s %s" % (k, p[k]))
        print()
        o = obs.get(name, {})
        for d in ("pull", "push"):
            if d in o:
                r = o[d]
                print("last %-5s %s event(s) of %s offered, watermark %s"
                      % (d, r.get("received"), r.get("offered"), r.get("watermark") or "-"))
            else:
                print("last %-5s never" % d)
        print()
        if p.get("ssh"):
            print("reach it:   ssh %s" % p["ssh"])
            print("read it:    ssh %s 'SELF_HOME=%s %s brief'"
                  % (p["ssh"], p.get("self_home") or "~/.self", p.get("self_bin") or "self"))
        print("converge:   self run sync.pull %s" % name)
        return 0

    print("# this self has %d %s" % (len(peers) + 1, "body" if not peers else "bodies"))
    print()
    if me:
        print("here    %s — %s" % (me, mine.get("role") or mine.get("description") or "this body"))
    else:
        print("here    (unnamed — name this body: self run peer here <name> role=...)")
    for name in sorted(peers):
        p = peers[name]
        o = obs.get(name, {})
        if not p.get("_live"):
            state = "retired: %s" % (p.get("_reason") or "")
        elif not o:
            # This body may never have driven a sync and still be converged:
            # only one of two bodies needs a route. Its own door says so.
            if doors.get(name):
                state = ("%d moment(s) have arrived through learn:%s — that body drives the sync"
                         % (doors[name], name))
            else:
                state = "never synced — self run sync.pull %s" % name
        else:
            bits = []
            if "pull" in o:
                bits.append("pulled to %s" % (o["pull"].get("watermark") or "?"))
            if "push" in o:
                q = o["push"]
                bits.append("in step" if q.get("mine_digest_sha") == q.get("peer_digest_sha")
                            else "last gave %s" % q.get("given"))
            state = ", ".join(bits)
        print("peer    %s — %s (%s)" % (name, p.get("role") or "?", p.get("os") or "?"))
        print("        %s" % state)
    # A learn: door is not evidence of a body. Most are accounts someone carried
    # here by hand, and calling those siblings would invent family.
    other = sorted(k for k in doors if k not in peers)
    if other:
        print()
        print("also learned, not bodies: %s" % ", ".join("%s (%d)" % (k, doors[k]) for k in other))
    print()
    print(USAGE)
    return 0


sys.exit(main())
