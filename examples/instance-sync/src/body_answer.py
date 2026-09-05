
USAGE = """usage: self run answer.peer <key> <text...>

Close an ask that another body of this self addressed here. The answer travels
back on the next sync and drops out of both inboxes.

Answer with what the asking body cannot see for itself: the finding, the number,
the outcome. A transcript of how you got there is not durable knowledge and
costs the next waking its context."""


def main():
    args = sys.argv[1:]
    if len(args) < 2:
        sys.stderr.write(USAGE + "\n")
        return 2
    key, text = args[0], " ".join(args[1:]).strip()
    if not text:
        sys.stderr.write(USAGE + "\n")
        return 2

    asks, answered, me = {}, set(), None
    for e in read_log(sys.stdin):
        nm, p = e.get("name"), e.get("payload") or {}
        if nm == "peer.asked" and p.get("key"):
            asks[p["key"]] = p
        elif nm == "peer.answered" and p.get("key"):
            answered.add(p["key"])
        elif nm == "instance.named" and is_local(e) and p.get("instance"):
            me = p["instance"]

    if key not in asks:
        sys.stderr.write("answer.peer: no ask with key %r — see self view inbox\n" % key)
        return 1
    if key in answered:
        sys.stderr.write("answer.peer: %s is already answered; append a fresh ask rather than "
                         "overwriting the record\n" % key)
        return 1
    a = asks[key]
    if me and a.get("to") and a["to"] != me:
        sys.stderr.write("answer.peer: %s was addressed to %s, not to this body (%s)\n"
                         % (key, a["to"], me))
        return 1
    print(json.dumps({"name": "peer.answered", "payload": {
        "key": key, "from": me or "", "to": a.get("from") or "", "answer": text}}, sort_keys=True))
    return 0


sys.exit(main())
