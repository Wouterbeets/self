# tables — Priya's map of where to eat

## who this is for

Priya, 42, pharmaceutical sales across three cities, dinner out four nights
a week — half of it with clients she needs to impress, most of it on the
company card. Her problem is not finding restaurants; it is remembering
them: which place had the tasting menu worth the detour, what she ordered,
where she can safely take a vegetarian regional director on a Tuesday — and
where the receipt went, because expense day is the worst day of her month.

## surface

- `self run meal <restaurant> <dish> <rating> <spend> <notes…>` appends one
  `tables.meal` event: where, what she ate, a rating 1–5, what it cost, and
  notes — the things that matter for next time ("quiet enough to talk
  business", "book the counter, not the room"). An optional receipt photo
  rides along, its hash in the event.
- `/tables` renders her ranking: restaurants ordered by average rating,
  each with visit count, best dish so far (the highest-rated meal's dish),
  and the latest note. This is the page she opens when someone says "you
  pick".
- `tables/journal` renders every meal newest first — the raw diary the
  ranking is computed from, one link down.
- `tables/expenses` renders the money: months newest first, each meal with
  its spend and its receipt photo linked — the month's paperwork already
  assembled, one link down.
- `self run expense-report <month>` produces a file: a CSV of that month —
  date, restaurant, amount, and the receipt's `/files/…` link per row —
  deposited into the store and recorded by one `tables.report` event. She
  downloads it once and feeds the corporate tool in one sitting.

## constraints

- Ratings clamp to 1–5 on render; a typo'd 9 counts as 5, never crashes.
- Spend is text she typed. A number sums into the month's total; "client
  paid" or a blank lists in the month, marked, uncounted — the meal still
  counts toward the ranking either way.
- The ranking is recomputed from all `tables.meal` events every time. A
  restaurant that slips serves worse meals and sinks on its own; nothing is
  ever edited to move it.
- Restaurant names group case-insensitively, so "Chez Denise" and "chez
  denise" are one table, not two.
- The report is a rendering of events that already happened: making it
  twice for the same month with the same meals yields the same rows.

## anti-goals

- No stars imported from anywhere. Priya's 4 means Priya's 4; the whole
  value of the page is that every number on it passed through her palate.
- No reservations, no menus, no food photos. This is memory, not a feed.
  A receipt is not a food photo — it is a number with a date on it, and it
  rides along so expense day stops being archaeology.
- No currency conversion, no per-diem rules, no policy. The CSV carries
  what she typed; the corporate tool can do its own arguing.

## what good looks like

A client says "somewhere good near the station, nothing loud". Priya opens
`/tables`, and the third entry has her own note from March: "quiet, corner
tables, the duck". She books it, it lands, and from the taxi she adds the
meal — rating, 84 euro, receipt photographed before it dissolves in her
bag. On the last Friday of the month she runs `expense-report`, downloads
one CSV, and does a month of expenses in eleven minutes — every receipt one
click away, none of them in a coat pocket.
