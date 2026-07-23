# Round four: doors and speakers

A continuation of [actors.md](actors.md). Round three claimed an actor's
stake is *computable* — reputation replayable from the log, testimony
permanent and attributable. This round starts from a bug found in that
claim: **attributable to whom?** An event is `{id, seq, name, occurred_at,
payload}`. When a consulting Claude session POSTs a form on an actor's
surface and a fact lands in the log, nothing records who spoke. The
human's click, the actor's own mind, a peer body, a frontier session — all
append identically. Scripts have signed provenance (`by` on every receipt,
covered by the HMAC); events — the part that accumulates, the part that
*is* the identity — have none.

In a single body this ambiguity was harmless: one human, one mind, one
door. The cast breaks it. An actor accumulating interactions from many
origins is a witness whose deposition has no speaker labels — and every
load-bearing claim of round three quietly assumed the labels existed.
Reputation needs attribution. Belief-contagion forensics ("how did we come
to believe this?") needs an origin chain. The thread critic needs to know
who is burning the thread. Round three's stake argument is not wrong; it
is unfunded. This round funds it.

---

## 1. What can actually be known, and by whom

Separate two things the word "provenance" blurs:

- **The door**: which channel the event entered through. The kernel
  witnesses this directly — it is the code path. A CLI invocation, an HTTP
  POST from a remote address, a mind's stdout, a learn's deposit, the
  kernel's own bookkeeping. This is a *fact*, and the kernel can attest it.
- **The speaker**: who was behind the door. `claude-main`? The human? Peer
  actor `build`? The kernel cannot know this without an identity layer —
  it can only record what the caller *claims*. This is a *claim*, and the
  kernel can carry it.

The protocol already has exactly the right grammar for this pair, and it
is the same grammar it used for account integrity: `lesson.learned`
records `record_sha256` (what verifiably happened) **beside**
`manifest_sha256` (what was claimed), and a mismatch is not an error — it
is an intervention, made visible. Provenance should speak the same way:
an attested fact beside a carried claim, never conflated, both rendered
as what they are.

## 2. The proposal: `via` and `by` on the event envelope

Extend the envelope by two fields:

```
{id, seq, name, occurred_at, via, by, payload}
```

**`via` — the door. Kernel-attested, never accepted from outside.**
Stamped at append time by the code path itself:

- `cli` — a local `self run` / `self learn` / kernel CLI verb
- `http:<remote-addr>` — a form POST on `/run/<command>`
- `mind:<SELF_MIND_ID>` — events the mind emitted (think, reflect, learn
  declarations, chat's re-emissions land as the command's door — see §4)
- `learn:<account-name>` — a deposit from an account
- `kernel` — the kernel's own receipts and bookkeeping

A caller cannot set `via`. Anything arriving in a payload or header
claiming to be a door is just data. The invariant the kernel enforces is
small and absolute: **no event enters the log with its door unstamped.**
Anonymity at the channel level becomes structurally impossible — which is
the only enforcement the kernel can honestly provide, and the only one it
needs to.

**`by` — the speaker. Caller-claimed, recorded verbatim.**
From `SELF_CALLER` (or `SELF_MIND_ID`) in the environment for local
invocations; from an `X-Self-Caller` header for HTTP. Empty when nothing
was claimed — a browser click from the owning human carries no header,
and `via: http:127.0.0.1` with an empty `by` *is* the honest record of
that moment. `by` is testimony about testimony: rendered as a claim
("claims to be claude-main"), weighted by the reader, never presented as
verified.

Verification, where it is ever needed, stays on the ops membrane exactly
as round two placed the network: a reverse proxy that authenticates
callers strips inbound `X-Self-Caller` and injects the verified one. The
kernel still just records what arrives at its door. No keys enter the
kernel; the claim simply arrives pre-laundered on deployments that care.
Trust in an unverified claim is the receiving actor's own policy — which
is what sovereignty means at this layer.

## 3. Why this is kernel — the first kernel change in four rounds

Three rounds produced zero kernel changes, each time by showing the
capability could be grown. This one cannot be, even in principle, and the
reason is the pipe contract itself: **a command receives argv and the log
on stdin — it has no way to know which door its invocation came through.**
The HTTP request, the CLI environment, the mind's identity: all of it is
gone by the time any grown script runs. Only the kernel witnesses the
door. The harvest lesson's payload-level `by` was the grown approximation,
and it shows both the demand and the ceiling: opt-in per lesson,
self-reported per caller, invisible to every capability that didn't think
of it.

The round-two slogan already carved the space for this: *views are grown;
gates are kernel.* Provenance is not view-work — it is the membrane
recording what passed and through which door, which is gate-work as surely
as the vocabulary refusal. The kernel's job description in the README is
"a record of who generated each script"; the cast extends the same
sentence to who occasioned each event. The cost is honest: a two-field
envelope extension, `via` threaded through the handful of `appendEvent`
call sites (each already knows its own door), and the header/env read.
Afternoon-readability survives.

Additive and backward-compatible: old events simply lack the fields
(legacy = unknown door), `omitempty` keeps old logs byte-identical, and
`rehydrate` replays events as written — reconstruction is untouched.

## 4. What travels and what stays local

The envelope now has a clean symmetry worth pinning as a rule:

- **Local fields** — `id`, `seq`, `via`: minted by *this* log, about *this*
  log. When an account is learned, the deposit gets this body's ids, this
  body's seqs, and `via: learn:<account>` — because that is the door the
  events actually entered through here.
- **Portable fields** — `occurred_at`, `by`: properties of the moment and
  the speaker, preserved verbatim on deposit, exactly as moments already
  are. Testimony keeps its speaker across bodies the way it keeps its
  time.

So a fact born as `via: http:…, by: claude-main` in the build actor's log
arrives in a peer's log as `via: learn:build-account, by: claude-main` —
this body's door, the original speaker. The giver's doors remain visible
in the severed record itself (the account directory carries the fields as
text) for anyone doing deep forensics; the receiving log's `via` never
lies about its own doors to preserve someone else's.

One subtlety to spec honestly: events emitted *by a command* (chat
re-emitting the mind's declarations, a tick firing timers) carry the door
of the command's invocation — `via: http:…` or `cli` — because that is
what the kernel witnessed. The finer distinction "the mind composed this
line, the script merely relayed it" lives in payloads and receipts, as it
already does. `via` answers "how did this enter the log," not "who
authored every byte" — scripts' authorship is the receipts' job, and it
is already signed.

## 5. What the cast gets for two fields

- **Round three's stake, funded.** Reputation is now computable for real:
  filter an actor's claims by speaker, replay whether they held up. An
  actor's answer-quality, a consulting session's question-quality, a peer's
  deposit-quality — all become projections someone can grow, because the
  data exists.
- **Forensics with a chain.** "How did we come to believe this" traces
  door by door: entered the build actor via HTTP from claude-main, traveled
  to the migrations actor via learn, cited in an answer given onward. The
  epidemiology of round two gets its contact-tracing field.
- **Surfaces that show who is speaking.** The dialogue projection renders
  a thread with speaker labels — human, own-mind, peer, frontier visitor —
  without any lesson having to invent per-payload conventions. The thread
  critic can rate-limit by origin. The chat page can finally distinguish
  the owner's click from a visiting mind's POST for free.
- **Honesty preserved in the rendering.** The conventions matter: `via`
  states, `by` claims. A projection that prints "claude-main said X" from
  an unverified `by` has overclaimed; "X (via http, claims claude-main)"
  is the timers-style truthful sentence. The lesson intents should carry
  this rule the way timers carries "waiting for a tick."

## 6. What this deliberately does not do

- **No event signing.** Receipts stay the only signed objects. The log's
  integrity-at-rest model is unchanged (file ownership, one writer
  discipline); signing every event is new machinery chasing a threat the
  trust boundary already handles, in both frames.
- **No identity layer.** Keys, registration, verification — still outside,
  still ops, still optional. Round one's conclusion stands: if
  inter-instance identity ever arrives, its first job is signing severed
  accounts, not authenticating live callers.
- **No policy.** The kernel records doors and claims; it never decides
  which speakers to believe, rate-limit, or refuse. Those are grown
  capabilities and operator judgment — per body, per the fractal rule that
  local problems get local organs.

---

The arc of the four rounds, then: the conversation needed no kernel (round
one), the live surface needed no kernel (round two), the cast needed no
kernel (round three) — and the first thing that genuinely does need the
kernel is the thing the kernel was always for: **the integrity of the
record.** Not who may speak — that is policy, and policy is grown — but
the guarantee that when anyone speaks, the log remembers which door they
came through and who they said they were. A witness with a deposition full
of unlabeled speakers is not much of a witness. Two fields fix it.

*Round four of an ongoing design conversation.*
