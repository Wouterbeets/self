# kitchen — the week's meals and the list that follows

## what it's for

Plan the week's meals (Mon..Sun) and keep the shopping/prep list that follows from
them — in the same home as the board, so life lives in one place.

## the core intuition

A day holds one meal: planning a day again overwrites it, planning it empty clears
it — so the week never accumulates stale entries. The shopping list is a running
set you add to and check off (or remove, if an item was a mistake); checked and
removed items drop out of "to get" but stay in the log. Friendly day spellings
normalize ("tuesday" → tue).

## the feel

- Zero JavaScript: a set/clear form per day, an add form for the list, a "got it"
  and a "remove" per item. Brain-callable ("plan tacos for tuesday", "add olive oil").
- The week reads Mon..Sun; the list shows what's still to get versus what's got.

## the surface

- `/kitchen` — the weekly plan plus the shopping list.
- the verbs: plan, shop, got, unshop.

## anti-goals

- Don't accumulate stale days — re-planning overwrites, empty clears.
- Don't lose the record — bought and removed items remain in the log.
