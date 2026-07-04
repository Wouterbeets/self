# self

Every agent session is an amnesiac: brilliant for one context window, then
gone. `self` is the body your minds share — one append-only event log that
outlives every session, every model, every vendor, with a kernel small enough
to read in an afternoon.

A local-first, self-growing capability system, cut to its spirit. The kernel
is one Go file. Everything else grows.

> One append-only event log + shared projections. A small kernel; everything
> else grows as seeds. Every capability, projection, and byte of state is a
> plain file you can open.

## sixty seconds

```sh
git clone https://github.com/Wouterbeets/ks && cd ks
go install .                    # puts `self` on your PATH (via GOBIN)

# meet a living body first — this repo ships one, a dozen minds deep
SELF_HOME=$PWD/garden self      # browse its memory at :7777

# then give your own project a body
cd ~/my-project
export SELF_HOME=$PWD/.self     # the log lives beside the code
export SELF_BRAIN="claude -p"   # your coding agent is the brain (see: plug a brain)
self                            # first contact: a key, a birth event, a served garden
```

Last step: paste the card in [`AGENTS.md`](AGENTS.md) into your project's agent
instructions (`CLAUDE.md`, `AGENTS.md`, a system prompt — wherever your
sessions read). From then on, every coding-agent session wakes up a citizen of
the same garden.

A fresh body starts bare — a key, a birth event, no verbs — and that is the
design: your agent's first session grows the first organs it actually needs,
or adopts them from a body that has them. The garden in this repo will give:

```sh
SELF_HOME=/path/to/ks/garden self share note | self adopt -
```

The payoff is the **second** session — it starts where the first one stopped,
and can prove it, because the log replays.

## mental model

- **One truth.** `events.jsonl` is an append-only log — the only source of
  truth. Nothing is ever mutated or destroyed; a "delete" is just another event.
- **Projections are replays.** A projection is a pure function of the log,
  rendered as HTML in `site/` that you read in a browser and your agent reads as
  context — the same reality.
- **Seeds are intent.** A seed is a directory with an `intent.md` — prose about
  how a surface should feel, not a parts-list. An LLM orchestrator reads the
  garden and grows the decomposition that realizes the intent *here*. Same
  intent, different garden, different capabilities.
- **The strange loop.** Emitting a `command.declared` or `projector.declared`
  event makes the kernel compile it into a live capability on the spot — at grow
  time and at run time — so a running capability (or the brain) grows new
  capabilities just by declaring them. The loop carries *specs, never code*: the
  LLM compiler is the single ingress for code, and every compile is logged as a
  `script.compiled` receipt signed with the home's `.secret`. Anything may
  append a receipt; only a kernel-signed one ever installs. A forged receipt is
  inert.
- **A body is two files.** `self rehydrate` rebuilds `capabilities/` and `site/`
  from `events.jsonl` + `.secret` alone — no LLM, no network. See `garden/` for
  a real organism stored exactly this way; the test suite resurrects it.

## the loop

```sh
go build -o self .
./self                    # rehydrate the body from the log, then serve it at :7777
./self grow seeds/chat    # grow the chat seed; then ask it to grow everything else
./self run chat "add a habit tracker"
./self think "..."        # ask the brain; returns {response, declarations} JSON
./self heartbeat          # one self-improvement cycle: the brain reflects & grows
./self show <name>        # render a projection to stdout
./self rehydrate          # rebuild the body from the log's signed receipts (no LLM)
./self share graze > graze.seed.jsonl   # a capability's whole history as a seed
./self adopt graze.seed.jsonl           # re-grow it here, through this body's own compiler
```

Routes when serving: `/` (my identity), `/<projection>` (re-rendered live),
`/run/<command>` (plain HTML forms, zero JS), `/events` (the raw log).

## the pipe contract

Capabilities are standalone scripts in any language, orchestrated as Unix
pipeline nodes:

- **command**: args as argv, current events as JSONL on stdin → new events as
  JSONL on stdout (`{name, payload}` per line; the kernel assigns the rest).
- **projector**: all events as JSONL on stdin → HTML on stdout; the kernel
  persists it to `site/<name>.html`.

A capability that needs intelligence calls `self think` — the brain. The brain
is itself just a process behind that contract (prompt in, event JSONL out),
swappable via `$SELF_BRAIN`: an LLM, a script, or a human. The kernel can't
tell the difference.

## plug a brain

Every ask for intelligence — `think`, `heartbeat`, `grow`, and every compile
the strange loop triggers — passes through one seam. Plug it three ways:

```sh
SELF_BRAIN="claude -p"       # any executable: a coding agent, a script, a human
SELF_LLM_URL=http://...      # …or any OpenAI-compatible endpoint (built-in loop)
SELF_LLM_STUB=1              # …or nothing: offline stubs, the whole loop testable
```

The `SELF_BRAIN` contract is small enough to hold in one hand: the process gets
the kind of ask in `$SELF_ASK` (`think` / `heartbeat` / `grow` / `compile`),
the prompt as its last argument, and the whole log as JSONL on stdin. It
answers in event JSONL — `command.declared` / `projector.declared` to grow,
`script.authored` (`{"script": "..."}`) to answer a compile — and any bare
prose is tolerated as the reply. That is the entire integration surface: a
coding agent that can read stdin and print JSON is a full citizen of the
strange loop, compiler included, with its receipts signed under its own
`SELF_BRAIN_ID`.

The brain and compiler explore through a **playpen**: a jailed full-bash shell
holding an ephemeral copy of the body at `/body` (events.jsonl, capabilities/,
site/ — never `.secret`), built from Linux user namespaces by the kernel
itself. Writes cannot leave the jail, the network namespace has no interfaces,
and nothing done inside installs anything — declarations remain the only
ingress, and only the kernel signs. So a mind can *test* a candidate organ
against the real log before declaring it, instead of squinting at it. Where
namespaces are unavailable (or `SELF_SANDBOX=0`), bash falls back to a
fail-closed read-only allowlist — it never fails open.

## sharing between bodies

Two bodies never exchange code — they exchange a **seed**: a verbatim slice of
the sender's log. `self share <cap>` prints, as event JSONL, every declaration
of the capability (the intent — re-teachings and dead ends included, the
selection rather than the survivor) and every kernel-signed receipt for it
(the evidence), then logs the giving. `self adopt <seed>` records the whole
seed inside one `capability.adopted` event — foreign receipts ride there,
where rehydrate never looks, inert by construction — then re-declares the
capability so the strange loop authors bytes *for this body*, through this
body's own compiler, signed by this body's key. The sender's latest script
rides only in the `implementation` reference slot a seed author already uses.
Same intent, different garden, different bytes; the log's own format is the
wire format.

Provenance survives adaptation — the seed sits in the receiver's log verbatim,
sender by-lines intact — but it is asserted, not provable: signing keys are
symmetric and never leave a home, so cross-body authorship is a claim the
receiver records, not a fact it can verify. That honest gap is an open
question in the garden's log.

## environment

```
SELF_HOME         the body — a dir holding events.jsonl + .secret (default ~/.self)
SELF_BRAIN        any executable plugged in as the brain — think, heartbeat, grow,
                  and compile all pass through it ($SELF_ASK carries the ask's
                  kind, prompt as last arg, log as JSONL on stdin, event JSONL
                  or prose out)
SELF_LLM_URL      OpenAI-compatible endpoint (default http://127.0.0.1:8080)
SELF_LLM_API_KEY  its key
SELF_LLM_MODEL    its model
SELF_LLM_STUB     "1" → offline stub scripts (no LLM, no network)
SELF_SANDBOX      "0" → disable the brain's jailed playpen (bash falls back
                  to a fail-closed read-only allowlist; never fails open)
SELF_BRAIN_ID     provenance by-line signed into script.compiled receipts
                  (default: the model @ its endpoint, or "stub (no LLM)")
```

Every `script.compiled` receipt records **who authored the bytes** — the
brain's identity, covered by the same signature as the script, so authorship
can no more be forged or relabeled than the code. The kernel page shows each
organ's *grown by* line. In a garden tended by many minds — a Gemma growing
one organ in the background, a Claude verifying it later, a human at a
bridge — the lineage of code stays as queryable as the lineage of minds.

## what's in the repo

- `main.go` — the whole kernel: event log, signed install, pipe orchestrator,
  LLM compiler/brain, web server.
- `seeds/chat` — the front door: talk to self and it grows the rest.
- `seeds/herd` — meta-context for [herdr](https://herdr.dev), the agent
  terminal multiplexer: an HTML overview of everything your agents are doing —
  who's blocked, what each is building, and whether the work coheres per
  domain — with the memory herdr doesn't keep.
- `garden/` — a living body (one organism's log + signing key), left exactly as
  its minds committed it. `SELF_HOME=garden ./self` resumes it.
- `main_test.go` — the spirit, pinned: the log, the strange loop (offline via
  stub scripts), the forged-receipt gate, the playpen's containment, receipt
  provenance, sovereign share/adopt, the pluggable brain seam, and the
  garden's resurrection.

This repo was once ~9k lines across 65 files; it was deliberately cut down to
this. The full history — invariant selection, cross-node attestations, teach /
restore / watch, the build log — lives in git.

## status

Experimental. The thesis: a minimal kernel — event log, signed install, replay —
plus an LLM compiler is enough for a system that grows, tests, and rewrites its
own capabilities while staying local-first and fully inspectable. Two honest
costs: trust is *provenance, not containment* (the compiler is the single code
ingress and therefore the attack surface; installed scripts run unsandboxed),
and the log is unbounded (every compile stores its bytes; every projection
replays the whole log).
