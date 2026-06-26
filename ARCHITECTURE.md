# ks PoC — evolving architecture spec

A living spec, updated after each validated slice. The thesis under test: the
five-repo body of work (cubff → emera → knowledge-seed-protocol → household →
ks) converges on **ks as the minimal kernel**, and everything else — emergence
dynamics, household-style surfaces, sharing — can be added **as seeds**
(LLM-compiled commands/projectors over an append-only log) rather than as
compiled-in subsystems. Re-growing the kernel instead of the seed layer is a
failure, not progress.

Build method: small testable slices. Hypothesis → minimal slice → evidence →
validate against the anchors → keep/refine/refute → log it. The build log is
itself a ks seed (`build.hypothesis` / `build.evidence` / `build.decision`
events, rendered by a `buildlog` projector) — the process is a projection of
itself.

Real invariants checked (never fabricated): replay determinism, seed
portability/adaptation, kernel-LOC delta, tests. Everything else is labelled
qualitative.

---

## Slice 1 — emergence as a seed (VALIDATED)

**Hypothesis.** emera's energy/selection ecology can run as a ks seed (a `tick`
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

Run: `ks invoke tick` × 120.

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
items, grouped into lanes) can run as a ks seed — three commands + a `board`
projector — with zero kernel changes, driven by plain HTML forms AND by the
brain calling the commands as tools.

**Slice.** A seed declaring `capture`, `move`, `resolve` commands and a `board`
projector (folds `memory.captured` / `item.stage_changed` / `item.resolved`
into lanes inbox/this_week/waiting/done; each card carries a move `<select>` +
resolve button as plain forms posting to `/invoke`).

**Evidence.**
- CLI `capture` works; **form-driven move & resolve via Post/Redirect/Get**
  (303 back to `/board`, zero JS).
- **The brain drove the board in plain language**: "add a task to call the
  plumber" → the brain called the `capture` command tool → the item appeared in
  the inbox lane. (Commands-as-tools from the act-verb work.)
- **Complexity (view layer, apples-to-apples)**: ks `board` projector = **46
  LOC**; household's kanban projection = **279 LOC of Go**. The whole ks board
  capability is a **7-line declaration** → 62 LOC of compiled scripts. To change
  a capability: household edits Go + rebuilds; ks edits a declaration, recompiled
  live. (Honest: household's command/aggregate Go is shared across its whole
  domain — meals, recipes, sync — not just the board, so only the projection
  layer is a clean comparison.)
- **Replay deterministic** ✓. **Kernel Go changed: 0** ✓.

**Decision: keep.** The accessible-entry-point anchor and the household→ks port
both hold; an everyday surface runs as a seed, form-driven and brain-driven,
with zero kernel change. Limit: board slice only (no meals/recipes/Planka/relay).

**Next (slice 3 candidates).** Receiver-adaptation across nodes (plant the board
seed into a foreign garden whose vocabulary differs); or port a second household
surface (meals) to test breadth; or the deepen-emergence path (kept out of ks —
emera stays a sovereign compute node that *emits events into* a ks commons).

---

## Slice 3 — the seed format upgrades itself (VALIDATED)

**Hypothesis.** Planting from a *holistic spec* needs zero kernel change. A
planted `plant-spec` command logs the spec (`seed.spec`, provenance) and calls
the brain to derive the atomic `command.declared`/`projector.declared` events;
the kernel's existing strange-loop hook compiles them. So the higher-level seed
format is itself a seed.

**Slice.** `board` re-expressed as one holistic markdown spec
(`poc/plant-spec/board-spec.example.md`: Intent / Capabilities / Behavior /
Examples / Content). A `plant-spec` command (planted normally). Then
`ks invoke plant-spec board-spec.md`.

**Evidence.**
- `plant-spec` is a **planted command** — a seed, not kernel code.
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

## Slice 4 — the strange loop carries code, not just a spec (VALIDATED)

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
exist**: nothing ever read a `script.compiled` payload back into the registry.

Predicted: the missing half is a single primitive — let the kernel **act on**
`script.compiled` (install the bytes verbatim, no LLM), the same way it already
acts on the two `*.declared` events. With that one primitive, deterministic
self-replication, generational evolution, and rollback all become expressible —
and rollback/restore needs **no kernel command**, because a command already
receives the whole event log on stdin, so it can find and re-emit any past
`script.compiled` itself.

**Slice.** The kernel's compile hook (`CompileDeclarations`, run at invoke; and
the plant replay loop) now also honours `script.compiled`: it installs the
script verbatim into the registry via one shared `InstallScript` helper. The
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
- `poc/replicant` planted with **no LLM** (no `KS_LLM_*`, no stub): plant
  reported *"shipped verbatim — skipping compile"* and installed both the
  command and its `lineage` projector. Invoked 4×: the registry script stayed
  **byte-identical (sha256) across all 4 generations**, the generation counter
  advanced 1→4, and `lineage` auto-rendered all four — proving the declaration
  still wired the projector even though its binary was shipped.
- The self-install is logged **exactly once per invoke** (no double-logging).
- A `restore <name>` seed re-emitted an older `greet` version and the kernel
  installed it verbatim — `greet` v2 → v1 — with **no kernel code for rollback**.
- The spec-loop boundary is now documented by its contrast: the same
  self-redeclaration under the *spec* path drifts (stub → generic), under the
  *code* path it does not.

**Decision: keep — with an honest caveat.** This slice **did change the kernel**
(~80 Go LOC: `InstallScript` + the hook case + plant's ship-aware skip),
unlike slices 1–3 which were zero-kernel. By the PoC rule ("re-growing the
kernel is a failure"), that demands justification: the primitive is minimal and
it *moves capability out of the kernel*. Rollback/restore would otherwise have
been a kernel subcommand (read log → write registry, which only the kernel can
do); honouring `script.compiled` makes restore, replication, and quines all
**seeds** instead. The kernel's acted-on set grows from two events to three, but
the alternative was a bespoke `ks restore` plus no replication story at all.
Net: one small primitive, a large expansion of what the seed layer can express.

**Honest gaps.** Installing arbitrary bytes verbatim is, in production, an
arbitrary-code-execution path (a malicious seed could ship a hostile script) —
acceptable in PoC mode, but a real seed-trust / signing story is owed before
this ships. The brain can't yet emit `script.compiled` directly (its `declare`
tool is scoped to the two `*.declared` events), so brain-driven rollback isn't
wired — only command/seed-driven. And replay-from-log still isn't a kernel
operation: the registry is mutated in place, not rebuilt from the event stream.

**Next (slice 5 candidates).** Let the brain restore/replicate (widen `declare`
to `script.compiled`, so "undo that" becomes brain-callable); a seed-signing /
trust tier gating verbatim installs; or rebuild-registry-from-log so a fresh
receiver reaches identical registry state by pure replay.

---

## How to reproduce

```sh
go build -o ks .
export KS_HOME=$(mktemp -d) KS_LLM_*=...   # an LLM, or the Claude bridge, compiles the seeds
ks init
ks plant poc/buildlog        # buildlog projector + slice-1 hypothesis
ks plant poc/emergence       # world + tick + population
for i in $(seq 1 120); do ks invoke tick; done
ks serve                     # browse /population and /buildlog
```

The seeds are declarations; an LLM compiles them into the `tick`/`population`/
`buildlog` scripts at plant time (garden-aware). Same seed, different receiver,
different binary — but the same emergent behaviour.

Slice 4's `poc/replicant` needs **no LLM** — it ships its code, so the loop is
deterministic:

```sh
go build -o ks .
export KS_HOME=$(mktemp -d)        # no KS_LLM_*, no stub: nothing to compile
ks init
ks plant poc/replicant             # "shipped verbatim — skipping compile"
ks invoke replicant                # generation 1; re-installs its own exact source
ks invoke replicant                # generation 2; registry script byte-identical
ks serve                           # browse /lineage to watch the generations
```
