# firstyear — Noor's first year

## who this is for

Sam and Alex, first-time parents. Noor is three weeks old. Every entry
into this thing will be typed one-handed, on a phone, in the dark, by
someone who has slept in ninety-minute pieces. The questions it must
answer are brutally small: when was the last feed, how many today, and —
months from now — when did she first laugh.

## surface

- `self run feed <notes…>` appends one `firstyear.feed` event. Notes are
  whatever the awake parent can manage: "left side", "90ml", or nothing
  at all — an empty feed event is a complete, valid entry.
- `self run nap <notes…>` appends one `firstyear.nap` event, same rules.
- `self run milestone <text…>` appends one `firstyear.milestone` event:
  "first smile, at the ceiling fan of all things".
- `/firstyear` renders, in order of 4 a.m. importance: time since the
  last feed and today's feed count, huge, at the top; then today's feeds
  and naps as a simple timeline; then the milestones, oldest first — the
  keepsake growing under the logistics.

## constraints

- The top of the page must be legible in one bleary glance: last feed,
  count today. Everything else is below the fold.
- "Today" is computed from `occurred_at` at render time. No entry ever
  asks for a time; the moment of typing is the truth.
- Every form on the page is one tap and optional text. Any field that is
  required is a field that loses a 4 a.m. entry.

## anti-goals

- No averages, no curves, no comparisons to "typical babies", no advice.
  A tired parent reading "below usual" at 3 a.m. is harm, not a feature.
  The page reflects only what Sam and Alex recorded, and never grades it.
- No medical anything. If it looks like a chart a doctor would want, it
  has gone too far.

## what good looks like

3:47 a.m. Alex feeds Noor, taps once, types nothing, sleeps. At 7, Sam
opens the page and knows the night in one glance without waking anyone.
In June, someone asks when Noor first laughed, and instead of "some point
in spring?" there is a date, and the sentence about the ceiling fan.
