# Writing a lesson

A lesson is how you teach an instance a new set of capabilities. It is not
code. It is a description of intent that the instance's own brain reads and
compiles into scripts locally. The same lesson learned by two instances can
produce two different implementations — each adapted to what that instance
already has. Learning, not copying.

If you want to contribute to this project, writing lessons is the most useful
thing you can do. This is the guide.

## What a lesson is

A lesson is the simplest form of an **account** — the one directory format
that moves between instances (see the README's Accounts section for the full
exchange). A hand-written lesson usually carries just the intent:

```
mylesson/
  intent.md      required — prose: what this capability set is for
  record.jsonl   optional — events to plant verbatim at learn time
  manifest.json  optional — an attestation over the record (written by self give)
```

`self learn mylesson/` does the rest:

1. It records an `intent.declared` event.
2. It hands `intent.md` to the brain (a real brain writes real capabilities;
   `examples/brain-stub` is a deterministic offline one that declares a
   minimal command + projection, enough to exercise the loop). The brain reads
   the intent, looks at what the instance already has, and decides how to
   decompose it into **commands** (verbs that emit events) and **projections**
   (HTML views over events).
3. It declares each capability. The kernel compiles each one into a script,
   installs it, and records a signed receipt.
4. If `record.jsonl` is present, its events are planted verbatim — this
   instance's ids, the events' own moments — so the new views have something
   to render from the first moment. A `lesson.learned` receipt closes the
   learn, attesting to what was planted.

## The contract your capabilities must honor

The brain writes the scripts, but they must fit the kernel's pipe contract, so
describe capabilities that can be built this way:

- **A command** receives its arguments as `argv` and the current log as JSONL on
  stdin, and writes new events as JSONL on stdout — one `{name, payload}` object
  per line. The kernel assigns `id`, `seq`, and `occurred_at`.
- **A projection** receives the events matching its declared `consumes` list as
  JSONL on stdin (an empty list — or `"*"` — means every event) and writes HTML
  on stdout. The kernel saves it to `site/<name>.html`. Declare `consumes`
  precisely: the script then never filters, and the kernel re-runs it only when
  events it consumes arrive. A projection is a pure function of its events:
  same events in, same bytes out. Do not read the clock, the network, or
  anything else — determinism is what makes rebuilds reproducible.
- **Names may nest.** A projector named `finances/bills` renders to
  `site/finances/bills.html` and serves at `/finances/bills`. Only top-level
  pages appear in the shell's nav; the parent page links down. This is the
  surface's progressive unfolding: a front page that stays small (`finances` —
  the global balance), with depth one link away for whoever wants it — human
  or agent. Commands nest the same way (`finances/add-bill`).
- Any language with a shebang works; use only its standard library.

## Writing a good `intent.md`

The top-level directories in `lessons/` are worked examples. Read them.
`journal` is the smallest; `chat` is a full surface. A good intent tends to
cover:

- **Purpose** — what this is for, in a sentence or two.
- **Surface** — the exact public names: which commands, which projections, which
  event names, and what arguments each command takes. Fix the names you care
  about; leave the brain free on everything else.
- **Constraints / mechanics** — anything the implementation must get right for
  the idea to work (field names, ordering, how views consume which events).
- **Anti-goals** — what it must *not* do. These are as useful as the goals.
- **What good looks like** — a short end-to-end walkthrough. If you can describe
  the demo, the brain can build toward it.

Two rules of thumb:

- **Fix the public surface, not the implementation.** Name the commands and
  views a user will type and click; let the instance choose how to realize them.
- **Write each capability's description so it stands alone.** A capability may be
  compiled in isolation, so its description should name its sibling capabilities
  and the events they share, enough that building just that one piece still
  serves the whole intent.

## Accounts that travel

When a lesson comes from another instance's life — written by `self give`
rather than by hand — the same rules apply plus three the kernel enforces:
planted events keep their own `occurred_at`; the kernel's lifecycle
vocabulary (`command.declared`, `script.compiled`, …) is refused in a record
and travels only renamed as `lineage.*` events, which land inert; and the
`lesson.learned` receipt records the sha256 of what was actually planted
beside what the manifest claimed, so curating an account before learning it
is visible in the log forever. Read an account before you learn it — the
intent tells you what the giver hopes for; the record is theirs, verbatim.
