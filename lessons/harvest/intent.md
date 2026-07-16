# harvest — what execution taught, kept on purpose

## purpose

Sessions record execution faithfully: started, progressed, ended, outcome
retained. But an outcome is history, not knowledge — a week later the
deploys, regressions, and promises buried inside it are invisible to the
work and followup surfaces, and the morning brief says "no notes" over a
log full of finished work. Harvest is the deliberate promotion step: after
a session ends, a mind reads its outcome and decides what deserves to
outlive it — a decision, a note, a signal, a commitment — records those
through the verbs the instance already has, and then marks the session
harvested. The queue of ended-but-unharvested sessions stays visible on
the surface until it is empty. Nothing is promoted automatically: the
judgment is the mind's; the log only remembers that the judgment happened.

## surface

- `self run harvest <session-seq> <note…>` appends one `session.harvested`
  event `{ref, note, by}` — `ref` is the sequence number of the session's
  `session.started` event, and the note says what was kept and where
  ("kept: worklog 590, signal 591") or states plainly that nothing was
  ("nothing durable — routine deploy watch, clean"). `by` comes from
  `SELF_MIND_ID` when the caller's environment provides it, falling back
  to `SELF_BRAIN_ID` for older environments.
- The projection that already shows sessions (`/agents` on instances that
  have one) grows a **harvest queue** in place — not a new page: every
  session that has ended and whose seq no `session.harvested` ref covers,
  newest first, each with its outcome text and the exact harvest command
  to run. When the queue is empty, the section says so in one line. An
  instance with no sessions surface has nothing to harvest and should not
  learn this lesson.

## constraints

- Exactly one command (`harvest`), one event name (`session.harvested`),
  zero new pages: the existing sessions projection is revised to also
  consume `session.harvested` and render the queue.
- Harvesting a seq that is not an ended session, or one already
  harvested, appends nothing and prints a plain-text warning to stderr —
  never a crash, never a duplicate receipt.
- The queue is computed at render time from `session.ended` minus
  `session.harvested` refs — no cache, no cursor, no state outside the
  log.
- Durable facts travel through the verbs that already exist (decide,
  worklog, signal, owe, waiting); harvest records only the receipt that
  the outcome was read and considered. The note carries pointers, not
  content.

## anti-goals

- No automatic promotion. A script must never guess what an outcome
  taught; the command records a mind's explicit act, nothing more.
- Not a second worklog. If harvest notes start carrying the knowledge
  itself instead of pointing at promoted events, the step has failed.
- No nagging machinery: no timers, no reminders, no chase. The visible
  queue is the whole pressure.

## what good looks like

A session ends: "Tagged v10.76.3, deployed to production, merged the tag
into master." The next mind to wake — the same one, or a cold one reading
the queue — runs `self run worklog monolith "v10.76.3 tagged and deployed
…"`, then `self run harvest 578 kept: worklog 586`. A read-only audit
session that found nothing gets `self run harvest 583 nothing durable —
clean audit`. The queue shrinks to zero; the work page now tells the story
of the week the sessions actually lived; a cold start reads the brief and
learns what execution taught, not merely that execution occurred.
