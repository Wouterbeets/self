# LEAP-1 — design spec

A radio-controlled car for a 6-year-old co-designer that does three things:
drives very fast, jumps, and transforms into a robot, Optimus Prime style.

This file is a human-readable render of the ten `design.decided` events in
[`account/record.jsonl`](account/record.jsonl) — the events are the
authoritative record. The 3D animation of the whole show is
[`leap1.html`](leap1.html) (self-contained, open in any browser).

## Concept

**LEAP-1: a 1/16-scale 4WD cab-over truck (Optimus silhouette) that is a
race car, a ramp jumper, and a standing robot in one toy.**
One machine, three tricks. 1/16 scale (~35 cm, under 900 g) is the sweet
spot: light enough to survive jump landings, big enough to hold a transform
mechanism, safe for a 6-year-old's hands.

## Speed

Brushed 390 motor, 4WD shaft drive, 2S LiFe battery, ~20 km/h top speed —
with a parent-set 50% training mode in the ESC. Feels genuinely fast at kid
height but stays steerable; 4WD keeps it tracking straight on jump run-ups
and landings. Training mode means the same car grows with the driver.

## Jumping

Jumps come from ramps, not actuators: 35 mm travel oil-damped coilovers on
double wishbones, 40 mm ground clearance, belly skid plate, foam-filled
rubber tires, slight nose-heavy balance (55/45). Physics-honest: an onboard
jump actuator would add weight and break. The suspension's whole job is the
landing; nose-heavy balance makes it land flat instead of looping onto its
tail.

## Transformation

Three servo-driven stages — a simplified Robosen:

1. **Legs** — the rear chassis folds down at a mid hinge; rear wheels become
   heels.
2. **Arms** — the front wheel pods slide out and up to the shoulders.
3. **Head** — pops up from behind the cab.

Five metal-gear micro servos with mechanical hard stops. Each stage is one
rotation of one hinge, so a kid can watch and understand the mechanism.
Hard stops mean the plastic, not the servo gears, defines the end pose.

## The jump-vs-transform conflict

Jumping and transforming fight each other: landings hammer exactly the
joints the transform needs free. Resolution — in car mode every folding
joint locks into an over-center latch (landing forces load solid plastic
and steel pins, never servo teeth), and firmware only allows transforming
when the IMU says the car is still and level, so a mid-air button-mash
can't strip gears.

## Robot mode

The robot poses, turns at the waist, lights up its chest, and scoots on
small wheels hidden in its feet — it does not walk. Walking is a far
harder, more fragile machine; skating on hidden foot wheels keeps robot
mode drivable with the same radio.

## Electronics

Standard 4-channel 2.4 GHz radio. Channel 3 triggers a transform
choreography on a small microcontroller (RP2040) with an IMU for the
still-and-level interlock and an auto-recover fold-back if it tips over.
The radio stays ordinary and replaceable; all cleverness lives in one cheap
board.

## Safety

No finger-pinch gaps under 12 mm on folding joints, rounded bumpers,
battery behind a parent-only screw latch, LiFe chemistry (far more
crash-tolerant than LiPo), training-mode throttle.

## Build path — three weekends

1. Cheap 1/16 4WD donor chassis, get it driving fast.
2. Build ramps, tune the suspension for jumps.
3. Bolt on the 3D-printed transform module (PETG) — it mounts to the
   chassis plate and can be rebuilt after breakage.

Each stage is a complete toy on its own, so the project never stalls
waiting for the hard part.

## The animation

`leap1.html` shows the full show in three acts — sprint, ramp jump,
three-stage transform — with the same stage order the servo choreography
will use. Geometry is deliberately boxes and cylinders in Optimus red/blue
so the mechanism reads clearly. Buttons seek to each act; the scrubber
drives the whole timeline (the show is a pure function of time).

**You drive!** mode hands the driver the controls: hold-to-steer left/right,
gas and reverse, a jump button (a suspension hop — and much bigger air off
the ramp), and a transform toggle that works in both shapes. Big touch pads
for small thumbs, or arrows/WASD, SPACE to jump, T to transform. The
transform button refuses mid-air, exactly like the real car's IMU interlock
will.
