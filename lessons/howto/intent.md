# howto — procedures that worked, kept with their proof

## purpose

The second time a mind meets a problem it should not rediscover the first
solution. A howto is a procedure that was run and worked, saved with when to
use it, its limits, and the evidence that it did work. It is not a note and
not a plan: it earned its place by being verified.

## surface

- `self run howto set <id> <title> <markdown…>` appends `howto.saved`. Saving
  the same id again revises it.
- `self run howto retire <id> <reason…>` appends `howto.retired`.
- `self view howto` lists live howtos: id, title, when saved.
- `self view howto <id>` prints one in full.
- `self view howto --search <words…>` lists the ones matching every word.

## constraints

- A howto states its trigger first: the situation in which a mind should reach
  for it.
- It carries evidence: the command that was run, the output that was seen, the
  date. A procedure nobody ran is a guess and does not belong here.
- Revision is a later event under the same id; the latest wins and history
  stays in the log. Retirement hides a howto without erasing it.
- The bare view is an index a mind can scan in one read. Bodies are fetched by
  id, so the list stays short however many there are.
- No secrets. A howto names where a credential lives, never the credential.
