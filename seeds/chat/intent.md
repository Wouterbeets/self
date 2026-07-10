# chat — the front door

## purpose

Talking to self in plain language should feel like one continuous
relationship, not a series of stateless prompts — and it is how self grows:
ask for something it cannot do yet and it builds the capability
mid-conversation, live on the next refresh. This is the seed a demo grows
first; first impressions are its spec.

## the mechanics (exact — the loop lives or dies on these)

- `chat` is a command with ONE param, `message` (the kernel's HTML forms pass
  each field as one positional argument, so one input box means one param).
- The chat script assembles REAL `{role, content}` turns from the log on its
  stdin — never rendered HTML. The leading system turn is the current
  `self.identity` text; then the prior `chat.message` turns (see compaction
  below); then the new user message. It calls `self think` with that JSON
  array as the argument and parses the `{response, declarations}` it returns.
- `self think` appends NOTHING — the caller owns persistence. The chat script
  must emit, in order: the user's `chat.message`, the assistant's
  `chat.message`, and then every declaration verbatim as its own event.
  Re-emitting the declarations is what makes the kernel compile them; a
  declaration left unemitted is growth that never happens.
- When new capabilities were declared, the assistant may name what grew and
  where it lives if that helps the user orient, but this must be ordinary prose,
  not a mandatory footer. Do not invent a growth announcement for ordinary
  events like notes, tasks, memories, or verses.
- Degrade honestly: if `self think` fails, emit an assistant `chat.message`
  that says a mind is unreachable and how to plug one (`SELF_MIND`,
  `SELF_LLM_URL`) — inside the conversation, not as a stack trace. The user's
  message is already in the log; say so.

## the surface

- `/chat` renders the conversation in order, one `msg` per turn carrying the
  speaker's role as a modifier class (`msg user` / `msg assistant`) and a
  `who` label, and a single-input form at the bottom POSTing to `/run/chat`.
  An empty log renders the form and nothing broken. The page must be complete
  and legible completely bare: the kernel's serve-time shell supplies the
  bubbles, the pending state while the mind thinks, and the live re-render
  when the log grows — none of that is the projector's job, and none of it
  may be assumed.
- `welcome` — the kernel promotes a projector named `welcome` to the front
  page `/`. Grow one (it may be the same view as `/chat`) so a served demo
  lands in the conversation, not in kernel internals.
- `/identity` shows the current `self.identity` text in full, with a form to
  append a new one via the `identity` command. Identity is data, never
  kernel: the seed plants the first one; appending replaces it from then on.
- `compact` folds older turns: it asks the mind for a summary and emits one
  `chat.compacted` event `{summary, through_seq}`. From then on the chat
  script sends the summary (inside the system turn) plus only the turns after
  `through_seq`. Folding is a view change, never deletion — every raw turn
  stays in the log.

## memory, three layers

1. **Working turns** — the live conversation, replayed from the log into
   role/content turns on every message.
2. **Standing identity** — self's own system framing, first person, stored as
   `self.identity` events, prepended to every conversation.
3. **The raw log** — every turn, forever. Compaction is an overlay on top of
   untouched history: reversible, inspectable, recoverable.

## anti-goals

- Never hand the mind rendered HTML as context. Hand it real turns.
- Never delete or rewrite history to save space. Summarize as an overlay.
- Never bake the conversational identity into a script. It is data.
- Never a dead send: if the mind is unreachable, the conversation itself
  says so and says how to fix it.

## what good looks like (the demo, end to end)

1. `self grow seeds/chat` — the surface exists: chat, identity, compact,
   and the chat, welcome, and identity views.
2. Open `:7777` — the conversation IS the front page, already carrying the
   seeded greeting.
3. "track my habits: meditation and running" — the reply names the new
   capability and its page; refresh and `/habits` is live with working forms.
4. `self rehydrate` in a copy of the two source files rebuilds the identical
   site. Nothing was hidden anywhere.

The public names are fixed: the `chat`, `identity`, `compact` commands and
the `/chat`, `/`(welcome), `/identity` views. How they are realized — which
events beyond `chat.message`, `self.identity`, `chat.compacted`, how many
scripts, what they share — is the orchestrator's to decide here.
