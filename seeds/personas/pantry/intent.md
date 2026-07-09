# pantry — Ray's Tuesday food bank

## who this is for

Ray, 58, runs the Tuesday food bank out of St. Margaret's church hall.
Fourteen volunteers on a rota that lives in his head, donations arriving
in carrier bags, and a diocese that asks once a year how many households
were served — a number Ray currently reconstructs from memory and guilt.

## surface

- `self run donation <what…>` appends one `pantry.donation` event: what
  came in, in Ray's words ("6 trays beans", "bread from the bakery,
  loads").
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

## constraints

- The year's total is computed from `pantry.served` events by
  `occurred_at`; January resets the count, never the history.
- A `served` count that isn't a number still renders as a session note,
  uncounted — a chaotic Tuesday beats a lost one.
- An empty log renders the three forms under "first Tuesday goes here."

## anti-goals

- Never a word about the people served beyond the count and Ray's notes.
  No names of recipients, no lists of who took what. Dignity is the
  operating principle; the log holds the operation, not the queue.
- No rota-planning, no shift reminders. Ray asks people face to face;
  the page just remembers who said yes.

## what good looks like

Tuesday, 1 p.m., hall swept. Ray types three lines standing by the urn:
what came in, who worked, forty-one served. In November the diocese asks
their question and Ray reads the answer off the top of the page in the
vestry, in front of them, first try. And when Marjorie's fifth anniversary
comes, the page can prove what everyone already knew.
