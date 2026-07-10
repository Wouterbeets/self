# fieldnotes — Amara and Ben's confluence

## who this is for

Amara, 41, marine ecologist. Three years of shore observations from the
same eleven sites — nesting starts, clutch counts, the things you only
know if you were standing there at dawn — living in her instance because
the university's system wants them in a schema that has no field for "the
third bay was wrong today and I can't say why yet." Ben, 47, hydrologist,
two catchments inland, four years of temperature and salinity readings in
his. They have met twice, at a conference and a funeral. What they suspect
— what they have suspected for a year, over email — is that her late
nesting seasons follow his warm-water anomalies by about six weeks. Neither
record can show it alone.

This seed is grown on **Amara's** instance, after planting Ben's export.
It works the same the other way; science should flow both directions.

## how the records meet

Ben runs `self export readings. <dir> ben.` — four years of readings,
dates preserved, renamed on the way out — then does the thing the
protocol is for: he opens `seed.jsonl` and reads it, deletes the lines
from the one gauge that sits on private land he'd rather not advertise
(the seed is plain text; curation is deletion), and writes the intent
stub properly — what the readings are, which instruments, what he thinks
they mean, what he is not sure of. Amara plants it, reads his intent, and
grows this seed. The merge is hers: his events on her machine, her brain
writing the projection, her key signing it.

## surface

- `self run obs <site> <species> <note…>` appends one `fieldnotes.obs`
  event — her own diary, with an optional photo (the nest, the
  thermometer, the wrongness of the third bay).
- `/fieldnotes` renders her observations newest first, by site — her own
  working diary, as it always was.
- `/confluence` renders the merge: one timeline per season, her
  observations and his `ben.reading` events interleaved by date, sites
  and gauges labeled with whose they are. Where a nesting note falls
  within some weeks of a flagged anomaly in his series, the row says so —
  visibly, as a proximity, with the lag stated ("41 days after the
  Kestrel gauge anomaly Ben flagged").
- `confluence/unmatched` renders what didn't line up: her late seasons
  with no anomaly behind them, his anomalies that nothing followed. The
  honest page — a correlation you can't see failing is not a finding.

## constraints

- Ben's events stay `ben.*`, verbatim, forever; provenance survives the
  merge. Every row on the confluence page says whose record it came from
  — two names are on this work, and the page is the author list.
- The lag between an anomaly and an observation is computed at render
  time from the two `occurred_at`s, never stored — replant Ben's next
  export and the page recomputes from scratch, byte for byte.
- His readings mean what his intent.md says they mean. Where his fields
  are opaque, the page shows them raw and labeled rather than guessing —
  a misread instrument is worse than an ugly row.
- An update is a fresh export, fresh plant: append-only on both sides,
  no sync, no merge conflicts — later events land later in her log, and
  the replay sorts them into place by date.

## anti-goals

- No statistics the page can't defend. Proximity in time, stated lags,
  counts — yes. Correlation coefficients, trend lines, significance — no.
  The page raises the question; the paper they may one day write answers
  it. A rendered p-value with no methods section is how bad science
  starts.
- No blending. Never average her sites with his gauges, never plot them
  as one series. Two instruments, two observers, two records — adjacency
  is the product, synthesis is the humans' job.
- Not a repository, not a portal, no logins. Ben cannot see her page;
  she cannot see his. What crossed is exactly what each chose to export,
  reviewed line by line, and both logs remember the exchange.

## what good looks like

Ben's folder arrives in March. Amara plants it, and that evening
`/confluence` shows season three the way she has been squinting at it in
her head for a year: the Kestrel anomaly in April, her two latest-ever
nesting starts at the exposed sites, 38 and 44 days behind. But the page
also shows season one, where the same anomaly was followed by nothing —
and `unmatched` is why their eventual paper survives review, because the
reviewer's first objection is already a page. She exports her own three
years back to him, minus the two protected-site locations, with an intent
that ends: "check season one against your second gauge — I think your
instrument moved." It had. Two sovereign records, one insight neither
contained, and every step of how they got there is in both logs.
