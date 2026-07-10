# Writing a seed

A seed is how you teach an instance a new set of capabilities. It is not code.
It is a description of intent that the instance's own brain reads and compiles
into scripts locally. The same seed grown on two instances can produce two
different implementations — each adapted to what that instance already has.

If you want to contribute to this project, writing seeds is the most useful
thing you can do. This is the guide.

## What a seed is

A seed is a directory with one required file and two optional pieces:

```
myseed/
  intent.md     required — prose: what this capability set is for
  seed.jsonl    optional — initial content events, laid down once at grow time
  files/        optional — files carried by file.stored events in seed.jsonl
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
   something to render from the first moment. A deposit event named
   `file.stored` with `{"name": "sunset.jpg"}` carries a file: growing copies
   `files/sunset.jpg` from the seed into the instance's content-addressed
   store (`SELF_HOME/files/<sha256>`) and completes the event's payload —
   hash, size, mime — from the bytes themselves. Pin a `sha256` in the event
   if you want it verified; omit it and it is computed for you.

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
- **Files are hashes in events, bytes in the store.** A form's file input (or
  an `@<path>` CLI arg) deposits the file at `SELF_HOME/files/<sha256>`,
  appends a `file.stored` event `{name, mime, size, sha256}`, and hands the
  command the sha256 as that argument's value. A command that needs the bytes
  reads `SELF_HOME/files/<sha256>`; a projection that shows the file links
  `/files/<sha256>/<name>` (an `<img>`, a download link) and never inlines
  bytes into HTML. Describe file-taking commands accordingly: the argument is
  a file, the event field carries its hash. Commands can also **produce**
  files — see the next section.
- **Names may nest.** A projector named `finances/bills` renders to
  `site/finances/bills.html` and serves at `/finances/bills`. Only top-level
  pages appear in the shell's nav; the parent page links down. This is the
  surface's progressive unfolding: a front page that stays small (`finances` —
  the global balance), with depth one link away for whoever wants it — human
  or agent. Commands nest the same way (`finances/add-bill`).
- Any language with a shebang works; use only its standard library.

## What files unlock

The reflex, once files exist, is to treat them as attachments — a photo
stapled to a diary entry. That underuses them. A file argument gives a
command *bytes to compute over*, and the derived-file ingress lets a command
*hand something back*. Four patterns, in rising order of ambition:

- **Attach.** The photo rides the event as a hash; the projection shows it.
  The baseline — an event gains evidence.
- **Extract.** The command reads the deposited bytes once, at deposit time,
  and writes what it learned into the event payload: the filament grams a
  slicer wrote into its G-code comments, the total on a receipt's filename,
  the duration in an audio file's header. The person stops typing what the
  file already knows. Extract into events rather than making projections read
  blobs: the log stays self-describing, and a projection keeps consuming
  events alone.
- **Import.** One file becomes many events. The abandoned spreadsheet, the
  bank's CSV export, the old app's data dump — a command parses the upload
  and emits an event per row. This is how a person's history *before* the
  instance gets in, and it is the single strongest answer to "I already have
  three years of this in a file somewhere."
- **Produce.** A command derives a file and deposits it: write the bytes to a
  scratch path, emit `file.stored {"name": …, "path": …}`, and the kernel
  stores the blob content-addressed, completes the payload from the bytes,
  and verifies before anything is appended. The instance stops being a place
  where records go and starts being a place documents come *from* — the
  accountant's CSV, the numbered invoice, the printable booklet, the
  lab-ready zip of chosen frames. When another event in the same run must
  reference the produced file, hash the bytes in the script (sha256 is
  standard library everywhere), pin it in the `file.stored` payload, and use
  the same hash in the domain event — a pinned hash that does not match the
  bytes refuses the whole run.

Two disciplines keep all four honest. A produced file is *user content*, not
derived state: `rehydrate` will not regenerate it (though re-running the
command will), and it must be backed up with the rest of `files/`. And the
log stays the truth: a produced file is a rendering of events that already
happened, never the only place a fact lives.

## Acting on the world

Commands may have effects; projections never may. That one asymmetry is the
whole rule, and it means a seed can describe capabilities that *do things*:
send the G-code to the printer on the garage LAN, print the zine on the
household printer, email the invoice, message the Monday list through
whatever gateway the user configured. The kernel does not mediate any of
this — a command is a process, and the OS's own tools (`lp`, `curl`,
`sendmail`) are fair game — so the honesty comes from three disciplines your
intent should demand:

- **Every effect leaves a receipt.** The command appends an event for what
  it attempted and what came back — `…sent`, `…failed`, with the response
  worth auditing. The log must always answer "what did this instance do on
  my behalf?"
- **The log is the dedup.** An effectful command reads the log on stdin
  *first* and never repeats what the log already records: the chase email
  not re-sent, the print job not re-queued. Idempotence by replay, no state
  file anywhere.
- **Secrets stay out of the log.** Tokens, SMTP hosts, printer URLs come
  from the environment or a config file outside the instance; event
  payloads carry what happened, never how to authenticate. A log must be
  exportable without leaking the keys to anything.

**Timers** make effects self-starting. One event —
`timer.declared {"name": …, "every": "168h", "command": …, "args": […]}` —
and the serving kernel runs that command on that cadence: `every` is a Go
duration, `"off"` disables, the latest declaration per name wins, and every
firing appends `timer.fired` before the command runs. The pattern to reach
for: the timer supplies the *moment*, the log-reading command decides what
is actually *due*. A weekly timer bound to a command that scans for unpaid
invoices, lapsed clients, or this-week-last-year is a follow-up machine —
and an empty week costs one no-op event. When an intent says "every
Monday", "after thirty days", "when it's been six weeks" — that is a timer
plus a command, and the brain may declare both.

## Sharing content: export and the merge

Capabilities travel by `share`/`adopt`. *Records* travel by
`export`/`grow`: `self export matchday. <dir>` writes a seed directory
holding every `matchday.*` event (original dates preserved), the files
those events reference (hash-verified on arrival), and an `intent.md` stub
the sender edits before sending — who I am, what these events mean, what I
hope grows from them. The receiver grows the directory like any seed, and
this is where it gets interesting: the receiver's brain reads the sender's
intent *against what already lives in this instance* and decides what the
merge should look like. Two supporters' seasons become one head-to-head
page; two researchers' field notes become a timeline where her nesting
dates finally sit next to his temperature series. The insight lives in the
merge projection — which the receiver's own brain writes, for the
receiver's own log, under the receiver's own key.

Intents for shareable seeds should say so: name which events are meant to
travel, and what a good merged view would show a receiver. And remember the
collision rule from the protocol: the sender's event names and field
conventions may not match the receiver's ("2-1" is ours-first on *both*
sides of a derby). Translate in the projection; never rewrite the planted
events.

## Writing a good `intent.md`

The three seeds in `seeds/` are the worked examples. Read them. `journal` is the
smallest; `chat` is a full surface; `renga` shows a seed with no initial content.
`seeds/personas/` holds thirteen more, each written for a person who will never open
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

## Sharing and adopting capabilities

Capability seeds also travel between instances, and only intent and evidence
cross — never runnable code:

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
