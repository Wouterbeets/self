# flare — June's own record

## who this is for

June, 45, rheumatoid arthritis, diagnosed nine years ago. Twice a year she
gets fifteen minutes with her rheumatologist and spends the first five
trying to reconstruct months from memory: "worse in… March, I think?" She
knows exactly what she wants — her own record, in her own words, that
turns those five minutes back into medicine. And she knows something the
waiting room taught her: "you should have seen it last month" convinces
nobody, but a photograph with a date on it ends the discussion.

## surface

- `self run day <severity> <notes…>` appends one `flare.day` event: how
  bad today is, 1–10 in her own calibration, and what she wants to
  remember about it ("hands bad by noon, rain all week"). An optional
  photo rides along — the swollen knuckles, the rash — evidence with a
  date, taken in ten seconds at the kettle.
- `self run med <name> <notes…>` appends one `flare.med` event: a
  medication started, stopped, or changed ("methotrexate up to 20mg").
- `/flare` renders the last month or so: each day's severity as a bar of
  fixed-width marks with its notes — a shape she can see at a glance —
  med changes marked inline on the days they happened, and a small camera
  mark on photographed days, each linking its picture.
- `flare/doctor` renders the consultation summary: severity by month for
  the last twelve (worst, best, and a rough middle, listed not charted),
  every med change with its date, her most recent notes, and the
  photographed days as a dated strip at the end. Printable, boring, and
  exactly what fifteen minutes needs.
- `self run handover <notes…>` produces a file: the doctor summary as it
  stands today, frozen into one standalone document — deposited into the
  store, recorded by one `flare.handover` event with its hash and her note
  ("spring appointment, Dr. Osei"). What the doctor saw in April stays
  what the doctor saw in April, even as the living page moves on.

## constraints

- Severity clamps to 1–10 on render. Days she logs nothing are shown as
  gaps, never interpolated — an unrecorded day is unknown, not average.
- Med events must appear on both pages; a dose change is the single most
  important fact in the record.
- Photos are dated by their events, never by anything read out of the
  image file, and they render exactly as taken. On the doctor page they
  appear with their day's severity and note — the sentence and the
  evidence, together.
- A handover is frozen when made; the page lists every handover by date,
  so the record of what each appointment was given is itself part of the
  record.
- Everything renders from her events only. Empty log: the two forms and
  no advice.

## anti-goals

- No diagnosis, no trend interpretation, no "your symptoms suggest". The
  page never says anything about her body that she didn't say first. It
  is a mirror, not an opinion.
- No wellness anything: no streaks, no reminders to log, no "you've got
  this!". Missing a week because her hands hurt is data the gaps already
  express; the page never nags her about it.
- The handover goes where June sends it and nowhere else. Nothing
  uploads, syncs, or "shares with your care team". It is a file in her
  store; the choosing is hers.

## what good looks like

June logs most days in ten seconds at the kettle; the bad mornings get a
photo. In April her rheumatologist gets a printed handover instead of
"March, I think": February was an 8 for two weeks — photographed — the new
dose started March 3rd, the bars sink after. The appointment spends its
fifteen minutes on what to do next instead of on archaeology. And when a
locum in September doubts the spring was that bad, June's phone has the
April handover, frozen, dated, exactly as Dr. Osei saw it.
