# Heresies of the chosen body

This fork asks what `self` becomes when its primary social fact is not a
stranger at a boundary, but two sovereign participants who have deliberately
chosen a shared life or undertaking.

The answer is not "trust everyone". Familiarity is not verification, consent
is not permanent, and affection does not remove asymmetry. The answer is a new
body for the durable **between**: neither participant's private body, and not a
sync of both, but a commons they constitute together.

`lessons/covenant` is the executable first probe. It runs on the current
kernel. The larger design below names what a real co-custodied body still
needs.

## Thesis

`self` has three forms:

1. **Sovereign body** — one participant's memory, capabilities, and key.
2. **Covenant body** — a shared, bounded history constituted by named members.
3. **Account** — a deliberate crossing between bodies, carrying testimony and
   lineage rather than authority.

The covenant body is not a group chat. It can hold promises, boundaries,
different accounts of one event, repair, shared work, and a bequest. It makes
"between us" durable without making either participant less their own.

Its fundamental operation is not synchronization but **offer and answer**.
Nothing private becomes shared because it was observed, inferred, or reachable.
A participant offers a moment, claim, boundary, or commitment to the commons;
the other can answer it. Mutual facts require mutual acts.

## Chesterton ledger

### Fence broken: accounts are the only inter-body relationship

**What it protected:** sender curation, receiver review, inert foreign kernel
vocabulary, and a human airlock against strangers and prompt injection.

**What carries the purpose instead:** sovereign bodies still cross by account.
A covenant body adds a continuous commons only after an explicit proposal and
acceptance. Private histories never sync into it. Contributions are offers,
membership can be withdrawn, and closure freezes its social authority.

**Status:** broken in the design; approximated by the lesson. A production body
needs authenticated member doors before "accepted by" means more than a claim.

### Fence broken: one local key is sufficient authority

**What it protected:** simple offline reconstruction and the absolute rule that
only the local owner can install executable state.

**What carries the purpose instead:** two planes. One custodian key keeps the
machine operable; a covenant constitution controls social authority. Changes to
membership, boundaries, and closure should require threshold attestations from
the affected members. Capability execution remains locally signed and never
becomes foreign code.

**Status:** proposed, not implemented. The current HMAC cannot prove co-consent.
The lesson records ceremonies; it does not cryptographically enforce them.

### Fence broken: `by` is only a claim with no policy consequence

**What it protected:** epistemic honesty. The kernel knows the door it
witnessed, not who was on the other side.

**What carries the purpose instead:** keep `via` and claimed `by`, then add a
separate attestation layer. A member identity is a pinned public key; an act is
authorized only when its signature verifies and its scope is allowed by the
current covenant. Human-readable pages must still distinguish speaker, door,
and attestation.

**Status:** left standing in the executable probe. Treating `by` as verified
before the attestation layer exists would be a lie.

### Fence broken: no authentication on `/run`

**What it protected:** kernel smallness, with loopback as the actual access
boundary.

**What carries the purpose instead:** capability-scoped member doors: pinned
keys, signed requests, replay protection, explicit allow-lists, and revocation
derived from covenant events. Loopback remains the default; remote exposure is
never implied by membership.

**Status:** proposed, not implemented. Use the lesson only on a loopback body
whose process boundary you control.

### Fence challenged: no automatic exchange

**What it protected:** deliberateness, prevention of propagation, and a pause
before foreign prose reaches a script-authoring mind.

**What carries the purpose instead:** two-lane exchange. The **living lane** may
deliver signed events of an already-negotiated vocabulary between chosen bodies.
The **growing lane** carries declarations, capability requests, and unfamiliar
content through review and local compilation. Relationship does not waive the
airlock around growth.

**Status:** left standing for now. The current kernel cannot mechanically keep
the two lanes apart, so transport remains external and deliberate.

### Fence challenged: no in-kernel clock

**What it protected:** deterministic replay, no receipt spam, and no hidden
autonomous trigger.

**What carries the purpose instead:** nothing better yet. A relationship can
record deadlines and rhythms, but an external tick must still turn a due moment
into an event.

**Status:** standing. Chosen people do not make nondeterminism reconstructible.

### Fence challenged: projections are pure

**What it protected:** byte-stable reconstruction, inspectable causality, and
the separation of intelligence from presentation.

**What carries the purpose instead:** nothing better. The needed broad,
contextual view is another deterministic projection over more event types, not
a model hidden in the render loop.

**Status:** standing. A living view need not be an impure view.

### Fence challenged: the log is append-only

**What it protected:** testimony, responsibility, repair history, and complete
offline reconstruction.

**What carries the purpose instead:** no complete substitute. Cryptographic
erasure, expiry, and mutually acknowledged forgetting deserve experiments, but
silent mutation would let the custodian rewrite a shared past.

**Status:** standing in this fork. The design adds closure and withdrawal of
authority, not retroactive deletion. This leaves a serious consent problem;
see Risk Analysis.

### Fence challenged: the mind is stateless and external

**What it protected:** model portability and the rule that durable identity
lives in the body rather than an opaque provider session.

**What carries the purpose instead:** nothing better. A covenant may summon
different minds for mediation, planning, or play, but none is a member merely
because it spoke.

**Status:** standing.

### Fence challenged: foreign code never installs

**What it protected:** local sovereignty and supply-chain safety.

**What carries the purpose instead:** nothing better. Chosen partners may share
intent and evidence more freely, but each body still authors and signs its own
implementation.

**Status:** standing. Intimacy is not a package manager.

### Fence challenged: no derived authoritative state

**What it protected:** one recoverable source of truth and freedom from hidden
cursors, caches, and competing authorities.

**What carries the purpose instead:** nothing better for the first probe.
Materialized indexes may later accelerate replay, but they must remain disposable
and verifiable against the braid rather than becoming a second authority.

**Status:** standing.

### Fence challenged: `main.go` shrinks

**What it protected:** a legible kernel and the ability to grow culturally
specific behavior as lessons.

**What carries the purpose instead:** the covenant starts as a lesson. Kernel
growth is justified only for facts a script cannot witness: signatures,
threshold authorization, and authenticated ingress.

**Status:** standing.

## Architecture: a braid, not a merge

Each participant retains a private log. The covenant body has its own log and
contains only acts addressed to it. A shared event carries:

- a stable event ID and the covenant's local sequence;
- the locally witnessed door;
- the claimed speaker;
- optional member-key attestations;
- a reference to what it answers or supersedes;
- the originating body's event digest when imported;
- the covenant revision under which it was admitted.

The body derives at least four simultaneous views:

- **commons** — what has been offered and accepted;
- **perspectives** — distinct accounts that must not be collapsed;
- **commitments** — obligations and their present state;
- **lineage** — how the current shared map came to be.

This is MindPalace's state/history dual made social, Emera's cooperative field
made consensual, CuBFF's public commons protected from premature partitioning,
and the Account Protocol's sovereignty retained at every private boundary.

## Rhythm of cognition

The body should alternate between four phases:

1. **Attend broadly.** Render context, silence, contradiction, and unfinished
   repair without deciding what they mean.
2. **Name a possible order.** A person or mind proposes a claim, commitment,
   boundary, projection, or capability.
3. **Act narrowly.** The affected participants accept, counter, decline, or
   execute a bounded verb.
4. **Reopen the field.** Consequences become events and can change the map.

McGilchrist supplies the warning not to mistake the narrow map for the living
whole. Peterson supplies anomaly and voluntary responsibility: order becomes
habitable by confronting what does not fit. Levin supplies nested goal scales:
a command can succeed while a relationship fails, so evaluation must span the
verb, person, covenant, and longer horizon. Weinstein supplies lineage and
trade-offs: present function does not erase origin, and every protection spends
capacity elsewhere.

These are design lenses, not claims that a log is conscious, alive, morally
responsible, or a cerebral hemisphere.

## Intellectual and project lineage

- **MindPalace** contributed the double obligation to preserve immutable history
  and render useful present state. A covenant therefore shows both current
  status and the acts that produced it.
- **Emera** contributed the shared field, provenance-weighted cooperation, and
  the possibility that roles emerge through contribution. This fork adds the
  missing constitutional step: emergence may propose a pattern; people must
  ratify a relationship.
- **CuBFF Culture** contributed the empirical warning that a common cultural
  field can outperform identity-gated private channels. The covenant begins as
  one legible commons rather than a personalized feed.
- **The Knowledge Seed Protocol** contributed declarations over code, lineage
  over bare conclusions, and sovereign intervention on both sides of exchange.
  The account remains the membrane around every private body.
- **Jordan Peterson** contributes maps of significance, anomaly, and voluntary
  responsibility. The projection is a corrigible map; the log preserves what
  did not fit it.
- **Iain McGilchrist** contributes complementary modes of attention: broad,
  contextual openness and narrow, precise grasping. The architecture expresses
  these as a rhythm, not as fake neurological modules.
- **Michael Levin** contributes multi-scale agency and goal spaces. The design
  asks separately whether a verb, participant, covenant, and larger household
  are moving toward their goals.
- **Bret Weinstein** contributes attention to evolutionary trade-offs and the
  distinction between lineage and present function. Every fallen fence must
  disclose the capacity its protection used to buy.

Primary anchors for the four lenses include Peterson's *Maps of Meaning* and
Peterson/Flanders, "Complexity Management Theory" (2002); McGilchrist,
"Reciprocal Organization of the Cerebral Hemispheres" (2010); Levin, "The
Computational Boundary of a Self" (2019) and "Technological Approach to Mind
Everywhere" (2022); Weinstein/Ciszek, "The Reserve-Capacity Hypothesis" (2002),
and Lahti/Weinstein, "The Better Angels of Our Nature" (2005). They are sources
of questions and constraints, not borrowed authority for software claims.

## Constitutional core

The eventual kernel fork should know only mechanisms:

- member-key pinning and rotation;
- signed event admission with replay protection;
- threshold rules over named constitutional event classes;
- capability-scoped ingress;
- closure and revocation enforcement;
- deterministic reconstruction from the braid plus member attestations.

It should not know what friendship, marriage, family, mentorship, or a team is.
Those are covenants expressed as lessons and histories. The mechanism enables
plural forms of chosen life without freezing one culture into the kernel.

## Risk analysis

### Coercion can masquerade as consent

A signed acceptance proves key use, not free choice. Shared bodies need visible
cooling-off periods, asymmetric exit rights where appropriate, and no metric that
turns participation or response latency into loyalty.

### A shared archive can become intimate surveillance

Local-first storage does not create subject consent. Ambient capture is outside
this design. Only deliberately offered, semantically bounded events belong in a
covenant body. Raw audio, location exhaust, and inferred emotional states must
not enter by default.

### Append-only memory can obstruct forgiveness

Preserving testimony supports accountability but can keep injuries permanently
active. Closure, sealed views, mutually agreed redaction keys, and expiry of
ordinary low-stakes material need experiments. This fork does not pretend the
problem is solved.

### Familiarity expands blast radius

Known people possess more context for persuasion, social engineering, and harm.
The living lane must be incapable of declaring or installing capabilities, and
resource-bearing verbs need narrower grants than conversational verbs.

### Custody can become unilateral control

Whoever owns the disk can withhold service, copy plaintext, or fork history.
Threshold attestations make rewriting detectable, not impossible. Independent
replicas and reciprocal export receipts are required for real co-custody.

### Coherence can erase disagreement

Generated summaries can produce a persuasive common story that neither person
actually endorsed. Claims, verification, perspectives, and consensus must remain
different event types. No model may silently turn overlap into agreement.

### The commons can become a score

Contribution weights helped Emera discover cooperation, but relationship energy,
closeness scores, streaks, and responsiveness rankings would invite gaming and
coercion. Patterns may be surfaced as questions, never minted as social fact
without mutual ratification.

### Relationship scope can creep

A household goal is not automatically each member's personal goal. Events and
capabilities need explicit scope, and a covenant must not act through a private
body without a revocable grant.

## Falsifiable first trial

The first serious trial should use two fresh sovereign bodies and one covenant
body:

1. Both participants independently accept one constitution.
2. Each offers one curated moment; no other private event appears in the commons.
3. They record incompatible perspectives without either being overwritten.
4. They jointly accept and fulfill one commitment.
5. One withdraws and replaces a boundary.
6. One requests repair and the other answers without erasing the injury.
7. They close the covenant and reconstruct it independently offline.

The trial fails if foreign code installs, an unaccepted private event appears,
one custodian can fabricate threshold consent undetectably, prior testimony can
be rewritten, or authority survives closure.

## The wager

The stranger-safe architecture remains the membrane between sovereignties. The
chosen-body architecture grows inside deliberate relationship. Together they
form a scale-free social substrate: private selves, durable betweens, households,
teams, and cultures can each have a body without requiring one total body.

The model remains visiting intelligence. The bodies keep history. The accounts
carry lineage. The covenants carry obligation. Culture is what compounds between
them.
