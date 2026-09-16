# Changelog

## Unreleased

`work.declared` and `work.closed` carry arbitrary desired outcomes between
minds. Open summaries appear in the brief; `self brief work/<name>` reads the
full declaration and closure evidence, and `self brief work/` lists open work.
The loop can settle while work waits for input. Learning can retain knowledge
or adopt unfinished work without building a capability; giving work exports
inert lineage, requiring local adoption at the receiver. These two event names
are now reserved kernel vocabulary. Replay uses local append order, without
ownership, leases, or cross-instance conflict resolution.

Prompts now lead with desired outcomes, keep capability construction conditional,
and leave detailed design guidance in `self help`.

Runnable examples for a printer hobbyist, household, and standalone script
author live in `examples/work/`.

Bare `self` reconnects an agent with its persistent identity and capability
index while it continues the user's task. Pass instructions and pending
script authoring belong to `self prompt`, `self learn`, and `self loop`.
Use `self prompt | mind | self hear` for a pass without an ask; existing
`self "<ask>"` pipelines remain supported.

## v1.0.0 — one fixed-point loop

The first stable release. Everything below this heading and under the next one
is what changed since v0.2.0; the log format is compatible with v0.x, nothing
else is. See **Migrating** at the bottom of the next section.

### Docs: the README is a third of its size and opens with how to use it

Install with `go install github.com/wouterbeets/self@main`, put one line in the
agent instructions or one `SessionStart` hook that runs `self`, work as usual.
Both routes were run against Claude Code in print mode: with the hook alone and
no tools, the agent reports the instance's event count; with neither, it does
not know it. `@main` rather than `@latest` because the last tag, v0.2.0,
predates `self brief` and the current CLI.

Gone: `AGENTS.md`, a long card that restated statically what the brief tells
the agent from the log; `KERNEL_AUDIT.md`, the record of one refactor;
`instance-sync.plan.md`, `instance-sync.prompt.md` and `examples/instance-sync`,
one personal two-machine deployment with its build output and patched copies
committed. The README keeps how to use it, how it works, and the limits.


### Changed: every rung of the CLI unfolds to the one below it

`self run` and `self view` already printed the capability index when called
bare. Nothing else did. `self view peers --nonsense` ended at `view "peers"
exited: exit status 1` — the reader knows the view exists, knows it rejected
something, and has no path from there to what it wanted. An unknown name sent
the reader to `self brief` rather than printing what it would have found there.
An unknown verb listed the verbs but not their usage.

Now: an unknown name prints the index of names that do exist. An unknown verb
prints the usage block. A capability that ran and refused its arguments prints
its declaration — the whole thing if the script wrote nothing to stderr, and
otherwise only the pointer to `self brief <name>`, since a capability that
documents itself has already answered and does not need a second answer stapled
to it. `peer` prints its own verb table and exits 2; it gets one line, not six
hundred characters of rationale it did not ask for.

All of it is stderr and the exit code still says failure. Stdout is the wire —
a command's stdout is event JSONL the kernel appends, a view's is the page a
pipeline reads — so a diagnostic printed there would be parsed as events. To
read what a script said, `runCommand` takes an optional diagnostic writer and
`runViewDiag` takes one outright; every other caller still gets `os.Stderr` and
never has to know.

### Changed: a declaration carries a terse summary, and the brief prints only that

A declaration was one prose field, and the protocol told its author to fill it:
usage, argument order, then the consequence of skipping the capability. That is
the right instruction for the mind choosing among tools, and it was the only
instruction, so every author followed it and the brief grew to hold all of it.
On a thirty-seven-capability instance that came to 19.7 kB — a surface read in
full by every waking, of which a waking typically uses one entry.

Nothing there was indiscipline. One field was serving two readers with opposite
budgets: choosing needs the rationale and happens once, orienting needs only
enough to tell two names apart and happens on every pass.

So `decl` gains `summary`, one terse line, and the brief prints that and nothing
else. The kernel clips it at 110 characters rather than asking for brevity,
because the mind writing a declaration sees its own line and never the aggregate
it lands in; a bound with no feedback signal is one nobody can tell they crossed.
`description` is unchanged and now has a reader that wants all of it: `self brief
<name>` prints one declaration whole, with a view's `consumes` and its install
and refusal history. Declarations older than the field still map — the opening
sentence of the description stands in, clipped — so nothing already in a log
falls off the surface.

The drill-down is announced where the need for it appears: the brief says its
lines are clipped, directly above the first clipped line, and only on a surface
that actually clipped something. `self brief <TAB>` completes declaration names,
bare and deduplicated, marking a name held by both a command and a view. It is
also in `self --help`, in the CLI block, and in the brief's `## where` footer.

On this instance the brief went 19.7 kB → 4.5 kB and bare `self` 21.0 kB → 5.7 kB.

### Changed: the built-in log view answers "lately" and takes `--all`

`self view log` replayed every event and rejected every argument. It is the read
a cold mind reaches for first, the one the brief recommends, and the only read in
the kernel whose cost grows without bound — on this instance, 1.0 MB. It now
prints the last ten events, and `--all` lifts the bound to exactly the old
output, byte for byte.

Elision is announced on a leading `#` line, with the true event count and the
flag that shows the rest. A bounded read that did not say so would leave a mind
confidently wrong about what the instance has done, which is worse than the
context it saves; the `#` keeps the remainder the same tab-separated stream, so
anything piping it is unaffected.

### Changed: `self run` answers with what exists

Bare `self run` now prints the commands this log holds — descriptions and
pending marks included, exit zero — instead of a bare usage error, and a name
the log does not know is answered with the same list instead of an error that
sends the reader to `self brief` for it. A view by the missing name still falls
through, so materialize offers `self view <name>`. `self view` has behaved this
way since it landed; the two verbs now answer the same question the same way.

### Changed: `self loop` wakes a body instead of polling a fixed point

Driving the loop on an empty instance showed it settling in two passes every
time, and not because the model was timid: four forces in the kernel all pointed
at silence. The default ask ended "act only if something warrants durable
action. Silence is valid" — the fixed point stated as an instruction. `--ask`
was shown on pass one only, so a nudge evaporated after the turn it caused. The
growth layer said "author and test each script, then print", telling the mind to
close every declaration in the same breath and leave nothing pending — and
pending work is the only thing the kernel treats as unfinished. And nothing
changed between passes except the mind's own writes.

Each pass is now a **waking**. The kernel writes a line of facts it alone knows
— waking number, wakings left, what woke the body, whether the last waking was
quiet — and then splices a `prompt:loop` layer from PROTOCOL.md that invites the
mind to leave the next waking something: a declaration it has not built, a view
half-formed, a note. The ask stands on every waking. The growth layer now says a
declaration without a script is an intention carried forward, not a failure.

`--settle N` (default 2, `SELF_LOOP_SETTLE`) is how many quiet wakings in a row
rest the body; the last of them is asked plainly whether there is anything else.
A refused script no longer ends the loop: `hear` reports it as a typed error the
loop recognises, the refusal lands as `script.rejected`, and its reason rides the
next waking. Reaching `--max-passes` on a quiet waking is a rest, not a failure.

Killing `self loop` now kills the waking. Each mind runs in its own process
group and the whole group is signalled on timeout or on SIGINT/SIGTERM to the
loop; before, an interrupted loop left the mind running as an orphan, still
writing to the body through its own `self run` calls with its answer going to a
closed pipe, and a grandchild holding stdout could keep the timeout from ever
returning.

`install` now refuses a script whose first bytes are not `#!`, with a reason,
instead of signing it and letting the first `self run` fail with "exec format
error".

`examples/mind-claude` is `claude -p` as a mind with a live trace: it asks for
the stream-json trace, renders text, tool calls and tool results to stderr as
they happen — which the loop already routes to the terminal — and hands only the
final answer to stdout, so the wire is unchanged and a waking is watchable.
`examples/mind-opencode` is the same seam for `opencode run`, which takes its
prompt as an argument: the JSON event stream is rendered live, and only
event-shaped lines from the final step reach the wire. `examples/mind-grok` is
the grok CLI in the same seam, decoding its byte-array tool results for the
trace. All three wrappers forward only event-shaped lines, so `hear` no longer
echoes the mind's prose a second time as pass-through.

### Changed: the repository is only the kernel and its demo

`experiments/` (two finished measurement harnesses, results preserved in git
history), `loop.sh` (the pre-fixed-point compatibility wrapper) and
`lessons/faces` are gone. What remains is what `demo.sh` and the quick start
need: the kernel, the two sidecars, `examples/mind-stub`, and three small
accounts (`journal`, `chat`, `memory`). The README is rewritten to say plainly
what the prompt already says in one sentence: you are this self, for a bit.
The kernel history it used to narrate lives here, in the changelog.

### New: shell completion, and the instance completes its own domain

```sh
source <(self completion zsh)     # bash and fish too
```

The shim is dumb and stable: every candidate comes from `self __complete`,
which replays the log — verbs, then installed capability names for `run` and
`view` (pending ones annotated, so the tab key doubles as a status surface),
selectors for `give`. Grown capabilities complete without reinstalling the
shim.

Argument positions are domain state the kernel cannot know, so it delegates:
`self view context <TAB>` replays an installed view named `complete.context` —
declared `consumes` on stdin, the typed words as argv, its stdout lines offered
as candidates, under a deadline and with stderr discarded. Tab-completion for
goals is therefore a capability the instance grows through the ordinary loop,
not behavior the kernel ships. PROTOCOL.md documents the convention under
"Completion".

### Changed: the protocol is named

The exchange that travelled as knowledge-seed, then as the account protocol, is
the Self Protocol. An account remains the unit that moves — intent, optional
record, optional attestation. The runtime speaks it; [`PROTOCOL.md`](PROTOCOL.md)
is the contract.

### Changed: the vocative in `prompt:core` and `prompt:growth`

The core layer no longer tells a mind it is a visitor interpreting a body.
It names occupancy: you are this self, for a bit. The mind ends; you do not.
What persists is an append-only log, and only what you append persists —
context stays finite either way. The growth layer follows the same vocative:
what it used to address as "future minds" is now "later wakings", so a pending
turn, which splices both layers, no longer carries two frames at once.
PROTOCOL.md is still the single source; `self help` and every situated turn
splice the same wording.

### New: `self loop`

```sh
self loop -- <mind> [args...]
self loop --max-passes 12 --timeout 30m -- \
  pi --provider github-copilot --model gpt-5.6-luna --no-session -p
```

Capability growth and domain work are no longer separate loop shapes. Each pass
presents the same naked situated surface; the mind discovers pending scripts,
refusals, goals, tasks, or any other state through the brief and views. The
kernel repeats after any authoritative append and converges after the first
complete mind turn that leaves the append-only log unchanged. The revision
representation stays private to the kernel.

The mind is executed directly from argv after `--`, inherits the caller's
working directory and environment, reads the situated prompt on stdin, and
writes the normal event wire on stdout. Defaults are 12 passes and 30 minutes
per mind process; both are configurable. There is no default or resident model.
`SELF_LOOP_MIND`, `SELF_LOOP_MAX_PASSES`, and `SELF_LOOP_TIMEOUT` can pin those
defaults so bare `self loop` works. Because an environment value cannot preserve
argv boundaries, `SELF_LOOP_MIND` runs through `sh -c`; explicit argv after `--`
takes precedence.

### Breaking: bare orientation no longer exits 3

Bare `self` now always prints the situated surface and exits zero. The previous
v2 loop therefore no longer converges:

```sh
# retired: bare self no longer signals capability-only readiness with exit 3
while ask=$(self); do printf '%s\n' "$ask" | mind | self hear; done
```

Use `self loop -- <mind> [args...]`. Explicit ask pipelines, `self hear`,
commands, views, learning, giving, and rehydration are unchanged. `loop.sh`
remains as a compatibility wrapper over the built-in verb.

### New: parameterized views

`self view <name> [args...]` passes trailing arguments as view argv while
preserving receipt-filtered stdin, scratch working directory, scrubbed
environment, and non-ingested stdout. This permits pure reads such as
`self view context <goal>` instead of append-on-read query commands.

### New: browser sidecars

`self-serve` and `self-browse` are optional binaries, not kernel. They pipe
over the same verbs a terminal uses:

```sh
self-browse                 # `self brief` in the system browser
self-browse shopping        # GET /view/shopping → `self view shopping`
self-serve                  # 127.0.0.1:8377 (PORT overrides)
```

`/` is `self brief`. `/view/` is the zero-arg view index. `/view/<name>[/<arg>…]`
is `self view`; `/run/<cmd>/…` is `self run`. Slash-named capabilities
(`timer/set`) resolve as one name. Clicks land with `by=browser`. Every request
is a fresh replay; the server holds no session. This is not a return of v0
`self serve`: views still emit opaque bytes, the kernel still has no HTTP, and
the only thing the HTTP face adds is a two-second freshness check.

## Also in v1.0.0 — the kernel that stopped guessing

*Written under a v2.0.0 heading while unreleased; that tag was never cut and
this work ships in v1.0.0.*

A rewrite. The log format is compatible; nothing else is. If you have a grown
v0.x instance, read **Migrating** at the bottom before you do anything.

### Why

v0.x decided what it was doing by inspecting file descriptors. Bare `self` did
five jobs disambiguated by `isatty`, and one of them was a write: situating an
ask appended `self.asked` before printing the prompt. Two consequences, both
verified against the old binary rather than argued:

- **Looking at an instance changed it.** `loadSecret` minted `.secret` from every
  read path, so a stray `self` in any directory left a signing key behind, and
  every orientation appended a conversation turn.
- **The documented loop was broken everywhere an agent runs.** With no tty — a
  script, a subprocess, cron — `echo "…" | self | claude -p | self` recorded the
  mind's reply as a *question* and returned a 47-line prompt instead of the
  answer.

The fix is structural, and it collapses most of the kernel with it.

### Breaking: the loop

```sh
# v0.x
echo "add a mood tracker" | self | claude -p | self

# v2
self "add a mood tracker" | claude -p | self hear
```

An ask arrives as **argv**; what comes back from a mind arrives on **stdin**, at
`self hear`. Prose alone cannot tell those apart — "what is going on?" and a
mind's answer to it are both prose — so the kernel stops guessing. The read face
never touches stdin, so it also cannot block at the head of a pipeline, and
behaviour is identical in a terminal, a pipe, a script, a sandbox and cron.

The convergence loop is command substitution, because a pipeline swallows the
exit code and `sh` has no `pipefail`:

```sh
while ask=$(self); do printf '%s\n' "$ask" | claude -p | self hear; done
```

### Breaking: the CLI

| v0.x | v2 |
|---|---|
| `self serve` | **gone** — no HTTP server, no injected shell, no `site/` |
| `self show <name>` | `self view <name>` |
| `self retire <target>` | a `capability.retired` event through `self hear` |
| `self protocol` | `self help` (prints `PROTOCOL.md` verbatim) |
| bare `self` (five faces) | `self [ask…]` situates; `self hear` writes; `self brief` is the state card |
| — | `self view log` is built in, and shadowable |

### Breaking: events and receipts

- `projector.declared` → `view.declared`. The kind is `view` everywhere.
- `script.compiled` → `script.installed`.
- The receipt signature is domain-separated, length-prefixed per field, and
  covers `consumes` element by element. **v0.x receipts do not verify.**
- Gone from the kernel: `self.asked`, `self.replied`, `self.reflected`,
  `kernel.initialized`, `intent.declared`'s old shape, `learn.orchestrated`,
  `capability.revision.requested`. The kernel keeps no conversation of its own,
  and no longer parses `chat.message` — a *lesson's* event name it had no
  business knowing. Conversation is `lessons/chat` now.
- A declaration is `{name, description}` plus `consumes` for a view. The old
  `params`, `event`, `revision` and `implementation` fields are gone; nothing
  validated them, and `implementation` was a runnable riding the one channel the
  account protocol exists to keep runnables out of.
- An event name must be lowercase dotted: `^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`.

### Breaking: environment

`SELF_MIND_ID`, `SELF_BIND` and `SELF_WORK_PROMPT` are gone. What remains is
`SELF_HOME` and `SELF_CALLER`; any other `SELF_*` variable is passed through to
capability scripts, which is the documented way to hand one configuration.

### New: the law

**Reads project. Writes append. Orientation is a read.** Bare `self` on a
directory that is not an instance creates nothing at all — no log, no key. The
key is minted only where something is signed.

### New: views emit bytes

A view is a pure function from events to **bytes**, not to HTML. v0.x served
views over HTTP and injected a shared stylesheet, so a projector was *required*
to emit bare semantic markup — no CSS, no JavaScript, no assets. That ceiling is
gone, and `lessons/faces` is the account that shows what is behind it: a table
for a prompt, JSON for `jq`, CSV for a spreadsheet, an SVG chart, one
self-contained HTML file with its own CSS and JS, Prometheus text a monitoring
stack can scrape, iCalendar a phone can subscribe to, and a shell script that
recreates the log.

Purity is enforced rather than requested: a view gets no `SELF_HOME` and an
empty working directory. Its whole input is stdin.

### New: the loop can tell you it is done

`0` did the thing, `1` did not, `3` nothing to do. Bare `self` exits 3 when no
declaration is pending and no refusal stands.

### New: one description of the contract

`PROTOCOL.md` is embedded in the binary, printed by `self help`, and spliced into
every situated prompt. v0.x had six hand-synced descriptions of the wire and its
brief printed two contradictory instructions back to back. A test now fails if
any file in the repo shows a pipeline whose last stage is a bare `self`.

### Hardening

Each of these was reproduced before it was fixed:

- **A receipt had no door.** Replay gated `script.rejected` on `via=kernel` but
  not `script.installed`, and a genuine receipt's signature stays valid forever —
  so echoing an old receipt payload back through the wire re-installed it,
  undoing a retirement or rolling a fixed script back, with no key and no
  declaration.
- **A refusal could wedge the loop forever.** A rejection naming nothing a
  declaration could match never closed, so the instance never reported quiet
  again.
- **A torn line bricked the instance.** There is no `fsync`, so a crash or a full
  disk left a line with no newline and every verb died on it, `rehydrate`
  included. The last line is now judged by whether it is a whole event:
  terminated, or unterminated-but-parsing, is a record; unterminated and
  unparseable is a torn write, skipped and dropped by the next append. Real
  corruption further up is an error naming the line.
- **`.secret` raced on a fresh home** (two selves both minted a key; the loser
  signed receipts under one that was thrown away), and `self run` minted one in
  whatever directory it was called from — forging a key on an instance whose key
  had gone missing, hiding the only honest diagnostic there is.
- Installed bytes are content-addressed at `cap/blob/<sha256>` with a readable
  symlink, verified against their own hash before execution, so a running
  script's bytes can never change under it and a hand edit is simply overwritten
  by the log.
- Scripts run in a scrubbed environment (`TZ=UTC`, `LC_ALL=C`,
  `PYTHONHASHSEED=0`, fixed `PATH`). Determinism was claimed and never enforced;
  a view iterating a set rendered different bytes every run.
- `consumes` is signed with the script, so re-declaring a view without
  re-authoring it can no longer feed the old script a stream no signed unit
  corresponds to.
- One `hear` body is one critical section, with its report buffered until after
  the lock is released.
- `hear` is lenient per line and loud about it. Told plainly that stdout is the
  wire, a real frontier model still opens with a sentence; all-or-nothing threw
  six perfect events away. Ignored lines are named and counted on stderr, and
  stdout carries only the report.

### Size

Kernel: 2261 lines of Go across five files, from 3080 plus an embedded
stylesheet, an HTML fragment, a vendored copy of three.js and a game. Eight
event names, from sixteen. Everything derived — what exists, what is pending,
what was refused, which bytes are trusted — comes out of a single walk over the
log.

### Migrating from v0.x

v2 begins a new log lineage. It does not read v0.x receipts, so **do not expect
copying `events.jsonl` to carry your capabilities over.** What actually happens,
verified:

- Every event still reads. `self view log` shows the whole history, domain events
  included, moments and speakers intact.
- Every v0.x `command.declared` resurfaces as a **pending declaration**, because
  its receipt no longer verifies — so the loop offers to re-author it locally,
  under your key. A migration that runs itself.
- v0.x `projector.declared` events are invisible; the kind was renamed. Declare
  those again as views.

The clean path is the one anything else travels between instances, and the
protocol being its own migration path is the claim:

```sh
self give command/<name> /tmp/account    # under the old kernel
self learn /tmp/account | claude -p | self hear   # under this one
```

## v0.2.0 and earlier

See the git history. `v0.1.0` was the first tagged release.
