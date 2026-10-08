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
// once per turn, not on every step. install-opencode.sh writes SELF_RECALL_HOOK
// and the SELF_* environment into the copy it installs; with SELF_RECALL_SKIP
// set (a nested model call) the plugin does nothing.
import { spawn } from "node:child_process"

const HOOK = process.env.SELF_RECALL_HOOK || new URL("./claude-hook", import.meta.url).pathname
const ENV = {}

function hook(data) {
  return new Promise((resolve) => {
    let out = ""
    let child
    try {
      child = spawn(HOOK, [], { env: { ...process.env, ...ENV }, stdio: ["pipe", "pipe", "ignore"] })
    } catch {
      return resolve("")
    }
    child.on("error", () => resolve(""))
    child.stdout.on("data", (b) => (out += b))
    child.on("close", () => resolve(out))
    child.stdin.end(JSON.stringify(data))
  })
}

function text(parts) {
  return (parts || [])
    .filter((p) => p.type === "text" && !p.synthetic && !p.ignored)
    .map((p) => p.text)
    .join("\n")
}

export const Recall = async ({ client, directory }) => {
  if (process.env.SELF_RECALL_SKIP) return {}
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
    },

    event: async ({ event }) => {
      const p = event.properties || {}
      if (event.type === "session.created" || event.type === "session.updated") {
        const s = sessions.get(p.info.id) || {}
        s.parentID = p.info.parentID
        sessions.set(p.info.id, s)
        if (event.type === "session.created" && !p.info.parentID) {
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
