# allotment — Bernadette's plot 14b

## who this is for

Bernadette, 68, retired primary-school teacher, plot 14b since 2011. Her
garden diary is three exercise books, one of which went soft in the rain in
2019 and took two springs' worth of sowing dates with it. Every March she
asks the same questions: what went in this bed last year, when did I sow
the leeks, and did the yellow courgettes actually earn their space.

## surface

- `self run sow <crop> <bed> <notes…>` appends one `allotment.sown` event:
  what went in, which bed ("B3", "the long bed"), and notes ("second
  batch, first bolted").
- `self run harvest <crop> <amount…>` appends one `allotment.harvest`
  event: what came out and how much, in her own units ("two trugs",
  "1.4 kg", "enough for the neighbours too").
- `/allotment` renders the year: what is currently in each bed (the
  sowings of this year, grouped by bed), then the harvest tally — every
  crop with its harvests listed, this year first.
- `allotment/almanac` renders past years, one section per year, sowings
  and harvests together — the exercise books, rain-proof, one link down.

## constraints

- Years are grouped from `occurred_at`; nothing asks Bernadette for a
  date, because the day she types it is the day it happened.
- Amounts are text, never parsed into numbers. "Two trugs" is data. The
  tally lists; it does not sum what cannot be summed.
- An empty log renders "nothing sown yet" over the two forms, in
  February's spirit rather than an error's.

## anti-goals

- No planting calendars, no frost-date advice, no companion-planting tips.
  Bernadette has forgotten more about gardening than the internet knows;
  this page holds her knowledge, it does not offer any of its own.

## what good looks like

March. Bernadette opens `/allotment/almanac`, finds she sowed leeks on the
9th last year and the year before that the 14th, and that the yellow
courgettes gave one trug for a whole bed. The leeks go in on the 10th, the
courgettes lose their bed to dahlias, and the exercise book stays dry on
the shelf, retired.
