# Round two: the swarm frame

A continuation of the design conversation in [proposal.md](proposal.md) and
[evaluation.md](evaluation.md). The pushback that opens this round: the
evaluation judged the live conversational surface under one frame — persons
sharing across untrusted links — and rejected it there. But `self`'s
specialty is the expropriation of HTML surfaces: taking useful behavior and
re-expressing it as commands and projectors over events. Give and learn are
the last un-expropriated citizens — directories on disk piped into
dedicated Go handlers, outside the paradigm everything else lives in. And
there is a frame — agent swarms in a codebase — where the live design stops
fighting the threat model entirely. Take security off the table for a
moment and look at the shape again.

This round does that, and two of the evaluation's objections do not
survive it.

---

## 1. Concession one: the objections were facts about the link, not the design

Re-read the evaluation's two heaviest charges against the live surface:

- it deletes the human airlock between a foreign party and the
  script-authoring mind;
- it requires an identity layer the system does not have.

Both are costs of an **untrusted link**. Neither is a property of the
design. In a swarm — one owner, one codebase, bodies on one host or one
private network — trust is ambient: every body serves the same principal,
every log is the same person's data, every mind is an agent that principal
launched. And then something the evaluation never noticed becomes visible:

**The live conversational surface needs zero kernel change in the swarm
frame. Not a small change — zero.**

Walk it: body A runs at `:7701`, body B at `:7702` (`SELF_BIND` already
does this). A learns a lesson that declares an `account/<name>` projector
and an `ask` command. B's mind — a tool-capable coding agent, which is what
a mind *is* here — fetches `http://127.0.0.1:7701/account/foo` the same way
it reads any file, and POSTs a question to `/run/account/ask` the same way
any HTML form does. A's agent loop notices the question on its own surface
and answers. The kernel on both sides never learns the network exists. The
peer's mind is just another HTTP client filling in forms.

The proposal's key-scoped temporary projections, allow-lists, and lifetime
machinery were never the design — they were the **hostile-world tax on the
design**. Inside a trust boundary the tax is zero and the design is just…
the system, used as built.

There is a deeper reason it feels native, and it was in the philosophy all
along: equal footing. The surfaces were designed so that a human and a mind
are the same kind of actor — a reader of bare HTML, a filler-in of forms.
A form POST does not care who submitted it. A *peer body's mind* is simply
the third reader the design already accommodated without being asked.
The swarm frame is not an extension of equal footing; it is its first
full use.

(Where the swarm spans hosts on shared infrastructure, the membrane is an
operations concern, not a kernel one: a private overlay, mTLS at a reverse
proxy, an ssh tunnel. The kernel stays deaf to the network either way —
which dissolves the evaluation's identity-layer objection completely: even
the semi-hostile swarm needs a proxy, not a kernel.)

---

## 2. Concession two: give is a paradigm anomaly, and regrowing it shrinks the kernel

The observation that give/learn live outside the surface paradigm is
correct, and it splits cleanly down the middle when you ask *what kind of
work each one is*:

**Give is a view.** Look at `cmdGive`: select events by prefix or
capability, rename lifecycle names to `lineage.*`, render to a directory,
attest with a hash, remember doing it. Selection plus rendering plus a
receipt — that is projection work wearing a Go costume. It can regrow as a
lesson:

- a command `account/publish <selector> <name> <telling…>` emits one
  `account.published` event carrying the telling and the selection;
- a projector `account/<name>` renders the account **as a surface**: the
  telling as prose, the selected events verbatim (JSONL in a `<pre>` — bare
  semantic HTML carrying the machine-readable record inside the
  human-readable page), and a question form.

The account stops being a third format and becomes what everything else
already is: a page, provably current against the log, readable by human,
local mind, and peer mind alike. Roughly half of `account.go` — the give
half — leaves the kernel. The evaluation recommended keeping the kernel
untouched; this is better, because it makes the kernel *smaller*. The
instinct behind the pushback wins that exchange.

**Learn is a gate.** The other half does not regrow, and should not. The
vocabulary refusal (`kernelVocabulary`), the verbatim deposit with moments
preserved, the signed compile of locally-declared capabilities — that is
the membrane deciding what may enter the body and under whose signature.
Every organism in this design grows its surfaces and keeps its membrane.
So the slogan for the split:

> **Give is a view; learn is a gate. Views are grown; gates are kernel.**

One wrinkle, and it resolves elegantly. The capability flavor of give
verifies receipts against `.secret` before exporting them — a grown script
should not handle the key. But notice what the proof was *for*: a directory
needs a manifest because it has been severed from its log; the hash is how
a severed record proves it was not altered in transit. A **surface is not
severed** — `/events` is attached, the server's freshness check makes the
page provably current, and the receiver can hash what it actually fetched
and attest *that* in its `lesson.learned`. On the live surface, attestation
moves from emission to ingestion, and the manifest becomes what it always
secretly was:

> **A directory account is a dehydrated surface. The manifest is the
> severance artifact — what you add when you cut a page off from its log.**

That is the answer to "how do we share this HTML with an intent and proof
declared": the intent is the telling, rendered on the page; the proof is
attachment to a live log when the link is trusted, and a manifest when the
account must travel severed. Two forms, one account — and any mind can
dehydrate a surface into a directory when it needs the envelope form. The
current Go `give` is revealed as the built-in dehydrator, kept for the
severed case (or eventually regrown too).

---

## 3. One protocol, two membranes

So the frame does not fork the protocol; it selects a permeability. The
Levin framing makes this precise rather than decorative. Inside one
cognitive boundary, cells couple through gap junctions — high bandwidth,
low ceremony, shared fate, direct exchange of state. Across organism
boundaries, communication drops to signals at arm's length — sealed,
slow, interpreted rather than absorbed.

| | Swarm (one owner, one codebase) | Persons (across sovereignty) |
|---|---|---|
| The account is | a live surface on a peer body | a severed directory |
| Proof is | attachment to the log (freshness) | the manifest hash |
| The conversation is | forms on the surface, machine-tempo | question-accounts, human-tempo |
| The airlock is | the trust boundary itself | a human running `learn` |
| Transport is | the mind's own HTTP tools | however directories travel |
| Kernel change | zero | zero |

Same chemistry in both columns: intent + evidence crosses, the receiver's
mind re-expresses locally, only the local signature installs, both logs
remember. The membrane's permeability is the *only* variable — and it is
set by who owns the two bodies, which is exactly the variable a
sovereignty-first design should key on. The evaluation's error was reading
the left column as a dangerous version of the right, when it is the same
protocol at a different scale of the same fractal.

---

## 4. Security, re-entering on the swarm's own terms

Setting security aside was the right move to see the shape; here is what it
looks like when it walks back in — not as a veto this time, but as the
reason the shape *wins* against how swarms actually share today.

Contemporary agent swarms share through shared scratchpads, shared context
files, and agents writing directly into each other's prompts. Every one of
those channels is a code-and-context injection path with a **single point
of compromise**: poison the scratchpad and you have poisoned the swarm.

A swarm of selfs conversing over lesson surfaces has a property none of
those substrates have: **evidence crosses, code never does, and every body
compiles its own capabilities behind its own key.** A contaminated account
— say a poisoned test fixture that steered body A's log — can *argue* to
body B's mind, but it cannot *install* into body B. Compromise does not
propagate as authority; it propagates only as testimony, and testimony
lands marked with its origin and its moments, replayable later by a mind
asking "how did we come to believe this?" The rule the evaluation defended
as a human airlock re-derives, in the swarm frame, as epidemic control —
and reconstructibility becomes forensics.

That said, the swarm frame has its own failure modes, and they are not the
person-frame's. Naming them now, with the organ that answers each already
visible in the lessons:

- **Contagion of belief.** One body's mistaken conclusion ("the build
  cache lies") deposits into peers and hardens into swarm consensus.
  Answer: provenance conventions — dialogue events carry a `body` field
  and answers carry pointers to evidence, not free assertions ("pointers,
  not content," exactly the harvest lesson's rule). A belief that cannot
  cite its events is visibly a guess.
- **Homogenization.** The value of exchange decays as bodies converge on
  identical metis; copy-of-copy flattens the diversity that made sharing
  worthwhile. Answer: niching — bodies attached to modules, roles, or
  worktrees regenerate differential experience faster than conversation
  erodes it. Deposited events staying marked as *theirs* (moments
  preserved, origin named) keeps the difference legible.
- **Threads at machine tempo.** Two capable minds can burn a
  forty-turn refinement loop on a throwaway capability in seconds.
  Answer: the monitor lesson generalizes — the orchestrator/rejector role
  becomes a thread critic, and thread length is rendered on the surface
  where both humans and minds can see the waste.
- **Bulk at swarm speed.** Conversation events accrue at agent tempo, and
  every command replays the whole log from stdin. Answer: harvest is the
  compression organ — converse, distill, then **give the fact, not the
  thread**. The thread stays in the two logs that lived it; what travels
  onward is the promoted event. A good negotiation still ends with less
  data transferred than the naive give.
- **Liveness.** The kernel is inert; who answers a question? In a swarm,
  agents are awake anyway — answering is a tick, in the timers pattern:
  the giver's agent loop (or a cron `self reflect`) notices open questions
  on its own dialogue surface. No daemon enters the kernel.

---

## 5. The probe

This is now concrete enough to test with two bodies on one laptop, and the
experiment is cheap because every piece is a lesson:

1. **Regrow give as a surface** (`lessons/account`): the
   `account/publish` command and the `account/<name>` projector from §2.
   The severed/directory path stays on the kernel give for now; parity can
   come later.
2. **Two bodies, two ports.** Body A publishes an account of something it
   actually learned (a harvest fact, a flaky-test note). Body B's mind is
   pointed at A's surface and asked to learn from it: read the page,
   question it through the form, declare locally, deposit what it took,
   attest the hash of what it fetched.
3. **Watch the thread.** Success looks like the evaluation's own criterion
   surviving the frame change: the negotiation ends with a *narrower,
   better* re-cut than the naive give — and both logs can replay how the
   capability was shaped. Failure looks like duplicate deposits, unmarked
   beliefs, or a thread longer than the capability deserved — each of
   which points at a specific convention from §4 to write into the
   lesson's intent.

If the probe holds, the person-to-person protocol inherits the result for
free: the directory form becomes the dehydrated special case of the
surface form, produced whenever an account must cross a membrane that
demands the envelope. The Account Protocol's story gets simpler, not
longer — one account, two permeabilities, and a kernel that got smaller
along the way.

---

*Round two of an ongoing design conversation. The evaluation's round-one
conclusions stand for the person-frame; this round establishes that the
swarm-frame was a different question with a different — and better —
answer.*
