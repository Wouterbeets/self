# crossnode — does knowledge survive translation between strangers?

The knowledge-seed-protocol's central claim is that knowledge can move between
**sovereign systems that don't share a vocabulary**: you share the *method and
the evidence*, not a frozen conclusion or a foreign binary, and the receiver
re-derives it against its own reality. Every earlier slice in this repo was
*one* garden, or succession within one body. This is the first run of the actual
cross-node claim: **two gardens that speak different languages, one shared seed.**

## the setup

Two sovereign gardens record the same kind of local reality under **different
vocabularies**:

| | event | fields |
| --- | --- | --- |
| **North** garden | `observation.logged` | `{what, where, severity}` |
| **Harbor** garden | `report.filed` | `{issue, location, urgency}` |

North grows a piece of knowledge — `hotspots`, a projector that ranks places by
the total severity of what's been observed there — and exports it as a seed
(`hotspots-seed/`): a `projector.declared` carrying the *method* (description +
a reference implementation written in North's vocabulary), **no compiled bytes,
no signature** (a receipt from another home is inert anyway). The seed's
description explicitly invites the receiver to adapt the field mapping.

The *same* seed is then planted in both gardens. The compiler (a Claude, by hand
through `../../brain/bridge.py`) explores each garden before compiling, so each
gets a binary authored for its own vocabulary — "same seed, different receiver,
different binary."

## the result

North, native (`observation.logged`):

```
place        total severity   reports   what
Elm & 3rd    12               3         broken streetlight, pothole, storm flooding
Market Sq    3                2         graffiti, broken bench
```

Harbor, **recompiled from the same seed** against `report.filed` — the compiler
saw the garden had no `observation.logged`, only `report.filed`, and rebound the
method (`location`→place, `urgency`→severity, `issue`→what):

```
place       total severity   reports   what
Pier 7      12               3         oil spill, rotten dock planks, bad smell
Boardwalk   3                2         litter, broken railing
```

Hand-check: Pier 7 = 5+4+3 = 12, Boardwalk = 1+2 = 3. Correct. **North's analysis
method ran correctly on Harbor's data, though the two never shared a word.**

## verified, not assumed

- **The two compiled binaries differ** (not a copy): Harbor's consumes
  `("report.filed", "observation.logged")` and maps fields; North's consumes only
  `observation.logged`. Receiver adaptation actually happened.
- **Harbor accepts both dialects.** Fed one North-style `observation.logged`
  event (`Pier 7`, severity 2), Harbor's `hotspots` folded it in: Pier 7 rose
  12 → 14, reports 3 → 4. So Harbor could ingest North's *raw evidence*, not just
  its method — the receiver adapted by extending its filter, not replacing it.

## verification: verify the result, don't trust the compiler

The seed also ships **examples** — input → output-must-contain assertions written
in North's vocabulary (`observation.logged`). They are a portable conformance
contract: when *any* receiver recompiles the seed, the kernel runs the new binary
against them **before it installs**, and a binary that fails them is rejected.
A `script.verified` receipt records the outcome either way.

Crucially, the examples are in the *author's* vocabulary, so they only pass on
the receiver if its adaptation **extends** rather than **replaces** — i.e. it
keeps consuming `observation.logged` while adding `report.filed`. That turns
"good adaptation" from a judgment call into a checkable property. Demonstrated
end-to-end in Harbor:

```
attempt 1 — adaptation consumes ONLY report.filed (drops North's dialect)
            → script.verified passed=False 0/2  → REJECTED, not installed
attempt 2 — adaptation consumes BOTH, maps fields
            → script.verified passed=True  2/2  → installed
```

The rejection is in Harbor's own log as evidence (the failing receipt lists the
missing strings). So a stranger node can recompile shared knowledge to its own
vocabulary *and prove* the result still honors the original contract — without
trusting the compiler that produced it. North, recompiling natively, passes the
same examples 2/2.

## reproduce

```sh
go build -o self .
# wire a compiler/brain (any OpenAI-compatible endpoint, or ../../brain/bridge.py)
export SELF_LLM_URL=... SELF_LLM_MODEL=...

# North: a garden that speaks observation.logged
export SELF_HOME=$(mktemp -d); ./self init
./self grow poc/crossnode/seed-north-data      # its data (memory seed, no compile)
./self grow poc/crossnode/hotspots-seed        # grows hotspots natively
./self show hotspots

# Harbor: a stranger that speaks report.filed
export SELF_HOME=$(mktemp -d); ./self init
./self grow poc/crossnode/seed-harbor-data     # different-vocabulary data
./self grow poc/crossnode/hotspots-seed        # SAME seed — the compiler adapts it
./self show hotspots                            # ranks Harbor's data, correctly
```

The exact compiled bytes will differ per receiver (the compiler re-authors every
time) — but the method, and the ranking it produces, will hold. That is the
point: you inherited the knowledge, not the code.
