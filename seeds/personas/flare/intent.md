# flare — June's own record

## who this is for

June, 45, rheumatoid arthritis, diagnosed nine years ago. Twice a year she
gets fifteen minutes with her rheumatologist and spends the first five
trying to reconstruct months from memory: "worse in… March, I think?" She
knows exactly what she wants — her own record, in her own words, that
turns those five minutes back into medicine.

## surface

- `self run day <severity> <notes…>` appends one `flare.day` event: how
  bad today is, 1–10 in her own calibration, and what she wants to
  remember about it ("hands bad by noon, rain all week").
- `self run med <name> <notes…>` appends one `flare.med` event: a
  medication started, stopped, or changed ("methotrexate up to 20mg").
- `/flare` renders the last month or so: each day's severity as a bar of
  fixed-width marks with its notes — a shape she can see at a glance —
  with med changes marked inline on the days they happened.
- `flare/doctor` renders the consultation summary: severity by month for
  the last twelve (worst, best, and a rough middle, listed not charted),
  every med change with its date, and her most recent notes. Printable,
  boring, and exactly what fifteen minutes needs.

## constraints

- Severity clamps to 1–10 on render. Days she logs nothing are shown as
  gaps, never interpolated — an unrecorded day is unknown, not average.
- Med events must appear on both pages; a dose change is the single most
  important fact in the record.
- Everything renders from her events only. Empty log: the two forms and
  no advice.

## anti-goals

- No diagnosis, no trend interpretation, no "your symptoms suggest". The
  page never says anything about her body that she didn't say first. It
  is a mirror, not an opinion.
- No wellness anything: no streaks, no reminders to log, no "you've got
  this!". Missing a week because her hands hurt is data the gaps already
  express; the page never nags her about it.

## what good looks like

June logs most days in ten seconds at the kettle. In April her
rheumatologist gets a printed page instead of "March, I think": February
was an 8 for two weeks, the new dose started March 3rd, the bars sink
after. The appointment spends its fifteen minutes on what to do next
instead of on archaeology — which is what the record was for.
