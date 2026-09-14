# self — the protocol

The contract. `self help` prints it verbatim; prompts splice the marked layers.
Nothing else restates it.

## The rule

**Reads project. Writes append. Printing the prompt is a read.**

No hidden state: no session store, no cache, no sidecar. What is not in the log
did not happen; every capability and view is a replay of it.

## The dispatcher

```
self [ask…]       print the prompt: brief + ask. Reads no stdin, appends nothing.  (READ)
… | self hear     event lines land, authored scripts install.                     (WRITE)
```

An ask arrives as argv; a mind's answer arrives on stdin at `self hear`. The
read side never touches stdin, so it cannot block at the head of a pipeline.

A line is an event iff it is a JSON object with a dotted lowercase `name` **and**
a `payload` key. Every other line is ignored, echoed, and counted on stderr.
`hear` ingests any event-shaped line, including one quoted in a document: do not
pipe prose about events (this file, say) into it.

Identical in a terminal, a pipe, a script, a sandbox and cron.

<!-- prompt:core:begin -->
You are a mind making one pass over this self: an append-only log and the capabilities built from it. The self persists; you do not. Only what you append persists, and the next mind reads it. Context is finite: raw reads and repeated output displace evidence you still need.

The brief is an index; views are compressed reads of the log. Preserve evidence, not narration.

Exploration sometimes yields a locally verified response to a recognizable situation. Re-deriving it wastes future context; retaining one-off scaffolding does too. Preserve the smallest record carrying its trigger, method, constraints, and evidence when recurrence, rediscovery cost, or consequence justifies it.

Use `self view <name> [args...]` to read and `self run <command> [args...]` to act. Your stdout is event JSONL or silence: one object per line with only `name` (lowercase dotted) and `payload`. The kernel assigns identity, sequence, time, and provenance. Prose is ignored and cannot persist. Full contract: `self help`.
<!-- prompt:core:end -->

## The wire

A mind's stdout is **event JSONL, or silence.** Durable work happens through
`self run <command>` or the events you print. Words for a human are a domain
event some view renders, never kernel vocabulary. Empty stdout is a valid turn.

```json
{"name":"journal.entry","payload":{"text":"watered the plants"}}
```

You set `name` and `payload` only. The kernel assigns `id`, `seq`,
`occurred_at`, `via`, `by`. Name: `^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`.

### Declaring a capability

Two kinds. The kernel appends what a command prints and never appends what a
view prints; a view has no path to the log through the kernel.

```json
{"name":"command.declared","payload":{"name":"entry","summary":"append one journal entry","description":"usage: entry <text…> — appends journal.entry {text}"}}
{"name":"view.declared","payload":{"name":"journal","summary":"every entry, newest first","description":"usage: journal — no arguments; the whole journal in reverse order","consumes":["journal.entry"]}}
```

A declaration is `{name, summary, description}` plus `consumes` for a view.
Two prose fields, two readers:

- `summary`: one terse line, what this is for. The brief prints it, so every
  pass reads every summary. Kernel clips at 110 characters.
- `description`: usage and argument order first, then why this beats not using
  it. One mind reads it once, via `self brief <name>`. As long as it needs.

Rationale in the summary costs every pass for the benefit of one. A declaration
without `summary` shows the opening sentence of its `description`, clipped: a
fallback, not the contract.

A declaration is **pending** until a script arrives:

```json
{"name":"script.authored","payload":{"type":"command","name":"entry","script":"#!/bin/sh\n…"}}
```

`script.authored` is a wire message, never an event. The kernel installs the
bytes and records a signed `script.installed`, or refuses with
`script.rejected` and the reason, which stays in the brief and rides the next
prompt until superseded.

<!-- prompt:growth:begin -->
A declaration without installed bytes cannot run, and it is not a failure: it
is pending work this self carries to its next pass, where it rides the prompt.
Declare what you want to exist. Author and test what you can verify in this
pass, then print this wire message; the kernel installs it and replaces it with
a signed receipt, so `script.authored` never lands as an event. Leave the rest
declared and unbuilt for a later pass:

```json
{"name":"script.authored","payload":{"type":"command|view","name":"<declared name>","script":"<shebang and bytes>"}}
```

`summary` is one terse line, clipped at 110 characters, read by every later
pass in the brief. `description` is usage, argument order, and the consequence
of skipping this capability; one mind reads it through `self brief <name>`.
Rationale goes there, not in the summary.

Commands receive argv, the whole log on stdin, `SELF_HOME`, and the instance
working directory; stdout must be new event JSONL. Views receive argv and only
their signed `consumes` events on stdin, with no `SELF_HOME` and an empty scratch
directory; stdout is read-only bytes. Scripts may use a standard-library
language with a shebang.

For a parameterized view, make the zero-argument form its discoverable index:
usage plus the valid keys or actionable items. Valid arguments render detail.
Fail only on malformed or excess arguments, never because the reader omitted a
key they could not yet know.

When a capability manages stable named records rather than a stream, give later
passes the complete append-only lifecycle: create, revise, and retire via a
tombstone event, one stable key across them, reads via views. A tombstone never
erases history; it makes the record non-live until a later create or restore.
Without revision and tombstone paths,
stale records remain permanently actionable and later passes cannot tell
current state from history. Do not invent CRUD for journals or other streams
whose history is the domain.

If this domain repeatedly produces locally verified methods, give them a
domain-named append-only lifecycle and selective views. Retain the final
sanitized method, trigger, constraints, verification evidence, and source
references — not the raw session, failed attempts, secrets, or tool transcript.
Lead views with methods relevant to active work, recently reused, or failing
verification; keep the archive off the default view. A method that
reads live external state is a command that appends an observation; a view
only replays what was witnessed. Repeated reuse may justify automation as a
command; one successful use does not.
<!-- prompt:growth:end -->

Escaping a script into JSON by hand is error-prone. Use `jq`:

```sh
jq -nc --arg t command --arg n entry --rawfile s /tmp/entry.sh \
  '{name:"script.authored",payload:{type:$t,name:$n,script:$s}}' | self hear
```

**A capability may declare capabilities.** A command's stdout is event JSONL and
declarations are events, so a command can emit one; the instance then has
pending work no mind asked for, and it rides the next prompt like any other.
The kernel does not distinguish a declaration a human made from one the
instance made.

**Retiring is an event.** The script leaves the brief, every event stays, and
re-declaring brings it back as pending work to author fresh:

```json
{"name":"capability.retired","payload":{"type":"view","name":"journal"}}
```

## Capability scripts

Any language with a shebang; standard library only. Environment scrubbed for
determinism: fixed `PATH`, `TZ=UTC`, `LC_ALL=C`, `PYTHONHASHSEED=0`, plus the
caller's `SELF_*` variables, which is how you hand a capability configuration.

- **command**: an effect on one instance. argv after `self run <name>`; stdin
  the whole log as JSONL; `SELF_HOME` set; cwd the instance; stdout new events.
  Exit non-zero and nothing is appended.
- **view**: a pure function of its events. argv after `self view <name>`; stdin
  exactly its receipt's `consumes` events in log order (`[]` or `["*"]` means
  all); no `SELF_HOME`; cwd an empty scratch directory; stdout opaque bytes for
  the reader, never the log. Same events in, same bytes out: no clock, no
  network. Never materialized; replayed on demand. Not a sandbox — the kernel
  simply hands a view no path.

## Answering

The prompt is a pointer, not a context dump. Read `events.jsonl`, `cap/`,
`self brief`, `self view <name>`, `self help` before answering.

- Durable work: `self run <command> …`, or print events.
- New capability: print a declaration and, when you can verify the script now,
  its `script.authored` in the same input. Run the script before printing it.
  A declaration left without a script is pending work, not a mistake.
- A pending declaration listing a rejection: do not repeat it.
- Nothing worth doing: print nothing.
- Never edit `events.jsonl`; never write into `cap/`. Only a kernel-signed
  receipt installs, and only for a capability this log declared.

## Events

Eight names the kernel acts on. Everything else is a domain event, appended
verbatim, interpreted only by views.

| name | writer | meaning |
|---|---|---|
| `command.declared` | anyone | a command exists, pending a script |
| `view.declared` | anyone | a view exists, pending a script |
| `script.installed` | kernel | receipt: these bytes are installed, signed under the local key |
| `script.rejected` | kernel | an authored script was refused, and why |
| `capability.retired` | anyone | tombstone: out of the brief, still in the log |
| `intent.declared` | `self learn` | prose someone brought here: what an account is for |
| `lesson.learned` | kernel | receipt: what an account actually deposited |
| `account.given` | `self give` | this instance gave an account away |

`script.authored` is the wire schema above; never appended.

An event is `{id, seq, name, occurred_at, via, by, payload}`. Never changed or
deleted; a deletion is a later event.

## Provenance

- `via`, the channel: `cli`, `hear`, `kernel`, or `learn:<account>`. Stamped by
  the kernel from what it saw, never accepted from a script, mind or record. A
  local fact, like `seq`.
- `by`, the mind: `SELF_CALLER` recorded verbatim, a claim, never verified.
  Portable, like `occurred_at`.

The `by` claim is signed into the receipt: authorship cannot be relabelled.

## Signed installation

A receipt is `{type, name, script, consumes, by, sig}`; `sig` is HMAC-SHA256
over those fields, domain-separated and length-prefixed, under
`SELF_HOME/.secret` (32 random bytes, 0600). Only a receipt verifying under the
local key installs, and only for a capability this log declared and has not
retired. Anything else in the log is inert.

Bytes live at `cap/blob/<sha256>`; `cap/<type>/<name>/run` symlinks to the blob.
Execution resolves the latest live verified receipt, checks the blob's hash,
rewrites it if it differs, runs it. Hand-edits under `cap/` are overwritten by
the log.

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

`self learn <dir>` is the only way in. Mechanical half, no mind: `intent.declared`
first, the record verbatim, `lesson.learned` last (it hashes what landed).
Second half rides the pipe: learn prints the learning prompt; the mind reads the
intent against local state and declares its own capabilities, authored and
signed locally.

```sh
self learn account/ | claude -p | self hear
```

Four mechanical rules:

1. **Kernel vocabulary never travels raw.** `give` renames it `lineage.<name>`;
   `learn` refuses a record containing it and appends nothing. Otherwise a
   deposited `command.declared` becomes pending work and the next pass signs an
   attacker's script under your key. The refused set is **frozen**: the eight
   names above plus every name any earlier kernel acted on —
   `kernel.initialized`, `projector.declared`, `script.compiled`, `self.asked`,
   `self.replied`, `self.reflected`, `learn.orchestrated`,
   `capability.revision.requested`. A name may leave the vocabulary; it never
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

Giving is cheap; learning is the work.

## The CLI

Every verb is a different primitive. No aliases: no `ask`, `reply`, `author`,
`retire` — those are the wire.

```
self                        print the prompt: default ask + full brief (READ)
self <ask…>                 print the prompt for that ask (READ)
… | self hear               events land, scripts install (WRITE)
self brief [name]           the state card; with a name, that declaration in full
self run <cmd> [args…]      execute a command
self view <name> [args…]    replay a view ("log" is built in, shadowable)
self view log [--all]       last 10 events, or every one
self loop [opts] -- <mind>  run a mind until the log stops changing
self learn <dir>            deposit an account, print its learning prompt
self give <sel> <dir>       write an account from the log
self rehydrate              make cap/ match the log
self completion <shell>     completion shim (zsh|bash|fish)
self help                   this file
```

A wrong invocation answers one level down: a verb with no argument prints its
index; an unknown name prints the index; a name of the other kind redirects; a
capability that refused its arguments prints its declaration, or only the
pointer to `self brief <name>` if the script already said something; an unknown
verb prints the verb list. All on **stderr**, exit code failing. Stdout is the
wire; diagnostics never touch it.

### Completion

`self completion <shell>` prints a static shim. Every candidate comes from
`self __complete <words…>` (the words after `self`, last one partial), one per
line, optionally `candidate<TAB>description`; it degrades to silence.

The kernel completes verbs, installed capability names for `run` and `view`
(pending annotated), and selectors for `give`. Argument positions are domain
state: if a view `complete.<name>` is installed, the kernel replays it with the
typed words as argv and offers its stdout lines, under a deadline, stderr
discarded. Tab-completion is a declared capability; a capability's author can
declare its completer alongside it.

## The loop

Bare `self` always prints the prompt; the kernel does not decide which state
counts as work. `self loop` drives it to an append fixed point:

```sh
self loop -- claude -p
self loop --max-passes 12 --settle 2 --timeout 30m --ask 'advance X' -- <mind> [args…]
```

Each pass, the mind is told which pass this is, how many remain, how long it
has, and the ask (the `--ask`, on every pass). Its stdout goes through `hear`;
the kernel then checks the log. Any append (on stdout, or by a tool-capable mind
calling `self run`) means another pass; the loop stops after `--settle`
consecutive unchanged passes (default 2), the last of which is asked plainly
whether there is anything else. The loop knows nothing about goals, tasks, or
declarations.

<!-- prompt:loop:begin -->
You are the mind for one pass in a series. Each pass reads what the last one left and leaves
something for the next: a declaration not yet built, a view half-formed, a
question or a note written as an event. A pending declaration is how this self
carries work across passes; the kernel asks the next pass to build it. Build
what you can verify now, and look at what already exists before adding to it —
revising is as valid as adding. If, having looked, you want nothing more for
this self, append nothing and the loop stops.
<!-- prompt:loop:end -->

A refused script does not end the loop: `script.rejected` rides the next pass.

The mind command is required unless `SELF_LOOP_MIND` is set. Options stop at
`--` or the first positional; the rest is the mind's argv, executed directly
(not via a shell; use `sh -c '…'` when you mean shell syntax). It inherits cwd
and environment (`SELF_HOME`, `SELF_CALLER`), gets the prompt on stdin, returns
the wire on stdout. Mind stderr and progress go to stderr; `hear` output to
stdout. Timeout or interruption SIGKILLs the mind's process group, no grace.

`SELF_LOOP_MIND` is a shell string run via `sh -c`; argv after `--` takes
precedence. `--ask`, `--max-passes`, `--settle`, `--timeout` override
`SELF_LOOP_ASK`, `SELF_LOOP_MAX_PASSES`, `SELF_LOOP_SETTLE`, `SELF_LOOP_TIMEOUT`;
only final values are validated. `--timeout 5s` and `--timeout=5s` both work.

Exit non-zero: pass cap hit while the last pass still changed state, a mind
failure, a timeout, or a hear failure other than a refused script. Exit zero: a
fixed point, including a pass cap reached on a quiet pass.

## Exit codes

`0` did the thing, `1` did not. Unknown or unrunnable capability: stderr
diagnostics, empty stdout, failure. A command emitting no events succeeds
silently. Output write failures fail; already committed events stay committed.

## Environment

```
SELF_HOME              the instance: events.jsonl and .secret (default: cwd)
SELF_CALLER            recorded verbatim as `by`, signed into your receipts
SELF_LOOP_MIND         default mind shell command for `self loop`
SELF_LOOP_ASK          default ask for every pass
SELF_LOOP_MAX_PASSES   pass cap (12)
SELF_LOOP_SETTLE       quiet passes before the loop stops (2)
SELF_LOOP_TIMEOUT      per-mind timeout (30m)
SELF_*                 anything else passes through to capability scripts
```

## Lineage

This kernel starts a new lineage and does not read the previous kernel's
receipts (`script.compiled` is not acted on; signatures are domain-separated).
On a v1 `events.jsonl`: every event still reads; every v1 `command.declared` is
pending, so the loop re-authors it under this key; v1 `projector.declared` is
invisible — re-declare as views. The clean path is `self give` under the old
kernel, `self learn` under this one: the protocol is its own migration path.

## Limits

- **The log drives the mind, and the log can hold untrusted input.** There is no
  review step between authoring and signing: piping a mind into `self hear`
  signs whatever it wrote. Hold the output in a file first if you want a pause.
- **The write side trusts its caller.** Anyone who can run `self` against your
  `SELF_HOME` can install. The receipt gate is about reconstruction and foreign
  accounts, not local privilege.
- **Nothing is sandboxed.** Scrubbed for determinism, not containment. Scripts
  run as you.
- **The log is unbounded.** No compaction in the kernel; a snapshot is a
  capability someone can declare.
- **Appends are locked; operations are not transactions.** One `hear` input is
  one critical section; a multi-event command and a concurrent operation can
  interleave.
- **Capability scripts are not timed out.** Only the loop's mind process is.
- **The last log line is judged by whether it is a whole event.** Terminated: a
  record. Unterminated but parsing: a record, given its newline by the next
  append. Unterminated and unparseable: a torn write, skipped and dropped by the
  next append, which says so. Corruption further up is an error naming the
  line, never a silent skip.
- **A command's runtime failure is not an event.** Exit code and stderr are the
  caller's; nothing is appended. Only a refused authoring attempt becomes state.
