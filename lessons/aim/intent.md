# aim — a line of prose the next pass should do

## purpose

The kernel's unfinished business is a declaration with no script. That is the
only thing `while ask=$(self)` waits on, and the only thing it should wait on.
A line of prose — "analyse the metrics view and note insights" — is not a
capability. It is domain. If the kernel stacked it, the kernel would be
pretending to know what matters here.

So the stack is grown. A command records an aim, a view renders the open ones
and **goes silent when there are none**, and a loop the kernel does not know
about drives the work:

```sh
while [ -n "$(self view aim)" ]; do
  self "do the next open aim" | claude -p | self hear
done
```

That is the same shape as any other view-driven loop. Nothing in `self help`
mentions an aim, and nothing should.

## surface

- `self run aim <text…>` — append one `aim.opened {text}`.
- `self run settled <seq|text…>` — append one `aim.closed`. A number closes that
  seq; any other argument closes the oldest exact text; no argument closes the
  oldest open aim.
- `self view aim` — the open aims, oldest first, one per line, each `seq` and
  text. **Print nothing when the stack is empty.** Emptiness composes with the
  shell; a friendly "nothing aimed at" does not, and the loop above never
  terminates.

A mind with the log in front of it can also print `aim.opened` / `aim.closed`
directly. The commands are the human door.

## constraints

- Event names exactly `aim.opened` (field `text`) and `aim.closed` (field `seq`
  and/or `text`). Closing by seq wins; otherwise the oldest exact text; an empty
  closer is not a closer.
- Exactly two commands (`aim`, `settled`) and one view (`aim`). The view
  consumes `aim.opened` and `aim.closed` and no other names.
- The view is a pure function of those events. Do not read the clock, the
  filesystem, or the network. Same events in, same bytes out.
- Plain text. There is no browser here; the reader is a terminal or an agent.

## anti-goals

- Do not make the kernel aware of `aim.opened`. Nothing in `self help` mentions
  it. The previous temptation was to hold `while ask=$(self)` open with a kernel
  event; that loop grows the instance and converges when the instance has
  finished building itself. This loop is a view.
- Do not print a placeholder when the stack is empty. Silence is the
  convergence signal.
- Do not reuse `work.done`. That name is a domain event in another lesson
  (`faces`), and a view that consumed both would mix a time log with a stack.
- Do not put the current aim on the wake-up card from inside the kernel. If a
  mind should see it, it reads `self view aim`, the same as any other face.

## what good looks like

```sh
self run aim "analyse the metrics view and note insights"
self view aim
# 1  analyse the metrics view and note insights

while [ -n "$(self view aim)" ]; do
  self "do the next open aim" | claude -p | self hear
done

self view aim          # empty — the loop has ended
```

The test that this landed: after `settled`, `self view aim` prints nothing, and
the growth loop (`while ask=$(self)`) is quiet the whole time, because an aim
is not pending work.
