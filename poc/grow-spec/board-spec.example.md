# board — household kanban

## Intent
A family's shared open loops, captured fast and seen at a glance. The board is
one projection of an append-only memory; moving a card is an event, not a
mutation. The accessible entry point for the whole system.

## Capabilities
- command  capture(title)      -> memory.captured {id, title, stage="inbox"}
- command  move(ref, stage)    -> item.stage_changed {ref, stage}
- command  resolve(ref)        -> item.resolved {ref}
- projector board  consumes memory.captured, item.stage_changed, item.resolved

## Behavior
Lanes in order: inbox, this_week, waiting, done. `resolve` sends an item to done.
`move`/`resolve` match `ref` by id OR case-insensitive title substring. `capture`
assigns a sequential id (item-N counting existing memory.captured). The board
projector renders a capture form at the top, then each lane as a section of
cards; each card carries a move <select> of the four lanes + a resolve button,
all plain forms posting to /run. Bare semantic HTML.

## Examples
- capture "Book dentist"  =>  memory.captured {id: item-1, title: "Book dentist", stage: inbox}
- [memory.captured item-1 inbox] then move "item-1 waiting"  =>  board shows item-1 under "waiting"
- [memory.captured item-2 inbox] then resolve "item-2"  =>  board shows item-2 under "done"

## Content
memory.captured {id: item-1, title: "Book Lila's dentist appointment", stage: this_week}
