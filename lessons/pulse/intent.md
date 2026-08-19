# pulse — the metronome's scoreboard

## purpose

A cron beat that only reflects is a heartbeat, not an improvement loop.
Pulse is how this instance *knows* whether beats improve it: the clock
stays outside (the metronome script), each tick lands as a `beat.closed`
receipt, and `/pulse` is a deterministic replay of those receipts plus
the aims the mind is supposed to be chasing.

The mind still does the improving. The metronome keeps score. The page
is the pressure.

## surface

- `self run aim <text…>` opens one improvement to chase. Emits
  `aim.opened` `{text}` — id is the event's seq.
- `self run settle <id> [note…]` closes that aim. Emits `aim.settled`
  `{aim, note}`. Settling an unknown or already-settled id appends
  nothing and prints a warning to stderr.
- `/pulse` — the scoreboard: open aims (oldest first, each with a
  settle form), north-star metrics, a strip of recent verdicts, and
  the beat log. An aim box at the top (`/run/aim`).

Events the metronome (not the mind) is expected to emit:

- `beat.closed` `{verdict, focus, note, seq_before, seq_after, names, started, ended}`

Verdicts: `ship` (a capability was declared or compiled), `keep`
(memory or an aim moved), `chat` (an assistant reply landed), `idle`
(the mind said nothing was worth changing and emitted `self.replied`),
`dead` (the pass ran and left no reply and no durable event), `skip`
(the previous pass still held the lock).

## constraints / mechanics

- Exactly two commands (`aim`, `settle`), one projection (`pulse`).
- `/pulse` consumes `beat.closed`, `aim.opened`, `aim.settled`,
  `script.compiled`, `command.declared`, `projector.declared`,
  `capability.retired`, `self.reflected`, `self.asked`, `self.replied`,
  `chat.message`, `memory.noted`. It does not read the clock.
- The metronome is the clock: it snapshots seq, runs one mind pass,
  classifies what landed, and appends `beat.closed`. Do not trust the
  mind to keep score — that is how dead beats hid themselves.
- Classification (so a future scorer and this page agree):
  - `script.compiled` or a new declaration → `ship`
  - assistant `chat.message` → `chat`
  - `memory.noted` / `aim.opened` / `aim.settled` → `keep`
  - `self.replied` and nothing durable → `idle`
  - anything else (including a silent pass) → `dead`
  - lock not acquired → `skip`
- Open aims outrank the rotating inspection focus. The foci themselves
  are the metronome's, not this page's: render, memory, lessons, kernel,
  home, protocol.
- One aim per event, self-contained. Corrections are later aims.

## anti-goals

- No model in the scorer. Classification is a fold over event names.
- Not a second memory and not a second board. Aims are the next
  improvement, not life tasks.
- No nagging, no timers inside the kernel. The visible queue and the
  dead-rate number are the whole pressure.
- Do not score by grepping `selfmind.log`. The log is the authority.

## what good looks like

Cron fires. The work face still answers pending compiles and unanswered
chat first. On an idle tick the metronome pins a focus (oldest open aim,
else the next rotating inspection), the mind ships one small thing or
honestly idles with `self.replied`, and `beat.closed` lands. `/pulse`
shows the verdict strip moving; dead rate falls toward zero; open aims
drain; a cold mind reading the page knows whether the home is improving.

## metrics (what the page must show)

- **Dead rate** — `dead / scored`. Should be 0. A dead beat is a
  failure mode, not a rest.
- **Pending chat / pending compiles** — should be none. Age is the
  seq of the waiting item.
- **Open aims** — the queue. Pressure is the list, not a reminder.
- **Ship+keep per last 8 scored beats** — is the loop compounding?
- **Skip rate** — beats that collided with a still-running pass.
- **Live capabilities** — commands + projectors currently declared.
- **Last verdicts** — a short strip, newest last.
