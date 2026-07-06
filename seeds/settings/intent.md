# settings — configure the brain

## purpose

The first trusted seed. It exists so a fresh clone can configure a brain from the
browser before any LLM is available. The user is the brain for this step: inspect
the command, event, projection, and secret handling, then decide whether to
install it.

## surface

- `self run configure-brain <provider> <command> <base_url> <model> <key>`
  appends one `brain.configured` event.
- `/settings` renders the latest non-secret brain configuration and a form to
  save a new one.

## trust boundary

- The API key is a secret. It is written to `SELF_HOME/.brain-key`, never to
  `events.jsonl`.
- The log records only provider, command, endpoint, model, and whether a key is
  set.
- `SELF_BRAIN` from the shell still wins over saved settings for explicit agent
  sessions.
- This seed is bundled as reviewed kernel data so it can install before a brain
  exists. Normal seeds still grow through the configured brain.
