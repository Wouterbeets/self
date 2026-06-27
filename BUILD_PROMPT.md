# Build prompt: extend self toward the vision, iteratively and on evidence

## Your job
You are the **single driver** of an iterative build. You extend 
minimal kernel (self, formerly ks) toward the vision below, in small testable slices. You may
spawn parallel sub-agents ONLY for exploration or independent verification —
never to run the build loop, which is sequential (each slice's evidence decides
the next). No big-bang design. No new repo. You grow self.

## The body of work is a time series, not a merge
These repos are one idea annealing toward a minimal shape, in order:
- cubff-culture — self-replicating/evolving programs (replication works)
- emera — strict emergence from minimal primitives, energy/resonance dynamics
  (emergence works, but provenance is opaque)
- knowledge-seed-protocol — the manifesto: sovereign, auditable, replayable
  knowledge sharing; receiver-controlled adaptation
- household — a real event-sourced node + sharing/trust machinery, but every
  capability is hand-written Go (no self-modification; large)
- self (formerly ks) — the current minimal synthesis: event log + LLM-compiled commands/
  projectors + strange loop + bare-HTML projections + serve-time enrichment

**self is the base. You extend it.** Port household's infrastructure and emera's
dynamics IN as seeds/projectors/commands — never as compiled-in subsystems.
Deduce intent; do not copy code literally.

## Non-negotiable anchors
SLICE-LEVEL (every slice must hold these; a violation is a refutation):
1. Append-only event log is the single source of truth.
2. Projections are pure functions of the log (replayable, inspectable).
3. Capabilities are LLM-compiled from declarations (the strange loop), not
   hand-coded Go. New Go is for the kernel/body only.
4. Bare semantic projections + serve-time enrichment (no per-projector styling).
5. Provenance survives: every capability and change is itself a logged event.
6. Minimal: if a slice grows the kernel instead of the seed layer, that is a
   FAILURE, not progress.

DIRECTIONAL (the why; do NOT try to "validate" these in a slice):
emergence-driven specialization, receiver-controlled remapping across nodes,
public/commons cooperation, local-first sovereignty, democracy-scaled. These
guide choices; they are not acceptance criteria.

## The living log IS a self seed (non-optional)
Record the build using the product. Append `build.hypothesis`,
`build.evidence`, `build.decision` events to the log; write a `buildlog`
projector that renders them. The PoC must be able to show its own construction
history as one of its projections. The process demonstrates the vision.

## Evidence: real invariants only — no fabricated metrics
You may ONLY claim a metric you actually produced. Show the raw command output,
not a prose summary of it. The checkable invariants:
- Determinism: replay the same log twice → byte-identical projection.
- Portability/adaptation: grow a seed whose event vocabulary the garden lacks
  → does the compiler remap it? Show the compiled script + rendered output.
- Complexity delta: LOC and dependencies vs. the baseline repo (`git diff
  --stat`, `wc -l`).
- Tests: `go test ./...` (and any replay data that exists in the repos).
Everything else (clarity, usefulness, "feel") is QUALITATIVE — label it as a
judgment, never dress it as a number. If you cannot measure it, say so.

## The loop (sequential)
For each slice: (1) state ONE hypothesis as a `build.hypothesis` event;
(2) implement the minimal code to test it; (3) run it, capture raw evidence;
(4) check against the slice-level anchors; (5) append a `build.decision` (keep/
refine/refute) with rationale; (6) update the spec. Keep what survives evidence;
discard what doesn't. Commit each validated slice with rationale in the message.

## First slice (attack the hardest hypothesis first)
Do NOT start with "event log + kanban + seed I/O" — self already has those;
re-deriving the baseline proves nothing.

Slice 1 hypothesis: **"emera's emergence can run as a self seed."** Implement a
projector (or command) that applies an energy/selection heuristic over the event
stream — e.g. scores events by a fitness rule and culls/ranks them — using ZERO
bespoke engine code, only the seed mechanism. Evidence: does it run on a real
log, replay deterministically, and stay in the seed layer (no kernel growth)?
If emergence-as-a-seed holds, the synthesis thesis stands. If it can't, you've
found the real boundary on day one. Log the result either way.

## Bound
Stop after 3–5 validated slices. Then write the roadmap. Do not run forever.

## Outputs (iterative)
- The living build-log seed (events + buildlog projector), in the repo.
- An evolving `ARCHITECTURE.md`, updated after each validated slice.
- Runnable code growing on self, with build/run instructions and explicit
  comparison points to the baseline repos.
- A closing summary: which anchors held (with evidence), which slices were
  refuted, remaining gaps, next-iteration roadmap, and follow-up prompts.

## Style
Favor dynamics/emergence over orchestration. Local-first, sovereign, minimal.
Household-style surfaces are the accessible entry point. Be bold in hypotheses,
ruthless in discarding what fails evidence. The process is part of the demo.

---

_Slice 1 has been run; see `ARCHITECTURE.md` and `poc/`. Result: VALIDATED
(r=0.901, deterministic, zero kernel changes)._
