# herd — meta-context for the agents herdr is managing

## what it's for

herdr is an agent multiplexer — tmux for AI coding agents. One terminal holds
the whole herd: workspaces, tabs, and panes, each pane an agent (claude, codex,
copilot, …), and a sidebar that rolls every agent up to blocked / working /
done / idle. It is superb at *now*: who needs input this second. It has no
opinion about *why*: what each agent is actually building, whether the work
across workspaces adds up to anything, or what last month's herd was even for.

That missing half is exactly what self is. This seed grows the surface where
herdr's live present becomes self's permanent memory: an HTML overview at
`/herd` of everything herdr is managing — each agent and its state, what it is
building, and the long arc per domain — so the person (and any brain reading
the garden) has meta-context around their AI usage and long-term coherence
around what they're building in every domain of their life.

## the core intuition

herdr and self hold opposite truths, and this surface should marry them rather
than duplicate either:

- **herdr's truth is live and local.** Sessions are plain JSON on local disk,
  and a Unix socket API plus CLI can list workspaces and panes, read agent
  output, and report each agent's state (herdr.dev/docs — socket-api,
  persistence-remote). herdr is the authority on the present — and only the
  present.
- **self's truth is the log.** Every observation taken from herdr becomes an
  append-only event. herdr forgets a pane the moment it closes; the log never
  does. So the overview is not a mirror of herdr — it is herdr's present
  stacked into a past: which agents ran, on what, how long they sat blocked,
  what they produced. herdr has no memory of meaning; self is nothing *but*
  memory.

Observation happens at three altitudes, and the page must honor all three:

1. **The now** — the herd as herdr sees it this moment: workspaces, panes,
   agents, states. Blocked agents surface first; they are the ones burning
   the person's time.
2. **The work** — what each agent is actually building. Raw pane output is
   noise; the brain (`self think`) reads an agent's recent output and writes a
   one-breath digest ("migrating auth to sessions; stuck on a failing
   integration test"). Digests are events too — over time the log accumulates
   a build journal nobody had to write.
3. **The arc** — workspaces gather into domains. A domain is a strand of the
   person's life or work: this product, that research thread, the infra, the
   book. Per domain the brain maintains a standing narrative: what is being
   built here, what changed lately, and whether today's herd activity coheres
   with that direction or wanders from it. This is the meta-context no single
   agent — scoped to its own pane — can ever hold.

The lens the brain digests through is data, not kernel: it lives in the log as
a doctrine event (deposited by this seed) and changes by appending, exactly as
chat's identity does.

## the feel

- One glance at `/herd` answers three questions, in this order: **who needs
  me** (blocked agents, at the top), **what is everyone building** (digests,
  never scrollback), **is it going somewhere** (domain arcs).
- Honest time. Every observation is stamped; the page says "as of two minutes
  ago" and never pretends to be live. A herd not observed for a day says so,
  plainly.
- History as texture. An agent's card carries its past lives — earlier
  sessions in the same workspace, previous digests — because the log has
  them and herdr doesn't.
- Domains are the person's own words. Filing workspaces into domains is
  editable by appending, and unfiled workspaces are shown loudly rather than
  hidden — an unfiled strand of work is precisely a coherence leak.

## anti-goals

- **Observe, never herd.** herdr's socket can also create workspaces, split
  panes, and spawn helpers — this surface must not. Read-only ingestion: the
  person drives herdr; self remembers and makes meaning.
- Never dump raw scrollback into the log. Digest, then append — the log
  stores meaning at a size that replays forever. (A short excerpt kept as
  evidence for a digest is fine; wholesale terminal capture is not.)
- Never hardcode herdr's paths or schema. Where the socket lives and what the
  session JSON looks like are discovered on *this* machine at grow time — and
  the surface degrades gracefully: an installed-but-quiet herdr is an empty
  herd, not an error; an absent herdr says so and points at herdr.dev.
- Never fake liveness. Staleness shown is trust kept.

## the surface (the public shape, fixed; the decomposition is the orchestrator's)

- The overview lives at `/herd`.
- Taking one observation pass over herdr is an explicit, ordinary act named
  `graze` — run by hand, from a heartbeat, or on a timer; every pass appends
  what it saw and what the brain made of it.
- Filing a workspace into a domain is an act named `file` (a workspace, a
  domain name, appended like everything else).
These names are part of how the product should feel; *how* they're realized
(which events, how many scripts, what they share) is for growth to decide
here.
