# chat — how a mind and a human talk

## purpose

The kernel keeps no diary. It has no `self.asked`, no `self.replied`, no
conversation of its own — that was a second conversation living beside this one,
and it is gone. So a mind that wants to say something to a human needs a channel,
and this lesson is it. Learn it early: without it, a pass through
`self "…" | claude -p | self hear` can do durable work but cannot answer you in
words.

Conversation is a capability, not a kernel feature. That is the whole point of
this account.

## where the mind lives (the loop lives or dies on this)

The kernel holds no model and a command cannot reach one. The mind is outside,
piped between two selves, so a conversation advances in two ordinary moves:

- **A human speaks through a command.** `self run say <text…>` appends one
  `chat.message` with role `user`. That is all it does — no waiting, no network,
  no mind.
- **A mind speaks through the wire.** A pass (`self | claude -p | self hear`)
  reads the conversation by replaying it, and answers by printing one
  `chat.message` with role `assistant` — plus any declaration or
  `script.authored` the request called for, in the same breath.

A user message with no assistant message after it is an honest state, not an
error: it means the next pass has work. The view shows it as it stands.

## surface

- `self run say <text…>` — append one `chat.message` `{role:"user", content}`.
- `self view chat` — the conversation in order, oldest first, one turn per line
  or paragraph, each labelled with its speaker and marked when a trailing user
  message is still unanswered. Plain text; an empty log renders an invitation,
  not an error.
- `self view identity` — the current `self.identity` text in full. Identity is
  data, never kernel: the record below deposits the first one, and appending a
  new `self.identity` event replaces it from then on.

## constraints

- Event names exactly `chat.message` (fields `role`: `user` | `assistant`, and
  `content`) and `self.identity` (field `text`). The record deposited by this
  account already uses them; your views must read what is already there.
- The `chat` view consumes only `chat.message`. `identity` consumes only
  `self.identity` and shows the latest.
- Nothing is edited or deleted. A correction is a later message.
- No HTML, no forms, no server. The reader is a terminal or an agent.

## anti-goals

- Do not make the kernel aware of `chat.message`. Nothing in `self help`
  mentions it and nothing should: the previous kernel parsed this event name
  itself, and that leak is exactly what this rewrite removed.
- Do not build a "waiting for reply" queue, a notification, or a poller. The
  view showing an unanswered message *is* the queue.

## what good looks like

```sh
self run say "what can you do?"
self view chat                  # shows the question, marked unanswered
self | claude -p | self hear    # the mind reads it and answers
self view chat                  # shows both turns
```
