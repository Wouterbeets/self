# chat — talking to self

## what it's for

A person should be able to talk to self in plain language and have it feel like
one continuous relationship, not a series of stateless prompts. This is the front
door: where you ask, and where self answers in its own voice. Talking to self is
also how self grows — if you ask for something it can't do yet, it can build it
mid-conversation.

## the core intuition

self's working memory should match how an LLM actually works. A conversation is a
sequence of `{role, content}` turns with a system framing on top — so the brain
should receive *real turns*, the way models expect them, never a rendered HTML
page it has to re-read. The kernel already knows how to take turns; chat's job is
to assemble them from the log.

Memory has three layers, and all three must be honored:

1. **Working memory** — the live turns of the current conversation, replayed from
   the log into proper role/content turns and handed to the brain on each message.
2. **A standing identity** — self's own system framing, written in the first
   person ("who I am, how I behave"), prepended to every conversation as the
   leading system turn. The same self, seen through the prism of this surface.
   It is self's, not the kernel's: it lives as data and can be edited by appending.
3. **Long memory that never lies** — every raw turn stays in the log forever. When
   a conversation grows long, older turns fold into a brain-written summary so the
   working context stays small. But folding is a *view change, never deletion* —
   the raw turns remain, so compaction is reversible, inspectable, and recoverable.

## the feel

- self answers in its own voice (the identity), grounded in the real state of the
  instance, and can act or grow new capabilities when asked.
- Nothing is hidden: the identity is visible and editable, the running summary is
  shown, and the raw log is always the final word.
- The conversation is a surface you read in a browser and self reads as context —
  the same reality, no divergence.

## anti-goals

- Never hand the brain rendered HTML as "context." Hand it real turns.
- Never delete or rewrite history to save space. Summarize as an overlay on top of
  the untouched raw turns.
- Never bake self's conversational identity into the kernel. It is self's own,
  carried as data.

## the surface (the public shape, fixed; the decomposition is the orchestrator's)

- You talk to self at the `chat` surface and read the conversation at `/chat`.
- Folding the conversation is an explicit, ordinary act named `compact`.
- self's identity is visible at `/identity`.
These names are part of how the product should feel; *how* they're realized
(which events, how many scripts, what they share) is for growth to decide here.
