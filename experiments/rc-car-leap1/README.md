# rc-car-leap1 — an experiment through the pipe

A father-and-son ask — *"an RC car that drives very fast, jumps, and does
an Optimus Prime transformation"* — run through the loop this repo is
about:

```sh
echo "<the ask>" | self | <mind> | self
```

A fresh instance was spawned (gitignored, at the repo root's `.self/`), the
ask went through the ask face, and a Claude session stood in the pipe as
the mind. Its answer appended ten `design.decided` events, declared and
authored a `decide` command and a `design` projection (installed under
signed receipts), and replied. The instance's live state stays out of git
by design — what's committed here is what the protocol says travels:

- **`account/`** — `self give design. account` — the design record as a
  plain-text account: intent, ten events verbatim, manifest. To pick the
  project up in your own instance:

  ```sh
  self learn experiments/rc-car-leap1/account | claude -p | self
  ```

- **`DESIGN.md`** — the same ten decisions rendered for humans.
- **`leap1.html`** — the deliverable: a self-contained three.js animation
  (three.js r160 inlined, no network needed) of LEAP-1 sprinting, hitting
  the ramp, and transforming. Open it in a browser; the buttons seek the
  acts, the scrubber drives the timeline — and **You drive!** switches to
  interactive mode: steer, gas, jump, fire, transform, on touch pads or
  keyboard, with coins, a lap timer, and dinosaurs to zap.

To rebuild the instance itself, replay the account into a fresh home and
let a mind re-author the capabilities it wants — declarations travel,
code does not.
