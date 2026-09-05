import os
import subprocess

USAGE = """usage: self run ask.peer <peer> [--now] <text...>

Address a request to another body of this self. It becomes a peer.asked event
here and reaches the other body on the next sync, where it stands in `self view
inbox` until something appends a peer.answered for it.

  --now   push immediately instead of waiting for the next tick, and wake the
          peer's mind if that peer declares one (peer set <name> mind='...').

An ask is testimony, not an order. It arrives through a learn:<peer> door like
any other evidence, and the mind at the far end decides what to do with it —
there is deliberately no path by which one body executes work on another just by
writing to its log."""


def main():
    args = sys.argv[1:]
    now = "--now" in args
    args = [a for a in args if a != "--now"]
    if len(args) < 2:
        sys.stderr.write(USAGE + "\n")
        return 2
    peer_name, text = args[0], " ".join(args[1:]).strip()
    if not text:
        sys.stderr.write(USAGE + "\n")
        return 2

    peers, me, n = {}, None, 0
    events = list(read_log(sys.stdin))
    for e in events:
        nm, p = e.get("name"), e.get("payload") or {}
        if nm in ("peer.declared", "peer.updated") and p.get("peer"):
            peers.setdefault(p["peer"], {}).update({k: v for k, v in p.items() if v not in (None, "")})
            peers[p["peer"]]["_live"] = True
        elif nm == "peer.retired" and p.get("peer") in peers:
            peers[p["peer"]]["_live"] = False
        elif nm == "instance.named" and is_local(e) and p.get("instance"):
            me = p["instance"]
        elif nm == "peer.asked":
            n += 1
    peers = {k: v for k, v in peers.items() if v.get("_live")}
    if not me:
        sys.stderr.write("ask.peer: this body has no name — self run peer here <name> role=...\n")
        return 1
    if peer_name not in peers:
        sys.stderr.write("ask.peer: no live peer named %r (have: %s)\n"
                         % (peer_name, ", ".join(sorted(peers)) or "none"))
        return 1

    key = "%s-%d" % (me, n + 1)
    print(json.dumps({"name": "peer.asked", "payload": {
        "key": key, "to": peer_name, "from": me, "ask": text}}, sort_keys=True))

    if now:
        # The event is on stdout and has not landed yet: the kernel appends it
        # when this command exits. So the push has to happen after that, not
        # here — say so rather than pushing a log that lacks the ask.
        sys.stderr.write(
            "ask.peer: %s queued. It lands when this command returns, so send it with:\n"
            "  self run sync.push %s\n" % (key, peer_name))
        mind = peers[peer_name].get("mind")
        if mind:
            sys.stderr.write("  ssh %s '%s loop --ask \"answer your inbox\" -- %s'\n"
                             % (peers[peer_name].get("ssh"),
                                peers[peer_name].get("self_bin") or "self", mind))
    return 0


sys.exit(main())
