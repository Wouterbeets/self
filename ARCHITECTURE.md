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
substring presence; `expect_order` (added in a follow-up beat) asserts a sequence
so an example can prove a *ranking*, but full structural equality (JSON-subset for
command events, DOM assertions for projectors) is still the next refinement; the
`script.verified` receipt was unsigned (audit only) — **now signed with an
asymmetric key (Slice 10)**, so a third node can trust a receiver's verified
claim without re-running.

---

## Slice 10 — signed attestations: trust a stranger's verification without re-running (VALIDATED)

**Hypothesis.** Slice 9 made verification local: a receiver can check that its own
recompiled binary passes a seed's examples. But the protocol's end state is
*sovereign nodes trusting each other's verified work without a central authority*.
For that, a `script.verified` receipt must be a claim a **third** node can check —
"node X verified that this binary passes these examples" — without X's secret and
without re-running. HMAC (the `.secret` that signs `script.compiled`) can't do
this: it is symmetric, verifiable only by the home that holds it. The claim under
test: an asymmetric signature makes a verification attestation publicly checkable,
and the two key types cleanly separate two trust needs.

**Slice.** A home gains a second key — an **ed25519 identity** (`.identity`,
minted at `self init`, private seed 0600), distinct from the HMAC `.secret`.
`script.verified` payloads are now an `Attestation`: the result, the **sha256 of
the exact script and examples** it concerns, the signer's **public key**, and an
**ed25519 signature** over all of it. `kernel.VerifyAttestation` checks the
signature from the payload alone. Two new built-ins: `self identity` (print the
shareable public key) and `self verify-attestation` (check a piped attestation).
`script.compiled` is deliberately left HMAC-signed — install-authority must stay
symmetric and private (you can't install a stranger's bytes), whereas a
verification claim is meant to be public. Install-authority private; verification
claims public.

**Evidence (e2e + unit).**
- A signer home grew `hotspots` and emitted a signed `script.verified` (pubkey
  `1fe6…`, `passed 2/2`, bound to the script + examples hashes).
- A **separately-minted verifier home** (`c22f…`), with no access to the signer's
  keys, ran `self verify-attestation` on it → **✓ VALID**, naming the signer and
  the claim. A tampered copy (`passed` flipped) → **✗ INVALID**.
- Unit tests: sign/verify round-trip, verification from the payload alone (no
  secret), a flipped verdict and a swapped script-hash both invalidate the
  signature, and one home's signature does not verify under another's public key.
  Full suite green.

**Decision: keep.** ~110 Go LOC (identity.go + the attestation build in
VerifyAndLog + two thin CLIs), and it earns them: it completes the trust spine the
whole lineage points at — knowledge shared as replayable evidence (Slice 8),
re-derived under the receiver's own contract (Slice 9), and the receiver's verdict
now **independently checkable by anyone** (Slice 10), with no platform in the
middle. The split between a private install key and a public attestation key is
the conceptual core: provenance of *what runs here* stays sovereign; provenance of
*what I claim to have verified* becomes shareable. Honest gaps: a public key is
just bytes — binding it to a real-world identity (web-of-trust, a key directory)
is out of scope and a deliberate non-goal for a local-first PoC; trusting *who* a
key is remains the reader's call (the tool says so). And the attestation binds
script + examples by hash, so a verifier must hold those bytes to confirm the
claim is about the capability they mean — which is exactly the auditable,
no-blind-trust property the protocol wants.

---

## Slice 11 — selftest: the projection is the oracle, and the loop runs it (VALIDATED)

**Hypothesis.** `self` is unusually testable by an agent: a command's whole effect
is a projection — plain HTML/JSONL that can be read perfectly — and projections
are deterministic, replayable, and free of hidden state. So "did this change
break a capability?" can be answered mechanically: feed each capability its
example inputs, read the output, assert. If that runs before every merge, an
autonomous self-improvement loop becomes self-*checking*, not just self-modifying.

**Slice.** `kernel.SelfTest(home)` re-runs every *installed* capability's declared
examples against the binary on disk (reusing `seed.VerifyScript`), returning
pass/fail/untested per capability. `self selftest` reports it and exits nonzero on
any failure. Where the compile-time gate (Slice 9) checks a freshly compiled
script, selftest checks what is actually installed *now* — catching drift from a
hand-edit, a rehydrate, or any regression. `scripts/preflight.sh` is the gate the
heartbeat loop runs before merging: build → `go test ./...` → rehydrate the
shipped `home/` board body (no LLM) → `self selftest`.

**Evidence.** Preflight green end-to-end: the board body rehydrates and all three
capabilities (`capture`, `move`, `board`) pass their examples against the
installed binaries (3/3). Unit tests: selftest passes a correct binary, reports an
example-less capability as untested-but-OK, and **fails a deliberately drifted
binary** (a `greet` that no longer echoes its argv). The methodology had already
proven itself in practice one slice earlier — the board's `move` shipped with a
multi-word-lane bug that the examples caught at compile time, and lane placement
was confirmed by reading the rendered board back as the oracle.

**Decision: keep.** Small (~70 Go LOC + a shell script), and it closes the loop the
whole session was building toward: knowledge is shared as evidence (8), re-derived
under contract (9), the verdict is independently signed (10), and now every change
is **regression-checked against the projection before it's trusted** (11). The
autonomous loop's per-beat procedure is to run `scripts/preflight.sh` and refuse
to merge on red. Honest gap: selftest runs each capability's examples in isolation
(unit level); a full end-to-end driver — run real `capture`/`move` commands into a
temp home, then assert the live `board` projection — is the next fidelity step
(the examples already exercise the projector with synthetic multi-event input, so
the compositional path is covered, just not the literal command pipeline).

---

## Slice 12 — the brain is a process, not a wire (VALIDATED)

**Hypothesis.** Everything in self composes the Unix way — a command or a
projector is a process the kernel pipes the event log through (argv + JSONL in →
JSONL/HTML out). One thing didn't: the brain/compiler was an OpenAI-shaped HTTP
call (`/v1/chat/completions`) linked *into* the kernel — a networked oracle with
its own 15-round tool-calling loop, opencode default, and llama fallback, ~600
LOC of transport living inside the kernel module. `brain/bridge.py` already hinted
the endpoint was swappable (it parks each request for a human/Claude to answer),
but it was still HTTP, and `self think` still called `CallBrain` as a library and
returned a `{response, declarations}` struct. The claim under test: thinking is
just another process the kernel pipes events to. Put the brain behind the *same*
stdin/stdout contract as a command — prompt in, **event JSONL out** — and the
kernel stops needing a "brain" concept at all; the brain becomes swappable,
composable, and able to read the garden directly (no tool round-trips), because a
local process can just `ls`/`cat`.

**Slice.**
- `runCommand`'s body split into two reusable halves: `pipeProcess` (exec a node
  with `cwd=SELF_HOME`, feed the log on stdin, parse events off stdout) and
  `ingestEvents` (append + strange-loop compile + restore + projector auto-run).
  A command and a brain are now literally the same call shape; only the
  executable differs.
- `self brain` is the **brain-as-process primitive**: prompt in (argv), event
  JSONL out (stdout) — the user + assistant `chat.message`s, then any
  declarations verbatim. It wraps the in-tree compiler (`CallBrain`) — explore,
  act, grow. It's the *default* `$SELF_BRAIN`, swappable for anything that honors
  the same contract.
- `self think` is the **brain's call interface for capabilities, kept
  byte-compatible** with every garden ever grown. Same contract as before: read a
  prompt, return `{response, declarations}` JSON, **append nothing** (the caller
  owns appending). What changed is only the plumbing — instead of linking the LLM
  in-process it spawns the brain *process* (`brainCommand`: `$SELF_BRAIN` if set,
  first word is the exe with the prompt appended as one arg; else self's own
  `brain` mode) via `pipeProcess`, then folds the emitted events back into the
  legacy JSON shape (assistant `chat.message`s → `response`; `*.declared` →
  `declarations`). So an existing `chat`/`grow-spec` keeps working untouched while
  the brain is now a swappable process behind the pipe.
- The split is the point: install-authority and the conversational interface
  (`self think`) stay stable; the *intelligence* (`self brain`) becomes a
  replaceable plugin. The ~600 LOC of LLM transport didn't shrink, but it moved
  off the kernel's hot path into a process you can swap, fan out, or pipe.

**Evidence.**
- **The kernel can't tell a brain from a shell script** (e2e test +
  reproduction): with `$SELF_BRAIN` pointed at a 5-line Python script and **no LLM
  at all**, `self think "…"` spawned it, the script explored the garden itself
  (`os.listdir("capabilities/commands")`), emitted its `chat.message`s + a
  `command.declared`, and `self think` returned exactly the legacy
  `{response, declarations}` JSON — appending nothing itself.
- **Backward compatibility is proven, not asserted**: a hand-built `chat` command
  shaped like the LLM compiles from `seeds/chat` (call `self think`, parse the
  JSON, emit `chat.message`s) ran unchanged against the new binary driven by the
  fake brain — user + assistant messages landed correctly, no parse error.
- **Growth still flows through the strange loop**: the `command.declared` the
  brain produced reached `chat`'s output, was appended by `chat`'s own
  `runCommand`, and `CompileDeclarations` installed the capability — same path as
  always, no special-casing for brain-grown capabilities.
- **Existing bodies migrate with zero steps**: `home/` (35 events) and `garden/`
  (45 events) both `rehydrate` and selftest cleanly on the new binary; no event
  type, kernel state, or schema changed. Preflight (rehydrate + 10/10 examples)
  green.
- **Swappability is the contract, not a knob**: the same `self think` drove an LLM
  (`self brain` default), a human-in-the-loop (`bridge.py` behind `self brain`),
  or a fake script, with zero kernel change between them.

**Decision: keep.** Net **subtraction of concept** — the "networked oracle"
category is gone; there is now one kind of thing the kernel talks to, a process it
pipes events through. The trust model is unchanged because it never trusted the
compiler: the brain's output is just events/JSON, and the only privileged step
(sign + install a `script.compiled`) still happens kernel-side via the
signed-receipt path (Slices 6–7), gated by examples (Slice 9). The composition
payoff the lineage points at — multiple LLMs, swarms, a human, a deterministic
transpiler, all interchangeable — is now just "set `$SELF_BRAIN`."

**Scope is narrow, and that is why migration is zero-cost.** Slice 12 changed
exactly one thing behaviorally: the *transport* of the conversational `think`
path (now a process, kept byte-compatible). The two operations a migration
actually relies on are **untouched**:
- *Replay* (`rehydrate` / `restore`) reinstalls the log's kernel-signed
  `script.compiled` receipts verbatim — no brain, no LLM, no network (Slice 7). A
  home moves as `events.jsonl` + `.secret`; the secret verifies the receipts. So
  pulling Slice 12 into a repo with hundreds of events and rehydrating it
  *circumvents the brain entirely* — the changed code never runs. Verified: the
  shipped `home/`/`garden/` bodies rehydrate + selftest clean on the new binary
  with the brain set to `/bin/false`; compiled `capabilities/` come out
  byte-identical (only `kernel.html` differs, by the embedded absolute home
  path); a *foreign* `.secret` installs nothing (signature gate holds).
- *Compilation* (`grow` and the strange-loop `CompileDeclarations`) still uses the
  in-process `seed.NewCompiler` — it was **not** extracted. So re-growing from a
  log's declarations (the keyless cross-node path) is also unaffected.

The brain-process change is confined to the live conversational surface, which is
backward-compatible; neither replay nor compile — the paths that author and
reconstruct state — were altered. That orthogonality is the real reason an
existing garden just works.

**Honest gaps.** (1) Only the `think` path was converted; `self heartbeat` and
`self watch` still call `CallBrain` in-process (next beat — they carry a
feedback-guard and a `brainOn` gate that want care). (2) `self brain`'s acting
still uses an in-process `invoke` closure rather than shelling out to `self run`,
so "the brain composes by calling the CLI like anything else" is true in shape but
not yet literal. (3) The collapse of bash/declare/run into shell/stdout/`self run`
is realized only by a *local* brain with filesystem access (demonstrated by the
fake brain); the default `self brain` still inherits the remote-LLM tool loop. (4)
`$SELF_BRAIN` swappability is reachable through `self think` (which resolves it);
`self brain` is the default *implementation* and deliberately does not re-resolve
`$SELF_BRAIN` (that would recurse) — so a capability wanting the swappable brain
calls `self think`, not `self brain` directly.

**Next (slice 13 candidates).** Convert heartbeat/watch onto `runBrain`; make
`self brain` act via `self run` subprocesses (full CLI composition); fan-out — run
N brains and vote/diff before the kernel signs (a natural fit with the
`script.verified` tier); or the symmetric move for the *compiler* (`CompileCommand`
/`CompileProjector` behind `$SELF_COMPILER`), so even capability compilation is a
swappable process.

---

## Slice 13 — the brain configures itself through its own projection (VALIDATED)

**Hypothesis.** Slice 12 made the brain swappable, but wiring one in still meant
knowing `$SELF_BRAIN` / `SELF_LLM_*` and editing the environment — invisible to a
fresh user who clones the repo. The accessible-entry-point anchor says the first
run should be a *page*. But "configure the LLM" is a chicken-and-egg: a normal
capability is LLM-compiled, and there is no LLM yet. The claim under test: the
brain-setup surface can be an ordinary projection + command — the same
forms-post-to-`/run` pattern the board uses — installed at `self init` *before any
LLM*, because once `init` mints the home's `.secret` the kernel can author and
sign its own onboarding scripts (the same provenance Rehydrate/Restore trust). And
the one hazard — an API token — stays out of the log by construction.

**Slice.**
- `kernel.InstallBuiltin(home, kind, name, script)` installs a kernel-authored
  script with a signed `script.compiled` receipt — no LLM, rehydratable like any
  capability. The bootstrap escape: the kernel ships these bytes in the binary and
  signs them with the freshly minted secret, so no foreign-code path opens.
- `self init` now installs a **`configure` command** + **`setup` projector**
  (declared so kernel.html wires them and `setup` auto-runs on `brain.configured`)
  and seeds an initial `brain.configured {provider:"none"}`.
- The `setup` projector renders the **one HTML page**: a provider picker
  (human / llama.cpp / Ollama / OpenAI / opencode / Anthropic / custom) + base-URL,
  model, and token fields, posting to `/run/configure`. Bare semantic HTML, no JS.
- The `configure` command splits the form's fields by destination — the **secret
  rule**: `provider`/`base_url`/`model` → a `brain.configured` *event* (in the log,
  portable, replayable); the **token → `$SELF_HOME/.brain-key`** (0600), never an
  event. The event records only `key_set: bool`.
- `self brain` calls `loadBrainConfig`, which folds the chosen provider into
  `SELF_LLM_*` (token read from the key file) *unless* those env vars are already
  set — so the page-driven choice drives the LLM, with env override still winning.
- The serve root lands a fresh user on `setup` until a brain is configured.

**Evidence (e2e).**
- `self init` installs `configure` + `setup` and renders `site/setup.html` (form
  → `/run/configure`) with **no LLM**.
- Configuring **through the live web form** (`POST /run/configure`) records
  `{provider:"openai", base_url, model, key_set:true}` in the log and writes the
  token to `.brain-key`; the token appears **0 times** in `events.jsonl`. The page
  cannot render it — it isn't in any event the projector reads.
- The configured provider drives the brain: `self brain` then targets
  `…:11434/v1/chat/completions` (the chosen endpoint), not the default.
- The whole surface **rehydrates from `events.jsonl` + `.secret` alone** (no LLM).
- Root `/` serves the setup page before config, the kernel view after. Full suite
  green; preflight (the pre-onboarding `home/` body) unaffected.

**Decision: keep.** It changed the kernel (`InstallBuiltin` + the init/serve hooks
+ `onboarding.go`), justified the PoC way: it adds no new trust surface (the
signed-install path is the existing one) and it makes the LLM-wiring an
in-paradigm projection rather than out-of-band env editing — the
accessible-entry-point anchor reaching the brain itself. The token-split also does
real conceptual work: a secret is the one thing that must *not* be in "the log is
the only truth," so it lives beside the log like `.secret`/`.identity`, and the
projection is structurally incapable of leaking it. Honest gaps: Anthropic is in
the picker but needs its adapter (OpenAI-compatible providers work as-is); the
"human" choice records the intent but still points at `brain/bridge.py` rather
than an in-page bridge (the web human-in-the-loop brain is the next tier); and the
committed `home/` body predates onboarding (a fresh `self init` gets the page).

---

## Slice 14 — the human-in-the-loop brain, as a coding interview (VALIDATED)

**Hypothesis.** Slice 13 made wiring an LLM a page, but a fresh user with *no* LLM
and no key still hits a wall — and every slice so far was validated only against a
stub/fake brain, never a real cognition. Both gaps have one answer: a *human* is a
real brain. If "the brain is a process" (Slice 12) is true, a human answering
through a page is a valid brain — and it's the zero-dependency first run. The
asymmetry to resolve: an LLM brain answers synchronously inside `self think`; a
human answers minutes later. The claim under test: the human brain fits the
event-log model with no blocking and no bridge server — park the question as an
event, render it as a projection, let a person answer it through a form.

**Slice.** Two more baby-kernel builtins (signed at init, no LLM), plus a brain
mode:
- With provider `human`, `self brain` does not call an LLM. It **appends a
  `brain.asked {id, prompt}` directly to the log** (it can't ride out on stdout:
  `self think` is a pure query that appends nothing, so the parked question must
  persist itself) and returns a placeholder reply.
- An **`interview` projector** folds `brain.asked` / `brain.answered` into the set
  of *open* questions and renders each with a form — a free-text reply and an
  optional capability declaration — posting to `/run/answer`.
- An **`answer` command** emits `brain.answered {id}` (closing the question), the
  reply as a `chat.message`, and — if the human wrote a declaration — that event,
  so the strange loop can grow the capability. The human is the brain; with a
  compiler also wired, the human is the architect and the LLM the builder.

**Evidence (e2e, no LLM).** Choose `human`; `self think "what should I build?"`
parks a `brain.asked` and replies "open /interview"; the interview projection
(read back as the oracle) shows the question + answer form; `self run answer <id>
"…" ""` emits `brain.answered` + the assistant `chat.message` and the question
flips to "no open questions". Selftest covers all four onboarding capabilities
(`configure`/`setup`/`answer`/`interview`, 4/4) — they ship examples. The hybrid
grow path (human declaration + compiler) builds the capability when a compiler is
present. Full suite + preflight green.

**Decision: keep.** It is the accessible-entry-point anchor reaching its limit —
self now runs end-to-end on a machine with nothing installed but self itself, and
the first *real* (non-stub) brain in the whole lineage is a person. No new trust
surface: `answer` is an ordinary command, the question/answer are data events, and
growing still goes through the signed compiler. Honest gaps: the pure no-LLM grow
(the human pastes a *script*, not a spec) is deliberately not built — it would
install operator-authored bytes, the foreign-code line Slices 4–6 drew, and
deserves its own decision; the interview is single-tier (no threading/identity of
who answered); and a parked question has no timeout/expiry.

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

Slice 12 — the brain is a swappable process. `self brain` is the primitive
(prompt in, event JSONL out); `self think` is the backward-compatible call
interface that spawns the configured brain process (`$SELF_BRAIN`, default
`self brain`) and folds its events back into the legacy `{response, declarations}`
JSON. Any program that reads a prompt (argv) and emits event JSONL is a valid
brain — here a 6-line script, no LLM:

```sh
go build -o self .
export SELF_HOME=$(mktemp -d)
self init
cat > /tmp/brain.py <<'PY'
#!/usr/bin/env python3
import sys, json, os
p = sys.argv[1] if len(sys.argv) > 1 else ""
caps = os.listdir("capabilities/commands")          # the brain reads the garden itself
print(json.dumps({"name":"chat.message","payload":{"role":"user","content":p}}))
print(json.dumps({"name":"chat.message","payload":{"role":"assistant","content":f"I see {len(caps)} commands; you said: {p}"}}))
PY
# self think resolves $SELF_BRAIN, runs it, returns the legacy JSON (appends nothing):
SELF_BRAIN="python3 /tmp/brain.py" self think "what is here?"
# → {"response":"I see N commands; you said: what is here?","declarations":[]}
#
# `self brain` is the DEFAULT brain implementation (the in-tree LLM) — what
# $SELF_BRAIN points at when unset; configure an LLM to run it directly.
```

Existing gardens need **no migration**: `self think`'s output is unchanged, so an
already-compiled `chat` (or `grow-spec`) keeps working; only the plumbing behind
`self think` became a swappable process.

Slice 13 — wire in your LLM from a page, no env editing, no LLM needed to do it:

```sh
go build -o self .
export SELF_HOME=$(mktemp -d)
self init            # installs the signed `configure`+`setup` surface (no LLM)
self live            # open http://localhost:7777/ — you land on the setup page
# pick a provider, paste a token, save → POSTs /run/configure, which writes:
#   provider/url/model  -> a brain.configured event (in the log, portable)
#   the token           -> $SELF_HOME/.brain-key (0600, NEVER in the log)
# the same from the CLI:
self run configure ollama http://localhost:11434 llama3.2 ""   # local, no token
grep brain.configured "$SELF_HOME/events.jsonl"                # config is in the log
grep -c "$(cat "$SELF_HOME/.brain-key" 2>/dev/null)" "$SELF_HOME/events.jsonl" 2>/dev/null  # token is NOT
```

The setup surface rehydrates from `events.jsonl` + `.secret` with no LLM, like
any capability — because the kernel authored and signed it at init.

Slice 14 — the human-in-the-loop brain (a coding interview). With provider
`human`, there is no LLM: self parks each prompt and a person answers it through a
page — pure event log, zero external dependencies.

```sh
self init
self run configure human "" "" ""          # choose the human brain
self think "what should I build?"           # parks a brain.asked; reply: "open /interview"
self live                                   # open /interview — the open question + an answer form
# answer in the page (or CLI): a plain reply, and/or a capability declaration to grow
self run answer <id> "build a timer" ""     # emits brain.answered + a chat.message; question closes
```

A plain reply works with no LLM (you ARE the brain). Growing a capability from a
declaration still routes through the compiler — so "human spec + LLM build" is a
hybrid that needs a compiler; the pure "human writes the script" path is a
deliberate non-goal for now (it is a kernel trust-model decision, since installing
operator-authored bytes is exactly the foreign-code line Slices 4–6 drew).
