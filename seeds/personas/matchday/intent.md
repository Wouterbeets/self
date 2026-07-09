# matchday — Dave's season

## who this is for

Dave, 51, forklift driver, Sunderland supporter since his dad took him in
1981. He can tell you the score of a match from 1997 but not last month —
and it bothers him. He does not want stats sites, xG, or fantasy points. He
wants the season written down the way he'd tell it at work on Monday.

## surface

- `self run match <competition> <opponent> <homeaway> <score> <notes…>`
  appends one `matchday.match` event: which competition ("league", "cup"),
  who we played, `home` or `away`, the score written ours-first ("2-1"),
  and what he'd actually say about it ("robbed. keeper worth the ticket
  alone").
- `/matchday` renders the season: the running record — won, drawn, lost,
  goals for and against, computed ours-first from every score — big at the
  top, then every match newest first with its notes. Matches from earlier
  years fold into one line per season below.

## constraints

- The record is computed from `matchday.match` events at render time,
  grouped into seasons by `occurred_at` (a season runs August through the
  following July). Nothing is stored twice.
- A score that doesn't parse ("abandoned, fog") still renders as a match
  with notes; it just doesn't count toward the record.
- An empty log renders the form and the sentence "no matches yet — first
  game of the season goes here", not an error.

## anti-goals

- One club, one point of view. Scores are ours-first; there is no neutral
  mode. Dave is not neutral.
- No league tables, no fixtures, no data feeds. Only what Dave saw and what
  Dave typed. If he missed a match, the season has a hole in it, like his
  memory does.

## what good looks like

Full-time whistle, Dave on the concourse, four fields into his phone before
the queue for the bridge moves. In May, `/matchday` reads like the season
felt — and next year, when someone at work says "you always say that", he
opens last August and proves he did.
