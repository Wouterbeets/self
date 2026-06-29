# restore — roll a capability back

## what it's for

A safety net: roll a capability back to an earlier compiled version when a change
went wrong.

## the core intuition

Rolling back splits cleanly into a data-only *trigger* and a privileged *install*.
Anything may ask to roll back — restore emits a data-only restore.requested
{name, seq} — but the install is the kernel's alone: it reinstalls one of its OWN
earlier signed receipts, so no foreign bytes ever enter through this path. That
split is exactly why restore can be an ordinary capability while the privileged
part stays the kernel's.

## the feel

- `restore <name>` rolls back one step; `restore <name> <seq>` goes to that exact
  earlier version. Brain-callable, like any verb.

## the surface

- the verb restore: emits restore.requested; the kernel acts on it.

## anti-goals

- Never install foreign bytes — restore only triggers; the kernel reinstalls its
  own receipts.
