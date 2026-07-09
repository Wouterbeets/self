# gigbook — Theo's band admin

## who this is for

Theo, 27, drummer, and by unlucky election the organized one in a wedding
covers band. Three questions chase him around every group chat: what did
we get paid at that barn wedding, do we actually know that song or do we
just say we do, and how much has the band made this year (asked every
December, answered never).

## surface

- `self run gig <venue> <fee> <notes…>` appends one `gigbook.gig` event:
  where they played, what they were paid, and the stuff bands actually
  need to remember ("load-in round the back, sound guy great, don't take
  the A14").
- `self run song <title> <notes…>` appends one `gigbook.song` event: a
  tune entering the repertoire, with the working notes ("in G not A,
  Sarah takes the second verse").
- `/gigbook` renders the band's year: this year's fee total at the top,
  then gigs newest first with venue, fee, and notes; past years fold to
  one line each with their totals.
- `gigbook/setlist` renders the repertoire alphabetically, each song with
  its latest note — the page that settles "do we know it" in the group
  chat, one link down.

## constraints

- Yearly totals are computed from `gigbook.gig` events by `occurred_at`.
  A fee that isn't a number ("free, favour for Dan") lists in the year,
  marked, uncounted.
- Repeating `song` for the same title updates its story; the newest note
  wins the setlist page. Titles group case-insensitively.
- An empty log renders both forms and "first gig goes here."

## anti-goals

- No per-member splits, no expenses, no settling up. The band splits cash
  in the van like civilized people; the page only remembers the top line.
- No setlist ordering, no per-gig setlists. Which songs, not which order —
  the order changes when the dance floor does.

## what good looks like

December. Someone types "how much did we make this year" into the chat and
Theo answers with a number in under ten seconds, screenshot attached. Next
August, booked back at the barn, the gig note from last time saves them
the A14 and forty minutes. Nobody thanks Theo. The page knows.
