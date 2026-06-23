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
