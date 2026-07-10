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

| seed | who | what they keep |
|---|---|---|
| `printfarm` | Marco, 34, dental technician, three printers in the garage | spools, print jobs, failures |
| `matchday` | Dave, 51, forklift driver, Sunderland since 1981 | every match, the season so far |
| `tables` | Priya, 42, pharma sales, eats out four nights a week | meals, ratings, where to take people |
| `zine` | Lena, 15, posts from her phone after homework | posts, moods, retractions |
| `allotment` | Bernadette, 68, retired teacher, plot 14b | sowings, harvests, the year's tally |
| `firstyear` | Sam & Alex, new parents, baby three weeks old | feeds, naps, milestones |
| `salon` | Fatima, 39, mobile hairdresser | client colour formulas, visits, takings |
| `pantry` | Ray, 58, runs the Tuesday food bank | donations, shifts, households served |
| `gigbook` | Theo, 27, drummer in a wedding covers band | gigs, fees, the repertoire |
| `flare` | June, 45, lives with rheumatoid arthritis | symptoms, meds, a summary for the doctor |
| `contactsheet` | Ana, 29, film on weekends, weddings for friends | the photos she keeps, tagged her way |

Try one:

```sh
export SELF_BRAIN="claude -p"
self grow seeds/personas/tables
self
```

Then imagine handing the resulting page — not this repo, just the page —
to the person it was written for.
