# awayend — Fred and Jake's derby

## who this is for

Fred, 54, Newcastle. Jake, 52, Sunderland. Brothers-in-law, three hundred
miles apart since Jake moved south, and thirty years into an argument that
restarts every derby weekend by text. Each keeps his own season the
matchday way — scores ours-first, one point of view, proudly biased — and
neither would be caught dead reading the other's page. What they want is
not a shared database. It is the argument, with receipts: both records on
one page, disagreeing in public.

This seed is grown on **Jake's** instance, after planting Fred's export.
The other direction works the same with the names swapped.

## how the records meet

Fred runs `self export matchday. <dir> fred.` — his whole record, dates
preserved, stub photos included, renamed to `fred.*` on the way out so it
cannot contaminate Jake's own season — edits the intent stub ("this is my
record, it is correct, cope"), and sends the folder however brothers-in-law
send things. Jake runs `self grow` on it, then grows this seed. From then
on, updates travel the same road: a fresh export each month, a fresh plant;
same-hash photos and already-planted moments cost nothing to receive twice
— the projection is a pure replay either way.

## surface

- `self run match <competition> <opponent> <homeaway> <score> <notes…>` —
  Jake's own record, exactly as `matchday` fixes it, with the optional
  stub photo. This seed adds nothing to how he logs; it adds what his page
  can see.
- `/derby` renders the two records head to head: every date where both
  logs hold a match against each other, one row — Jake's score and words
  on the left, Fred's on the right, stub photos side by side. Above them,
  the all-time tally as each record tells it, which will not agree, and
  the page must not care. Below, the matches only one of them has: "Fred
  was there, Jake wasn't" is its own column of quiet gloating.
- `derby/liars` renders the contradictions: every match both men logged
  where the scores, translated to a common orientation, do not match. The
  best page on the site, by design.

## constraints

- Fred's events are `fred.*` and stay that way forever. Translation —
  flipping his ours-first "2-1" to compare with Jake's ours-first "1-2" —
  happens in the projection at render time, never by rewriting a planted
  event. Fred's record on Jake's machine still says what Fred said.
- Matches pair by date (same day, both calendars) — never by trusting
  scores to match, since the whole point is that they sometimes don't.
- The page takes no side. Both tallies render at the same size. Where the
  records disagree, the page shows both and shuts up: it is the table the
  argument happens across, not a referee.
- Fred's photos arrived hash-verified in the seed; the derby page links
  them like any stored file. His `occurred_at`s are his — a 1993 stub
  planted in 2026 renders in 1993, which is the entire value of it.

## anti-goals

- No messaging, no comments, no reactions. The argument lives in the pub
  and the group chat, where it belongs; the page is the evidence table
  both of them are pointing at.
- No merged "true" record. There is no neutral mode — there are two
  records, and the disagreement is data, not noise to reconcile. A page
  that decided who was right would be deleted by both of them within the
  hour.
- No fixtures, no feeds, nothing either man didn't type. Two memories,
  side by side, is the whole product.

## what good looks like

Derby week. Jake opens `/derby/liars` and reads the entry for October
2013: Fred logged 2-1 with "deserved, barely", Jake logged 1-2 with
"robbed blind". Same match, same rain, ten years of arguing — and the
page shows both men said their own team won the day they got home. He
screenshots it into the group chat at 07:40. Fred's reply is unprintable.
At Christmas, Fred hands over a USB stick with three more seasons on it:
"got to the shoebox before you did." The tally above the fold has never
once agreed, and it never will, and that is why both of them keep it.
