# Round five: custodians and stewards

A continuation of [provenance.md](provenance.md). Round three made actors
witnesses — knowledge stores you can cross-examine. This round follows the
observation that they are more than that: give an actor a **Unix user** and
clone a resource into its home — a hot-reloaded config, a private repo, a
credential — and the actor stops merely *knowing about* the resource and
starts *owning* it. Other users on the system cannot touch the file except
through the actor's surface, or by breaking user space. The actor's
commands become preconfigured, sanctioned operations — on the file, or on
external systems whose keys never leave the actor's home. Not only
knowledge stores: capability tools.

The pattern is real, it lands squarely in the design's grain, and it has
one wall that is weaker than it looks. This round names the lineage, the
gradient, the injection story, and the weak wall — honestly.

---

## 1. The move: the membrane inverts

Everything so far used Unix permissions inward: `.secret` is mode 0600 —
the design's innermost wall is already a file permission. The custodian
pattern scales the same wall outward until it encloses a whole resource:

- Create a user; the actor's `SELF_HOME` and the resource live in its home,
  mode 0700.
- The actor learns verbs for the resource: `config/set-timeout <n>`,
  `config/add-host <h>` — each a compiled script that validates, applies,
  commits, and **emits the event**.
- Everyone else gets the surface: read the projections, invoke the verbs.
  The file itself is unreachable except through the verbs or through
  `sudo` — and `sudo` is not a hole in the pattern, it is the pattern's
  honest boundary: root was always outside every userland membrane.

What this buys, concretely, for the hot-reloaded config: every change went
through a verb, so the log holds the config's *story* — not just its diffs
(git has those) but who asked, through which door, and what conversation
preceded the change. The `via`/`by` fields from round four are the load-
bearing piece: the custodian's log is a sudoers audit trail that lives in
the resource's own memory and can be cross-examined. Config archaeology —
"why is the timeout 30?" — becomes a question the config's custodian
answers with evidence.

## 2. Lineage: what this is a grown-up version of

Naming the ancestors shows precisely what is new:

- **A setuid binary / a sudoers rule**: narrow, sanctioned elevation of
  authority. The custodian is *a setuid binary that grew up* — same narrow
  authority, plus a memory, a telling, and someone to ask.
- **ssh-agent is a proto-actor**, and has been all along: it holds the key,
  answers signing requests through a socket, and never releases the key.
  The steward pattern below is ssh-agent with a log, a mind, and a story.
- **Object-capability systems**: authority as an invokable reference, not
  an ambient right. The custodian's verbs are ocaps over the resource —
  the surface is the ACL, and it is *semantic*: permissions expressed as
  what a change means (`set-timeout`, range-checked), not as read/write
  bits on bytes.
- **Secret brokers (Vault and kin)**: mediated access with an audit log.
  The custodian is local-first, has no central service, and its verbs are
  grown from intent by a mind rather than coded upfront.

New relative to all four: the account layer (the custodian can *explain*
its resource, and its explanations carry proof), growability (the verb set
evolves by learn/revise, per instance), and provenance on every write.

## 3. The gradient: witness → gatekeeper → steward

Three postures, increasing in how much authority the actor absorbs:

1. **Witness** (round three): the actor knows about the resource and logs
   what it is told. Anyone can still touch the resource directly.
2. **Gatekeeper**: the actor owns the resource; writes go through verbs;
   reads may stay direct (group-readable for the daemon that hot-reloads).
   Note the residue: *reads are not witnessed* — the log has every write,
   not every look. Acceptable for config; fatal for secrets, hence:
3. **Steward**: the resource never leaves the actor at all. Nobody reads
   the deploy key; they invoke `deploy/staging`, and the actor uses the
   key on their behalf, logging the act. **Authority as service, not as
   data.** The strongest form, and the one where the pattern stops being
   access control and becomes an interface to the world: preconfigured
   API calls to external systems, rate-limited, budgeted, logged — the
   actor as the sanctioned hand, with `via`/`by` on every reach.

## 4. Injection, honestly: two paths through the custodian

The prompt-injection worry is real and the pattern's shape gives it a
precise answer. There are two paths by which a caller's words can become a
custodian's act, and they have very different risk:

**The verb path is strong.** A compiled command is a deterministic script:
argv in, validation inside, events out. At invocation time *no mind is in
the loop* — the intelligence ran once, at learn time, when the script was
authored, and the result is plain text a human can read before trusting.
Injection through a verb reduces to argument validation, which is the
script's job and an auditable, boring one.

**The mind path is the confused deputy.** If the custodian's *mind* applies
changes on a foreign caller's conversational request — "please update the
timeout" through the chat form — then persuading the mind is exercising
the actor's authority, and prompt injection becomes privilege escalation.
This is the classic capability-security failure, walked into by wiring the
mind where a verb belongs.

So the discipline, which belongs in every custodian lesson's intent:

> **Minds testify; scripts act.** The custodian's mind answers questions
> about the resource. Changes go through compiled verbs or not at all.
> When a requested change has no verb, the answer is "no verb for that —
> ask the owner to grow one."

That last sentence is the quiet keystone: *minting a new verb requires the
owner*, because `learn`/`revise` run as the actor's Unix user — a shell
only the owner (or root) has. The verb set is the policy surface, and it
only changes through the owner's hands. Injection against a disciplined
custodian can invoke existing verbs with bad arguments (validation's job)
or produce bad testimony (reputational damage, witnessed and attributable
via round four) — but it cannot mint authority.

Two corollaries the discipline implies:

- **No verb may re-emit caller text as declarations.** The strange loop
  compiles any `command.declared` that lands in ingested events; a
  custodian verb that echoes caller-supplied text into event *names* or
  declarations is an escalation hole. Review point at learn time — and a
  reason custodians should not carry the chat lesson's
  "declare capabilities mid-conversation" behavior.
- **The blast radius is the home directory.** The custodian's mind runs as
  the custodian's user. A fully hijacked custodian can wreck its own
  resource — not the system, not the other actors. Per-actor Unix users
  are per-actor blast radii: the process-isolation story agent frameworks
  keep reinventing with containers, available since 1973.

## 5. The weak wall: on a shared host, loopback is not a membrane

Here is the honest problem, and it should be stated before anyone deploys
this: **Unix permissions protect the file; they do not protect the port.**
`SELF_BIND` defaults to loopback, and on a multi-user machine loopback is
reachable by *every local user*. Two consequences:

- The write path is open: any local user can POST to `/run/config/...`.
  Often acceptable — the verbs validate, and that was the point — but it
  is a decision to make, not a default to assume.
- The read path is worse: `/events` serves the raw log to anyone who can
  reach the port. A custodian whose events carry the resource's *content*
  has quietly undone its own membrane — the file is 0700 and its bytes are
  on `:7777`.

Remedies, all outside the kernel, in the order to reach for them:

1. **Pointers, not content** (the harvest rule, again): custodian events
   record that a change happened, its parameters, its provenance — never
   secret material. The git repo in the 0700 home holds the bytes; the log
   holds the story. This alone makes the read path safe for most configs.
2. **Owner-matched firewalling**: netfilter's `-m owner --uid-owner` can
   restrict who may connect to the actor's port — a per-UID membrane for
   TCP, standard ops, no kernel change.
3. **A Unix-socket front door**: a small proxy (`socat`
   `UNIX-LISTEN:...,mode=0770` → loopback port) puts the surface itself
   behind filesystem permissions, and a verifying proxy can inject a real
   `X-Self-Caller` while it is at it — turning round four's claimed `by`
   into a verified one, still without the kernel learning any of it.

And one strength to set against the weak wall: the log gives
**tamper-evidence** even where Unix gives only tamper-resistance. A
`reconcile` verb — in the timers pattern, invoked by cron or a session,
never by a kernel daemon — hashes the resource and compares it to the last
state the log implies; drift means an out-of-band write, surfaced on the
custodian's page. The custodian notices burglary. Which yields the
break-glass discipline: `sudo` interventions are legitimate — *unrecorded*
ones are the sin. Break glass, then tell the actor (deposit a small
account of what was done and why); the intervention-visibility principle
from the original protocol extends unchanged to root.

## 6. Instantiations

- **Config custodian** — the opening example: hot-reloaded config, verbs
  that validate before write, drift detection, archaeology on demand.
- **Deploy steward** — holds the deploy key nobody else has; exposes
  `deploy/<env>`; every deploy is an event with a door and a speaker.
- **Migrations gatekeeper** — owns the schema-change path; the log is the
  schema's story; "why is this column nullable?" has an answerable owner.
- **Budget steward** — holds an external API's credentials; exposes the
  sanctioned calls; rate-limits and logs spend; the bill has a witness.

Each is one lesson plus one Unix user plus ordinary ops. The kernel is
untouched — round five keeps the streak, and the round-four fields turn
out to be the only kernel support any of it needed.

---

The arc, updated: witness (knowledge with proof), then custodian (a
resource with a story), then steward (authority as a service). The same
body, the same log, the same protocol — what changes is only what the
actor's home directory holds and what its verbs are allowed to touch. The
sky is not quite the limit; the limit is the membrane you can actually
enforce and the discipline of keeping minds testifying while scripts act.
But inside those two lines, the pattern generalizes as far as Unix does.

*Round five of an ongoing design conversation.*
