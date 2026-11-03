// recall for pi: every turn written into self, the brief in every system prompt.
//
// A pi extension that hands each turn to claude-hook as the hook JSON Claude
// Code would have sent, so every harness shares one recorder, one compaction
// and one memory under the same SELF_HOME:
//
//   session_start        a session marker (SessionStart); the orientation brief is taken
//   before_agent_start   the prompt (UserPromptSubmit); brief and memory join the system prompt
//   tool_result          the call and its result or error (PostToolUse / PostToolUseFailure)
//   agent_end            remembers the run's last assistant text
//   agent_settled        the reply (Stop); compacts
//
// install.sh writes HOOK and the SELF_* environment into the copy it installs.
// Turns are heard as SELF_RECALL_HARNESS (default pi). SELF_RECALL_ORIENT, a
// shell command, runs once per session and its output joins the system prompt:
// the brief a Claude Code SessionStart hook would inject (e.g. `self`). With
// SELF_RECALL_SKIP set (a nested model call) the extension does nothing.
// @ts-nocheck
import { spawn } from "node:child_process"

const HOOK = process.env.SELF_RECALL_HOOK || new URL("./claude-hook", import.meta.url).pathname
const ENV = {}

function run(cmd: string, args: string[], input: string): Promise<string> {
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

function text(content: any): string {
  if (typeof content === "string") return content
  return (content || [])
    .filter((c) => c && c.type === "text" && typeof c.text === "string")
    .map((c) => c.text)
    .join("\n")
}

export default function (pi) {
  if (process.env.SELF_RECALL_SKIP) return
  ENV.SELF_RECALL_HARNESS ||= process.env.SELF_RECALL_HARNESS || "pi"
  process.env.SELF_CALLER ||= ENV.SELF_RECALL_HARNESS // the agent's own `self` calls are attributable
  const ORIENT = ENV.SELF_RECALL_ORIENT || process.env.SELF_RECALL_ORIENT || ""

  let session = ""
  let cwd = process.cwd()
  let brief = ""
  let memory = ""
  let last = ""
  let chain = Promise.resolve("") // records land in the order pi emitted them
  const record = (data) => (chain = chain.then(() => run(HOOK, [], JSON.stringify({ cwd, session_id: session, ...data }))))

  const sessionOf = (ctx) => {
    try {
      return ctx?.sessionManager?.getSessionId?.() || ctx?.sessionManager?.getSessionFile?.() || session
    } catch {
      return session
    }
  }

  pi.on("session_start", async (event, ctx) => {
    session = sessionOf(ctx) || `pi-${process.pid}-${Date.now()}`
    cwd = ctx?.cwd || process.cwd()
    brief = ORIENT ? (await run("sh", ["-c", ORIENT], "")).trim() : ""
    await record({ hook_event_name: "SessionStart", source: event?.reason || "startup" })
  })

  pi.on("before_agent_start", async (event, ctx) => {
    session = sessionOf(ctx) || session
    const snapshot = await record({ hook_event_name: "UserPromptSubmit", prompt: event?.prompt || "" })
    if (snapshot.trim()) memory = snapshot.trim()
    const extra = [brief, memory].filter(Boolean).join("\n\n")
    if (!extra) return
    return { systemPrompt: `${event.systemPrompt}\n\n${extra}` }
  })

  pi.on("tool_result", async (event) => {
    const failed = !!event?.isError
    const out = text(event?.content)
    void record({
      hook_event_name: failed ? "PostToolUseFailure" : "PostToolUse",
      tool_name: event?.toolName,
      tool_input: event?.input,
      ...(failed ? { error: out } : { tool_response: out }),
    })
  })

  pi.on("agent_end", async (event) => {
    const msgs = event?.messages || []
    for (let i = msgs.length - 1; i >= 0; i--) {
      const m = msgs[i]
      if (m?.role === "assistant") {
        const t = text(m.content)
        if (t.trim()) {
          last = t
          break
        }
      }
    }
  })

  pi.on("agent_settled", async (_event, ctx) => {
    if (ctx?.isIdle?.() === false) return
    const reply = last
    last = ""
    await record({ hook_event_name: "Stop", last_assistant_message: reply })
  })
}
