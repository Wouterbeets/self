

USAGE = """usage: self view sync.digest [lines|hash|names] [--exclude <csv>]

  (bare)  this card: how much of the log can cross, and the digest hash
  hash    one sha256 over the sorted fingerprint set — the cheap "has this
          body changed?" probe a peer runs before pulling anything
  lines   the fingerprint set itself, sorted, one per line (the repair path:
          a full digest diff when a watermark is not trusted)
  names   how many crossable events per wire name
  --exclude csv  name prefixes to leave out; empty string leaves out nothing.
          Default: %s

A fingerprint is sha256 over (wire name, occurred_at, by, canonical payload) —
the four fields that survive give/learn verbatim. Two bodies holding the same
moment print the same fingerprint, which is what lets a delta be computed
without a shared clock or a shared sequence.

The set covers exactly what sync.export would give, so the hash moves only when
this body has something new to SAY — never merely because it wrote down that it
synced.""" % ",".join(DEFAULT_EXCLUDE)


def main():
    args = sys.argv[1:]
    prefixes = list(DEFAULT_EXCLUDE)
    if "--exclude" in args:
        i = args.index("--exclude")
        if i + 1 >= len(args):
            sys.stderr.write(USAGE + "\n")
            return 2
        prefixes = [p.strip() for p in args[i + 1].split(",") if p.strip()]
        args = args[:i] + args[i + 2:]
    mode = args[0] if args else "index"
    if len(args) > 1 or mode not in ("index", "lines", "hash", "names"):
        sys.stderr.write(USAGE + "\n")
        return 2

    fps = set()
    dupes = 0
    names = {}
    total = 0
    stuck = held = 0
    for e in read_log(sys.stdin):
        total += 1
        w, f = shippable(e)
        if f is None:
            stuck += 1
            continue
        if excluded(e.get("name"), w, prefixes):
            held += 1
            continue
        if f in fps:
            dupes += 1
        fps.add(f)
        names[w] = names.get(w, 0) + 1

    body = "".join(f + "\n" for f in sorted(fps))
    digest = hashlib.sha256(body.encode("utf-8")).hexdigest()

    if mode == "lines":
        sys.stdout.write(body)
    elif mode == "hash":
        print(digest)
    elif mode == "names":
        for n in sorted(names, key=lambda k: (-names[k], k)):
            print("%7d  %s" % (names[n], n))
    else:
        print("# sync.digest — %d event(s), %d crossable, %d distinct" % (total, total - stuck - held, len(fps)))
        print()
        print("hash    %s" % digest)
        print("crosses %d distinct moment(s) a peer can diff against" % len(fps))
        if dupes:
            print("dupes   %d event(s) repeat a moment this log already held" % dupes)
        if held:
            print("held    %d event(s) kept back by policy: %s" % (held, ", ".join(prefixes)))
        if stuck:
            print("stuck   %d event(s) can never cross (no valid wire name)" % stuck)
        print()
        print(USAGE)
    return 0


sys.exit(main())
