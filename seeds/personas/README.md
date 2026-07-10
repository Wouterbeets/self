# personas — what mass adoption feels like

Thirteen people who will never open a terminal, never read `events.jsonl`,
and never care that a projection is a pure function of the log. Each
directory is a seed written from one of their lives: what they would
actually want, named in their words, honoring the kernel's contract
underneath.

They matter because the runtime's builders all look the same — coder, CTO,
unix. These seeds are the corrective lens. If `self` only makes sense to
someone who already thinks in append-only logs, it is a tool. If Bernadette
can see her allotment in it, it is a product.

None of these people type `self run` anything. They reach every command
through the HTML form the kernel serves at `/run/<command>`, or through
chat. The CLI spellings in each intent fix the public surface for the mind
that grows it; the person only ever sees a page with a form on it.

The natural shape of a log is a diary, and the first draft of every one of
these seeds was one: type a line, render a list. Three unlocks break that
ceiling, and the personas now exercise all of them:

- **Files** (see "What files unlock" in [`SEEDS.md`](../../SEEDS.md)): a
  photo attaches evidence, a command extracts what a file already knows,
  one upload imports a pre-instance history, and a command produces the
  document — invoice, tax pack, annual return, zine.
- **Effects and timers** ("Acting on the world"): commands press real
  buttons — the printer in Marco's garage, the mail that carries Theo's
  invoice, the Monday list on Fatima's phone — on a schedule if a timer
  binds them, with every act and every restraint left in the log as an
  event.
- **Content sharing** (`export`/`grow`): a slice of one person's record,
  curated by hand, planted in another person's instance, where the
  receiver's own mind writes the merge. Fred and Jake's derby; Amara and
  Ben's confluence.

A word on what got *removed* to make room. Early drafts of these intents
carried anti-goals like "no printer APIs" and "no reminders sent to
anyone" — timidity from when the system could only remember. Those are
gone. What stayed are the anti-goals that were never about capability:
nobody photographs the food-bank queue, no page grades a sleep-deprived
parent, June's record renders no opinion about her body, the derby page
crowns no winner. The line to hold when writing new personas: loosen what
the machine may *do*, never what it may *judge* — and give every new power
its receipt event and its restraint.

| seed | who | what they keep | beyond the diary |
|---|---|---|---|
| `printfarm` | Marco, 34, dental technician, three printers in the garage | spools, print jobs, failures | the G-code weighs its own print; the dead spreadsheet imports; `start` sends a job to the printer from the sofa |
| `matchday` | Dave, 51, forklift driver, Sunderland since 1981 | every match, the season so far | stub photos; the shoebox scanned into seasons; a printable season card for the drawer |
| `tables` | Priya, 42, pharma sales, eats out four nights a week | meals, ratings, where to take people | receipts ride each meal; the month's expense CSV mails itself when the month closes |
| `zine` | Lena, 15, posts from her phone after homework | posts, moods, retractions | images in posts; `issue` makes a photocopiable zine; `print` wakes the printer downstairs |
| `allotment` | Bernadette, 68, retired teacher, plot 14b | sowings, harvests, the year's tally | packet photos; the exercise books photographed in; Sunday's email is her own diary, this week, other years |
| `firstyear` | Sam & Alex, new parents, baby three weeks old | feeds, naps, milestones | photos on milestones only; `yearbook` freezes the first year into one printable keepsake |
| `salon` | Fatima, 39, mobile hairdresser | client colour formulas, visits, takings | result photos in the client book; the Monday due-list comes to her; a nudge goes out only when she taps a name |
| `pantry` | Ray, 58, runs the Tuesday food bank | donations, shifts, households served | `return` produces the diocese's document; Tuesday's call-out messages the volunteer group, logistics only |
| `gigbook` | Theo, 27, drummer in a wedding covers band | gigs, fees, the repertoire | contracts on gigs, charts on songs; invoices number themselves, send themselves, and chase themselves — three times, then stop |
| `flare` | June, 45, lives with rheumatoid arthritis | symptoms, meds, a summary for the doctor | dated photos as evidence; `handover` freezes what each appointment was given |
| `contactsheet` | Ana, 29, film on weekends, weddings for friends | the photos she keeps, tagged her way | the photos ARE files; `printorder` zips the chosen frames, lab-ready |
| `awayend` | Fred, 54 (Newcastle) & Jake, 52 (Sunderland), thirty years of derby arguments | two rival match records | Fred exports his record as `fred.*`; Jake plants it; `/derby` puts both memories on one page and takes no side |
| `fieldnotes` | Amara, 41, marine ecologist & Ben, 47, hydrologist | shore observations; water readings | Ben curates his export line by line; Amara's mind writes the merge; `/confluence` shows the six-week lag neither record contained |

Try one:

```sh
export SELF_MIND="claude -p"
self grow seeds/personas/tables
self
```

Then imagine handing the resulting page — not this repo, just the page —
to the person it was written for.
