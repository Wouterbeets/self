import os

USAGE = """usage: self run peer <verb> ...

  peer here <name> [k=v ...]        name THIS body (instance.named)
  peer add <name> [k=v ...]         declare another body of this self
  peer set <name> <k=v> [k=v ...]   revise one, same key, append-only
  peer retire <name> <reason...>    tombstone it: off the roster, history kept

Keys: ssh (user@host), self_home, self_bin, transport (ssh|local), os, role,
      exclude (csv of name prefixes to hold back), note

A peer is a stable named record, so it gets the whole lifecycle: without a
revision and a tombstone path a stale address stays permanently actionable and
a later waking cannot tell the roster from its history."""

KEYS = ("ssh", "self_home", "self_bin", "transport", "os", "role", "exclude", "note",
        "description")


def kv(args):
    out = {}
    for a in args:
        if "=" not in a:
            sys.stderr.write("peer: %r is not key=value\n" % a)
            return None
        k, v = a.split("=", 1)
        if k not in KEYS:
            sys.stderr.write("peer: unknown key %r (known: %s)\n" % (k, ", ".join(KEYS)))
            return None
        out[k] = v
    return out


def live(events):
    peers, me = {}, None
    for e in read_log(events):
        n, p = e.get("name"), e.get("payload") or {}
        if n in ("peer.declared", "peer.updated") and p.get("peer"):
            peers.setdefault(p["peer"], {}).update(p)
            peers[p["peer"]]["_live"] = True
        elif n == "peer.retired" and p.get("peer") in peers:
            peers[p["peer"]]["_live"] = False
        elif n == "instance.named" and is_local(e) and p.get("instance"):
            me = p["instance"]
    return {k: v for k, v in peers.items() if v.get("_live")}, me


def main():
    args = sys.argv[1:]
    if not args or args[0] not in ("here", "add", "set", "retire"):
        sys.stderr.write(USAGE + "\n")
        return 2
    verb, rest = args[0], args[1:]
    if not rest:
        sys.stderr.write(USAGE + "\n")
        return 2
    name, rest = rest[0], rest[1:]
    peers, me = live(sys.stdin)

    if verb == "here":
        fields = kv(rest)
        if fields is None:
            return 2
        fields["instance"] = name
        print(json.dumps({"name": "instance.named", "payload": fields}, sort_keys=True))
        return 0

    if verb == "retire":
        if name not in peers:
            sys.stderr.write("peer: %r is not on the roster\n" % name)
            return 1
        print(json.dumps({"name": "peer.retired", "payload": {
            "peer": name, "reason": " ".join(rest) or "unspecified"}}, sort_keys=True))
        return 0

    fields = kv(rest)
    if fields is None:
        return 2
    if verb == "add" and name in peers:
        sys.stderr.write("peer: %r is already on the roster — revise it with: peer set %s k=v\n" % (name, name))
        return 1
    if verb == "set" and name not in peers:
        sys.stderr.write("peer: %r is not on the roster — declare it with: peer add %s k=v\n" % (name, name))
        return 1
    if verb == "add":
        fields.setdefault("transport", "ssh")
        if fields["transport"] == "ssh" and not fields.get("ssh"):
            sys.stderr.write("peer: an ssh peer needs ssh=user@host\n")
            return 1
    if me and name == me:
        sys.stderr.write("peer: %r is this body's own name — a peer is another body\n" % name)
        return 1
    fields["peer"] = name
    print(json.dumps({"name": "peer.declared" if verb == "add" else "peer.updated",
                      "payload": fields}, sort_keys=True))
    return 0


sys.exit(main())
