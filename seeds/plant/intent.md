# plant — grow capabilities from the browser

## what it's for

A page where you grow self without the CLI: paste a spec — a seed's declarations
as JSONL or JSON — or click a ready-made starter, and the strange loop compiles it
into a running capability wired into the same garden. The browser becomes a place
to grow, not just to use.

## the core intuition

A spec is a fragment of genotype you hand the kernel; planting emits its events so
the kernel's strange loop compiles any command.declared / projector.declared on the
spot. A UI planting is recorded exactly like a CLI grow (a seed.planted receipt),
so the browser and the command line share one history. A bad spec is shown back
with its error — never a silent failure. Compiling needs a brain wired on /setup;
planting itself only emits.

## the feel

- Zero JavaScript: a spec box prefilled with a working example, one-click starters
  carrying ready-made specs, the capabilities already in the garden, and a log of
  recent plantings (ok / failed).

## the surface

- `/plant` — the grow page.
- the verb plant: a spec in, the spec's events out, plus a seed.planted receipt.

## anti-goals

- Never fail silently — a bad spec comes back visible, with the reason.
- Don't invent a separate history — UI plantings are seed.planted, like CLI grows.
