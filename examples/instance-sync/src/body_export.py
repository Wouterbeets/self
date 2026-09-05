
USAGE = """usage: self view sync.export [all | since <rfc3339>] [--exclude <csv>]

  (bare)         this card: what would cross, over what span
  all            every crossable event, as account record.jsonl lines
  since <ts>     only events at or after that occurred_at (inclusive, so a
                 moment on the boundary is offered again rather than lost)
  --exclude csv  name prefixes to hold back; empty string means hold back
                 nothing. Default: %s

Lines carry exactly the four fields a deposit keeps — name, occurred_at, by,
payload — because learn reads only those and discards id, seq and via. The
output IS an account's record.jsonl: a peer pipes it into a directory, drops the
moments it already holds, and learns the rest.""" % ",".join(DEFAULT_EXCLUDE)


def main():
    args = sys.argv[1:]
    prefixes = list(DEFAULT_EXCLUDE)
    if "--exclude" in args:
        i = args.index("--exclude")
        if i + 1 >= len(args):
            sys.stderr.write(USAGE + "\n")
            return 2
        raw = args[i + 1]
        prefixes = [p.strip() for p in raw.split(",") if p.strip()]
        args = args[:i] + args[i + 2:]

    mode, since = "index", None
    if len(args) == 1 and args[0] == "all":
        mode = "all"
    elif len(args) == 2 and args[0] == "since":
        mode, since = "since", tskey(args[1])
        if since is None:
            sys.stderr.write("sync.export: %r is not an RFC3339 UTC moment like 2026-09-04T07:27:17.172449Z\n" % args[1])
            return 2
    elif args:
        sys.stderr.write(USAGE + "\n")
        return 2

    out = []
    total = held = 0
    lo = hi = None
    for e in read_log(sys.stdin):
        total += 1
        oa = e.get("occurred_at")
        rec = to_record(e, prefixes)
        if rec is None:
            if wire_name(e.get("name")) is not None:
                held += 1
            continue
        k = tskey(oa)
        if mode == "since" and k is not None and k < since:
            continue
        if k is not None:
            lo = k if lo is None or k < lo else lo
            hi = k if hi is None or k > hi else hi
        out.append(rec)

    if mode == "index":
        print("# sync.export — %d of %d event(s) would cross" % (len(out), total))
        print()
        if held:
            print("holding back %d event(s) under: %s" % (held, ", ".join(prefixes) or "(nothing)"))
        if lo:
            print("span         %s … %s" % (lo, hi))
        print()
        print(USAGE)
        return 0

    for rec in out:
        sys.stdout.write(record_line(rec) + "\n")
    return 0


sys.exit(main())
