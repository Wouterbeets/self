# salon — Fatima's client book

## who this is for

Fatima, 39, mobile hairdresser. Her salon is other people's kitchens; her
business is forty regulars and their colour formulas. The formulas are the
crown jewels — "6.3 base with 20 vol, 35 minutes, she goes brassy if you
rush it" — and today they live in one paper notebook in a bag that has
been left on a bus twice.

## surface

- `self run client <name> <notes…>` appends one `salon.client` event: a
  person and what must never be forgotten about their hair. Repeating the
  command for the same name adds to their story; the newest formula note
  is the current one.
- `self run visit <client> <service> <price> <notes…>` appends one
  `salon.visit` event: who, what was done, what was charged, and anything
  worth knowing next time ("trim only, growing it out for the wedding").
- `/salon` renders the client book: every client with their latest formula
  note, last visit date and service — the page she opens on the doorstep,
  thirty seconds before the kettle goes on.
- `salon/takings` renders money by month: each month's visits and their
  total, newest month first — her tax-time page, one link down.

## constraints

- Client names group case-insensitively; "Mrs Achebe" is one person
  however she's typed.
- A visit for a client never declared still renders in both views — the
  appointment happened whether or not the paperwork did.
- Prices are summed for the monthly total; a price that isn't a number
  ("trade for babysitting") still lists in the month, marked, uncounted.

## anti-goals

- No booking, no calendar, no reminders sent to anyone. Clients text her;
  that already works.
- Nothing client-facing. This is the back of the notebook, not a website.

## what good looks like

Doorstep, Tuesday. Fatima opens `/salon` on her phone: Mrs Achebe, 6.3
base, brassy if rushed, last visit eight weeks ago wanting to go warmer.
The kitchen appointment goes perfectly because March-Fatima briefed her.
In January, `salon/takings` gives her accountant twelve honest lines. The
paper notebook stays home, demoted to shopping lists.
