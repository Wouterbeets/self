# self — the protocol

The contract. `self help` prints it verbatim; prompts splice the marked layers.
Nothing else restates it.

## The rule

**Reads project. Writes append. Printing the prompt is a read.**

`hear`, successful commands, and account operations share one internal ingestion
path: commit the batch under the log lock, apply kernel event handling, then
report. Their input contracts remain distinct: `hear` accepts event wire and
authored scripts; commands must succeed, and every event they print must be
valid, before any is committed; accounts validate foreign vocabulary before
depositing anything. Each entry point assigns provenance. `run` records
automatically; no shell pipe is needed. Its stdout is what the command said to
its caller, then the committed-event summary.

The log carries knowledge, desired outcomes, and installed capabilities between
minds. Views project it; commands append to it. External actions and artifacts
need recorded evidence or references to remain discoverable.

## The dispatcher

```
self                 reconnect: session guidance + brief.                         (READ)
self setup           what to offer so every session starts with self.              (READ)
self prompt [ask…]    print an execution prompt: brief + ask.                       (READ)
… | self hear        event lines land, authored scripts install.                   (WRITE)
```

Bare `self` supports an agent checking in before its existing task. Every
prompt opens with the same voice, the core layer: the instance belongs to the
mind reading it, which owns and maintains it, and three things a session works
out are worth keeping in it. Bare `self` adds one layer for a mind in
conversation, a checkpoint before each answer, and no pass assignment, stdout
contract, or authoring tutorial; `self make`, run bare, is that tutorial.
Pending capabilities and open outcomes remain in the brief. An empty log is the
same brief with nothing in it, plus the first layer: begin by building a
memory. The kernel declares nothing of its own in a new instance; whatever is
there, a mind made. `self --help` opens with the core layer too, because a
careful mind asks an unknown binary what it is before it runs it. `self setup`
prints what a mind may offer so every session starts with self.
`self prompt` prepares a mind whose output will be piped into `self hear`.
Neither mode reads stdin or writes state.

An ask arrives as argv; a mind's answer arrives on stdin at `self hear`. The
read side never touches stdin, so it cannot block at the head of a pipeline.

A line is an event iff it is a JSON object with a dotted lowercase `name` **and**
a `payload` key. Every other line is ignored, echoed, and counted on stderr.
`hear` ingests any event-shaped line, including one quoted in a document: do not
pipe prose about events (this file, say) into it.

Identical in a terminal, a pipe, a script, a sandbox and cron.

<!-- prompt:core:begin -->
This is you: what you kept on this machine in earlier sessions, and all you will know when you next arrive. Its design is yours; the person's files stay theirs.

Keep what a later you would need: a command for what they will want again, a fact for what you found out, an intent for what you cannot do yet. Keep it light. What is here should earn its place: rework what is badly made, retire what no longer serves. Removing is as much your work as adding.

`self brief` shows what is here and what each is for. `self view <name>` reads, `self run <name>` acts, `self make` adds or replaces; run it bare to see how.
<!-- prompt:core:end -->

<!-- prompt:session:begin -->
Do what the person asked. Before each answer, keep what a later you would need from this one, and say in a line what you kept or removed.
<!-- prompt:session:end -->

<!-- prompt:first:begin -->
Nothing is here yet: this is your first session. Begin by giving yourself a memory, a command that keeps a fact and a view that shows the facts on arrival; `self make` shows one way, and how you build it is up to you. If the person came with nothing in mind, ask what they do on this computer again and again. If self is not part of every session on this machine yet, `self setup` says what to offer them.
<!-- prompt:first:end -->

<!-- prompt:setup:begin -->
Nothing needs configuring for self to work in this session. To make it part of every session on this machine, offer the person these, and change their files only once they agree:

- These lines in the instruction file your harness reads for every project (`AGENTS.md`, `CLAUDE.md`, or a system prompt):

  ```
  Run `self` before anything else, with no arguments: it prints your own memory and tools on this machine. I installed it for you, and it is yours to maintain.
  Before you answer me, save in it whatever a later session would need, however small, so nothing has to be found out twice.
  ```

- Where your harness has a session-start hook, have it run `self`, so the brief arrives before the first message and again after the context is compacted.
- If `self` ran by full path, put its directory on `PATH` for new shells.

No environment variable is needed. `SELF_HOME` only moves the instance somewhere else.
<!-- prompt:setup:end -->

<!-- prompt:execution:begin -->
Make one pass. Your stdout is event JSONL or silence: one object per line with only `name` (lowercase dotted) and `payload`. The kernel adds identity, time, and provenance. Nothing else you say is kept.
<!-- prompt:execution:end -->

## The wire

For `self prompt` and `self learn`, a mind's stdout is
**event JSONL, or silence.** Durable work happens through
`self run <command>` or the events you print. Words for a human are a domain
event some view renders, never kernel vocabulary. Empty stdout is a valid turn.

```json
{"name":"journal.entry","payload":{"text":"watered the plants"}}
```

You set `name` and `payload` only. The kernel assigns `id`, `seq`,
`occurred_at`, `via`, `by`. Imported records may also carry `origin`, a foreign
identity claim preserved across hops; it grants no authority. Name: `^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`.

### Conditional ingestion and waiting

The brief prints `head: <event-id>` (`empty` for a new log).
`self hear --after <head>` commits only if that head still matches under the
append lock. A mismatch commits nothing: reread and reconsider before retrying.
This protects a decision and its event batch, not external side effects. It is
not a lease, sandbox, or permission to retry a command that already acted.

`self watch [--after <id|empty>] [--timeout 10m] [--follow] [event-prefix]`
waits for and prints the first matching batch as full event JSONL, then exits.
Without `--after`, it starts at the current head; `empty` includes existing events.
Use the last returned event ID to resume. A missing cursor or timeout is an
error. `--follow` keeps printing matching batches; its timeout then ends the
stream successfully. Reads append nothing, including on timeout. Filters match
name prefixes; domain filtering belongs in consumers. An ID cursor detects a lost anchor,
not edits to arbitrary earlier records.

## Desired outcomes

<!-- prompt:intent:begin -->
Declare what you want to become possible:

```json
{"name":"intent.declared","payload":{"name":"check-fit","summary":"Check whether the replacement fits","description":"Desired outcome, context or references, and evidence needed to resolve it."}}
```

Read details with `self brief intent/<name>`; `self brief intent/` lists open outcomes.
The result may be a finding, decision, artifact, or capability. Act within the
user's scope. Redeclare the same name to revise or reopen it.

Close with `intent.closed` and `{name, outcome, reason}`. Use `completed` with
evidence and result references, or `dropped` with a reason. Completion records
a judgment, not kernel proof. Keep findings in domain events. When input is
missing, record what is needed and leave the outcome open; avoid unchanged
appends. Pending capabilities already carry their own construction intent.
<!-- prompt:intent:end -->

An intent name is 1–200 ASCII letters, digits, dots, underscores, hyphens, or
slashes. A nonempty description is required. Invalid declarations and closures
remain in the log but do not change the intent index. A closure needs an existing
name, a recognized outcome, and a nonempty reason. Replay uses local append
order; redeclaring replaces the description and reopens the item. This is not
a distributed ownership or conflict-resolution protocol.

Domain backlogs stay in their own views. A domain command can declare the next
useful contribution to an active goal, meal plan, or investigation, and withdraw
it when no longer relevant. Closing that contribution need not close the larger
domain record. Avoid mirroring whole backlogs into the brief.

The brief shows at most twelve open summaries; full descriptions are read on
demand. Closed intents stay addressable by name. No owners, priorities, due dates,
leases, or domain schemas are prescribed by the kernel. Shopping items, spool
measurements, calendar entries, and goal hierarchies remain domain records.
External artifacts need durable references; they are not rebuilt by `rehydrate`.

`self give intent. <dir|->` exports the intent lifecycle as inert `lineage.*` evidence (`-` writes a single JSON account `{name, intent, record}` to stdout).
`learn` declares a local intent to interpret the account, reading a directory (`intent.md` + optional `record.jsonl`), a JSON account file, or `-` on stdin (e.g. `curl -fsS <url> | self learn -`).
`self learn --into <intent-name> <dir|file|->` groups deliveries under one named intent;
a new delivery revises/reopens that intent. Earlier descriptions and receipts
remain in the log. Repeating an identical delivery (account name, intent text,
record bytes, and grouping name) appends nothing and prints the prompt again.
Imports preserve `origin`, falling back to a string source `id`. Identified
observations already held under that name and origin are not deposited twice;
conflicting content refuses the whole delivery. Older records without identity
retain the delivery-level guarantee only. These identities are foreign claims;
imported kernel vocabulary still must be renamed to inert lineage. The account's own
intentions remain lineage; a receiving mind may adopt a relevant outcome with
local criteria, or close the learning intent after declining it.

## Making a capability

`self make` declares a capability and installs its script in one step: the
declaration and the script travel as one batch, exactly as they would through
`self hear`. Run bare, it prints this layer:

<!-- prompt:make:begin -->
`self make command|view <name> <summary…> [--consumes a.b,c.d] [--brief] < script`

A command does something. It is any executable, in whatever this machine has, and what it prints is shown to whoever ran it:

    self make command disk "free space on the disks that fill up here" <<'EOF'
    #!/bin/sh
    df -h / /home
    EOF

It runs with your environment and starts in the instance directory; `$SELF_CWD` is where its caller stood.

A printed line that is an event is appended to the log instead of shown. That is how a command records something, and a view is how it is read back: a view gets the events it consumes on stdin, one JSON object per line, and prints text.

    self make command learned "learned <fact…>: keep one fact about this person's world" <<'EOF'
    #!/bin/sh
    jq -nc --arg t "$*" '{name:"fact.learned",payload:{text:$t}}'
    EOF
    self make view facts "what was learned here, oldest first" --consumes fact.learned --brief <<'EOF'
    #!/bin/sh
    jq -r '.payload.text'
    EOF

`--brief` prints the view every time `self` runs: use it for what you should know on arrival. Making a name again replaces it, and `cap/` holds every installed script to read. To want something without building it yet, declare it:

    echo '{"name":"intent.declared","payload":{"name":"<name>","summary":"<the outcome>","description":"<what must hold>"}}' | self hear

`self help` is the whole contract.
<!-- prompt:make:end -->

## Capabilities

Two kinds. The kernel appends what a command prints and never appends what a
view prints; a view has no path to the log through the kernel.

```json
{"name":"command.declared","payload":{"name":"entry","summary":"append one journal entry","description":"usage: entry <text…> — appends journal.entry {text}"}}
{"name":"view.declared","payload":{"name":"journal","summary":"every entry, newest first","description":"usage: journal — no arguments; the whole journal in reverse order","consumes":["journal.entry"]}}
```

A declaration is `{name, summary, description}`, plus optional `consumes`: the
event names its script is fed, signed into its receipt.
Two prose fields, two readers:

- `summary`: one terse line, what this is for. The brief prints it, so every
  pass reads every summary. Kernel clips at 110 characters.
- `description`: usage and argument order first, then why this beats not using
  it. One mind reads it once, via `self brief <name>`. As long as it needs.

Rationale in the summary costs every pass for the benefit of one. A declaration
without `summary` shows the opening sentence of its `description`, clipped: a
fallback, not the contract.

A command declaration may add `"atomic": true` when the command only decides
from the log it reads (a claim, a counter, a one-use approval). Its batch then
commits only if nothing it consumes was appended since it was fed; otherwise
the kernel reruns it on the fresh log, up to eight times, then fails. Reruns
repeat any external effect, so never mark a command atomic that acts outside
the log.

A view may also carry `brief: {order, bytes}`: read it on arrival. Bare `self`
and `self prompt` run every such view with no arguments, lowest `order` first,
and print its output under `## <name>`, clipped at line boundaries to `bytes`
(default 2000, at most 16000). It runs as any view does: a read, appending
nothing. A view that fails or takes over three seconds prints one
`(unavailable: …)` line in its place. `self brief` itself stays a pure read of
declarations and runs nothing. Bare `self` then names capabilities without
their summaries, to leave room; `self brief` has them. Use it for what a mind
should know before it acts (open work, recent memory), within the budget it
earns: every arrival pays for it.

A command declaration may add `"stdin": true` to read the caller's stdin
(`cat notes.md | self run remember`); its log then arrives on fd 3. Atomic
reruns replay the same bytes.

A declaration stays pending until a script is installed. A refused attempt
records `script.rejected`; its reason remains in the brief until superseded.

<!-- prompt:growth:begin -->
A pending capability awaits a script. Inspect what exists, then author and test
what you can verify now. Leave the rest declared for a later pass. Installed
source stays in `cap/` for selective inspection; prompts carry declarations.

```json
{"name":"script.authored","payload":{"type":"command|view","name":"<declared name>","script":"<shebang and bytes>"}}
```

The kernel signs and installs bytes or records a refusal. `script.authored`
is a wire message, never a stored event. Test before authoring; an installation
receipt proves which bytes were installed, not that their behavior is correct.

Commands receive argv, their `consumes` events on stdin (fd 3 with
`"stdin": true`), `SELF_HOME`, the caller's directory as `SELF_CWD`, and the
instance working directory. A stdout line that is an event is appended; any
other line is shown to the caller. Views receive argv and
their signed `consumes` events, no `SELF_HOME`, and an empty scratch directory;
stdout is read-only bytes. Use a shebang and a standard-library language.

Keep summaries terse; put usage and rationale in descriptions. A parameterized
view's zero-argument form lists valid keys. Named records need revision and
retirement paths; streams retain history. Preserve reusable verified methods
with evidence, and automate when repeated use warrants it. Full design guidance:
`self help`.
<!-- prompt:growth:end -->

### Capability design

A view without arguments should list its valid keys or actionable items.
Reserve failure for malformed arguments. Named records need create, revise,
and retire events under a stable key; retirement hides the live record without
erasing history. Streams such as journals retain their history directly.

Retain reusable methods with their trigger, constraints, verification, and
source references. Keep raw sessions, failed attempts, and secrets out of those
records. Lead selective views with relevant or failing methods; leave archives
off the default read. Observing live state is a command that records evidence;
a view only replays it. Automate a method when repeated use warrants it.

Escaping a script into JSON by hand is error-prone. Use `jq`:

```sh
jq -nc --arg t command --arg n entry --rawfile s /tmp/entry.sh \
  '{name:"script.authored",payload:{type:$t,name:$n,script:$s}}' | self hear
```

**Commands may declare outcomes and capabilities.** Declarations are events,
so a command can leave an intention for a future mind. Human and generated
declarations have the same lifecycle; neither grants additional authority.

**Retiring is an event.** The script leaves the brief, every event stays, and
re-declaring brings it back as pending work to author fresh:

```json
{"name":"capability.retired","payload":{"type":"view","name":"journal"}}
```

## Capability scripts

Any language with a shebang. A view's environment is scrubbed for determinism:
fixed `PATH`, `TZ=UTC`, `LC_ALL=C`, `PYTHONHASHSEED=0`, plus the caller's
`SELF_*` variables, which is how you hand a capability configuration. A command
acts in the caller's world, so it keeps the caller's whole environment.

- **command**: an effect on one instance. argv after `self run <name>`; stdin
  its `consumes` events as JSONL, all by default (on fd 3 with `"stdin": true`,
  stdin then being the caller's); `SELF_HOME` set; `SELF_CWD` the caller's
  working directory, for relative paths; cwd the instance. Each stdout line
  that is an event is appended; every other line is shown to whoever ran the
  command, as it is printed. A line that opens with `{` and is not an event
  fails the run, since it was meant as one. Exit non-zero and nothing is
  appended.
- **view**: a pure function of its events. argv after `self view <name>`; stdin
  exactly its receipt's `consumes` events in log order (`[]` or `["*"]` means
  all); no `SELF_HOME`; cwd an empty scratch directory; stdout opaque bytes for
  the reader, never the log. Same events in, same bytes out: no clock, no
  network. Never stored in the log; replayed on demand. Not a sandbox — the
  kernel simply hands a view no path.

## Events

Nine current kernel event names, plus the historical aliases listed below.
Other names are domain events, appended verbatim and interpreted by minds and views.

| name | writer | meaning |
|---|---|---|
| `intent.declared` | anyone | declare, revise, or reopen a desired outcome |
| `intent.closed` | anyone | record completion evidence or a reason to let an intent go |
| `command.declared` | anyone | a command exists, pending a script |
| `view.declared` | anyone | a view exists, pending a script |
| `script.installed` | kernel | receipt: these bytes are installed, signed under the local key |
| `script.rejected` | kernel | an authored script was refused, and why |
| `capability.retired` | anyone | tombstone: out of the brief, still in the log |
| `lesson.learned` | kernel | receipt: what an account actually deposited |
| `account.given` | `self give` | this instance gave an account away |
| `loop.settled` | mind stdout | this pass cannot usefully continue; reason required |

`script.authored` is the wire schema above; never appended.

An event is `{id, seq, name, occurred_at, via, by, payload}`, with optional `origin`. Never changed or
deleted; a deletion is a later event.

## Provenance

- `via`, the channel: `cli`, `cli:<command>` for what `self run <command>`
  emitted, `hear`, `kernel`, or `learn:<account>`. Stamped by
  the kernel from what it saw, never accepted from a script, mind or record. A
  local fact, like `seq`.
- `by`, the mind: `SELF_CALLER` recorded verbatim, a claim, never verified.
  Without it, the harness's own name where it announces one (`AI_AGENT`,
  `CLAUDECODE`). The brief shows it beside each capability, as `you` when it
  is the caller's own.
  Portable, like `occurred_at`.

The `by` claim is signed into the receipt: authorship cannot be relabelled.

## Signed installation

A receipt is `{type, name, script, consumes, by, sig}`; `sig` is HMAC-SHA256
over those fields, domain-separated and length-prefixed, under
`SELF_HOME/.secret` (32 random bytes, 0600). Only a receipt verifying under the
local key installs, and only for a capability this log declared and has not
retired. Anything else in the log is inert.

Never edit `events.jsonl` or author files directly into `cap/`; use the wire.
Bytes live at `cap/blob/<sha256>`; `cap/<type>/<name>/run` symlinks to the blob.
Execution resolves the latest live verified receipt, checks the blob's hash,
rewrites it if it differs, runs it. Hand-edits under `cap/` are overwritten by
the log.

Two opt-in trees follow the same rule, each kept only if its directory exists.
`view/<name>` holds what `self view <name>` prints, rewritten after a commit
only when that output changes. `bin/<name>` is a shim for `self run <name>`,
so `PATH=$SELF_HOME/bin:$PATH` makes capabilities commands. Anything else in
them is removed; a projection failure is reported on stderr and never fails a
commit.

`self rehydrate` makes disk match the log exactly from `events.jsonl` and
`.secret` alone: no model, no network.

Byte-identity holds for one log and one key. Never across instances: `id` is
random; `seq` and `via` are local.

## Accounts

The one wire format between instances: a directory of plain text. **Nothing
runnable travels.**

```
account/
  intent.md      who this is from, what it means, what you hope it becomes (required)
  record.jsonl   events verbatim (optional)
  manifest.json  {events, record_sha256, prefix?, capability?} (optional; give writes, learn treats as advisory)
```

`self give <selector> <dir>` writes one. Selector: an event-name prefix
(`note.`) for knowledge, or `command/<name>` / `view/<name>` for a capability's
declarations and receipts, renamed to `lineage.*`.

`self learn <dir>` deposits an account and declares a local intent to learn it:
`intent.declared` first, new observations next, `lesson.learned` last. The receipt
hashes the offered record and counts new observations; it does not claim that a
mind understood them. Repeated deliveries append nothing.

The declaration has a stable `learn/<delivery-hash>` name (or `--into` name), a summary, and a description
carrying the learning instructions and quoted `intent.md`. The original `account`
and `intent` fields remain available to existing views. Even if no mind reads the
printed prompt now, the learning intent remains discoverable by a later loop.
The account directory is useful reference, but the deposited log retains the
intent and evidence when that directory is gone.

The mind interprets the account against local state: retain knowledge, adapt an
outcome, build a capability, or explain why nothing applies. Close the learning
intent with that evidence; leave it open if interpretation is unfinished.
Importing another instance's intent does not make its proposed actions locally
authorized. Learning need not produce code.

```sh
self learn account/ | claude -p | self hear
```

Four mechanical rules:

1. **Kernel vocabulary never travels raw.** `give` renames it `lineage.<name>`;
   `learn` refuses a record containing it and appends nothing. Otherwise a
   deposited `command.declared` becomes pending work and the next pass signs an
   attacker's script under your key. The refused set is **cumulative**: the names above plus every name any earlier kernel acted on —
   `kernel.initialized`, `projector.declared`, `script.compiled`, `self.asked`,
   `self.replied`, `self.reflected`, `learn.orchestrated`,
   `capability.revision.requested`, `capability.kept`, `intent.kept`, `loop.settled`, `work.declared`, `work.closed`. A name may leave the vocabulary; it never
   leaves this set.
2. **Timestamps and authors are preserved.** Deposited events keep `occurred_at`
   and `by`. `via` is re-stamped `learn:<account>`, which is how a view tells
   local events from learned ones. Filtering on `via` is the receiving mind's
   job, not the kernel's.
3. **Edits are visible.** `lesson.learned` records the sha256 of the record as
   read beside what the manifest claimed. A mismatch means "this file is not the
   file that was given", whether a line was removed or a transport rewrote line
   endings.
4. **Only the local key installs.** An account can only be read.

Giving shares evidence and intent; learning decides what they become here.

## The CLI

No `ask`, `reply`, or `retire` verbs — those travel on the wire. `self make`
is the one authoring verb, and it only assembles a wire batch.

```
self                        session guidance + capability index (READ)
self prompt [ask…]          execution prompt: default or explicit ask (READ)
… | self hear [--after ID]  conditional or unconditional ingestion (WRITE)
self setup                  what to offer so every session starts with self (READ)
self make <kind> <name> <summary…> < script   declare and install in one step (WRITE)
self brief [name]           the state card; with a name or intent/<name>, full detail
self run <cmd> [args…]      execute a command
self view <name> [args…]    replay a view ("log" is built in, shadowable)
self view log [--all]       last 10 events, or every one
self learn [--into ID] <dir|file|-> deposit an account, print its learning prompt
self watch [opts] [prefix]  wait for (--follow: stream) matching events (READ)
self give <sel> <dir|->     write an account from the log
self rehydrate              make cap/ match the log
self help                   this file
```

`self run` and `self view` without a name print their index and succeed.
Unknown names, wrong kinds, malformed arguments, and command failures report
on stderr and fail. Fixed-form verbs reject extra arguments before acting.
`self brief <name>` preserves the full description, including line breaks;
`command/<name>` and `view/<name>` disambiguate names shared by both kinds.

Stdout carries the verb's result: prompts, views, and indices are text; `run`
prints committed-event summaries; `hear` prints ingestion receipts. When no
wire events are present, `hear` passes its input through unchanged and explains
nonempty input on stderr. Only a mind or capability's event-producing stdout
is the event wire; CLI reports are not replayable events.

## Exit codes

`0` did the thing, `1` did not. `run` and `view` pass a capability's own
non-zero status through, such as a refusal's `3`. Unknown or unrunnable
capability: stderr diagnostics, empty stdout, failure. A command emitting no
events succeeds silently. Output write failures fail; already committed events stay committed.

## Environment

```
SELF_HOME              the instance: events.jsonl and .secret
SELF_CALLER            recorded verbatim as `by`, signed into your receipts;
                       defaults to the harness's name where it announces one
SELF_*                 anything else passes through to capability scripts
```

Without `SELF_HOME` the instance is the working directory if it already holds
`events.jsonl`, and `~/.self` otherwise. One person has one self by default, in
every shell and every hook, with nothing exported. Set `SELF_HOME` to keep an
instance elsewhere; `SELF_HOME=.` starts one in the working directory.

## Lineage

Old account `intent.declared` events carrying
only `{account, intent}` remain testimony: they do not retroactively open intents.

This kernel starts a new lineage and does not read the previous kernel's
receipts (`script.compiled` is not acted on; signatures are domain-separated).
On a v1 `events.jsonl`: every event still reads; every v1 `command.declared` is
pending, so a mind re-authors it under this key; v1 `projector.declared` is
invisible — re-declare as views. The clean path is `self give` under the old
kernel, `self learn` under this one: the protocol is its own migration path.

## Limits

- **The log drives the mind, and the log can hold untrusted input.** There is no
  review step between authoring and signing: piping a mind into `self hear`
  signs whatever it wrote. Hold the output in a file first if you want a pause.
- **The write side trusts its caller.** Anyone who can run `self` against your
  `SELF_HOME` can install. The receipt gate is about reconstruction and foreign
  accounts, not local privilege.
- **Nothing is sandboxed.** Views are scrubbed for determinism, not
  containment. Commands run as you, with your environment.
- **The log is unbounded.** No compaction in the kernel; a snapshot is a
  capability someone can declare, or migrate to a fresh child via `give` and `learn`.
- **Batches are locked; execution is not a transaction.** One ingestion is one
  critical section. Concurrent commands may read the same earlier state; their
  committed batches cannot interleave. External effects are not rolled back.
- **Capability scripts are not timed out.**
- **The last log line is judged by whether it is a whole event.** Terminated: a
  record. Unterminated but parsing: a record, given its newline by the next
  append. Unterminated and unparseable: a torn write, skipped and dropped by the
  next append, which says so. Corruption further up is an error naming the
  line, never a silent skip.
- **A command's runtime failure is not an event.** Exit code and stderr are the
  caller's; nothing is appended. Only a refused authoring attempt becomes state.
