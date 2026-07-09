# tables — Priya's map of where to eat

## who this is for

Priya, 42, pharmaceutical sales across three cities, dinner out four nights
a week — half of it with clients she needs to impress. Her problem is not
finding restaurants; it is remembering them: which place had the tasting
menu worth the detour, what she ordered, and where she can safely take a
vegetarian regional director on a Tuesday.

## surface

- `self run meal <restaurant> <dish> <rating> <notes…>` appends one
  `tables.meal` event: where, what she ate, a rating 1–5, and notes —
  including the things that matter for next time ("quiet enough to talk
  business", "book the counter, not the room").
- `/tables` renders her ranking: restaurants ordered by average rating,
  each with visit count, best dish so far (the highest-rated meal's dish),
  and the latest note. This is the page she opens when someone says "you
  pick".
- `tables/journal` renders every meal newest first — the raw diary the
  ranking is computed from, one link down.

## constraints

- Ratings clamp to 1–5 on render; a typo'd 9 counts as 5, never crashes.
- The ranking is recomputed from all `tables.meal` events every time. A
  restaurant that slips serves worse meals and sinks on its own; nothing is
  ever edited to move it.
- Restaurant names group case-insensitively, so "Chez Denise" and "chez
  denise" are one table, not two.

## anti-goals

- No stars imported from anywhere. Priya's 4 means Priya's 4; the whole
  value of the page is that every number on it passed through her palate.
- No reservations, no menus, no photos. This is memory, not a booking app.

## what good looks like

A client says "somewhere good near the station, nothing loud". Priya opens
`/tables`, and the third entry has her own note from March: "quiet, corner
tables, the duck". She books it, it lands, and after dinner she adds one
more meal event from the taxi — the map gets better every time she eats.
