# printfarm — Marco's garage

## who this is for

Marco, 34, dental technician in Lyon. Three Ender 3s on a shelf in the
garage, a box of half-used spools, and a spreadsheet he abandoned in
February because it lived on the wrong computer. What he actually wants to
know at 11 p.m. is: which spool has enough PLA left for a 140 g print, and
why did the last three attempts at that articulated dragon fail.

## surface

- `self run spool <material> <color> <grams>` appends one `printfarm.spool`
  event: a spool entering service with its starting weight.
- `self run print <model> <spool> <grams> <outcome…>` appends one
  `printfarm.print` event: what was printed, on which spool (named as
  `<material> <color>`, e.g. "PLA galaxy-black"), how many grams it
  consumed, and whether it worked — outcome text starting with `ok` or
  `fail`, the rest free notes ("fail — warped corner, bed too cold").
- `/printfarm` renders the shelf: each spool with grams remaining
  (starting weight minus the grams of every print charged to it), then
  recent prints newest first, failures visibly marked.
- `printfarm/failures` renders only the failed prints with their notes —
  Marco's debugging memory, one link down from the shelf.

## constraints

- Remaining filament is never stored and never edited: it is computed from
  the log at render time. A miscounted print is corrected by another
  `print` event (grams may be negative for a correction), never by
  rewriting history.
- A print naming a spool that was never declared still renders — grouped
  under "unknown spool" — because a lost event is worse than an untidy page.
- An empty log renders an empty shelf with the two forms, not an error.

## anti-goals

- No slicer integration, no printer APIs, no OctoPrint. Marco types five
  words into a form when a print comes off the bed; that is the whole deal.
- No judgment on the failure rate. The failures page is a lab notebook,
  not a report card.

## what good looks like

Marco weighs a new spool, adds it from his phone at the workbench. A dragon
fails; he logs `fail — first layer, dusty bed` in ten seconds. Two weeks
later the same failure starts again, he opens `/printfarm/failures`, and
February-Marco has already written the answer to November-Marco's problem.
