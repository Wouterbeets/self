# pulse: self watching its own machinery

Drivers and timers write most of a busy log, and nothing reads what they do.
One of them once sent the same alert every five minutes for six days: each
tick saw a "new" situation because the detail text carried a timestamp. The
log held 1,800 copies of the evidence and no view that said so.

`self view pulse` is that view. It is pure, reads no clock, and measures
everything against the newest event.

- **loop**: the same outward effect with the same content repeated inside one
  run. Free text is compared with digits and ids masked, goals and PR links
  exactly.
- **burst**: more of one kind of effect in a clock hour than its limit,
  reported once per busy stretch and only when no loop already explains it.
- **twice**: a loop both bodies took part in, which is two drivers doing one
  job.
- **timer**: a systemd user timer failing or unarmed for over a day, as
  witnessed by `self run pulse tick`.

An outward effect is an event recording something that left the machine or
cost money. The table is `EFFECTS` at the top of `view.py`: event name, label,
hourly limit, repeats that make a loop, how to read its content. Add a row
when a new kind of effect gets an event.

`self run pulse ack <key> <why>` records a verdict on a finding; a new run of
the same loop gets a new key. `drive.sh` is what a timer runs: `pulse tick`,
then each line of `self view pulse actions` goes to the phone once through a
`notify` command and is marked `told`.

Install: `SELF_HOME=~/.self sh examples/pulse/install.sh`, then run `drive.sh`
from a timer every ten minutes.

## Backtest

Replayed against the log that held that storm, cut at successive moments:

| log cut at | what `self view pulse actions` printed |
|---|---|
| 21 minutes in | `Slack DM repeated 4 times` |
| 16 hours in | `Slack DM repeated 177 times on both bodies`, and `team channel post repeated 2 times on both bodies` |

The second line of the second row was not known before: three pull requests
had been announced twice in a team channel, once by each body's driver.

On its first live run it reported a sync timer that had failed every run for
weeks because one peer was unreachable.
