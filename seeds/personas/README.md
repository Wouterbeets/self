# personas — what mass adoption feels like

Eleven people who will never open a terminal, never read `events.jsonl`, and
never care that a projection is a pure function of the log. Each directory
is a seed written from one of their lives: what they would actually want,
named in their words, honoring the kernel's contract underneath.

They matter because the runtime's builders all look the same — coder, CTO,
unix. These seeds are the corrective lens. If `self` only makes sense to
someone who already thinks in append-only logs, it is a tool. If Bernadette
can see her allotment in it, it is a product.

None of these people type `self run` anything. They reach every command
through the HTML form the kernel serves at `/run/<command>`, or through
chat. The CLI spellings in each intent fix the public surface for the brain
that grows it; the person only ever sees a page with a form on it.

The natural shape of a log is a diary, and the first draft of every one of
these seeds was one: type a line, render a list. Files are what break that
ceiling, four ways (see "What files unlock" in [`SEEDS.md`](../../SEEDS.md)):
a photo **attaches** evidence to an event; a command **extracts** what a
file already knows so the person stops typing it; one upload **imports** a
history that predates the instance; and a command **produces** a document —
an invoice, a tax pack, an annual return, a zine — so the instance stops
being only where records go and becomes where documents come from. Every
persona below now exercises at least one of these; several exercise all
four.

| seed | who | what they keep | what files do for them |
|---|---|---|---|
| `printfarm` | Marco, 34, dental technician, three printers in the garage | spools, print jobs, failures | the G-code weighs its own print; the dead spreadsheet imports; every failure links its exact file |
| `matchday` | Dave, 51, forklift driver, Sunderland since 1981 | every match, the season so far | stub photos on matches; the shoebox scanned into seasons; a printable season card for the drawer |
| `tables` | Priya, 42, pharma sales, eats out four nights a week | meals, ratings, where to take people | receipts ride each meal; one command emits the month's expense CSV |
| `zine` | Lena, 15, posts from her phone after homework | posts, moods, retractions | images in posts; `issue` gathers chosen posts into a print-ready file she photocopies |
| `allotment` | Bernadette, 68, retired teacher, plot 14b | sowings, harvests, the year's tally | seed-packet photos on sowings; the surviving exercise books photographed into the almanac |
| `firstyear` | Sam & Alex, new parents, baby three weeks old | feeds, naps, milestones | photos on milestones only; `yearbook` freezes the first year into one printable keepsake |
| `salon` | Fatima, 39, mobile hairdresser | client colour formulas, visits, takings | result photos in the client book; client screenshots pinned to people; a one-file tax pack |
| `pantry` | Ray, 58, runs the Tuesday food bank | donations, shifts, households served | donation-table photos for thank-yous; `return` produces the diocese's annual document |
| `gigbook` | Theo, 27, drummer in a wedding covers band | gigs, fees, the repertoire | contracts on gigs, charts on songs; `invoice` numbers itself from the log |
| `flare` | June, 45, lives with rheumatoid arthritis | symptoms, meds, a summary for the doctor | dated photos as evidence; `handover` freezes what each appointment was given |
| `contactsheet` | Ana, 29, film on weekends, weddings for friends | the photos she keeps, tagged her way | the photos ARE files; `printorder` zips the chosen frames, lab-ready |

Try one:

```sh
export SELF_BRAIN="claude -p"
self grow seeds/personas/tables
self
```

Then imagine handing the resulting page — not this repo, just the page —
to the person it was written for.
