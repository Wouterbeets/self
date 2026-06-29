# artifact — pages self made for you, kept

## what it's for

self's equivalent of a saved artifact: when you ask for a *page* — a generated,
one-off HTML answer — rather than a live view, the brain produces it once and it is
saved, viewable and linkable from then on.

## the core intuition

Generation is a one-time act of intelligence (the brain, non-deterministic);
rendering is a pure replay of the saved bytes (deterministic). So an artifact is
*created once* — its title and HTML captured as an event — and rendered from the
log forever after, newest first, each at a stable anchor. The discriminator that
keeps this honest: a live, always-current view is a projector; a one-off
synthesized page is an artifact.

## the feel

- The brain saves one by calling the verb with `Title || <html>`.
- `/artifacts` lists them newest-first, each linkable at its own anchor, the saved
  HTML embedded verbatim.

## the surface

- `/artifacts` — the saved pages.
- the verb artifact: title + html → an artifact.created event.

## anti-goals

- Never regenerate on view — render the saved bytes (a pure replay).
- Don't use an artifact for something that should stay current — that's a projector.
