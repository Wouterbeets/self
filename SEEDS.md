# Writing a seed

A seed is how you teach an instance a new set of capabilities. It is not code.
It is a description of intent that the instance's own brain reads and compiles
into scripts locally. The same seed grown on two instances can produce two
different implementations — each adapted to what that instance already has.

If you want to contribute to this project, writing seeds is the most useful
thing you can do. This is the guide.

## What a seed is

A seed is a directory with one required file and one optional file:

```
myseed/
  intent.md     required — prose: what this capability set is for
  seed.jsonl    optional — initial content events, laid down once at grow time
```

`self grow myseed/` does the rest:

1. It records an `intent.declared` event.
2. It hands `intent.md` to the brain (a real brain writes real capabilities;
   `examples/brain-stub` is a deterministic offline one that declares a
   minimal command + projection, enough to exercise the loop). The brain reads
   the intent, looks at what the instance already has, and decides how to
   decompose it into **commands** (verbs that emit events) and **projections**
   (HTML views over events).
3. It declares each capability. The kernel compiles each one into a script,
   installs it, and records a signed receipt.
4. If `seed.jsonl` is present, its events are appended so the new views have
   something to render from the first moment.

## The contract your capabilities must honor

The brain writes the scripts, but they must fit the kernel's pipe contract, so
describe capabilities that can be built this way:

- **A command** receives its arguments as `argv` and the current log as JSONL on
  stdin, and writes new events as JSONL on stdout — one `{name, payload}` object
  per line. The kernel assigns `id`, `seq`, and `occurred_at`.
- **A projection** receives all events as JSONL on stdin and writes HTML on
  stdout. The kernel saves it to `site/<name>.html`. A projection is a pure
  function of the log: same log in, same bytes out. Do not read the clock, the
  network, or anything else — determinism is what makes rebuilds reproducible.
- **Names may nest.** A projector named `finances/bills` renders to
  `site/finances/bills.html` and serves at `/finances/bills`. Only top-level
  pages appear in the shell's nav; the parent page links down. This is the
  surface's progressive unfolding: a front page that stays small (`finances` —
  the global balance), with depth one link away for whoever wants it — human
  or agent. Commands nest the same way (`finances/add-bill`).
- Any language with a shebang works; use only its standard library.

## Writing a good `intent.md`

The three seeds in `seeds/` are the worked examples. Read them. `journal` is the
smallest; `chat` is a full surface; `renga` shows a seed with no initial content.
`seeds/personas/` holds ten more, each written for a person who will never open
a terminal — a feel for what intents look like outside this repo's own walls.
A good intent tends to cover:

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

## Sharing and adopting

Seeds also travel between instances, and only intent and evidence cross — never
runnable code:

- `self share <capability>` prints a slice of your log: every declaration of that
  capability and every receipt your kernel signed for it, as JSONL.
- `self adopt <file>` (or `-` for stdin) takes such a slice, records it, and
  re-declares the capability. Your instance's own brain re-authors the script and
  signs it with your key. The sender's script rides along only as a reference for
  the compiler to check against — it is never installed as-is.

Because the wire format is the log's own format, a seed you share is inspectable
plain text. Adopting one still runs the result on your machine, so treat a seed
from someone else the way you would treat any code you are about to run: read it
first.
