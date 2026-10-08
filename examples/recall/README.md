# Recall: every agent turn in self, as one memory injected into every prompt

Claude Code hooks, or an opencode plugin, write every user prompt, tool call,
tool result, subagent report and reply into self as `chat.message` events: one
conversation that never ends, across every session, project and harness that
shares a `SELF_HOME`. The
`recall` view compresses it into a memory that fits in a prompt: old lines
summarize many messages, recent lines few. The hooks inject that memory into
every prompt. This follows
[VictorTaelin's design](https://gist.github.com/VictorTaelin/91837951a5ce5b38f341ec1ba1df6449).

```sh
export SELF_HOME=~/.self
examples/recall/install.sh                        # ~/.claude/settings.json
examples/recall/install.sh .claude/settings.json  # or one project
examples/recall/install.sh --opencode             # ~/.config/opencode/plugin/recall.js
examples/recall/install.sh --opencode .opencode/plugin
```

`self` must be on `PATH` inside the agent's shell so the agent can zoom. The
installer writes `SELF_HOME`, `SELF_BIN` and any `SELF_RECALL_*` you set into
the hook command or the plugin. Rerunning it replaces the earlier install.
Both harnesses can share one `SELF_HOME`: a session in one remembers the other.

## opencode

opencode has no shell hooks, so `opencode.js` is a plugin. It hands each turn
to `claude-hook` as the JSON Claude Code would have sent, so both harnesses
share one recorder and one compaction:

| opencode | recorded as |
|---|---|
| `chat.message` | the prompt (after taking the turn's memory snapshot) |
| tool part `completed` / `error` | the call and its result or error, once per call |
| `session.idle` | the reply, or a subagent's report for a child session; starts a compaction |
| `session.created` (top level) | a session marker |

The memory goes into the system prompt through
`experimental.chat.system.transform`, not into the conversation. It is
snapshotted once per user message, so the system prompt changes once per turn
rather than on every step, and copies do not pile up in the session as they do
in Claude Code. The plugin runs `claude-hook`, so it needs `python3`. The
default compaction mind is still `claude -p`. Without Claude Code, set
`SELF_RECALL_MIND` before installing, for example
`opencode run -m <provider/model> "$(cat)"`.

## What the agent sees

```
<recall>
Your memory of every earlier conversation on this machine, oldest first: …
0+16 10-02 09:14 User wanted lease renewals atomic; added "atomic": true to lease, test.sh passes …
16+8 10-03 11:40 …
24+2 10-08 16:58 …
26+1 10-08 17:20 user: rewrite the quick start, keep the AGENTS.md line
</recall>
```

Each line is a node `<first>+<count>` covering messages `first..first+count-1`.
`self view recall 16+8` opens a line into its two halves, and `self view recall
26+1` prints one whole message. The agent zooms until it has the detail it
needs.

## How it compacts

Every message is a leaf of a binary tree. A node covers `2^l` messages and
starts at a multiple of `2^l`. A summary is at most `SELF_RECALL_LINE` bytes
(default 200). A short message is its own line. A long one gets a summary
(`recall.node`), and until then the view shows a clipped draft ending in `…`.

The memory is the list of nodes that covers every message, oldest first. A pair
of sibling lines at size `s` is *due* `(T - last) / s`, where `T` is the
message count and `last` is the pair's final message. So a pair ages in units
of its own size, measured from its end. When the memory passes
`SELF_RECALL_HIGH` bytes, the most due pair whose parent is summarized merges
(`recall.merged`), again and again until the memory is under `SELF_RECALL_LOW`.
Between batches the memory only grows at its end, one line per message. Over
time its size rises and drops in a sawtooth, and the result is coarse in the
past and fine in the present.

`self view recall --plan` decides all of this as a pure function of the log. It
prints the merges that are ready and the summaries that are still needed, each
as a full prompt. `claude-hook --compact` runs those prompts through
`SELF_RECALL_MIND`, eight at a time. It shows the model a ruler, sends any line
over the limit back up to five times, then cuts it. It hears the results and
asks for the next plan. The kernel holds no model. The Stop and SessionStart
hooks start a compaction in the background, one at a time per instance.

## Changes from the gist

- **Budget.** Claude Code caps injected hook output at 10,000 characters, so
  the defaults are 6,000/9,000 bytes with 200-byte lines (the gist uses 64/128
  KB with 512-byte lines). Raise them if you inject the memory some other way.
- **No waiting.** A prompt does not wait for earlier messages to be summarized.
  It sees drafts instead.
- **No saved view file.** The view is replayed from the log, and every merge is
  an event in the log. Replaying the same events always gives the same bytes,
  so nothing needs saving.
- **Per-session context accumulates.** Claude Code keeps the session's
  conversation, so each prompt adds another copy of the memory to it. The gist
  starts every turn fresh.

## Configuration

Set these before running `install.sh`, which writes them into the hook command.

| variable | default | meaning |
|---|---|---|
| `SELF_RECALL_LINE` | 200 | bytes per memory line |
| `SELF_RECALL_LOW` / `_HIGH` | 6000 / 9000 | bytes the memory shrinks to / that start a merge batch |
| `SELF_RECALL_CLIP` | 2000 | characters of each tool result recorded |
| `SELF_RECALL_MIND` | `claude -p --model haiku --tools '' …` | prompt on stdin, one line on stdout |
| `SELF_RECALL_LOG` | (discarded) | file for background compaction errors |
| `SELF_RECALL_INJECT` | `prompt` | `start` injects on SessionStart only, so a long session holds one copy instead of one per prompt |

The mind runs with `SELF_RECALL_SKIP=1`, so a nested `claude` records nothing.
To avoid the subscription login and skip all hooks, use `claude --bare -p …`
with an `ANTHROPIC_API_KEY`.

## Limits

- Every message goes into `events.jsonl` verbatim, except tool results, which
  are clipped. Use a dedicated `SELF_HOME` if your main log should stay small.
- A long message costs one model call, and so does a merge.
- Hooks are synchronous but quick (about 0.1 s). They never fail and never
  block a prompt.

## Test

```sh
sh examples/recall/test.sh
```

The test uses a stub in place of the model. It checks the hooks, the retry on
over-long lines, the budget, that the lines tile all messages with coarse old
ones and fine recent ones, zooming, and that the view is deterministic. With
`bun` installed it also drives the opencode plugin with a stub client
(`opencode-test.js`).
