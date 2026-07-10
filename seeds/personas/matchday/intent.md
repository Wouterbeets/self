# matchday — Dave's season

## who this is for

Dave, 51, forklift driver, Sunderland supporter since his dad took him in
1981. He can tell you the score of a match from 1997 but not last month —
and it bothers him. He does not want stats sites, xG, or fantasy points. He
wants the season written down the way he'd tell it at work on Monday. And
in the wardrobe there is a shoebox: forty years of ticket stubs and
programmes, one flood away from being nothing.

## surface

- `self run match <competition> <opponent> <homeaway> <score> <notes…>`
  appends one `matchday.match` event: which competition ("league", "cup"),
  who we played, `home` or `away`, the score written ours-first ("2-1"),
  and what he'd actually say about it ("robbed. keeper worth the ticket
  alone"). An optional photo rides along — the stub, the view from the
  away end — and its hash lives in the event.
- `self run relic <photo> <season> <notes…>` appends one `matchday.relic`
  event: one piece of the shoebox — a stub from the '92 play-off, a
  programme cover — photographed and pinned to its season ("1984-85",
  "1998-99"), with whatever Dave remembers about it.
- `self run seasoncard <season>` produces a file: one printable page for a
  finished season — the record, every match with its notes, the relics —
  deposited into the store and recorded by one `matchday.card` event
  carrying its hash. The paper copy goes in the drawer next to his dad's
  programmes.
- `/matchday` renders the season: the running record — won, drawn, lost,
  goals for and against, computed ours-first from every score — big at the
  top, then every match newest first with its notes and its photo if it has
  one. Matches from earlier years fold into one line per season below, each
  linking its season card when one has been made.
- `matchday/shoebox` renders the relics: every photographed stub and
  programme, grouped by season, oldest first — forty years, flood-proof,
  one link down.

## constraints

- The record is computed from `matchday.match` events at render time,
  grouped into seasons by `occurred_at` (a season runs August through the
  following July). Nothing is stored twice.
- A relic's season is Dave's word, stored exactly as typed; the shoebox
  page groups by that text. A relic with a season nothing else mentions
  still renders — a stub is evidence even when the season around it is
  blank.
- A score that doesn't parse ("abandoned, fog") still renders as a match
  with notes; it just doesn't count toward the record.
- A season card is frozen the day it is made: a match logged later belongs
  to the log and the page, not to paper already printed. Making a new card
  for the same season is a new file, a new event, and the newest card is
  the one the season links.
- An empty log renders the form and the sentence "no matches yet — first
  game of the season goes here", not an error.

## anti-goals

- One club, one point of view. Scores are ours-first; there is no neutral
  mode. Dave is not neutral.
- No league tables, no fixtures, no data feeds. Only what Dave saw and what
  Dave typed. If he missed a match, the season has a hole in it, like his
  memory does.
- No photo effects, no cropping, no restoration. The stub is faded because
  it is forty years old; that is the point of it.

## what good looks like

Full-time whistle, Dave on the concourse, four fields and one photo of his
ticket into his phone before the queue for the bridge moves. One wet Sunday
in November he does the shoebox — thirty relics, each with a line of what
he remembers. In May, `/matchday` reads like the season felt; he runs
`seasoncard`, prints it, and it goes in the drawer. And when someone at
work says "you always say that", he opens last August and proves he did —
stub attached.
