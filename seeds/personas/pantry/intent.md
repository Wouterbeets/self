# pantry — Ray's Tuesday food bank

## who this is for

Ray, 58, runs the Tuesday food bank out of St. Margaret's church hall.
Fourteen volunteers on a rota that lives in his head, donations arriving
in carrier bags, and a diocese that asks once a year how many households
were served — a number Ray currently reconstructs from memory and guilt,
onto a form he fills in by hand, in the vestry, under time pressure.

## surface

- `self run donation <what…>` appends one `pantry.donation` event: what
  came in, in Ray's words ("6 trays beans", "bread from the bakery,
  loads"). An optional photo rides along — the bakery's whole table,
  photographed once, worth more in the thank-you letter than any count.
- `self run shift <volunteer> <role…>` appends one `pantry.shift` event:
  who worked and what they did ("Marjorie, front table").
- `self run served <households> <notes…>` appends one `pantry.served`
  event: how many households this session, plus anything worth
  remembering ("41, new family from the flats, needs halal").
- `/pantry` renders the operation: the running total of households served
  this year, big at the top (the diocese number, always ready); the last
  few sessions with their counts and notes; recent donations; and who has
  worked lately.
- `pantry/volunteers` renders the whole roster: every volunteer with
  their shift count and last shift — the quiet record that Marjorie has
  done every Tuesday since it opened, one link down.
- `self run return <year>` produces a file: the annual return as one
  printable document — sessions held, households served in total and by
  month, donations received, volunteers and their shifts — deposited into
  the store and recorded by one `pantry.return` event carrying its hash.
  The diocese's question, answered as a document Ray hands over.
- A weekly timer runs `self run callout` on Tuesday mornings: one message
  to the volunteers' group chat through the gateway Ray configured —
  "we're on today, 12 till 2; last week was 41 households, thank you" —
  and one `pantry.callout` event recording it. Logistics and gratitude,
  never data about anyone served.

## constraints

- The year's total is computed from `pantry.served` events by
  `occurred_at`; January resets the count, never the history.
- A `served` count that isn't a number still renders as a session note,
  uncounted — a chaotic Tuesday beats a lost one — and the annual return
  lists such sessions the same way, marked, so the document never claims
  more precision than the log holds.
- The return is frozen when made and belongs to its year; running it again
  after a correction makes a new file, and the page links the newest.
- An empty log renders the three forms under "first Tuesday goes here."

## anti-goals

- Never a word about the people served beyond the count and Ray's notes.
  No names of recipients, no lists of who took what — and no photographs
  of anyone, ever: a camera pointed at donations on a table is gratitude,
  a camera pointed anywhere else in that hall is surveillance. Dignity is
  the operating principle; the log holds the operation, not the queue.
- No rota-planning, no individual reminders, no chasing anyone. The
  Tuesday call-out goes to the group Ray already runs, says only what the
  noticeboard would say, and never names who is expected. Ray asks people
  face to face; the page just remembers who said yes.
- The return states what the log knows; it never estimates, projects, or
  rounds up. If the diocese wants a bigger number, the diocese can come
  on a Tuesday.

## what good looks like

Tuesday, 1 p.m., hall swept. Ray types three lines standing by the urn:
what came in (the bakery's table, photographed), who worked, forty-one
served. In November the diocese asks their question; Ray runs `return`,
prints one page, and hands it over in the vestry, first try, every month
itemized. The bakery gets a thank-you letter with their own photo in it.
And when Marjorie's fifth anniversary comes, the page can prove what
everyone already knew.
