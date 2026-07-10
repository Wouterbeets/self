# salon — Fatima's client book

## who this is for

Fatima, 39, mobile hairdresser. Her salon is other people's kitchens; her
business is forty regulars and their colour formulas. The formulas are the
crown jewels — "6.3 base with 20 vol, 35 minutes, she goes brassy if you
rush it" — and today they live in one paper notebook in a bag that has
been left on a bus twice. The other thing in the bag is her phone, full of
before-and-after photos she can never find when she needs them, and
screenshots clients send her captioned "this, exactly this".

## surface

- `self run client <name> <notes…>` appends one `salon.client` event: a
  person and what must never be forgotten about their hair. Repeating the
  command for the same name adds to their story; the newest formula note
  is the current one. An optional photo rides along — usually the
  screenshot the client sent, pinned to the person who sent it.
- `self run visit <client> <service> <price> <notes…>` appends one
  `salon.visit` event: who, what was done, what was charged, anything
  worth knowing next time ("trim only, growing it out for the wedding") —
  and an optional photo of the result, because the formula says what she
  did and the photo proves what it looked like.
- `/salon` renders the client book: every client with their latest formula
  note, their latest result photo, last visit date and service — the page
  she opens on the doorstep, thirty seconds before the kettle goes on.
- `salon/takings` renders money by month: each month's visits and their
  total, newest month first — her tax-time page, one link down.
- `self run taxpack <year>` produces a file: a CSV of the year — date,
  client, service, price, one row per visit, months subtotaled — deposited
  into the store and recorded by one `salon.taxpack` event. Her accountant
  gets one attachment, not a shoebox.

## constraints

- Client names group case-insensitively; "Mrs Achebe" is one person
  however she's typed.
- A visit for a client never declared still renders in both views — the
  appointment happened whether or not the paperwork did.
- Prices are summed for the monthly total; a price that isn't a number
  ("trade for babysitting") still lists in the month, marked, uncounted —
  and lands in the taxpack the same way, marked, for the accountant to
  judge.
- The client book shows each client's newest photo only; the older ones
  stay reachable from the client's visit history. The doorstep glance
  needs one picture, not a gallery.
- The taxpack is a rendering of visits already logged: same year, same
  visits, same rows. It computes nothing the takings page doesn't already
  show.

## anti-goals

- No booking, no calendar, no reminders sent to anyone. Clients text her;
  that already works.
- Nothing client-facing. This is the back of the notebook, not a website.
  Photos are working notes between Fatima and her own memory — never a
  portfolio, never posted.
- No tax advice, no deductions, no categories. The CSV states what
  happened; what it means is the accountant's trade, not the page's.

## what good looks like

Doorstep, Tuesday. Fatima opens `/salon` on her phone: Mrs Achebe, 6.3
base, brassy if rushed, last visit eight weeks ago wanting to go warmer —
and the photo from that visit, so both of them are looking at the same
"warmer". The kitchen appointment goes perfectly because March-Fatima
briefed her. The client's daughter sends a screenshot for her own
appointment; it goes on her record in ten seconds. In January, Fatima runs
`taxpack`, emails one file, and her accountant replies "this is the
easiest you've ever been". The paper notebook stays home, demoted to
shopping lists.
