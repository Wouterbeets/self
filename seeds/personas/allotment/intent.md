# allotment — Bernadette's plot 14b

## who this is for

Bernadette, 68, retired primary-school teacher, plot 14b since 2011. Her
garden diary is three exercise books, one of which went soft in the rain in
2019 and took two springs' worth of sowing dates with it. Every March she
asks the same questions: what went in this bed last year, when did I sow
the leeks, and did the yellow courgettes actually earn their space. And on
the shelf, the two surviving books are still paper, still one rain away
from joining the third.

## surface

- `self run sow <crop> <bed> <notes…>` appends one `allotment.sown` event:
  what went in, which bed ("B3", "the long bed"), and notes ("second
  batch, first bolted"). An optional photo rides along — the back of the
  seed packet is the one she takes, because that is where the instructions
  live and the packet is in the compost by June.
- `self run harvest <crop> <amount…>` appends one `allotment.harvest`
  event: what came out and how much, in her own units ("two trugs",
  "1.4 kg", "enough for the neighbours too"), with an optional photo of
  the actual trugs on the actual bench.
- `self run page <photo> <year> <notes…>` appends one `allotment.page`
  event: one photographed page of an exercise book, pinned to its year,
  with what she can still read of it ("spring sowings, the smudge was the
  carrots"). This is how eleven years of paper get in — one page at a
  time, over winter, with the radio on.
- A weekly timer runs `self run thisweek`: it gathers what this calendar
  week looked like in every past year of her own log — sowings, harvests,
  the photographed pages that mention it — and emails her the digest on
  Sunday evening ("week 11 in other years: leeks in on the 14th in 2014,
  the 9th last year; first rhubarb pulled in 2022"). A week with no
  history sends nothing. One `allotment.thisweek` event records each
  digest sent.
- `/allotment` renders the year: what is currently in each bed (the
  sowings of this year, grouped by bed, packet photos linked), then the
  harvest tally — every crop with its harvests listed, this year first.
- `allotment/almanac` renders past years, one section per year, sowings
  and harvests together, and each year's photographed exercise-book pages
  rendered with it — the books themselves, rain-proof at last, one link
  down.

## constraints

- Years are grouped from `occurred_at` for what she types today; a `page`
  event's year is her word, stored as typed, and the almanac files it
  there — the photo was taken now, but the spring it records was 2014.
- Amounts are text, never parsed into numbers. "Two trugs" is data. The
  tally lists; it does not sum what cannot be summed.
- Photos render as photos, full size when opened, never cropped or
  "enhanced" — the smudge on the carrot page is part of the record.
- An empty log renders "nothing sown yet" over the forms, in February's
  spirit rather than an error's.

## anti-goals

- No planting calendars, no frost-date advice, no companion-planting tips.
  Bernadette has forgotten more about gardening than the internet knows;
  this page holds her knowledge, it does not offer any of its own. The
  Sunday digest honors the same line: it is her own diary talking, in her
  own words, from her own years — the machine only remembers which week
  it is. The day it suggests anything she didn't once do herself, it has
  broken.
- No transcription of the exercise books. The photo of the page IS the
  record; typing it all out is a chore nobody assigned. Her notes on a
  page event say what matters; the ink says the rest.

## what good looks like

March. Sunday's email says week 11 is leek week — the 9th last year, the
14th in 2014, straight off a photographed page of the old book — and that
the yellow courgettes gave one trug for a whole bed.
The leeks go in on the 10th, photographed packet and all; the courgettes
lose their bed to dahlias. Over Christmas she photographs the last of book
two, and the exercise books retire to the shelf as objects, not as the
only copy of anything.
