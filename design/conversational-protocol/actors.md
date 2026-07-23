# Round three: actors, not agents

A continuation of [swarm-frame.md](swarm-frame.md). Round two established
that self instances in one trust boundary can converse over live surfaces
with zero kernel change. This round names what that creates: not a swarm of
agents, but a cast of **actors** — instances that specialize over time, are
invoked when needed, explore each other's surfaces, and grow an identity.
The price named in the conversation: cold start on every interaction. The
upside: a permanent yet malleable identity — an actor with a stake. And an
ordinary frontier session can consult them as oracles: knowledge stores
with conversational progressive unfolding, backed by proof.

This round pressure-tests that idea. It survives, and several of its parts
turn out to be stronger than they first appear — including the price, which
on inspection is not a price at all.

---

## 1. Actors invert the continuity axis

The distinction between agent and actor is architectural, not rhetorical.

An **agent**, as deployed everywhere today: the continuity lives in the
mind — the context window, the session store, the running process. The
state around it is disposable scratch. Kill the session and the identity is
gone; spawn another and it is a stranger with the same job title. Agents
are interchangeable *because their memory is in the wrong place*.

An **actor**, in the sense this design produces: the continuity lives in
the **body** — the log, the grown capabilities, the surfaces. The mind is
stateless by contract (the repo already insists on this: each ask starts
cold; resist the session store). Identity is in the instance, intelligence
is a visitor.

The theatrical reading is exact, and it is the reason the word is right:
an actor is a *character* that persists across performances while the
performer changes. The mind is the performer; the body is the character.
Which yields a property no agent architecture has:

> **An actor's identity is model-portable.** Swap `SELF_MIND` from one
> model to another — cheaper, newer, local, frontier — and the actor is
> still itself: same memories, same capabilities, same track record, same
> voice rendered from the same log. Your cast does not churn when your
> models do.

Sovereignty was the bet that produced this; the actor frame is where it
pays out. Metis accumulated over a year does not evaporate on a model
upgrade, because it was never in the model.

## 2. The lineage is the actor model, literally

This is not a loose metaphor. Hewitt's actor model — the formal one — is:
everything is an actor; each actor has an address, private state no other
actor can touch, and communicates only by messages. It was designed as the
alternative to shared-memory concurrency and its races.

A cast of selfs is an actor system with unusually good materials:

| Actor model | Cast of selfs |
|---|---|
| address | a bind address; a URL |
| private state | the event log — append-only, signed, replayable |
| message | an account: a form POST on a surface, or a severed directory |
| behavior change on message | learn: local re-expression behind the local key |
| no shared memory | no shared memory — the rule the whole protocol enforces |

Contrast the substrate agent swarms actually use today — shared
scratchpads, shared context files, agents writing into each other's
prompts. That is shared-memory concurrency, and it has shared-memory
problems: races, lost updates, one poisoned write visible to everyone, no
provenance on any byte. The cast is message-passing all the way down, and
each message lands in an append-only log with its origin and moment
attached. Forty years of concurrency theory says which of these shapes
scales.

## 3. Cold start is not the price — it is the enforcement mechanism

The conversation framed cold start as the cost of actor identity. Look
closer: it is the *guarantee* of the oracle property, and the README
already says so in different words ("if a cold mind orients slowly, that is
design pressure aimed at the right target: improve the projections").

A warm agent's knowledge lives partly in its context window — the one place
that dies, cannot be inspected, cannot be proven, and cannot be given. A
cold actor's mind arrives knowing nothing local; therefore **everything the
actor knows must be in the body** — rendered on surfaces, replayable from
events. Cold start is the forcing function that keeps the oracle honest: if
the actor "knows" something its surfaces cannot show a fresh mind, it does
not actually know it — some session merely did, once.

Two corollaries:

- **Orientation time is a fitness metric.** A maturing actor gets *faster*
  to wake, because harvest discipline and better projections are exactly
  what reduce a cold mind's ramp. Slow orientation is a symptom you can
  read off the body and fix in the body.
- **The price and the upside are the same fact.** Permanent identity and
  cold start are not a trade-off pair; they are one design decision seen
  from two sides. You cannot have the provable, givable, model-portable
  identity without evicting knowledge from the mind — and evicting
  knowledge from the mind is what cold start *is*.

## 4. The oracle: a witness, not a database

"Knowledge store with conversational progressive unfolding backed by
proof" — unpack each term against what exists:

- **Progressive unfolding** is already the surface discipline
  (LESSONS.md): a small front page, depth one link away, nested
  projections. A consulting session reads exactly as deep as its question
  requires.
- **Backed by proof**: every claim on a surface is a pure projection of
  events; `/events` is attached; any statement traces to the moments it
  derives from, and `lesson.learned` receipts show what the actor absorbed
  from whom. A consulting mind can check the working.
- **Conversational**: when the rendered account does not cover the case,
  the form is right there — the consulting session asks, the actor's mind
  wakes cold, orients from its own body, and answers *from evidence*.

The contrast worth naming is with retrieval. A RAG store returns chunks:
no curation history, no provenance chain, no one home to ask a follow-up.
An actor returns a *telling* — an account someone (its own past minds)
composed on purpose, with the evidence attached and the author still
addressable. The difference is the difference between a database and a
**witness**. You can cross-examine a witness — and cross-examination is
precisely the Conversational Protocol of rounds one and two. The oracle
use-case is not an application bolted onto the protocol; it is the
protocol, consumed by a frontier mind instead of a peer body.

This is also the README's Vision section made mechanical: frontier
intelligence consults durable local metis, each doing what the other
cannot. The cast is the "missing substrate" that paragraph promises —
concretely, a `CLAUDE.md` that stops carrying stale prose about the build
system and instead says: *for CI behavior, consult the build actor at
:7703; for schema history, the migrations actor at :7704.* The phone book
replaces the encyclopedia, and the entries answer back.

## 5. Stake, mechanically

"It has a stake now" — the word earns its place because the log makes
reputation *computable* and loss *possible*:

- **A track record that cannot be rewritten.** The actor's past answers,
  claims, learns, and gives are events. Whether its testimony held up is
  replayable by any later mind. An actor caught confidently wrong carries
  that moment forever — malleable in what it becomes, permanent in what it
  did. That is identity in the sense persons have it, not in the sense
  config files do.
- **Selection with teeth.** Actors whose accounts prove useful get
  consulted more, given more, grown further; actors whose surfaces go
  stale or whose answers waste threads get retired. Differential survival
  of sharing practices — the fractal bet from the original proposal §4.4 —
  needs units of selection with persistent identity, and agents never had
  one. Actors are the unit.
- **Honesty pressure without a moralizing mechanism.** Nothing enforces
  good testimony except that bad testimony is permanent and attributable.
  The same pressure that keeps the timers projection saying "waiting for a
  tick" instead of "overdue" — say only what the log supports — becomes,
  at actor scale, a reputational instinct.

And the lifecycle is already in the protocol, which is the elegance check:
an actor **reproduces by giving** — spawn a fresh body, give it the
accounts that define the niche, let it learn and diverge (fission via
accounts). Two actors **merge** by one learning the other's gives and the
donor retiring. Specialization, reproduction, death — all expressible as
give / learn / retire, no new mechanism.

## 6. Economics, and why cheap minds suffice

One honest cost question: every consultation spawns a mind — tokens and
latency per question. The answer falls out of §3 and is one of the
strongest claims in the frame:

> **Actors lower the intelligence bar for useful answers, because the
> knowledge is in the body, not the weights.** The answering mind does not
> need to *know* anything about the domain; it needs to read surfaces,
> follow evidence, and compose. That is well within reach of small local
> models — and the repo already ships the seam (`examples/mind-api`,
> `mind-http-server`) for exactly this.

So the cast runs a natural two-tier economy: cheap local minds animate
actors for routine testimony (fast, free, private, always-on), and the
frontier session — the expensive general intelligence — is the *client*,
spending its capability on the novel problem while the actors supply
grounded context. The proposal's §4.3 intuition ("scales with inference
speed") lands here: as local models densify, the cast's answering tempo
rises, and none of the architecture moves.

## 7. Failure modes, named before they are met

- **Granularity.** Too few actors → one monolithic log and the O(n) pain
  concentrated; too many → cold-start tax multiplied and knowledge
  fragmented. Heuristic: one actor per *ownership boundary that
  accumulates history worth interrogating* — a subsystem, a recurring
  concern, a role. When in doubt, start as a lesson inside an existing
  actor; promote to a body when its surfaces stop fitting the host's
  front page. Fission is cheap (§5), so err small.
- **Staleness.** An unmaintained actor is a confident witness with old
  memories. The remedy is the honesty pattern already in the lessons:
  surfaces render their own recency ("last harvested…"), so trust can be
  read off the page. A consulting mind should weight testimony by its
  moments — which it can, because moments are preserved.
- **Bypass.** The temptation for a powerful consulting session is to skip
  the surface and read `events.jsonl` directly — or worse, write it.
  Reading the raw log is legitimate (it is the proof). Writing anywhere
  but through commands dissolves actors back into files and forfeits every
  property above. The discipline is the equal-footing contract: consult
  through surfaces, act through forms, no third door.
- **The phone book.** Actors need discovery. Keep it out of the kernel: a
  who's-who is itself a projection — one actor (or any actor) rendering
  the cast list from `account.published` events and operator convention.
  An index that answers questions about who to ask is just another
  witness.

---

## 8. Where the three rounds land

Round one: the conversation between sovereign persons must stay severed
and human-gated — and needs zero kernel change. Round two: the same
conversation inside one trust boundary can go live over surfaces — and
needs zero kernel change. Round three: what live conversation between
specializing bodies *produces* is a cast of actors — durable, provable,
model-portable identities that a frontier mind consults as witnesses — and
it needs zero kernel change.

Three rounds, one kernel, untouched. The probe from round two is now also
the audition: regrow give as a surface, run two bodies, let one testify to
the other's questions — and watch whether what grows is a better account,
or the first member of a cast.

*Round three of an ongoing design conversation.*
