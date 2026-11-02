// recall for opencode: every turn written into self, the memory in every prompt.
//
// An opencode plugin that hands each turn to claude-hook as the hook JSON
// Claude Code would have sent, so both harnesses share one recorder, one
// compaction and one memory under the same SELF_HOME:
//
//   chat.message                 memory snapshot for the turn, then the prompt (UserPromptSubmit)
//   tool part completed / error  the call and its result or error (PostToolUse / PostToolUseFailure)
//   session.idle                 the reply (Stop), or a subagent's report (SubagentStop); compacts
//   session.created              a session marker for top-level sessions (SessionStart)
//   chat.system.transform        the turn's snapshot appended to the system prompt
//
// The snapshot is taken once per user message, so the system prompt changes
// once per turn, not on every step. install.sh writes SELF_RECALL_HOOK and the
// SELF_* environment into the copy it installs; with SELF_RECALL_SKIP set (a
// nested model call) the plugin does nothing. Turns are heard as
// SELF_RECALL_HARNESS (default opencode). SELF_RECALL_ORIENT, a shell command,
// is run once per session and its output joins the system prompt: the brief a
// Claude Code SessionStart hook would inject (e.g. `self`).
import { spawn } from "node:child_process"

const HOOK = process.env.SELF_RECALL_HOOK || new URL("./claude-hook", import.meta.url).pathname
const ENV = {}

function run(cmd, args, input) {
  return new Promise((resolve) => {
    let out = ""
    let child
    try {
      child = spawn(cmd, args, { env: { ...process.env, ...ENV }, stdio: ["pipe", "pipe", "ignore"] })
    } catch {
      return resolve("")
    }
    child.on("error", () => resolve(""))
    child.stdout.on("data", (b) => (out += b))
    child.on("close", () => resolve(out))
    child.stdin.end(input)
  })
}

function hook(data) {
  return run(HOOK, [], JSON.stringify({ ...data }))
}


function text(parts) {
  return (parts || [])
    .filter((p) => p.type === "text" && !p.synthetic && !p.ignored)
    .map((p) => p.text)
    .join("\n")
}

export const Recall = async ({ client, directory }) => {
  if (process.env.SELF_RECALL_SKIP) return {}
  ENV.SELF_RECALL_HARNESS ||= process.env.SELF_RECALL_HARNESS || "opencode"
  process.env.SELF_CALLER ||= ENV.SELF_RECALL_HARNESS // the agent's own `self` calls are attributable
  const ORIENT = ENV.SELF_RECALL_ORIENT || process.env.SELF_RECALL_ORIENT || ""
  const orient = new Map() // root sessionID -> brief, taken once
  const memory = new Map() // sessionID -> this turn's snapshot
  const sessions = new Map() // sessionID -> {parentID, agent}
  const recorded = new Set() // tool callIDs already written
  let chain = Promise.resolve() // records land in the order opencode emitted them
  const record = (data) => (chain = chain.then(() => hook({ cwd: directory, ...data })))

  async function reply(sessionID) {
    const res = await client.session.messages({ path: { id: sessionID } }).catch(() => null)
    const msgs = (res && res.data) || []
    const lastUser = msgs.findLastIndex((m) => m.info.role === "user")
    return msgs
      .slice(lastUser + 1)
      .filter((m) => m.info.role === "assistant")
      .map((m) => text(m.parts))
      .filter(Boolean)
      .slice(-1)
      .join("")
  }

  return {
    "chat.message": async (input, output) => {
      const s = sessions.get(input.sessionID) || {}
      s.agent = input.agent
      sessions.set(input.sessionID, s)
      const snapshot = await record({
        hook_event_name: "UserPromptSubmit",
        session_id: input.sessionID,
        prompt: text(output.parts),
        ...(s.parentID && input.agent ? { agent_type: input.agent } : {}),
      })
      if (snapshot.trim()) memory.set(input.sessionID, snapshot)
    },

    "experimental.chat.system.transform": async (input, output) => {
      const m = input.sessionID && memory.get(input.sessionID)
      if (m) output.system.push(m)
      const o = input.sessionID && orient.get(input.sessionID)
      if (o) output.system.push(o)
    },

    event: async ({ event }) => {
      const p = event.properties || {}
      if (event.type === "session.created" || event.type === "session.updated") {
        const s = sessions.get(p.info.id) || {}
        s.parentID = p.info.parentID
        sessions.set(p.info.id, s)
        if (event.type === "session.created" && !p.info.parentID) {
          if (ORIENT) run("sh", ["-c", ORIENT], "").then((o) => o.trim() && orient.set(p.info.id, o))
          await record({ hook_event_name: "SessionStart", source: "startup", session_id: p.info.id, cwd: p.info.directory || directory })
        }
      } else if (event.type === "message.part.updated" && p.part.type === "tool") {
        const { part } = p
        const st = part.state
        if ((st.status !== "completed" && st.status !== "error") || recorded.has(part.callID)) return
        recorded.add(part.callID)
        const failed = st.status === "error"
        await record({
          hook_event_name: failed ? "PostToolUseFailure" : "PostToolUse",
          session_id: part.sessionID,
          tool_name: part.tool,
          tool_input: st.input,
          ...(failed ? { error: st.error } : { tool_response: st.output }),
        })
      } else if (event.type === "session.idle") {
        const s = sessions.get(p.sessionID) || {}
        const last = await reply(p.sessionID)
        await record({
          hook_event_name: s.parentID ? "SubagentStop" : "Stop",
          session_id: p.sessionID,
          last_assistant_message: last,
          ...(s.parentID ? { agent_type: s.agent || "subagent" } : {}),
        })
      }
    },
  }
}

// opencode 2.x loads the default export ({ id, setup }) and ignores the v1
// hooks above; the v1 loader takes `server`. Same recorder, v2 shapes:
//
//   session.hook("prompt")          the prompt (UserPromptSubmit); first prompt of a session also marks it (SessionStart)
//   session.hook("context")         the session's brief and memory joined to the system prompt
//   tool.hook("execute.after")      the call and its result or error (PostToolUse / PostToolUseFailure)
//   event session.text.ended        remembers the turn's last assistant text
//   event session.execution.*       the reply (Stop); compacts
async function setup(ctx) {
  if (process.env.SELF_RECALL_SKIP) return
  ENV.SELF_RECALL_HARNESS ||= process.env.SELF_RECALL_HARNESS || "opencode"
  process.env.SELF_CALLER ||= ENV.SELF_RECALL_HARNESS
  const ORIENT = ENV.SELF_RECALL_ORIENT || process.env.SELF_RECALL_ORIENT || ""
  const cwd = ctx.location?.directory || process.cwd()
  const seen = new Map() // sessionID -> { brief, memory, last }
  let chain = Promise.resolve("")
  const record = (data) => (chain = chain.then(() => hook({ cwd, ...data })))
  const of = (id) => {
    let s = seen.get(id)
    if (!s) seen.set(id, (s = { brief: "", memory: "", last: "", started: null }))
    return s
  }

  await ctx.session.hook("prompt", async (e) => {
    const s = of(e.sessionID)
    if (!s.started) {
      s.started = (async () => {
        if (ORIENT) s.brief = (await run("sh", ["-c", ORIENT], "")).trim()
        await record({ hook_event_name: "SessionStart", source: "startup", session_id: e.sessionID })
      })()
    }
    await s.started
    const snapshot = await record({ hook_event_name: "UserPromptSubmit", session_id: e.sessionID, prompt: e.prompt?.text || "" })
    if (snapshot.trim()) s.memory = snapshot.trim()
  })

  await ctx.session.hook("context", (e) => {
    const s = seen.get(e.sessionID)
    for (const t of [s?.brief, s?.memory]) if (t) e.system.push({ type: "text", text: t })
  })

  await ctx.tool.hook("execute.after", (e) => {
    const failed = e.status === "error"
    const out = failed ? e.error?.message || "" : text(e.result?.content)
    void record({
      hook_event_name: failed ? "PostToolUseFailure" : "PostToolUse",
      session_id: e.sessionID,
      tool_name: e.tool,
      tool_input: e.input,
      ...(failed ? { error: out } : { tool_response: out }),
    })
  })

  const controller = new AbortController()
  ;(async () => {
    for await (const ev of ctx.event.subscribe({ signal: controller.signal })) {
      const d = ev.data || {}
      // The event stream is server-wide and the service runs one instance per
      // open directory: only the instance that took the session's prompt
      // reports it.
      const s = seen.get(d.sessionID)
      if (!s) continue
      if (ev.type === "session.text.ended" && d.text) s.last = d.text
      else if (ev.type === "session.execution.succeeded" || ev.type === "session.execution.failed") {
        const reply = s.last
        s.last = ""
        await record({ hook_event_name: "Stop", session_id: d.sessionID, last_assistant_message: reply })
      }
    }
  })().catch(() => {})
  // A short-lived server (opencode run --standalone) unloads right after the
  // last event: let the pending records land before it goes.
  return async () => {
    controller.abort()
    await chain
  }
}

export default { id: "self-recall", setup, server: Recall }
