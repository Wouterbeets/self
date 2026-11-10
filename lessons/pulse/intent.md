# pulse — notice when your own automation loops or goes quiet

## purpose

Once a self has timers and drivers, machines write most of its log and nobody
reads what they do. A driver that cannot tell it already spoke will send the
same message every few minutes until a person happens to look. Pulse is the
view that looks: it reads the log for effects that repeat and for scheduled
work that keeps failing.

## surface

- `self view pulse` reports what is wrong now, then the last day of outward
  effects by kind, then loops that have stopped.
- `self view pulse actions` prints one line per new finding, for a driver to
  deliver.
- `self run pulse tick` witnesses the machine's scheduled jobs and appends
  their state only when it changed.
- `self run pulse ack <key> <why…>` records a verdict on a finding so it stops
  being reported.

## constraints

- An outward effect is an event that records something leaving the machine or
  costing money: a message sent, a notification pushed, a worker started, a
  change published. The view holds the list of event names that count; a new
  kind of effect is one more row.
- A loop is the same kind of effect with the same content repeated in one run.
  Free text is compared with numbers and ids masked, because the detail that
  differs each time (a timestamp, a counter) is exactly what hides a loop.
- The view is pure and reads no clock. Everything is measured against the
  newest event, so a machine that is off looks quiet, not broken.
- A finding is reported once. Its key names the run, not the moment, so a
  thousand repeats are one finding and a new run is a new one.
- A scheduled job is wrong only after it has failed for a day. Shorter
  failures are listed, not raised: a peer that sleeps at night is not an
  incident.
- Whatever delivers a finding to a person has its own limit on how often it
  may speak, enforced where it sends and not in its callers. The caller is the
  part that goes wrong.
