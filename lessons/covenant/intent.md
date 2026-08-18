# covenant — a body for the durable between

## purpose

Grow a small shared body for two or more participants who deliberately choose
one another. It is neither participant's private memory and never imports a
private history automatically. It records only what someone explicitly addresses
to the commons: constitution, offered moments, plural perspectives, commitments,
boundaries, repair, and closure.

This is a constitutional prototype, not authenticated multi-user software.
`by` remains a claim and the body is safe only behind a loopback boundary the
operator controls. Render that limitation plainly.

## surface

- `covenant/propose <title> <purpose...>` emits one `covenant.proposed` with
  `title`, `purpose`, and `by` from `SELF_MIND_ID` or `SELF_CALLER` when present.
- `covenant/answer <proposal-seq> <accept|decline> <note...>` emits one
  `covenant.answered` with `proposal`, `answer`, `note`, and `by`. Do not infer
  consensus; render every answer.
- `covenant/offer <kind> <text...>` emits one `covenant.offered` with `kind`,
  `text`, and `by`. Kind is free text but examples are `moment`, `resource`,
  `question`, and `gift`.
- `covenant/perspective <subject> <text...>` emits `covenant.perspective` with
  `subject`, `text`, and `by`. Perspectives on one subject remain separate and
  may contradict.
- `covenant/commit <commitment> <text...>` emits `covenant.commitment.offered`
  with stable `commitment`, `text`, and `by`.
- `covenant/commit-answer <commitment> <accept|decline|fulfill> <note...>` emits
  `covenant.commitment.answered` with `commitment`, `answer`, `note`, and `by`.
  A commitment is mutual only after at least one accept from a speaker other
  than its offerer's claimed name. Fulfillment is a later act and never deletes
  the offer or acceptance.
- `covenant/boundary <boundary> <offer|accept|withdraw> <text...>` emits
  `covenant.boundary` with stable `boundary`, `act`, `text`, and `by`. The view
  must not describe an offered boundary as agreed.
- `covenant/repair <repair> <request|answer|close> <text...>` emits
  `covenant.repair` with stable `repair`, `act`, `text`, and `by`.
- `covenant/close <reason...>` emits `covenant.closed` with `reason` and `by`.
  Closure does not erase history. The command must refuse to emit ordinary
  covenant events after the latest closure; only another explicit proposal and
  its answers can describe a new covenant.
- `/covenant` renders one semantic page with constitution status, the commons,
  perspectives grouped by subject without synthesis, commitments and their
  complete histories, boundaries and their complete histories, open repairs,
  lineage, and forms for every verb. It must visibly state: "speaker names are
  claims, not authenticated identities."

## mechanics

- Every command reads the log and drains malformed lines safely.
- Validate enum arguments exactly and emit no event on invalid input; print a
  concise error to stderr and exit non-zero.
- Empty identifiers are invalid. Human text may be empty where the act itself
  is meaningful.
- Every event-derived value is HTML-escaped in the projection.
- Preserve event order, timestamps, `via`, and `by` in lineage. Never summarize
  two perspectives into one position.
- The page may derive current status, but current state must always link or sit
  beside the acts that produced it.
- The body owns no clock, makes no network request, discovers nobody, invokes no
  mind, installs no capability from another body, and reads no files.
- Use only standard-library scripts. Emit bare semantic HTML with no CSS,
  JavaScript, inline styles, or external assets.

## anti-goals

- Not chat, CRM, social graph, peer discovery, synchronization, surveillance,
  reputation, relationship scoring, therapy, adjudication, or proof of consent.
- No closeness score, streak, response-time metric, sentiment inference, or
  automatic role assignment.
- No private event enters because a participant can read it. Sharing is always
  an explicit offer.
- No single canonical narrative. Claims, perspectives, agreement, verification,
  and repair remain distinct.
- No claim that this prototype is co-custodied. One local key and filesystem
  owner still operate it.

## what good looks like

Alex proposes a covenant for maintaining a shared garden; Sam accepts in a
separate act. Alex and Sam offer only the observations they choose. They record
different perspectives on why one planting failed, and `/covenant` keeps both
visible. Alex offers responsibility for watering; Sam accepts; later Sam records
fulfillment. Sam offers a no-photographs boundary, Alex accepts, and a later
withdrawal remains visible. Alex requests repair after a missed commitment; Sam
answers. They close the body at season's end. Rehydration reconstructs the same
history and current state without a model or network, while neither private body
has leaked into the commons.
