# theater

The flashy, optional, deletable front end. The kernel's own surface is bare
semantic HTML on purpose — every page a pure function of the log, every
action a plain form. That is the right surface to *live* with, and the wrong
one to *demo* with. The theater is the demo: a Jarvis-flavored lens that
makes the machinery visible — and dramatic — without touching it.

```sh
self                # the instance, serving at 127.0.0.1:7777
./theater/serve     # the booth — open http://127.0.0.1:7788
```

What you see:

- **The log as a galaxy.** Every event is a gaussian splat (a radial-falloff
  sprite, additively blended, projected from 3D — the zero-dependency cousin
  of 3DGS) placed on a sunflower spiral by its `seq`. The layout is a pure
  function of the log: replay the log, get the same galaxy. New events fly
  out of the core and flash as they land. Click any splat to inspect the
  raw event; drag to orbit, wheel to zoom.
- **Capabilities as an orbiting ring.** Every verified `script.compiled`
  becomes a labeled satellite; retirements drop out.
- **The mind, live.** Speak (mic button; Web Speech API, works on
  localhost) or type, and the booth runs `self think` — the core flares
  gold while the mind explores the instance, the reply comes back as text
  and speech. A think is report-only, so the mind's proposed events render
  as ghosts that fade: shown, never appended.

## What the theater is not

It is a lens, not a surface. It reads the same append-only log every other
surface reads (`/events`, tailed by byte offset — append-only is exactly
what makes a Range tail read correct) and speaks through the same seam
(`self think`). It holds no state, appends nothing on its own, and the
kernel does not know it exists. Delete the directory and nothing is lost —
that deletability is the proof it hasn't compromised the architecture.

Two honest caveats. Browser speech recognition may leave the machine
(Chrome's implementation is server-backed); typing is the local-first path,
and replies are spoken with the local synthesis voice either way. And the
booth's `/think` runs the real mind — on a local model an answer can take a
minute; the flaring core is not a loading spinner, it is genuinely what the
money paid for.

## Environment

```
THEATER_BIND      bind address              (default 127.0.0.1:7788)
THEATER_SELF_URL  the instance to watch     (default http://127.0.0.1:7777)
THEATER_SELF_CLI  the self executable       (default "self"; think inherits
                  this environment, so SELF_HOME / SELF_MIND apply)
```
