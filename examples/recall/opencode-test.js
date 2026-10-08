// Drives an installed recall.js the way opencode would, with a stub client:
// bun opencode-test.js <plugin-dir>. Prints what the plugin adds to the
// system prompt; test.sh checks that and the log.
const { Recall } = await import(process.argv[2] + "/recall.js")
const msgs = [
  { info: { role: "user" }, parts: [{ type: "text", text: "read notes" }] },
  { info: { role: "assistant" }, parts: [{ type: "text", text: "notes say hello" }] },
]
const client = { session: { messages: async () => ({ data: msgs }) } }
const h = await Recall({ client, directory: "/w" })
if (!h.event) process.exit(0)
const ev = (type, properties) => h.event({ event: { type, properties } })
const tool = (callID, state) =>
  ev("message.part.updated", { part: { type: "tool", sessionID: "s1", callID, tool: "read", state } })

await ev("session.created", { info: { id: "s1", directory: "/w" } })
await ev("session.created", { info: { id: "s2", directory: "/w", parentID: "s1" } })
await h["chat.message"](
  { sessionID: "s1", agent: "build" },
  { message: {}, parts: [{ type: "text", text: "read notes" }, { type: "text", text: "ctx", synthetic: true }] },
)
await tool("c1", { status: "running", input: { filePath: "/w/notes" } })
await tool("c1", { status: "completed", input: { filePath: "/w/notes" }, output: "hello", title: "notes" })
await tool("c1", { status: "completed", input: { filePath: "/w/notes" }, output: "hello", title: "notes" })
await tool("c2", { status: "error", input: { filePath: "/w/gone" }, error: "no such file" })
await ev("session.idle", { sessionID: "s2" })
await ev("session.idle", { sessionID: "s1" })
const out = { system: [] }
await h["experimental.chat.system.transform"]({ sessionID: "s1" }, out)
process.stdout.write(out.system.join(""))
