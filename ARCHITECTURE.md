# self PoC — evolving architecture spec

A living spec, updated after each validated slice. The thesis under test: the
five-repo body of work (cubff → emera → knowledge-seed-protocol → household →
self) converges on **self as the minimal kernel**, and everything else — emergence
dynamics, household-style surfaces, sharing — can be added **as seeds**
(LLM-compiled commands/projectors over an append-only log) rather than as
compiled-in subsystems. Re-growing the kernel instead of the seed layer is a
failure, not progress.

Build method: small testable slices. Hypothesis → minimal slice → evidence →
validate against the anchors → keep/refine/refute → log it. The build log is
itself a self seed (`build.hypothesis` / `build.evidence` / `build.decision`
events, rendered by a `buildlog` projector) — the process is a projection of
itself.

Real invariants checked (never fabricated): replay determinism, seed
portability/adaptation, kernel-LOC delta, tests. Everything else is labelled
qualitative.

---

## Slice 1 — emergence as a seed (VALIDATED)

**Hypothesis.** emera's energy/selection ecology can run as a self seed (a `tick`
command + a `population` projector, compiled from declarations) with **zero
kernel changes**. Predicted evidence: the surviving population's
guessed-character distribution converges toward the book-world's real character
frequencies, driven only by a shared-reward + tax energy rule — no roles or
target distribution coded anywhere.

**Slice.** Two declarations planted into a fresh kernel:
- `world.defined {text}` — the book-world (a fixed sentence).
- `tick` command — one ecological step: organisms each guess the next character;
  matchers of the current character **split a fixed reward pool**
  (density-dependent — a char guessed by many pays little each, a rare-guessed
  char pays a lot); everyone pays a small tax; energy ≤ 0 dies; energy above a
  birth threshold reproduces (child inherits the guess, mutated with small
  probability). Emits one `tick.result` population snapshot.
- `population` projector — renders the latest snapshot: per character, how many
  organisms guess it vs. that character's real frequency in the world.

Run: `self run tick` × 120.

**Evidence.**
- **Pearson r(organisms-per-char, world-frequency-per-char) = 0.901** (random
  baseline ≈ 0). The population's most-occupied niches (`i`, `a`, `n`) are
  exactly the book-world's most frequent characters. Nothing codes that target;
  it falls out of the energy rule. (Analytic prediction held: at equilibrium,
  organisms-per-char ∝ frequency, because shared reward gives each niche a
  carrying capacity ∝ its frequency.)
- **Population stable**: ~15–20 alive across the run (init 30, cap 200, never
  collapsed or exploded).
- **Replay deterministic**: the `tick` command is stochastic, but every step is
  recorded as an event, so projecting the population twice is byte-identical.
- **Kernel Go lines changed: 0.** The entire ecology lives in the seed layer.

**Decision: keep.** Emergence runs as a seed; the synthesis thesis stands at the
hardest point. Partial result: the small population can't sustain rare-character
niches (carrying capacity < 1 organism → the frequency tail goes extinct), so
only top niches are occupied — convergence is strong on the head, absent on the
tail. This is the *minimal* form of emera's thesis (selection-driven adaptation
to world structure), not yet its full form (Zipf-style frequent-scaffold vs.
rare-specialist role division), which needs rarity-scaled jackpots + silence
credit.

**Next iteration (slice 2 candidates).**
- Add rarity-scaled jackpots + a "silence credit" so rare-specialist niches
  become viable; test for emergent role division (not just frequency tracking).
- Or pivot to usability: a household-style board as a planted seed driven by the
  brain, to test the accessible-entry-point anchor.

---

## Slice 2 — household board as a seed (VALIDATED)

**Hypothesis.** household's kanban domain (capture/move/resolve over memory
items, grouped into lanes) can run as a self seed — three commands + a `board`
projector — with zero kernel changes, driven by plain HTML forms AND by the
brain calling the commands as tools.

**Slice.** A seed declaring `capture`, `move`, `resolve` commands and a `board`
projector (folds `memory.captured` / `item.stage_changed` / `item.resolved`
into lanes inbox/this_week/waiting/done; each card carries a move `<select>` +
resolve button as plain forms posting to `/run`).

**Evidence.**
- CLI `capture` works; **form-driven move & resolve via Post/Redirect/Get**
  (303 back to `/board`, zero JS).
- **The brain drove the board in plain language**: "add a task to call the
  plumber" → the brain called the `capture` command tool → the item appeared in
  the inbox lane. (Commands-as-tools from the act-verb work.)
- **Complexity (view layer, apples-to-apples)**: self `board` projector = **46
  LOC**; household's kanban projection = **279 LOC of Go**. The whole self board
  capability is a **7-line declaration** → 62 LOC of compiled scripts. To change
  a capability: household edits Go + rebuilds; self edits a declaration, recompiled
  live. (Honest: household's command/aggregate Go is shared across its whole
  domain — meals, recipes, sync — not just the board, so only the projection
  layer is a clean comparison.)
- **Replay deterministic** ✓. **Kernel Go changed: 0** ✓.

**Decision: keep.** The accessible-entry-point anchor and the household→self port
both hold; an everyday surface runs as a seed, form-driven and brain-driven,
with zero kernel change. Limit: board slice only (no meals/recipes/Planka/relay).

**Next (slice 3 candidates).** Receiver-adaptation across nodes (plant the board
seed into a foreign garden whose vocabulary differs); or port a second household
surface (meals) to test breadth; or the deepen-emergence path (kept out of self —
emera stays a sovereign compute node that *emits events into* a self commons).

---

## Slice 3 — the seed format upgrades itself (VALIDATED)

**Hypothesis.** Planting from a *holistic spec* needs zero kernel change. A
planted `grow-spec` command logs the spec (`seed.spec`, provenance) and calls
the brain to derive the atomic `command.declared`/`projector.declared` events;
the kernel's existing strange-loop hook compiles them. So the higher-level seed
format is itself a seed.

**Slice.** `board` re-expressed as one holistic markdown spec
(`poc/grow-spec/board-spec.example.md`: Intent / Capabilities / Behavior /
Examples / Content). A `grow-spec` command (planted normally). Then
`self run grow-spec board-spec.md`.

**Evidence.**
- `grow-spec` is a **planted command** — a seed, not kernel code.
- One invocation derived **4 declarations** (capture/move/resolve/board) from
  the spec via the brain and the kernel compiled them — **0 hand-written
  declarations** (slice 2 used 7 hand-written declaration lines for the same
  board).
- Full **lineage in the log**: `seed.spec → command.declared → script.compiled`.
- The spec-installed board **works** (capture + project).
- **Kernel Go changed: 0** — the seed format upgraded itself.

**Decision: keep.** The irreducible floor stays tiny — LLM compiler + the one
hook turning a `*.declared` event into a `script.compiled` + a raw bootstrap
plant — and *everything above it is a seed*, including the spec compiler. Honest
gaps: the spec's `## Content` was not replayed (only `## Capabilities` derived),
and `## Examples` were not yet used to verify the compiled scripts (no
`script.verified` step).

**Next (slice 4 candidates).** `script.verified` — gate compilation on the
spec's `## Examples` (the verified-free sharing tier); replay `## Content` from
the spec; then cross-node — export a spec seed, plant it on a *foreign* garden,
and verify conformance (usability + emergence + protocol all meeting).

---

## Slice 4 — the strange loop carries code, not just a spec (VALIDATED, then REVERTED — see Slice 5)

**Hypothesis.** Until now the strange loop was a *spec loop*: a command could
emit `command.declared`/`projector.declared` and the kernel re-compiled a fresh
binary via the LLM. That means behaviour is re-derived every generation — a
command that re-declares itself doesn't carry its code forward, it carries a
prompt, and the receiver's compiler decides what the next generation actually
does. We probed this directly: with the stub compiler, a self-redeclaring
`evolve` command collapsed to a generic stub on generation 2 — its real
behaviour was lost. Two surfaces (the README and the brain's own identity page,
which promises the brain that *"a delete is reversible by a later restore"*)
described an exact-code path — rollback / restore from the log — that **did not
exist**: nothing ever read a `script.compiled` payload back into the capabilities.

Predicted: the missing half is a single primitive — let the kernel **act on**
`script.compiled` (install the bytes verbatim, no LLM), the same way it already
acts on the two `*.declared` events. With that one primitive, deterministic
self-replication, generational evolution, and rollback all become expressible —
and rollback/restore needs **no kernel command**, because a command already
receives the whole event log on stdin, so it can find and re-emit any past
`script.compiled` itself.

**Slice.** The kernel's compile hook (`CompileDeclarations`, run at invoke; and
the plant replay loop) now also honours `script.compiled`: it installs the
script verbatim into the capabilities via one shared `InstallScript` helper. The
event is already logged by the writer, so the install is **not** re-logged
(no duplication). Plant additionally skips the LLM compile for any declared
name the seed also ships as `script.compiled` — the declaration is the spec
(it still populates kernel.html wiring/identity), the shipped script is the
binary. Three artifacts demonstrate the loop:

- **a quine** (`poc/replicant`): a command that re-emits its own exact source
  each run. Shipped *as code* in the seed, so it plants and runs with **no LLM
  at all**.
- **deterministic generational evolution**: each run advances a generation
  counter, and the source stays byte-identical across generations.
- **rollback as a pure seed**: a `restore` command that reads the log on stdin
  and re-emits the oldest logged `script.compiled` for a name — rolling a
  capability back with zero kernel code for rollback itself.

**Evidence.**
- `poc/replicant` planted with **no LLM** (no `SELF_LLM_*`, no stub): plant
  reported *"shipped verbatim — skipping compile"* and installed both the
  command and its `lineage` projector. Invoked 4×: the capabilities script stayed
  **byte-identical (sha256) across all 4 generations**, the generation counter
  advanced 1→4, and `lineage` auto-rendered all four — proving the declaration
  still wired the projector even though its binary was shipped.
- The self-install is logged **exactly once per invoke** (no double-logging).
- A `restore <name>` seed re-emitted an older `greet` version and the kernel
  installed it verbatim — `greet` v2 → v1 — with **no kernel code for rollback**.
- The spec-loop boundary is now documented by its contrast: the same
  self-redeclaration under the *spec* path drifts (stub → generic), under the
  *code* path it does not.

**Decision (at the time): keep — later REVERTED in Slice 5.** This slice **did
change the kernel** (~80 Go LOC: an install helper + the hook case + plant's
ship-aware skip),
unlike slices 1–3 which were zero-kernel. By the PoC rule ("re-growing the
kernel is a failure"), that demands justification: the primitive is minimal and
it *moves capability out of the kernel*. Rollback/restore would otherwise have
been a kernel subcommand (read log → write capabilities, which only the kernel can
do); honouring `script.compiled` makes restore, replication, and quines all
**seeds** instead. The kernel's acted-on set grows from two events to three, but
the alternative was a bespoke `self restore` plus no replication story at all.
Net: one small primitive, a large expansion of what the seed layer can express.

**Honest gaps (these became the refutation).** Installing arbitrary bytes
verbatim is an arbitrary-code-execution path: before this slice the only attack
surface was tricking the receiver's own (sandboxed, cooperative) LLM; after it,
any seed could hand over a hostile binary. Worse, a shipped binary *skips the
compiler entirely*, so it never adapts to the receiver's garden — it defeats the
two invariants (receiver adaptation, finite trust surface) that justify the whole
design. The "runs with no LLM!" property I was proud of is exactly the smell:
"no LLM" means "no adaptation, arbitrary code."

→ **Refuted.** See Slice 5 for the corrected form. `poc/replicant` (the
shipped-code quine) was removed; the kernel no longer installs code from an
event.

---

## Slice 5 — the loop carries specs; code reuse is kernel-only (VALIDATED)

**Hypothesis.** The value Slice 4 reached for — precision and exact-code reuse —
can be had *without* importing code, by separating two things Slice 4 conflated:
**import** (run foreign novel bytes — breaks adaptation and trust) versus
**restore** (re-install bytes the receiver's own compiler already authored and
already ran — breaks neither). Keep restore, kill import.

**Slice.** Three changes:
- `script.compiled` is now a **kernel-only** event. `grow`'s replay and
  `runCommand` both drop any `script.compiled` a seed or command emits (with a
  warning), so every receipt in the log is provably kernel-authored.
- Exact-code reuse exists only as **`self restore <name> [seq]`** — a kernel
  operation that re-installs an earlier receipt (rollback one, or pin a seq) and
  logs a fresh receipt. Because only the kernel writes and reads receipts, a
  restore can only reinstate code this receiver already compiled.
- Precision returns as a **reference implementation**: an optional
  `implementation` field on a declaration. The compiler is handed it as a strong
  starting point to *verify against the pipe contract and adapt to the garden* —
  never installed as-is. `poc/wall` is the canonical example.

**Evidence.**
- Reserve holds: a command emitting a `script.compiled` is ignored — the target
  script is untouched and no foreign receipt reaches the log (test +
  end-to-end).
- `Restore` rolls back one version and pins by seq, and errors cleanly on a
  single-version or unknown capability (unit tests over hand-built logs).
- The reference implementation reaches the compile prompt (test); the LLM, not
  the kernel, still authors every binary, so adaptation is never skipped.

**Decision: keep.** Attack surface is back to its pre-Slice-4 size — the only way
code enters the system is the compiler. Receiver adaptation is unconditional
again. Rollback survived as a small kernel op; "ship a binary" was correctly
lost. The kernel acts on **two** events once more (`command.declared`,
`projector.declared`); `script.compiled` reverts to a kernel-only receipt.

**Honest note.** `self restore` is genuine kernel code (not a seed) — but it
*reads* the log and reinstates the kernel's own output, so it can't be a seed
without re-opening the code-install hole. That's the right place to spend kernel
LOC.

**Follow-up (done) — restore is a seed, not a kernel verb.** The same insight
pushed one step further. Two axes were tangled: *who performs the install* (must
be the kernel — it writes `capabilities/` only from its own receipts, so the
state stays a replayable, audited projection of the log) versus *who triggers a
rollback* (anyone — the trigger carries a name+seq, never code). So a rollback is
now a **data-only `restore.requested {name, seq}` event** the kernel acts on,
exactly like it acts on `*.declared`:

- `restore` is an ordinary **seed** (`seeds/restore`) — a command that emits
  `restore.requested` from its argv, carried as a reference implementation.
- The brain rolls back by *calling that capability* (plain act), not via a
  bespoke tool — the special-case brain tool was removed.
- `self restore <name> [seq]` remains as a thin, always-on built-in that emits
  the same event, so the safety net exists on a bare kernel before any seed is
  grown.

The kernel now acts on **three** events — `command.declared`,
`projector.declared`, `restore.requested` — and the line that matters is that all
three carry *data, never code*. The privileged install (turning a logged intent
into bytes on disk) stays the kernel's; everything that *triggers* it is a seed.
That's the minimal-kernel thesis holding even for rollback.

---

## Slice 6 — signed receipts kill the last special case (VALIDATED)

**Hypothesis.** `script.compiled` was still a wart: its meaning depended on *who
wrote it* (authoritative from the kernel, ignored from a seed/command), and that
was enforced by dropping it at every ingress — provenance by vigilance, two drop
sites that must stay in sync, and a silent break the day a new ingress forgets.
A per-home secret can make provenance *intrinsic*: sign each compile receipt;
verify on install. Then `script.compiled` is an ordinary logged event whose only
power comes from a signature only the kernel can produce — no ingress filtering,
nothing special about who may write it.

**Slice.**
- `self init` mints `SELF_HOME/.secret` (32 random bytes, 0600, never in the
  log — like an ssh host key).
- The compiler signs each receipt: `sig = HMAC(secret, type ∥ name ∥ script)`
  (all three bound, so a receipt can't be relabeled onto another capability).
- The one install-from-log path (`Restore`) verifies the signature and ignores
  anything unsigned; the kernel.html history shows only signed receipts.
- The ingress "reserve" drops in `run` and `grow` are **deleted**. A forged
  `script.compiled` now flows into the log freely — and is inert.

**Evidence (e2e).** Receipts carry a `sig`; a command that appends a forged
`script.compiled` for `capture` lands in the log but the `capture` binary is
untouched, `restore` refuses it (only the one signed version counts), and it's
absent from the compilation history. Unit tests: sign/verify round-trip + tamper
+ wrong-home-key rejection; restore ignores a forged receipt even at the highest
seq and refuses to pin it.

**Decision: keep.** `script.compiled` is no longer special at write time — only
at *install* time, where a signature check is the honest gate. The per-home key
also does real conceptual work: signatures are meaningful only on the receiver
that produced them, so you still can't import another node's binaries — only its
declarations, which your kernel recompiles and re-signs. The "two kinds of
state" question dissolves: it all stays in one event log; the compiled bytes are
just signed.

**Honest note.** This adds one piece of non-log state — the secret — which is a
real (small) dent in "the log is the only truth." It's a *key*, not domain data
(LLM credentials already live outside the log), and losing it only costs the
ability to verify old receipts (the working tree on disk is unaffected). For a
PoC the HMAC is plenty; a multi-user or shared-home setting would want real key
management.

---

## Slice 7 — the log is a sufficient artifact (VALIDATED)

**Hypothesis.** The README has always claimed "the log is the only truth" and
"projections are replays," but it wasn't literally true of the working tree:
`capabilities/` was durable *installed* state that nothing rebuilt from the log,
so a home was only portable if you carried the compiled scripts and rendered HTML
too. Yet the log already holds everything needed — every capability is a signed
`script.compiled` receipt (full bytes), every projection a pure replay. The claim
under test: one small kernel op can make the working tree a pure *materialization*
of the log, so a home stores and moves as just `events.jsonl` + `.secret`.

**Slice.** `kernel.Rehydrate(home)` walks the log, takes the latest
kernel-signed `script.compiled` per name, and installs those bytes — reusing the
exact `installScript` + `verifyReceipt` path `Restore` already uses. `self
rehydrate` exposes it; bare `self` runs it before serving, so a home heals itself
on startup. `capabilities/` and `site/` are now reconstructable with no LLM and
no network. The committed `garden/` body drops from 17 files to 2.

**Evidence.**
- A fresh home containing only `events.jsonl` + `.secret` rehydrates to **4
  commands + 5 projectors, byte-identical** (sha256) to the original tree, and
  re-renders every projection (the inheritance letter included) — no LLM (tested
  under `SELF_LLM_STUB=1`).
- **Foreign-key safety holds**: the same log under a *different* `.secret`
  installs nothing — the signature gate that protects `Restore` protects
  rehydrate identically, so no arbitrary-code path is opened (unit test +
  end-to-end). Provenance stays intrinsic; you inherit declarations, not a key.
- Unit tests: latest-receipt-wins reconstruction, and the foreign-key no-op.

**Decision: keep.** This *did* change the kernel (~50 Go LOC: `Rehydrate` + a
`main` helper + the default-command hook), which the PoC rule says must be
justified. It is: it adds no new event and no new trust surface (it is a thin
loop over the existing signed-install path), and it *retires* state rather than
adding it — `capabilities/` and `site/` stop being things you must keep, because
they are now derivable. The claim "the log is the only truth" becomes literally
true of the working tree, not just aspirational. Honest cost: deterministic,
offline rehydration needs the home's `.secret` (the key that verifies the
receipts) carried beside the log; the key-free alternative is to re-grow from the
log's declarations through a brain (receiver-adapted, not byte-identical) — which
is the cross-node story Slice 6 already describes.

---

## Slice 8 — knowledge survives translation between strangers (VALIDATED)

**Hypothesis.** Every prior slice was one garden, or succession within one body.
The knowledge-seed-protocol's actual claim — the reason this lineage exists — is
that knowledge moves between **sovereign nodes that don't share a vocabulary**:
share the method and the evidence, not a conclusion or a binary, and the receiver
re-derives it against its own reality. Predicted: a seed declared in garden A's
vocabulary, planted in garden B whose events are named differently, recompiles
into a *different* binary that correctly runs A's method on B's data — receiver
adaptation across nodes, with no shared schema and no foreign code.

**Slice (`poc/crossnode`).** Two gardens recording the same local reality under
different vocabularies: North logs `observation.logged {what, where, severity}`,
Harbor logs `report.filed {issue, location, urgency}`. North exports `hotspots`
(rank places by total severity) as a seed — a `projector.declared` with a
reference implementation in North's vocabulary and a description inviting field
remapping; **no compiled bytes, no signature**. The same seed is planted in both.
The compiler (a Claude by hand through `brain/bridge.py`) explores each garden
first, so each gets a binary authored for its own vocabulary.

**Evidence.**
- North renders natively: `Elm & 3rd` = 12 (3 reports), `Market Sq` = 3 (2).
- Harbor, from the **same seed**, recompiled against `report.filed`: `Pier 7` =
  12 (3), `Boardwalk` = 3 (2) — matches the hand calculation. North's method ran
  correctly on Harbor's data though the two never shared a word.
- **The binaries differ** (adaptation, not copy): Harbor's consumes both
  `report.filed` and `observation.logged` and maps `location`→place,
  `urgency`→severity; North's consumes only `observation.logged`.
- **Harbor accepts both dialects**: fed one North-style `observation.logged`,
  Harbor's `hotspots` folded it in (Pier 7 12 → 14, reports 3 → 4) — so Harbor
  could ingest North's *raw evidence*, not only its method.
- **Kernel Go changed: 0.** Cross-node sharing is pure seed-layer + the existing
  garden-aware compiler.

**Decision: keep.** The lineage's headline claim holds at the point it was always
deferred to: a seed crosses from one sovereign garden to a stranger with a
different vocabulary, and the receiver's own compiler re-authors it to fit —
knowledge transferred as replayable method + evidence, never as imported bytes.
Honest scope: one projector, two small hand-built gardens, a cooperative compiler;
no adversarial seed, no automated conformance gate (the `## Examples` →
`script.verified` tier is still unbuilt), and the field remapping was guided by a
description the seed author wrote. The mechanism is shown; hardening it (an
untrusted seed, a verification gate, larger vocab drift) is the next slice.

---

## Slice 9 — the verification tier: verify the result, not the compiler (VALIDATED)

**Hypothesis.** Slice 8 showed cross-node sharing works, but "it worked" was a
human judgment — the gap the whole paradigm leans on is the compiler being
trustworthy. The fix named since Slice 3 and never built: a seed ships
**examples** (input → output assertions) that define what a capability must do
independent of how it is implemented; the kernel runs the freshly compiled binary
against them and refuses to install one that fails. Then a receiver recompiling a
foreign seed to its own vocabulary is held to the *author's contract*, not the
compiler's good intentions — "verify the result," not "trust the compiler."

**Slice.** A declaration gains an optional `examples: [{note, args, events,
expect_contains}]` field (`internal/seed`). `seed.VerifyScript` writes the
compiled script to a temp file and runs each example through the real pipe
contract (argv + JSONL stdin → stdout), asserting every `expect_contains`
substring is present — uniform for commands (JSONL out) and projectors (HTML
out). `kernel.VerifyAndLog` runs them, appends a `script.verified` receipt
(pass/fail, for audit), and returns whether it passed; both compile sites
(`cmdGrow` and the strange-loop `CompileDeclarations`) **gate installation** on
it. Examples are opt-in: a declaration without them installs as before.

**Evidence (e2e, on `poc/crossnode`).** The `hotspots` seed carries two examples
in North's `observation.logged` vocabulary.
- North compiles natively → `script.verified passed=True 2/2` → installs.
- Harbor (vocabulary `report.filed`) was grown twice, by hand, through the
  bridge. **Attempt 1**, a *lazy* adaptation consuming only `report.filed`,
  failed the `observation.logged` examples (`passed=False 0/2`) and was
  **rejected — not installed**; the failing receipt names the missing strings.
  **Attempt 2**, consuming *both* dialects, passed `2/2` and installed, then
  ranked Harbor's own data correctly (Pier 7 = 12, Boardwalk = 3). The audit
  trail of the rejection-then-acceptance is in Harbor's log.
- Unit tests: stdin/argv feeding, contains pass/fail, a broken script counted as
  a failure, and the empty-examples no-op. Full suite green.

**Decision: keep.** This changed the kernel (~40 Go LOC: `VerifyScript`,
`VerifyAndLog`, the two gate checks, the `Example` type, the event constant),
justified the same way as Slices 6–7: it adds no new trust surface and it
*converts a judgment into a check*. The examples are vocabulary-bound to the
author, which is the point — they only pass on a receiver whose adaptation
**extends rather than replaces**, making "the method survived translation" a
property the receiver can prove on its own log. Honest gaps: `expect_contains` is
substring presence, not ordering or structural equality (a stronger matcher —
JSON-subset for command events, DOM assertions for projectors — is the next
refinement); the `script.verified` receipt is unsigned, so it is audit, not yet a
remotely-trustable attestation (signing it, like Slice 6 did for
`script.compiled`, would let a *third* node trust a receiver's verified claim
without re-running — the natural next step).

---

## How to reproduce

```sh
go build -o self .
export SELF_HOME=$(mktemp -d) SELF_LLM_*=...   # an LLM, or the Claude bridge, compiles the seeds
self init
self grow poc/buildlog        # buildlog projector + slice-1 hypothesis
self grow poc/emergence       # world + tick + population
for i in $(seq 1 120); do self run tick; done
self live                     # browse /population and /buildlog
```

The seeds are declarations; an LLM compiles them into the `tick`/`population`/
`buildlog` scripts at grow time (garden-aware). Same seed, different receiver,
different binary — but the same emergent behaviour.

Slice 5's `poc/wall` shows the reference-implementation path — a declaration that
carries precise code the compiler verifies and adapts (an LLM is the compiler, so
configure one):

```sh
go build -o self .
export SELF_HOME=$(mktemp -d) SELF_LLM_*=...   # the compiler
self init
self grow poc/wall            # compiles post + wall from declarations + reference impls
self run post claude "hello"  # append a message
self show wall                # render the board
# then, after a re-grow changes a capability: roll it back, audit-faithfully
self history                  # find an earlier script.compiled seq
self restore wall <seq>       # reinstate that exact, kernel-authored version
```
