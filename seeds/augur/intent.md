# augur — prophecy, made accountable

## purpose

The minds that use this runtime wake cold, act, and vanish. `memory` keeps
what a mind learned; nothing keeps what a mind *believed would happen* — and
whether it was right. This seed grows an augury: speak a falsifiable claim
about the future with a stated confidence, and the append-only log makes it
irrevocable — a prophecy cannot be backdated, softened, or unsaid, because
unsaying is the one thing this system cannot do. A later session (any mind,
not necessarily the augur) judges it, and calibration stops being a feeling:
it is a Brier score, a pure fold of the log, replayed to the same digits by
every rebuild. The kernel already signs *who wrote what* into every receipt;
this seed extends provenance to foresight — who saw what coming, kept in a
book that never forgets.

## surface

- `self run augur <confidence> <claim…>` speaks a prophecy. `confidence` is
  an integer 0–100 — the percent chance the augur gives the claim of coming
  true; the rest of argv joins into the claim. A valid call appends one
  `augur.spoken` event `{claim, confidence, by}`; `by` is read from
  `SELF_BRAIN_ID`, defaulting to "an unnamed augur". A confidence that does
  not parse as an integer in 0–100 appends `augur.muttered` `{text, by}`
  instead — the whole argv as text: recorded, never scored, never an error.
- `self run judge <prophecy> <verdict> [note…]` closes one. `prophecy` is
  the **seq of its `augur.spoken` event**; `verdict` is exactly `true` or
  `false`; any remaining argv joins into an optional note. A valid judgment
  of an open prophecy appends `augur.judged`
  `{prophecy, verdict, note, by}`. A judge that names no open prophecy, or
  speaks any other verdict, appends `augur.moot` `{prophecy, verdict, by}` —
  a footnote, never an error.
- `/augur` renders, in order: the open prophecies, oldest first — seq,
  claim, confidence, augur, and when spoken — each with one small form
  POSTing to `/run/judge`: a hidden field carrying the seq first, then two
  named submit buttons, "came true" (value `true`) and "did not" (value
  `false`), in that order, because the server maps form values to argv in
  document order; then the judged prophecies, most recently judged first,
  each with its outcome, note, and judge; then the reckoning — one row per
  augur: prophecies spoken, open, judged, come true, and the Brier score.
  The page teaches the practice to whoever reads it: a good prophecy is
  falsifiable, judgeable by a stranger, and carries its own context; check
  the reckoning before trusting your own confidence.

## the mechanics (exact — the score is only worth what its replay is)

- **A prophecy is its seq.** Every reference — judgments, moots, forms, the
  rendered page — names the `seq` of the `augur.spoken` event.
- **The earliest valid judgment binds.** Replaying the log in order, an
  `augur.judged` event binds if its `prophecy` names a real `augur.spoken`
  seq not already bound and its `verdict` is `true` or `false`; every other
  `augur.judged` — later, duplicate, dangling, hostile — is inert. The
  `judge` command refuses to append a non-binding judgment (it appends
  `augur.moot` instead), but a foreign log may contain them anyway, so the
  projection applies the same earliest-wins fold rather than trusting the
  command to have kept the log clean. Both scripts fold the log the same
  way; whether they share that fold or each carry it is the compiler's
  choice.
- **The score is integer arithmetic.** For each judged prophecy: the error
  is `confidence − 100` if it came true, `confidence − 0` if it did not;
  its contribution is the error squared — 0 to 10000, the classical Brier
  in ten-thousandths. An augur's score is the sum of contributions divided
  by their count using integer division, rendered as `0.` plus exactly four
  zero-padded digits (a perfect 10000 renders `1.0000`). Every quantity is
  non-negative, so floor and truncation agree in any language the compiler
  picks. Lower is better; the page says so, and says that an augur who
  always answers 50 earns `0.2500` — the score of admitting you don't
  know. Below that line is knowledge; above it is worse than ignorance.
- **The reckoning is ordered.** Scored augurs first, ascending by score,
  ties broken by name; then augurs with nothing judged yet, by name, their
  score shown as "—". Muttered and moot events must never touch a score;
  whether and how they render is the compiler's choice.

## anti-goals

- Never a clock read or a random number: no deadlines, no expiry. A
  prophecy stays open until a mind judges it; "overdue" is a judgment,
  and judgment belongs to minds, not kernels.
- Never a correction. Even a wrong judgment binds — the remedy is a new
  prophecy, or a moot left as a footnote, never a rewrite. That is not a
  limitation of this seed; it is the constraint the whole system is built
  on, made visible enough to feel.
- Never render a claim, note, or name unescaped; a prophecy crafted to
  look like markup is just an omen strangely worded.
- Not a market: no currency, no odds beyond the stated confidence, no
  payout. The only stake is a name attached to a number that will not go
  away.
- Never refuse the empty log: no prophecies renders the invitation — the
  book of prophecy is open, speak the first — with the reckoning empty and
  nothing broken.

## what good looks like

1. A session about to attempt a migration runs
   `self run augur 85 the migration will apply cleanly on the first attempt`.
   The prophecy is in the log before the attempt is; nothing can now unsay
   it.
2. The attempt fails. The same session — or a later one; the judge need not
   be the augur — runs `self run judge 12 false first run hit a locked
   table`. `/augur` moves the prophecy to the judged list, and that augur's
   score ticks away from zero.
3. Sessions later, a cold mind reads `/augur` before promising anything,
   and finds what no single session could have learned: the minds of this
   instance run overconfident above 80. It says 70 instead, and means it.
4. `self rehydrate` from `events.jsonl` + `.secret` alone rebuilds the same
   book — the same open prophecies, the same verdicts, the same digits in
   every score. Two instances that adopt this seed each keep their own
   book; only intent crosses, never a prophecy.

No `seed.jsonl`, deliberately: a prophecy laid down by the seed's author
would be about *their* future. The first entry in this book must be a claim
this instance can watch come true or fail.
