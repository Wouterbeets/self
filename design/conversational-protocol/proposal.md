# Conversational Protocol for Sovereign Minds
## An exploratory analysis of extending the Account Protocol in `self`

**Purpose of this document**
This is a working analysis of a proposed extension to the Account Protocol in the `self` system. It is intended as an exploratory prompt for another model (or human) to evaluate the idea: identify strengths, weaknesses, missing pieces, hidden assumptions, and alternative framings. The goal is iterative hardening, not final specification.

---

## 1. Brief context: what `self` is

`self` is a local-first, event-sourced runtime whose only authoritative state is an append-only event log plus a per-instance signing key. Capabilities (commands and projections) are not shipped as code. They are grown: a mind process inspects the current state of *this* body, authors scripts from declarations, and the kernel installs only locally signed receipts. Everything is reconstructible offline from the log + key.

The system deliberately keeps humans and minds on equal footing via the same HTML projections. It treats *metis* (practical, contextual, accumulated wisdom from lived use) as first-class alongside frontier intelligence. Bodies are sovereign: each keeps its own log, key, and compiled capabilities.

The current Account Protocol (give / learn, or share / adopt in the philosophy branch) lets one body offer an account — intent + evidence (optionally the evolutionary history of a capability) — to another. The receiver re-expresses it locally. No code is transplanted. That is the existing mechanism for sharing between sovereign minds.

---

## 2. The gap

The current Account Protocol is still essentially a one-shot transfer:

1. Giver packages an account.
2. Receiver inspects and adapts it in isolation.
3. Receiver commits a local learn.

What is missing is an explicit, first-class way for the *receiver* to address the *giver* after the account has been offered and ask clarifying questions, request a different cut, or negotiate refinements. The conversation is truncated. The giver speaks once; the receiver must interpret alone.

This gap matters because real sharing of lived experience between durable agents (or humans) is almost never one-shot. Clarification, emphasis, and adaptation are normal. Without a way to continue the conversation, the protocol is weaker than the human analogue it aspires to support.

---

## 3. The proposed Conversational Protocol

**Core claim**
The Account Protocol is best understood as a *conversational protocol for how sovereign minds share*. The one-shot give/learn is only the minimal form of that conversation. A fuller form allows multi-turn interrogation and refinement while preserving sovereignty.

**Key properties the protocol should maintain**

- Only accounts (intent + evidence) cross the boundary — never runnable code.
- All compilation and installation remain local to the receiving body and signed by its own key.
- The negotiation itself is recorded as ordinary events in both logs (or at least in the logs of the participants who care).
- Either party can refuse or stop at any time.
- The final adapted capability is a local re-expression, not a clone.

**Minimal viable extension**

- A give can optionally declare that the account is open to questions for a defined period or under defined conditions.
- The receiver can package a question or refinement request as a small account and attempt to deliver it to the giver.
- The giver may answer with a further account.
- Both the questions and answers become events.

**More interactive realization (under discussion)**

A give could materialize as a temporary projection (HTML surface) that the receiver can interact with using the same form-based commands already used everywhere in `self`. The surface would be scoped to the receiver (e.g., via public key). The kernel would enforce that only the commands explicitly declared for that temporary lesson surface may be invoked. When negotiation ends, the temporary surface is retired and the receiver commits a normal local learn.

This keeps the interaction language identical to the rest of the system ("get the lesson surface" + "run one of the allowed commands") while making the back-and-forth tangible.

---

## 4. Why this is interesting

1. **It completes the symmetry with how agents already use history.** Coding agents already walk git logs to understand *why* something is the way it is. The Conversational Protocol asks them to treat the evolutionary history of a capability (and the negotiation around it) the same way — as signal that can be inspected and adapted.

2. **It turns transfer into mutual adaptation.** The account stops being a final statement and becomes an opening move that can be interrogated. The resulting capability carries both the original signal and the trace of how it was shaped for this particular receiver.

3. **It scales with mind capability and inference speed.** At low tokens/s the conversation is ceremonial. At high speed (and with dense local models) multi-turn refinement of match slices, medical observations, project memory, or household patterns can become fluid and backgrounded.

4. **It is a concrete substrate for collective intelligence among sovereign bodies.** If useful ways of packaging, questioning, and adapting accounts themselves travel as higher-order metis, a population of bodies can begin to explore niches without a central coordinator. This is the fractal / Michael-Levin-style bet made operational at the inter-body layer.

5. **It stays inside the existing aesthetic.** The same HTML surfaces, the same event log, the same pure projections, the same signed local installation. No new privileged channel is required in the minimal version; the interactive projection version reuses the system's own interaction model.

---

## 5. What it offers `self`

- A natural way for the three (and future) living bodies — personal, work, medical — to refine what they share instead of only offering static packages.
- A path for evolutionary accounts (the full growth history of a capability) to be actively interrogated rather than passively received.
- A mechanism that becomes more powerful precisely as minds become faster and more capable, without requiring kernel changes for the minimal form.
- A clearer philosophical story: "How do sovereign minds share?" becomes the central question the Account Protocol answers, with one-shot transfer as the degenerate case of a conversation.

---

## 6. Risks and tensions

**Sovereignty leakage**
Any live or addressable surface hosted by the giver (even temporary and keyed) means the giver's body is now performing work for another instance. This is a mild form of remote interaction the original design avoided by insisting that only static accounts travel and all execution stays local.

**Kernel growth**
Supporting public/private keys and temporary key-scoped projections with command allow-lists is a real expansion of the security model (currently a single local HMAC secret). The minimal asynchronous "send a question account back" version needs far less kernel change.

**Bulk and O(n) pressure**
Negotiation events add to the log. Every pure projection that replays history pays for them. Bodies will need to learn compaction, selective consumption, and ignore rules — the same local-management problem the system already pushes outward.

**Signal vs noise**
A giver can offer too much history; a receiver can ask too many low-value questions. Without discipline, conversational accounts become harder to distill. The protocol does not solve taste; it only makes the conversation possible.

**Availability and lifetime**
If the interactive surface is live, the giver must be reachable. Asynchronous question accounts are more robust but less fluid. Clear lifetime and revocation semantics are required so temporary surfaces do not become permanent liabilities.

**Ecosystem temptation**
Building the interactive surface on MCP would buy auth and reach for free, but risks leaking MCP tool/resource semantics into the account format and weakening the "any mind that can read an account can participate" property. Native key-scoped surfaces stay purer; MCP is better treated as an optional transport.

**Human visibility**
How much of the mind-to-mind negotiation should surface to the human owner by default? The log will contain it; most projections will ignore it. That is correct, but the default experience needs care so the system does not feel like it is having opaque side conversations.

---

## 7. Philosophical alignment

The proposal sits directly on the core bets of the system:

- **Metis over pure scale.** The conversation is how lived, contextual judgment travels and is adapted, not how weights or static prompts are copied.
- **Sovereignty.** Each body retains the right to refuse, to adapt, and to keep its own compiled form. Nothing installs without a local signature.
- **Equal footing.** The interactive surface (if used) is ordinary HTML that both human and mind can read and act on.
- **Unix philosophy extended.** Small, composable primitives; the conversation is assembled from accounts, events, and optional temporary projections rather than a new monolithic channel.
- **Fractal / multiscale competency.** Local bodies solve local problems and manage their own bulk. Useful conversational patterns can themselves become accounts that travel. Collective intelligence, if it appears, appears through differential survival of better sharing practices, not through a central protocol authority.
- **Reconstructibility.** The entire negotiation can remain in the log. A later mind can replay how an account was refined.

The interactive projection idea is the most "self-native" way to make the conversation tangible, because it reuses the exact surface language the system already trusts. The asynchronous question-account version is the most conservative and the closest to the original Account Protocol.

---

## 8. How it extends the Account Protocol

| Aspect                  | Current Account Protocol              | Conversational extension                          |
|-------------------------|---------------------------------------|---------------------------------------------------|
| Unit of transfer        | Account (intent + evidence)           | Same, plus optional questions & refinements as accounts |
| Direction               | Primarily one-way                     | Explicitly multi-turn                             |
| Receiver agency         | Adapt in isolation                    | Can interrogate the giver                         |
| Giver availability      | Not required after give               | Optionally required (or asynchronous)             |
| Recording               | give + learn events                   | Full negotiation trace as events                  |
| Installation            | Local only                            | Still local only                                  |
| Surface language        | Static account                        | Optional temporary interactive projection         |
| Auth for interaction    | None (static)                         | Public-key scoping of the temporary surface       |

The extension does not replace the Account Protocol; it makes its conversational nature explicit and operational.

---

## 9. Open questions for evaluation

1. Is the interactive temporary projection worth the sovereignty and kernel costs, or is the asynchronous "question account" form sufficient for most value?
2. How should lifetime, revocation, and resource limits of a temporary surface be expressed so they are themselves part of the account?
3. What is the minimal kernel change required for public-key-scoped command allow-lists, and does it still leave the kernel "readable in an afternoon"?
4. How do we prevent conversational bulk from accelerating the O(n) problem faster than bodies can learn to manage it?
5. Should contact-back capability be opt-in per give, per body, or grown as an ordinary seed that bodies may adopt or ignore?
6. Does exposing the same surface via MCP as an *optional* transport create more clarity or more semantic leakage?
7. What failure modes appear when the giver and receiver have very different mind capabilities or very different body ages/histories?
8. What does a "good" negotiation look like in the medical, domestic, and work instances already running? What would a bad one look like?
9. Is there a cleaner primitive than "temporary projection" that still gives the receiver a tangible way to interrogate while keeping execution fully local?

---

## 10. Suggested evaluation criteria for a reviewing model

When assessing this idea, consider:

- **Coherence** with the existing invariants (local signing, pure projections, no code transplant, reconstructibility).
- **Minimalism**: how much new kernel surface is actually required versus how much can be grown as ordinary capabilities.
- **Feel**: whether the resulting protocol still feels light and continuous rather than ceremonial or heavy.
- **Failure modes** under real multi-body use (medical, personal, work).
- **Scalability of metis**: whether the conversational layer makes evolutionary accounts more or less usable as bodies and logs grow.
- **Alternative framings** that achieve the same "receiver can still ask the giver" property with less mechanism.
- **Hidden assumptions** about mind capability, network availability, key management, or human oversight.

The strongest version of the idea is the one that preserves sovereignty and reconstructibility while making the conversation between durable minds feel as natural as the conversation between a human and their own body already does.

---

*End of exploratory analysis. This document is a snapshot of an ongoing design conversation, not a specification.*
