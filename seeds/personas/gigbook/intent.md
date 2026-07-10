# gigbook — Theo's band admin

## who this is for

Theo, 27, drummer, and by unlucky election the organized one in a wedding
covers band. Three questions chase him around every group chat: what did
we get paid at that barn wedding, do we actually know that song or do we
just say we do, and how much has the band made this year (asked every
December, answered never). Around the questions, a sediment of files: the
venue's contract PDF somewhere in email, the chart for the one song in G
not A somewhere in five phones, and an invoice he rebuilds in a word
processor from scratch, every single time, badly.

## surface

- `self run gig <venue> <fee> <notes…>` appends one `gigbook.gig` event:
  where they played, what they were paid, and the stuff bands actually
  need to remember ("load-in round the back, sound guy great, don't take
  the A14"). An optional file rides along — the contract, the rider, the
  set-times email printed to PDF — its hash in the event, findable forever
  under the gig it belongs to.
- `self run song <title> <notes…>` appends one `gigbook.song` event: a
  tune entering the repertoire, with the working notes ("in G not A,
  Sarah takes the second verse") and an optional chart file — the actual
  PDF everyone squints at, attached to the song instead of lost in a chat.
- `self run invoice <venue> <fee> <email> <details…>` produces a file — a
  plain, numbered invoice: number derived from how many the log already
  holds, band name, venue, fee, date — deposits it, records it with one
  `gigbook.invoice` event carrying its hash, number, and the venue's email,
  and **sends it**: the mail goes out through the account Theo configured
  (credentials outside the log), and a `gigbook.sent` event records the
  send. The word processor and the attach-forward-hope loop are both out of
  the band.
- `self run paid <number>` appends one `gigbook.paid` event. Ten characters
  typed when the transfer lands; everything else follows from it.
- A monthly timer runs `self run chase`: it reads the log for invoices
  past thirty days with no `gigbook.paid`, re-sends each one with a polite
  one-line reminder — never more than three times, never twice in the same
  month — and appends a `gigbook.chased` event per send. Theo finds out it
  happened from the page, usually after the money arrives.
- `/gigbook` renders the band's year: this year's fee total at the top,
  then gigs newest first with venue, fee, notes, and their attached files;
  invoices listed with their numbers; past years fold to one line each
  with their totals.
- `gigbook/setlist` renders the repertoire alphabetically, each song with
  its latest note and its chart linked — the page that settles "do we
  know it" in the group chat, one link down, chart included.

## constraints

- Yearly totals are computed from `gigbook.gig` events by `occurred_at`.
  A fee that isn't a number ("free, favour for Dan") lists in the year,
  marked, uncounted.
- Invoice numbers are counted from `gigbook.invoice` events at run time —
  the log is the ledger, so numbers never repeat and never skip. An
  invoice is frozen when made: a wrong fee is a new invoice and a note,
  never an edit, because the one already sent exists.
- `chase` earns its autonomy by reading the log: paid invoices are never
  chased, the three-chase ceiling is counted from `gigbook.chased` events,
  and every send — and every skip at the ceiling — is an event. The page
  shows unpaid invoices with their chase count, so Theo always knows what
  the machine has said in the band's name.
- Repeating `song` for the same title updates its story; the newest note
  and newest chart win the setlist page. Titles group case-insensitively.
- Attached files are linked by name from their pages, never inlined; the
  chart opens in the tab, the contract downloads, and either one survives
  every phone in the band being replaced.
- An empty log renders both forms and "first gig goes here."

## anti-goals

- No per-member splits, no expenses, no settling up. The band splits cash
  in the van like civilized people; the page only remembers the top line.
- No setlist ordering, no per-gig setlists. Which songs, not which order —
  the order changes when the dance floor does.
- The invoice is paper-plain: no logo uploads, no themes, no VAT wizardry.
  A band that needs real accounting software should buy some; this one
  needs a number, a name, and a fee on one page.
- Chasing stops at three. After the third reminder it is a phone call and
  a lesson about that venue, not a fourth email; the page marks the
  invoice "gone quiet" and leaves it to the humans.

## what good looks like

December. Someone types "how much did we make this year" into the chat and
Theo answers with a number in under ten seconds, screenshot attached. The
barn books them again; the gig from last time has the contract attached
and the note about the A14, and `invoice` makes number 23 and sends it
before the kettle boils. The barn pays five weeks late — after two polite
chases Theo never wrote, never sent, and only noticed on the page after
typing `paid 23`. At rehearsal someone claims they know "Superstition";
the setlist page has the chart, in the right key, one tap, on the music
stand before the argument develops. Nobody thanks Theo. The page knows.
