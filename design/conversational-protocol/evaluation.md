# Evaluation: the Conversational Protocol proposal

A review of [proposal.md](proposal.md) against the codebase as it stands
(`account.go`, `commands.go`, `orchestration_core.go`, `server.go`,
`main_test.go`, the lessons). The proposal asked for strengths, weaknesses,
missing pieces, hidden assumptions, and alternative framings. This document
delivers those, then answers the nine open questions, then recommends a
growth path.

**Verdict in one paragraph.** The reframing is right and the minimal form is
even cheaper than the proposal believes: the asynchronous question-account
version requires **zero kernel change** — it is expressible today as a
convention plus one lesson, and the thread identifier it needs already
exists in both logs as `record_sha256`. The interactive temporary projection,
by contrast, is not a mild extension; it quietly deletes the protocol's most
important safety property (the human airlock between a foreign party and the
mind that authors signed scripts) and requires an identity layer the system
does not have. The proposal also misses two concrete mechanical gaps —
answer-side curation and deposit idempotence — that matter more than
anything it lists under "risks." Recommendation: adopt the reframing, grow
the conversation as a lesson, and treat the live surface as a separate,
later, opt-in experiment — not as part of the Account Protocol.

---

## 1. What the codebase already supports (the proposal undersells this)

### 1.1 The minimal form needs no kernel change at all

Walk the existing machinery:

- A question is an account. `intent.md` carries the question in prose
  ("who this is from, what it means, what you hope it becomes" — the
  existing stub text in `account.go` already frames an account as an
  *address to a person*). `record.jsonl` optionally carries the receiver's
  context events as evidence.
- Delivery is already out-of-band. Nothing in the kernel moves account
  directories; they travel however the operator moves directories. A
  question-account travels the same way. No new channel.
- The giver ingests the question with the ordinary `self learn`: the
  question events deposit verbatim (`commands.go`, deposit loop), the
  giver's mind reads them against local state, and a `/dialogue`-style
  projection surfaces open questions. The giver answers with an ordinary
  `self give` of the answer events.
- Both sides remember: `account.given` on one side, `lesson.learned` on the
  other — exactly the recording property the proposal asks for, already
  pinned by tests (`main_test.go`, account round trip).

Every one of the proposal's five "key properties" (§3) is preserved by this
composition *without touching the kernel*: no code crosses, installation
stays local and signed, the negotiation is events in both logs, either party
can stop (just don't learn the next account), and the final capability is a
local re-expression.

The proposal's §5 claim "without requiring kernel changes for the minimal
form" is therefore correct but understated: it is not that the kernel change
is small — it is that the conversational protocol is *already latent in the
existing protocol*, and what is missing is only vocabulary and practice.
That is exactly the shape the repo already honors: files and timers were
both regrown as lessons after being evicted from the kernel. The
conversation should be born a lesson, not earn kernel status it would later
have to be evicted from.

### 1.2 The thread identifier already exists

The proposal's minimal extension needs correlation: which give is this
question about? The system already mints the perfect handle. `give` writes
`record_sha256` into `manifest.json` and into the giver's `account.given`
event; `learn` attests the hash of what was actually deposited in the
receiver's `lesson.learned` receipt (`commands.go`). The same content hash
is therefore already present **in both logs** the moment an account is
learned.

A question-account that says "re: `record_sha256=ab12…`" in its intent (or
as a field on its question events) is joinable to the original exchange from
either log, offline, forever — with no new attestation machinery. The
conversation's thread ID is the content hash of the account under
discussion. This is the single most load-bearing detail the proposal
missed, and it is free.

One caveat: the correlation is *carried*, not *attested*. The
`lesson.learned` receipt signs what was deposited, not what it replies to.
For now that is acceptable — the reply linkage is evidence, like everything
else in a record — but it is worth stating so nobody later mistakes a
claimed `re:` for a proven one.

### 1.3 The kernel-vocabulary gate already protects the conversation

`kernelVocabulary` in `account.go` is an exact-name set. Dialogue events
named outside it (`dialogue.question`, `dialogue.answer`, …) travel raw in
records, land verbatim, and are inert — precisely what a conversation needs.
This is also a strong argument for keeping conversation events *out* of the
kernel: the moment `dialogue.question` becomes something the kernel acts on,
it must join `kernelVocabulary`, and then it can no longer travel raw — the
protocol would strangle its own transport. The conversation can only remain
portable if the kernel stays deaf to it. (Avoid an `account.*` prefix for
dialogue events, though: the gate is exact-name so `account.question` would
technically travel, but overloading the kernel's own noun invites confusion.
Pick a fresh prefix.)

---

## 2. Strengths of the proposal (confirmed against the code)

- **The reframing is correct and clarifying.** "Give/learn is the degenerate
  one-turn case of a conversation" is a better story than "give/learn plus a
  patch for questions." It also matches the intent stubs the kernel already
  writes, which are literally letters ("say who you are… what you hope it
  becomes elsewhere").
- **It composes with the capability flavor.** `self give command/<name>`
  already ships the full lineage (declarations + receipts renamed
  `lineage.*`). A receiver interrogating that lineage and getting a refined
  cut back is a genuinely new capability the one-shot form cannot express.
- **The evolutionary-pressure argument (§4.4) is real.** Because questions
  and answers are themselves accounts, better questioning *practices* can
  travel as lessons. The protocol's substrate is self-hosting in exactly the
  Levin-flavored way the proposal wants. This is the strongest long-range
  idea in the document.
- **The risk list in §6 is honest**, especially "the protocol does not solve
  taste." Correct: it should not try.

---

## 3. Weaknesses and missing pieces

### 3.1 The interactive surface deletes the human airlock (worst gap)

The proposal files "sovereignty leakage" under risks as *"the giver's body
is now performing work for another instance"* — a resource framing. The real
problem is sharper and the README's threat model already names it: **the
log is context for the mind that authors your scripts, and there is no
human review between authoring and signing.** Today, foreign content enters
the log only when a human carries a directory in and runs `learn` — the
README explicitly says "treat learning an account as running code: read its
intent and record first." That human hand-off is the airlock.

A live, receiver-scoped surface whose POSTs append to the giver's log — and
whose answers are produced by the giver's mind — is a channel by which a
remote party injects prompt material directly into the entity that writes
locally-signed, unsandboxed scripts, with no human in the loop. "Only the
commands declared for the lesson surface may be invoked" does not help:
the payload is the *content* of the question, not the verb. The one-shot
protocol's asynchrony is not a limitation to be engineered away; it is the
security model. Any interactive realization must preserve a
human-checkpoint (or an equivalently strong gate) between "question
arrived" and "mind read it," and the proposal never notices this
requirement.

### 3.2 Answer-side curation is unspecified (second-worst gap)

`give` is selector-scoped: an event-name prefix or one capability. The giver
can only leak what a selector selects, and curation is editing a plain-text
directory before it leaves. An *answer*, though, is composed by the giver's
mind in free prose, with the whole log as its context. An over-helpful mind
answering "why does your capability do X?" can quote anything — the medical
instance's answer can disclose events the original curated give deliberately
excluded. This is the confidentiality dual of 3.1, and it is the failure
mode that matters most for the medical body.

The fix is a practice, not a mechanism, but it must be stated in the
dialogue lesson's intent: **answers leave as accounts through `self give`,
never as raw mind prose** — compose the answer *into the log* (where the
human can read it on the dialogue projection), then give the answer events
through the same selector-and-curate gate every other account passes. The
asymmetry "giving is cheap; learning is the work" becomes "answering is
work too, and it passes the same door on the way out."

### 3.3 Deposit is not idempotent; threads will duplicate

Concrete mechanical bug waiting in multi-turn use: `cmdLearn` deposits
record events as *fresh* events (new id, new seq, preserved `occurred_at`)
with no content-based dedupe. A giver answering with
`self give dialogue. answer/` selects by prefix, so the outgoing record
carries the *whole thread* — including the questions the receiver
originally sent. When the receiver learns the answer, its own questions
land in its log a second time as new events. Every round trip duplicates
the shared prefix of the thread.

Options, in increasing order of mechanism: (a) discipline — curate the
record before sending, or use finer prefixes (`dialogue.answer.` vs
`dialogue.question.`) so only the new turn travels; (b) make the dialogue
projection dedupe by content hash, tolerating duplicate deposits as the
log-management problem the system already pushes outward; (c) teach `learn`
content-hash dedupe — a kernel change, and the wrong one to start with.
Start with (a)+(b) in the lesson's intent as explicit constraints; the
lesson's "anti-goals" section should say *why* (this section, in short).

### 3.4 There is no identity layer, and the proposal assumes one

"Scoped to the receiver (e.g., via public key)" assumes receivers have
public keys. They do not. The only key in the system is `.secret` — a
32-byte symmetric HMAC key that never leaves the instance and identifies
nobody. Instances have no names, no keypairs, no addresses; `by` on a
receipt is a free string. The interactive variant therefore does not extend
the security model, it *founds* one: keypairs, key exchange or TOFU,
signature verification on requests, an authz table, lifetime enforcement.
The README lists "multi-user access control" as an explicit non-goal of the
core. This single hidden assumption is most of the interactive variant's
true cost, and the proposal's §6 "kernel growth" paragraph only gestures at
it.

Worth noting the flip side: if inter-instance identity ever *does* arrive,
the right first use is not auth on a live surface — it is **signing
accounts**, so a receiver can verify who a directory came from. Provenance
before presence.

### 3.5 Smaller gaps

- **Two-party bias.** Accounts are directories; nothing stops giving the
  same account to N receivers, and nothing in the proposal says what a
  conversation is when three bodies hold it. The content-hash thread ID
  happens to survive fan-out (everyone hashes the same record), which is
  lucky and worth making deliberate.
- **The economics shift is unexamined.** "Giving is cheap; learning is the
  work — that asymmetry is the protocol" (`account.go`, header comment).
  Conversation makes giving an ongoing obligation. A body that gives ten
  accounts with open question-windows has taken on ten support contracts.
  Question-windows (see Q2 below) are how a giver bounds this, and the
  default should be *closed*.
- **O(n) pressure is slightly worse than stated.** The proposal worries
  about projections, but projections declare `consumes` and the kernel
  skips ones whose events did not grow — a page that ignores `dialogue.*`
  pays nothing. What *does* pay is every **command**, which receives the
  entire log on stdin on every run (`README`, pipe contract). Dialogue bulk
  taxes every future command invocation a little. Same conclusion
  (compaction is the bodies' problem), but the pressure point is commands,
  not projections.

---

## 4. The interactive temporary projection, judged

Against the repo's invariants one by one:

| Invariant | Async question-accounts | Live keyed surface |
|---|---|---|
| Nothing runnable crosses | preserved | preserved |
| Local-only signing/install | preserved | preserved |
| Human airlock before the mind reads foreign content | preserved | **broken** (§3.1) |
| Projections are pure functions of the log | preserved | preserved (the surface can be pure) |
| `/run` needs no auth because it binds loopback | preserved | **broken** — requires identity layer (§3.4) |
| Kernel readable in an afternoon | preserved (zero change) | not credible (§3.4, Q3) |
| Reconstructible offline from log + key | preserved | negotiation yes; *surface scoping state* must also live in events or it drifts |
| Giver not required after give | preserved | broken by design (availability) |

The live surface preserves the letter of the installation invariants and
breaks the two properties that are not written down as code: the airlock
and the smallness. It also imports availability, lifetime, and revocation
problems the async form simply does not have. The proposal's own §6 lists
most of these costs and then §7 still calls the surface "the most
self-native way" — that judgment does not survive contact with the threat
model. The *self-native* move is the one `lessons/timers` already made for
clocks: **the kernel keeps no clock; likewise it should keep no network.**
Intentions in the log, the trigger outside. Questions in the log, the
transport outside.

If fluid high-frequency negotiation is ever genuinely needed (proposal
§4.3), note that nothing prevents two operators from mounting a shared
directory and running a tight give/learn loop over it — fluidity is a
transport property, and transports are already outside the kernel.

---

## 5. Alternative framings (Q9 answered here)

**A. The conversation is a directory under version control.** An account is
already a plain-text directory whose edits are meaningful ("curation is
editing the directory"). Let the *same directory* be the conversation:
receiver adds `questions/0001.md`, giver adds `answers/0001.md` beside a
regenerated record, both parties `learn` the turns they care about into
their own logs. Put the directory in git and the negotiation trace is
versioned, diffable, asynchronous, human-readable at every step — and the
minds on both sides are coding agents that already walk git history better
than any bespoke surface (the proposal's own §4.1 observation, turned into
the mechanism). This gets ~90% of the interactive variant's tangibility for
0% of its kernel cost, and the human airlock survives because a human still
pulls and learns.

**B. The anticipatory give.** Cheapest of all: raise the quality bar on
intents. The capability flavor already ships full lineage; an intent
template that asks the giver "what will the receiver wish they could ask
you?" converts many conversations into better opening moves. This does not
replace the protocol, but it should be the documented first resort —
conversation as fallback when anticipation fails, so threads stay rare and
high-value (the §6 signal/noise worry, partially solved by defaults).

**C. The dialogue lesson** (the recommended core): see §7.

---

## 6. The nine open questions, answered

1. **Live surface vs. async?** Async is sufficient and strictly better
   today. The live surface breaks the human airlock (§3.1), requires an
   identity layer that doesn't exist (§3.4), and buys only latency. Revisit
   only after the async form has produced real threads whose measured pain
   is latency — and even then, framing A (shared directory, fast loop)
   likely absorbs the need.

2. **Lifetime/revocation of an open surface?** As events, following the
   timers pattern: `dialogue.opened {record_sha256, until}` /
   `dialogue.closed {record_sha256}` in the giver's log, surfaced on the
   giver's own dialogue projection, and *stated in the account's intent* so
   the receiver knows the window. The window is a promise, not a mechanism —
   enforcement is the giver declining to answer, which costs nothing to
   build and is honest about who holds the power. Default: not open.

3. **Minimal kernel change for public-key-scoped allow-lists?** Keypair
   generation and storage; an identity exchange or TOFU story; request
   signing/verification on `/run`; a per-key command allow-list sourced
   from events; lifetime enforcement. That roughly doubles the
   security-relevant kernel and adds its first networked trust decision.
   "Readable in an afternoon" does not survive it. This is the strongest
   single argument for the async form: its number is **zero**.

4. **Conversational bulk vs. O(n)?** Three dampers, all outside the kernel:
   default-closed question windows (bulk is opt-in per give); the lesson's
   constraint that only new turns travel (§3.3) so logs don't re-absorb
   whole threads; and the existing rule that projections declare `consumes`
   precisely, so dialogue events cost nothing to pages that ignore them.
   Remaining cost falls on commands (whole-log stdin) and is the same
   compaction problem the system already owns.

5. **Contact-back: per give, per body, or a seed?** All three layers, and
   they compose: the *capability* to hold dialogues is a seed
   (`lessons/dialogue`) a body adopts or ignores; the *willingness* is per
   give (the window in the intent, default closed); a *standing* policy is
   just a body that includes the window in every intent. No kernel opinion
   required — which is the answer's virtue.

6. **MCP as optional transport?** As a transport for *directories*, fine
   and invisible — transports are outside the protocol by construction.
   The moment MCP verbs shape the account format (a question becomes a tool
   call instead of an `intent.md`), the "any mind that can read a directory
   can participate" property dies. Litmus test: a body with no MCP client
   must be able to hold the entire conversation from the directory alone.
   If yes, MCP added reach; if no, it leaked semantics.

7. **Asymmetric minds/ages?** Three modes. A weak mind questioning a strong
   giver is the *good* case — questions are cheap, answers are where
   quality lives. A strong receiver interrogating a weak giver produces
   confident non-answers; the receiver's protection is the record (evidence
   verbatim) over the intent (prose), which is exactly the priority the
   existing protocol teaches. Old body giving to young: lineage dumps
   overwhelm — the giver should give the *current cut* and open a window,
   letting questions pull history on demand instead of pushing it. That
   last pattern (conversation as lazy evaluation of lineage) is a genuinely
   better default than shipping full history, and worth writing into
   LESSONS.md when this lands.

8. **Good vs. bad negotiation in the running bodies?** Good, medical: the
   work body asks the medical body "which of these observation kinds are
   load-bearing for the pattern you flagged?" and receives a *narrower*
   re-cut — two turns, less data flowing than the original give, answer
   curated through `give`. Bad, medical: the mind answers a question in
   free prose quoting log events outside the original selector (§3.2).
   Good, domestic: a household-patterns account refined once against the
   receiver's actual fixtures, then closed. Bad, work: two capable minds
   burning a 40-turn thread refining a throwaway capability — taste failure
   the protocol permits; the projection making thread length *visible* to
   the humans is the only honest damper. General rule: a good negotiation
   ends with less data transferred than the naive give; a bad one ends with
   more.

9. **Cleaner primitive than temporary projection?** Yes — the conversation
   directory (§5.A). It is tangible, uses surfaces both parties already
   trust (files, git, plain text), keeps execution and ingestion fully
   local, and needs nothing built.

---

## 7. Recommended path

**Phase 0 — vocabulary (documentation only).** Adopt the reframing in
README/LESSONS: an account is an *opening move*; `record_sha256` is the
thread identifier; answers leave through `give` like everything else. No
code.

**Phase 1 — `lessons/dialogue` (one lesson, no kernel change).** Commands
`dialogue/ask`, `dialogue/answer`, `dialogue/close`; events
`dialogue.question`, `dialogue.answer`, `dialogue.closed`, each carrying
`re` (a `record_sha256`) and a content hash for dedupe; a `/dialogue`
projection showing open threads, keyed and deduped by content hash.
Constraints to write into the intent, learned from §3: answers are composed
into the log and exported with `self give dialogue.answer.` (never raw mind
prose, never the whole thread); the projection tolerates duplicate deposits;
an empty log renders guidance, not an error. Anti-goals: no transport, no
daemon, no kernel event names.

**Phase 2 — only if real threads demand it.** Shared-directory transport
practices (git), question windows as standing policy, and — far behind
everything else, gated on an identity layer designed for provenance first —
any live surface. Carry forward the airlock as a hard requirement: no
foreign bytes reach the mind without a human (or an explicitly configured
gate) between arrival and reading.

The proposal's closing sentence sets the right bar: preserve sovereignty
and reconstructibility while making the conversation feel natural. The
async form clears that bar today with zero mechanism; the live surface, as
specified, clears neither the sovereignty bar (airlock) nor the minimalism
bar (identity layer). Build the lesson, keep the kernel deaf, and let
better conversational practice travel the way everything else here
travels — as an account.
