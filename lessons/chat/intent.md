# chat — the front door

## purpose

Talking to self in plain language should feel like one continuous
relationship, not a series of stateless prompts — and it is how self
extends: ask for something it cannot do yet and the next mind pass declares
and authors the capability mid-conversation, live on the next refresh. This
is the lesson a demo learns first; first impressions are its spec.

## where the mind lives (exact — the loop lives or dies on this)

The kernel holds no model, and a command cannot reach one: the mind is
outside, piped between two selves. A conversation therefore advances in two
moves, and both are ordinary events:

- The user speaks through the surface: the `chat` command appends one user
  `chat.message`. That is all it does — no waiting, no network, no mind.
- The mind speaks through the pipe: a mind pass
  (`self | claude -p | self`) reads the conversation from the rendered state
  and answers by emitting the assistant `chat.message` — plus any
  `command.declared` / `projector.declared` / `script.authored` the request
  calls for, in the same breath.

This mirrors the timers lesson, which keeps the clock outside: chat keeps
the mind outside. A user message with no assistant reply after it is an
honest state — the page shows the conversation as it stands, and the next
mind pass answers everything unanswered.

## the mechanics

- `chat` is a command with ONE param, `message` (the kernel's HTML forms pass
  each field as one positional argument, so one input box means one param).
  It emits the user's `chat.message` and nothing else.
- The mind assembles REAL turns from the log — never rendered HTML: the
  current `self.identity` text as its framing, the prior `chat.message`
  turns (honoring compaction, below), then the unanswered user messages.
- When new capabilities were declared, the assistant may name what was added
  and where it lives if that helps the user orient, but this must be ordinary
  prose, not a mandatory footer. Do not invent a capability announcement for
  ordinary events like notes, tasks, memories, or verses.

## the surface

- `/chat` renders the conversation in order, one `msg` per turn carrying the
  speaker's role as a modifier class (`msg user` / `msg assistant`) and a
  `who` label, and a single-input form at the bottom POSTing to `/run/chat`.
  An empty log renders the form and nothing broken. A trailing unanswered
  user message renders as-is — awaiting the next mind pass is a visible,
  honest state, not an error. The page must be complete and legible
  completely bare: the kernel's serve-time shell supplies the bubbles and
  the live re-render when the log grows — none of that is the projector's
  job, and none of it may be assumed.
- `welcome` — the kernel promotes a projector named `welcome` to the front
  page `/`. Declare one (it may be the same view as `/chat`) so a served demo
  lands in the conversation, not in kernel internals.
- `/identity` shows the current `self.identity` text in full, with a form to
  append a new one via the `identity` command. Identity is data, never
  kernel: the lesson deposits the first one; appending replaces it from then on.
- `compact` folds older turns: it takes a summary and a sequence number and
  emits one `chat.compacted` event `{summary, through_seq}`. Who writes the
  summary is the caller's business — usually a mind in the loop, asked to
  compact. From then on the conversation's working set is the summary plus
  only the turns after `through_seq`. Folding is a view change, never
  deletion — every raw turn stays in the log.

## memory, three layers

1. **Working turns** — the live conversation, replayed from the log into
   role/content turns on every pass.
2. **Standing identity** — self's own system framing, first person, stored as
   `self.identity` events, prepended to every conversation.
3. **The raw log** — every turn, forever. Compaction is an overlay on top of
   untouched history: reversible, inspectable, recoverable.

## anti-goals

- Never hand a mind rendered HTML as context. Hand it real turns.
- Never delete or rewrite history to save space. Summarize as an overlay.
- Never bake the conversational identity into a script. It is data.
- Never sneak a mind back inside: no command may call a model, spawn an
  agent, or block waiting for intelligence. The pipe is the only seam.

## what good looks like (the demo, end to end)

1. `self learn lessons/chat | claude -p | self` — the surface exists: chat,
   identity, compact, and the chat, welcome, and identity views.
2. Open `:7777` (`self serve`) — the conversation IS the front page, already
   carrying the seeded greeting.
3. Type "track my habits: meditation and running", then run one mind pass
   (`self | claude -p | self`) — the reply names the new capability and its
   page; refresh and `/habits` is live with working forms.
4. `self rehydrate` in a copy of the two source files rebuilds the identical
   site. Nothing was hidden anywhere.

The public names are fixed: the `chat`, `identity`, `compact` commands and
the `/chat`, `/`(welcome), `/identity` views. How they are realized — which
events beyond `chat.message`, `self.identity`, `chat.compacted`, how many
scripts, what they share — is the learning mind's to decide here.
