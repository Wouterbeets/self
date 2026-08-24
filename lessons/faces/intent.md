# faces — one log, many formats

## purpose

A view is a pure function from events to **bytes**. Not to HTML — to bytes.
Nothing in the kernel knows or cares what shape they are, which means a single
log can present as many surfaces as you care to write, and each one composes with
whatever already reads that format.

This lesson exists because the constraint used to be real. The previous kernel
served views over HTTP and injected a shared stylesheet, so a view was *required*
to emit bare semantic markup — no CSS, no JavaScript, no assets — and every
reader was a browser. Removing the server removed that ceiling. This is the
lesson that shows what is on the other side of it.

## surface

Grow a small command and then as many faces over it as are useful here. The
command is not the point; the faces are.

- `self run did <minutes> <what…>` — appends `work.done {minutes, what}`.

Then views over `work.done`, each a different format. Build the ones that earn
their place on this instance; these are the ones worth having:

- `self view table` — an aligned plain-text table with a total. What you
  actually want at a prompt, and the reason a text-first kernel is not a
  regression.
- `self view json` — a JSON array, so `self view json | jq …` makes your own
  history queryable without the kernel knowing anything about queries.
- `self view csv` — the universal join. Feeds a spreadsheet, `sqlite3`,
  pandas, R.
- `self view chart` — an SVG bar chart. A picture that is a pure function of
  events: no clock, no network, same log same bytes.
- `self view page` — one self-contained HTML file with its own CSS and its own
  JavaScript. `self view page > /tmp/p.html && open /tmp/p.html`. No server, no
  injected shell, no external requests.
- `self view metrics` — Prometheus text exposition, so a monitoring stack can
  scrape an instance that has no idea what a metric is.
- `self view ics` — iCalendar, so a phone can subscribe to what this log knows.
- `self view replay` — a shell script that recreates this log by calling `did`
  once per event. A view whose output is a program.

## constraints

- Event name exactly `work.done`, fields `minutes` (integer) and `what`
  (string). Every view consumes only `work.done`.
- **Purity is not a style rule here, it is the contract.** A view gets its
  events on stdin and nothing else: no `SELF_HOME`, an empty working directory,
  a scrubbed environment. Do not read the clock, the filesystem or the network.
  If a face wants relative time ("3 hours ago"), compute it in the *reader* —
  the browser has a clock; the view does not.
- Same events in, same bytes out. Two of these are formats other programs
  parse; a view that reorders its output between runs is a broken view.
- A view may print a useful empty-state representation. `self loop` converges on
  authoritative log change, not on a view's output bytes, so presentation does
  not carry hidden control-flow semantics.
- Correct output for the format, not approximately correct: valid JSON, quoted
  CSV, escaped Prometheus labels, CRLF in iCalendar, escaped HTML.

## anti-goals

- Do not build a server, and do not build one face that switches on a flag.
  Eight small scripts that each do one thing beat one script with a `--format`
  argument: the kernel already dispatches by name, and a flag would be a second
  dispatcher.
- Do not put presentation in the command. `did` records what happened; how it
  looks is a view's business, and every view disagrees.
- Do not have a view shell out to another view. Each one replays the log itself;
  that is what makes it a pure function.

## what good looks like

```sh
self run did 95 "rewrite the kernel"
self run did 40 "write the protocol"

self view table                       # read it
self view json | jq -r 'group_by(.what)[] | "\(.[0].what): \(map(.minutes)|add)m"'
self view csv  > work.csv             # open it anywhere
self view page > /tmp/work.html       # a real page, no server
self view chart > /tmp/work.svg       # a real chart, no library
self view replay > rebuild.sh         # a program the log wrote
```

The test that this landed: `self view replay | sh` against a fresh instance that
has learned only `did` produces a log whose `table` is identical to the
original's. The log wrote the program that rebuilt the log.
