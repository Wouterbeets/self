# printfarm — Marco's garage

## who this is for

Marco, 34, dental technician in Lyon. Three Ender 3s on a shelf in the
garage, a box of half-used spools, and a spreadsheet he abandoned in
February because it lived on the wrong computer. What he actually wants to
know at 11 p.m. is: which spool has enough PLA left for a 140 g print, why
did the last three attempts at that articulated dragon fail — and where is
the exact file that finally worked, months later, when his nephew asks for
one too.

## surface

- `self run spool <material> <color> <grams>` appends one `printfarm.spool`
  event: a spool entering service with its starting weight.
- `self run print <sliced-file> <spool> <outcome…>` appends one
  `printfarm.print` event. The sliced file — the G-code Marco already has —
  does the typing: the command reads the comments every slicer writes into
  it (model name, filament grams, estimated time) and fills the event from
  the file, with the file's hash riding along. Marco supplies only which
  spool (named as `<material> <color>`, e.g. "PLA galaxy-black") and whether
  it worked — outcome text starting with `ok` or `fail`, the rest free notes
  ("fail — warped corner, bed too cold").
- `self run logprint <model> <spool> <grams> <outcome…>` appends the same
  event by hand, for prints that never had a file — resin, a friend's
  machine, a guess.
- `self run rescue <spreadsheet>` imports the dead spreadsheet: one upload,
  one `printfarm.print` event per row it can read, and one
  `printfarm.rescued` event naming the file, rows imported, rows skipped.
  February gets its history back.
- `/printfarm` renders the shelf: each spool with grams remaining (starting
  weight minus the grams of every print charged to it), then recent prints
  newest first, failures visibly marked, each print linking its sliced file
  — the exact bytes that produced that outcome, downloadable forever.
- `printfarm/failures` renders only the failed prints with their notes and
  their files — Marco's debugging memory, one link down from the shelf.

## constraints

- Remaining filament is never stored and never edited: it is computed from
  the log at render time. A miscounted print is corrected by another
  `logprint` event (grams may be negative for a correction), never by
  rewriting history.
- Reading G-code means reading comment lines (`;Filament used:`,
  `; filament used [g] =`, and their kin) with no slicer SDK and no external
  tools. A file the command cannot parse still deposits and still renders —
  grams fall back to 0 with a visible "unweighed" mark, because a lost print
  is worse than an unweighed one.
- The same file printed five times is five events, one blob: content
  addressing makes reprints free, and the shelf counts every attempt.
- A print naming a spool that was never declared still renders — grouped
  under "unknown spool" — because a lost event is worse than an untidy page.
- `rescue` never guesses: a row it cannot parse is skipped and counted, and
  the original spreadsheet stays linked from its `printfarm.rescued` event,
  so nothing is lost even when the import is imperfect.
- An empty log renders an empty shelf with the forms, not an error.

## anti-goals

- No slicer integration, no printer APIs, no OctoPrint. The G-code file is
  already on Marco's phone when the print starts; reading the comments it
  carries is not integration, it is literacy.
- No judgment on the failure rate. The failures page is a lab notebook,
  not a report card.
- No STL previews, no 3D rendering, no thumbnails. A filename and a
  download link; the file speaks slicer, the page speaks Marco.

## what good looks like

Marco slices the dragon, uploads the G-code with `fail — warped corner,
dusty bed`, and never types a number: the file said 142 g, and the shelf
already knew which spool could afford it. Two weeks later the same failure
starts again; `/printfarm/failures` holds February-Marco's note and the
exact file that failed, and this time the fix works — logged `ok`, same
file, same hash. In November his nephew wants a dragon, and the last `ok`
print links the bytes that made it. The dead spreadsheet? Rescued in March
with one upload, 47 rows. The history survived the wrong computer.
