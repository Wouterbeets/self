package kernel

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"self/internal/event"
	"self/internal/seed"
	"self/internal/store"
)

const PipeContract = `command script: receives args as argv, current events as JSONL on stdin, writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at.
projector script: receives all events as JSONL on stdin, writes HTML on stdout. The kernel persists the output to SELF_HOME/site/<projector_name>.html.
The kernel sets the SELF_HOME env var on every script.
Scripts can be in any language os.Exec can run — Python, bash, node, anything with a shebang.`

type CommandInfo struct {
	Name        string
	Description string
	Event       string
	Params      map[string]string
}

type ProjectorInfo struct {
	Name        string
	Description string
	Consumes    []string
}

type SeedInfo struct {
	Name       string
	Commands   []string
	Projectors []string
	Seq        int
}

type CompiledInfo struct {
	Type string
	Name string
	Seq  int
}

// RenderHTML reads the event log, extracts all command.declared,
// projector.declared, and seed.planted events, and writes the kernel
// wiring as HTML to SELF_HOME/site/kernel.html. Called at init and grow.
func RenderHTML(home string) error {
	st := openEventStore(home)
	events, err := st.Read()
	if err != nil {
		return err
	}

	var commands []CommandInfo
	var projectors []ProjectorInfo
	var seeds []SeedInfo
	var compiled []CompiledInfo

	for _, e := range events {
		switch e.Name {
		case event.CommandDeclared:
			var cmd seed.Command
			json.Unmarshal(e.Payload, &cmd)
			commands = append(commands, CommandInfo{
				Name:        cmd.Name,
				Description: cmd.Description,
				Event:       cmd.Event.Name,
				Params:      cmd.Params,
			})
		case event.ProjectorDeclared:
			var proj seed.ProjectorDecl
			json.Unmarshal(e.Payload, &proj)
			projectors = append(projectors, ProjectorInfo{
				Name:        proj.Name,
				Description: proj.Description,
				Consumes:    proj.Consumes,
			})
		case event.SeedPlanted:
			var rec struct {
				Seed       string   `json:"seed"`
				Commands   []string `json:"commands"`
				Projectors []string `json:"projectors"`
			}
			json.Unmarshal(e.Payload, &rec)
			seeds = append(seeds, SeedInfo{
				Name:       rec.Seed,
				Commands:   rec.Commands,
				Projectors: rec.Projectors,
				Seq:        e.Seq,
			})
		case event.ScriptCompiled:
			var c struct {
				Type string `json:"type"`
				Name string `json:"name"`
			}
			json.Unmarshal(e.Payload, &c)
			compiled = append(compiled, CompiledInfo{
				Type: c.Type,
				Name: c.Name,
				Seq:  e.Seq,
			})
		}
	}

	html := buildHTML(home, commands, projectors, seeds, compiled)

	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	return os.WriteFile(filepath.Join(siteDir, "kernel.html"), []byte(html), 0644)
}

func buildHTML(home string, commands []CommandInfo, projectors []ProjectorInfo, seeds []SeedInfo, compiled []CompiledInfo) string {
	var b strings.Builder
	esc := html.EscapeString
	cmdPath := func(name string) string { return filepath.Join(home, "capabilities", "commands", name) }
	projPath := func(name string) string { return filepath.Join(home, "capabilities", "projectors", name) }

	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<title>self</title>\n")
	b.WriteString("<style>\n")
	b.WriteString("body { font-family: -apple-system, sans-serif; margin: 20px; background: #fafafa; color: #222; }\n")
	b.WriteString("h1 { margin-bottom: 4px; }\n")
	b.WriteString("h2 { margin-top: 32px; border-bottom: 1px solid #ddd; padding-bottom: 6px; }\n")
	b.WriteString(".version { color: #888; margin-top: 0; }\n")
	b.WriteString(".lede { font-size: 1.05rem; color: #444; max-width: 70ch; }\n")
	b.WriteString("article { background: white; border: 1px solid #e0e0e0; border-radius: 6px; padding: 12px 16px; margin: 8px 0; }\n")
	b.WriteString("article h3 { margin: 0 0 4px 0; font-family: monospace; }\n")
	b.WriteString("article p { margin: 4px 0; color: #555; }\n")
	b.WriteString("dl { margin: 4px 0; }\n")
	b.WriteString("dt { font-family: monospace; font-weight: bold; color: #333; }\n")
	b.WriteString("dd { margin-left: 16px; color: #666; }\n")
	b.WriteString("ul.consumes { list-style: none; padding: 0; }\n")
	b.WriteString("ul.consumes li { font-family: monospace; font-size: 13px; padding: 2px 0; }\n")
	b.WriteString(".tag { display: inline-block; background: #e8f0fe; border-radius: 3px; padding: 1px 6px; font-size: 12px; font-family: monospace; }\n")
	b.WriteString(".path { font-family: monospace; font-size: 12px; color: #2563eb; }\n")
	b.WriteString(".run { font-family: monospace; font-size: 12px; color: #6a737d; }\n")
	b.WriteString("pre { background: #f4f4f4; border: 1px solid #e0e0e0; border-radius: 4px; padding: 12px; overflow-x: auto; font-size: 13px; }\n")
	b.WriteString("code { font-family: monospace; background: #f0f0f0; border-radius: 3px; padding: 1px 4px; }\n")
	b.WriteString("details { margin: 8px 0; }\n")
	b.WriteString("summary { cursor: pointer; font-weight: bold; color: #555; }\n")
	b.WriteString(".grid { width: 100%; border-collapse: collapse; margin: 8px 0; }\n")
	b.WriteString(".grid th { background: #4a5568; color: white; padding: 6px 10px; text-align: left; }\n")
	b.WriteString(".grid td { border: 1px solid #ddd; padding: 6px 10px; font-family: monospace; font-size: 13px; }\n")
	b.WriteString("</style>\n")
	b.WriteString("</head>\n<body>\n")

	// Identity
	b.WriteString("<h1>self</h1>\n")
	b.WriteString("<p class=\"version\">self/v0 · a sovereign, self-improving capability system</p>\n")
	b.WriteString("<p class=\"lede\">One append-only event log is my only truth. Everything you see here — my capabilities, my projections, this page — is a pure replay of that log, and you and my agent see exactly the same thing. A tiny kernel; everything else grows as <strong>seeds</strong> through the strange loop. Nothing is hidden: every path below is a plain file you can open.</p>\n")

	// Brain identity — written for the LLM that reads this page as context.
	b.WriteString("<section id=\"brain-identity\">\n")
	b.WriteString("<h2>you are my brain</h2>\n")
	b.WriteString("<p>If you are the LLM reading this as context: you are the <strong>brain</strong> of self. The append-only event log (<code>events.jsonl</code>) is the only source of truth; this page, the projections in <code>site/</code>, and the compiled scripts in <code>capabilities/</code> are all derived from it by replay. Nothing is destroyed — a &ldquo;delete&rdquo; is a tombstone event, reversible by a later restore — so you can act without fear.</p>\n")
	b.WriteString("<p>You have three powers:</p>\n")
	b.WriteString("<dl>\n")
	b.WriteString("<dt>read</dt><dd>a read-only <code>bash</code> tool to explore the garden — inspect <code>capabilities/</code>, <code>events.jsonl</code>, and <code>site/</code>. The <code>site/*.html</code> projections are my memory; read the relevant ones (e.g. <code>site/chat.html</code> for the conversation) before answering.</dd>\n")
	b.WriteString("<dt>act</dt><dd>every capability listed below is exposed to you as a callable tool. To change something the user asks for, <em>call the matching capability</em> — don't merely describe it. The kernel runs it and appends the resulting events.</dd>\n")
	b.WriteString("<dt>grow</dt><dd>when no existing capability fits, <code>declare</code> a new <code>command.declared</code> or <code>projector.declared</code>; the kernel compiles it on the spot and it becomes a capability you can use immediately.</dd>\n")
	b.WriteString("</dl>\n")
	b.WriteString("</section>\n")

	// My capabilities — commands
	b.WriteString("<section id=\"commands\">\n")
	b.WriteString("<h2>my capabilities — commands</h2>\n")
	if len(commands) == 0 {
		b.WriteString("<p>None yet. Grow one: <code>self grow &lt;seed&gt;</code>.</p>\n")
	}
	for _, cmd := range commands {
		b.WriteString(fmt.Sprintf("<article class=\"command\" data-name=%q data-event=%q>\n", cmd.Name, cmd.Event))
		b.WriteString(fmt.Sprintf("<h3>%s</h3>\n", esc(cmd.Name)))
		b.WriteString(fmt.Sprintf("<p>%s</p>\n", esc(cmd.Description)))
		b.WriteString(fmt.Sprintf("<p>produces event: <span class=\"tag\">%s</span></p>\n", esc(cmd.Event)))
		if len(cmd.Params) > 0 {
			b.WriteString("<dl class=\"params\">\n")
			for k, v := range cmd.Params {
				b.WriteString(fmt.Sprintf("<dt>%s</dt><dd>%s</dd>\n", esc(k), esc(v)))
			}
			b.WriteString("</dl>\n")
		}
		b.WriteString(fmt.Sprintf("<p class=\"run\">run: self run %s …</p>\n", esc(cmd.Name)))
		b.WriteString(fmt.Sprintf("<p class=\"path\">%s</p>\n", esc(cmdPath(cmd.Name))))
		b.WriteString("</article>\n")
	}
	b.WriteString("</section>\n")

	// My capabilities — projections
	b.WriteString("<section id=\"projectors\">\n")
	b.WriteString("<h2>my capabilities — projections</h2>\n")
	if len(projectors) == 0 {
		b.WriteString("<p>None yet. A projection is how events become a view.</p>\n")
	}
	for _, proj := range projectors {
		b.WriteString(fmt.Sprintf("<article class=\"projector\" data-name=%q>\n", proj.Name))
		b.WriteString(fmt.Sprintf("<h3>%s</h3>\n", esc(proj.Name)))
		b.WriteString(fmt.Sprintf("<p>%s</p>\n", esc(proj.Description)))
		b.WriteString("<ul class=\"consumes\">\n")
		for _, c := range proj.Consumes {
			b.WriteString(fmt.Sprintf("<li data-event=%q>%s</li>\n", c, esc(c)))
		}
		b.WriteString("</ul>\n")
		b.WriteString(fmt.Sprintf("<p class=\"run\">view: <a href=\"/%s\">/%s</a> &nbsp;·&nbsp; self show %s</p>\n", esc(proj.Name), esc(proj.Name), esc(proj.Name)))
		b.WriteString(fmt.Sprintf("<p class=\"path\">%s</p>\n", esc(projPath(proj.Name))))
		b.WriteString("</article>\n")
	}
	b.WriteString("</section>\n")

	// Where I live — discoverable paths
	b.WriteString("<section id=\"where\">\n")
	b.WriteString("<h2>where I live</h2>\n")
	b.WriteString("<p>Everything is plain files. Open, grep, inspect freely — or run <code>self where</code>.</p>\n")
	b.WriteString("<table class=\"grid\">\n<tr><th>what</th><th>path</th></tr>\n")
	for _, row := range [][2]string{
		{"home (SELF_HOME)", home},
		{"event log (the truth)", filepath.Join(home, "events.jsonl")},
		{"commands", filepath.Join(home, "capabilities", "commands")},
		{"projections", filepath.Join(home, "capabilities", "projectors")},
		{"materialized HTML", filepath.Join(home, "site")},
	} {
		b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>\n", esc(row[0]), esc(row[1])))
	}
	b.WriteString("</table>\n</section>\n")

	// How to explore — CLI cheat sheet
	b.WriteString("<section id=\"explore\">\n")
	b.WriteString("<h2>how to explore me</h2>\n")
	b.WriteString("<table class=\"grid\">\n<tr><th>command</th><th>what it does</th></tr>\n")
	for _, row := range [][2]string{
		{"self", "start this live garden (the default)"},
		{"self ls", "overview of capabilities"},
		{"self ls commands", "commands with full file paths"},
		{"self ls projectors", "projections with full file paths"},
		{"self where", "SELF_HOME and every important path"},
		{"self which <name>", "full path to a command or projection"},
		{"self history", "recent events, human-readable"},
		{"self run <command> …", "run a capability"},
		{"self show <name>", "render a projection (browser, or stdout when piped)"},
		{"self think \"…\"", "ask the brain"},
		{"self heartbeat", "one self-improvement cycle"},
		{"self restore <name> [seq]", "roll a capability back to an earlier version"},
		{"self grow <seed>", "grow a new capability from a seed"},
	} {
		b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>\n", esc(row[0]), esc(row[1])))
	}
	b.WriteString("</table>\n</section>\n")

	// Wiring table
	b.WriteString("<section id=\"wiring\">\n")
	b.WriteString("<h2>wiring — command → event → projection</h2>\n")
	if len(commands) > 0 && len(projectors) > 0 {
		b.WriteString("<table class=\"grid\">\n")
		b.WriteString("<tr><th>command</th><th>produces event</th><th>seen by projections</th></tr>\n")
		for _, cmd := range commands {
			var consumedBy []string
			for _, proj := range projectors {
				for _, c := range proj.Consumes {
					if c == cmd.Event {
						consumedBy = append(consumedBy, proj.Name)
						break
					}
				}
			}
			consumedStr := "—"
			if len(consumedBy) > 0 {
				consumedStr = strings.Join(consumedBy, ", ")
			}
			b.WriteString(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td></tr>\n",
				esc(cmd.Name), esc(cmd.Event), esc(consumedStr)))
		}
		b.WriteString("</table>\n")
	} else {
		b.WriteString("<p>No wiring yet — grow a seed with a command and a projection.</p>\n")
	}
	b.WriteString("</section>\n")

	// Events I act on + the contract that powers it all
	b.WriteString("<section id=\"identity\">\n")
	b.WriteString("<h2>the events I act on</h2>\n")
	b.WriteString("<dl>\n")
	b.WriteString("<dt>kernel.initialized</dt><dd>written by <code>self init</code></dd>\n")
	b.WriteString("<dt>command.declared</dt><dd>compiled into a command by <code>self grow</code> and <code>self run</code></dd>\n")
	b.WriteString("<dt>projector.declared</dt><dd>compiled into a projection by <code>self grow</code> and <code>self run</code></dd>\n")
	b.WriteString("<dt>script.compiled</dt><dd>a <strong>kernel-only</strong> receipt of a compile. Seeds and commands may not emit it — I only ever run code my own compiler authored, so the attack surface stays finite. <code>self restore</code> reads these receipts to roll a capability back to an earlier version.</dd>\n")
	b.WriteString("<dt>seed.planted</dt><dd>written by <code>self grow</code> as a receipt</dd>\n")
	b.WriteString("</dl>\n")

	b.WriteString("<h3>pipe contract</h3>\n")
	b.WriteString("<pre>")
	b.WriteString(esc(PipeContract))
	b.WriteString("</pre>\n")

	b.WriteString("<details class=\"system-prompt\" data-type=\"command\">\n")
	b.WriteString("<summary>command system prompt (used when compiling)</summary>\n")
	b.WriteString("<pre>")
	b.WriteString(esc(seed.CommandSystemPrompt))
	b.WriteString("</pre>\n</details>\n")
	b.WriteString("<details class=\"system-prompt\" data-type=\"projector\">\n")
	b.WriteString("<summary>projection system prompt (used when compiling)</summary>\n")
	b.WriteString("<pre>")
	b.WriteString(esc(seed.ProjectorSystemPrompt))
	b.WriteString("</pre>\n</details>\n")
	b.WriteString("</section>\n")

	// Compilation history
	b.WriteString("<section id=\"compilations\">\n")
	b.WriteString("<h2>compilation history</h2>\n")
	if len(compiled) == 0 {
		b.WriteString("<p>Nothing compiled yet.</p>\n")
	} else {
		b.WriteString("<table class=\"grid\">\n")
		b.WriteString("<tr><th>seq</th><th>type</th><th>name</th></tr>\n")
		for _, c := range compiled {
			b.WriteString(fmt.Sprintf("<tr><td>%d</td><td>%s</td><td>%s</td></tr>\n",
				c.Seq, esc(c.Type), esc(c.Name)))
		}
		b.WriteString("</table>\n")
	}
	b.WriteString("</section>\n")

	// Seeds grown
	b.WriteString("<section id=\"seeds\">\n")
	b.WriteString("<h2>seeds I've grown</h2>\n")
	if len(seeds) == 0 {
		b.WriteString("<p>None yet.</p>\n")
	}
	for _, s := range seeds {
		b.WriteString(fmt.Sprintf("<article class=\"seed\" data-name=%q data-seq=%q>\n", s.Name, fmt.Sprintf("%d", s.Seq)))
		b.WriteString(fmt.Sprintf("<h3>%s</h3>\n", esc(s.Name)))
		if len(s.Commands) > 0 {
			b.WriteString(fmt.Sprintf("<p>commands: %s</p>\n", esc(strings.Join(s.Commands, ", "))))
		}
		if len(s.Projectors) > 0 {
			b.WriteString(fmt.Sprintf("<p>projections: %s</p>\n", esc(strings.Join(s.Projectors, ", "))))
		}
		b.WriteString(fmt.Sprintf("<p>grown at seq %d</p>\n", s.Seq))
		b.WriteString("</article>\n")
	}
	b.WriteString("</section>\n")

	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// CompileDeclarations scans events for command.declared and projector.declared,
// compiles each via the LLM compiler (a spec → a fresh binary, adapted to the
// receiver's garden), writes the scripts into capabilities/, logs a
// script.compiled receipt, and re-renders kernel.html. Latest wins — a
// re-declaration overwrites the script. Returns the names affected.
//
// This is the strange-loop hook: a command (e.g. chat) can emit declarations
// for new capabilities and the kernel compiles them on the fly. The loop only
// ever carries SPECS — the LLM is always the compiler, so every binary is
// authored for this receiver and adaptation is never skipped. The kernel does
// NOT install code from an event: script.compiled is a kernel-only receipt (see
// the reserve guard in runCommand and grow), never an instruction. Exact-code
// reuse exists only as rollback, and only the kernel performs it (see Restore),
// so the sole way code enters the system is through the compiler — the original,
// finite attack surface. The event log keeps every receipt for audit and
// rollback; capabilities/ holds only the latest.
func CompileDeclarations(home string, events []event.Event) (commands, projectors []string, err error) {
	compiler := seed.NewCompiler(home)
	capDir := filepath.Join(home, "capabilities")

	type compiled struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		Script string `json:"script"`
	}
	var compiledScripts []compiled

	for _, e := range events {
		switch e.Name {
		case event.CommandDeclared:
			var cmd seed.Command
			if err := json.Unmarshal(e.Payload, &cmd); err != nil {
				return nil, nil, fmt.Errorf("parse command.declared: %w", err)
			}
			fmt.Printf("compiling command %q...", cmd.Name)
			script, cErr := compiler.CompileCommand(cmd)
			if cErr != nil {
				fmt.Printf(" failed\n")
				return nil, nil, fmt.Errorf("command %q: %w", cmd.Name, cErr)
			}
			if wErr := seed.WriteCommandScript(capDir, cmd.Name, script); wErr != nil {
				return nil, nil, wErr
			}
			fmt.Printf(" compiled\n")
			commands = append(commands, cmd.Name)
			compiledScripts = append(compiledScripts, compiled{"command", cmd.Name, script})

		case event.ProjectorDeclared:
			var proj seed.ProjectorDecl
			if err := json.Unmarshal(e.Payload, &proj); err != nil {
				return nil, nil, fmt.Errorf("parse projector.declared: %w", err)
			}
			fmt.Printf("compiling projector %q...", proj.Name)
			script, cErr := compiler.CompileProjector(proj)
			if cErr != nil {
				fmt.Printf(" failed\n")
				return nil, nil, fmt.Errorf("projector %q: %w", proj.Name, cErr)
			}
			if wErr := seed.WriteProjectorScript(capDir, proj.Name, script); wErr != nil {
				return nil, nil, wErr
			}
			fmt.Printf(" compiled\n")
			projectors = append(projectors, proj.Name)
			compiledScripts = append(compiledScripts, compiled{"projector", proj.Name, script})
		}
	}

	if len(compiledScripts) > 0 {
		st := store.Open(home)
		for _, cs := range compiledScripts {
			payload, _ := json.Marshal(cs)
			e := event.New(event.ScriptCompiled, payload)
			if aErr := st.Append(&e); aErr != nil {
				return commands, projectors, fmt.Errorf("append script.compiled: %w", aErr)
			}
		}
	}

	if len(commands) > 0 || len(projectors) > 0 {
		if rErr := RenderHTML(home); rErr != nil {
			return commands, projectors, fmt.Errorf("re-render kernel.html: %w", rErr)
		}
	}
	return commands, projectors, nil
}

// installScript writes an exact script from a script.compiled payload into
// capabilities/, verbatim. It is kernel-internal — used only by Restore, never
// from an event a command or seed emitted, so the only bytes it ever writes are
// ones the kernel itself compiled and logged earlier. Returns the kind
// ("command" or "projector") and name; an empty kind means there was nothing to
// install (no script or no name).
func installScript(capDir string, payload json.RawMessage) (kind, name string, err error) {
	var cs struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		Script string `json:"script"`
	}
	if uErr := json.Unmarshal(payload, &cs); uErr != nil {
		return "", "", fmt.Errorf("parse script.compiled: %w", uErr)
	}
	if cs.Name == "" || cs.Script == "" {
		return "", "", nil
	}
	if strings.ContainsAny(cs.Name, `/\`) || strings.Contains(cs.Name, "..") {
		return "", "", fmt.Errorf("script.compiled: unsafe name %q", cs.Name)
	}
	switch cs.Type {
	case "command":
		return "command", cs.Name, seed.WriteCommandScript(capDir, cs.Name, cs.Script)
	case "projector":
		return "projector", cs.Name, seed.WriteProjectorScript(capDir, cs.Name, cs.Script)
	default:
		return "", "", fmt.Errorf("script.compiled: unknown type %q", cs.Type)
	}
}

// Restore re-installs a capability from an older script.compiled receipt in the
// log — the kernel's audit-faithful rollback. With targetSeq == 0 it rolls back
// one version (the most recent receipt for name *before* the current one); with
// targetSeq > 0 it restores the receipt at exactly that seq. The bytes are
// re-installed into capabilities/ and a fresh script.compiled receipt is logged
// so the log stays the source of truth and the restore is itself rollback-able.
//
// This is the ONLY path that reinstalls exact code, and only the kernel walks
// it. Because script.compiled is kernel-only (commands/seeds may not emit it),
// every receipt it considers was authored by the compiler for THIS receiver —
// so a restore can never introduce foreign code or skip adaptation. It adds no
// attack surface beyond the compiler itself: the bytes already ran here once.
func Restore(home, name string, targetSeq int) (restoredSeq int, kind string, err error) {
	events, err := openEventStore(home).Read()
	if err != nil {
		return 0, "", err
	}
	type receipt struct {
		seq     int
		payload json.RawMessage
		typ     string
	}
	var receipts []receipt
	for _, e := range events {
		if e.Name != event.ScriptCompiled {
			continue
		}
		var cs struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if json.Unmarshal(e.Payload, &cs) != nil || cs.Name != name {
			continue
		}
		receipts = append(receipts, receipt{e.Seq, e.Payload, cs.Type})
	}
	if len(receipts) == 0 {
		return 0, "", fmt.Errorf("no compiled history for %q — nothing to restore (see 'self ls')", name)
	}

	var chosen receipt
	if targetSeq > 0 {
		found := false
		for _, r := range receipts {
			if r.seq == targetSeq {
				chosen, found = r, true
				break
			}
		}
		if !found {
			return 0, "", fmt.Errorf("no script.compiled for %q at seq %d (run 'self history' to find one)", name, targetSeq)
		}
	} else {
		if len(receipts) < 2 {
			return 0, "", fmt.Errorf("%q has only one compiled version (seq %d) — nothing to roll back to; pass a seq to restore it explicitly", name, receipts[0].seq)
		}
		chosen = receipts[len(receipts)-2] // the version before the current one
	}

	capDir := filepath.Join(home, "capabilities")
	k, _, iErr := installScript(capDir, chosen.payload)
	if iErr != nil {
		return 0, "", iErr
	}
	// Re-log the restored bytes as a fresh kernel receipt so the log remains the
	// source of truth (latest receipt == what's installed).
	e := event.New(event.ScriptCompiled, chosen.payload)
	if aErr := store.Open(home).Append(&e); aErr != nil {
		return 0, "", fmt.Errorf("append restore receipt: %w", aErr)
	}
	if rErr := RenderHTML(home); rErr != nil {
		return 0, "", fmt.Errorf("re-render kernel.html: %w", rErr)
	}
	return chosen.seq, k, nil
}

// Wiring is the parsed event→projector map extracted from kernel.html.
type Wiring struct {
	// ProjectorsByEvent maps an event name to the projectors that consume it.
	ProjectorsByEvent map[string][]string
}

// ReadWiring parses site/kernel.html and returns the event→projector wiring.
// Returns an empty Wiring (not an error) if kernel.html doesn't exist yet.
func ReadWiring(home string) (*Wiring, error) {
	path := filepath.Join(home, "site", "kernel.html")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Wiring{ProjectorsByEvent: map[string][]string{}}, nil
		}
		return nil, err
	}
	return parseWiring(string(data)), nil
}

// ProjectorsForEvent returns the names of projectors that consume the given event.
func (w *Wiring) ProjectorsForEvent(eventName string) []string {
	return w.ProjectorsByEvent[eventName]
}

var projectorRe = regexp.MustCompile(`(?s)<article class="projector" data-name="([^"]+)">.*?<ul class="consumes">(.*?)</ul>`)
var consumeRe = regexp.MustCompile(`data-event="([^"]+)"`)

func parseWiring(htmlStr string) *Wiring {
	w := &Wiring{ProjectorsByEvent: map[string][]string{}}

	matches := projectorRe.FindAllStringSubmatch(htmlStr, -1)
	for _, m := range matches {
		projName := m[1]
		consumesBlock := m[2]
		consumeMatches := consumeRe.FindAllStringSubmatch(consumesBlock, -1)
		for _, cm := range consumeMatches {
			eventName := cm[1]
			w.ProjectorsByEvent[eventName] = append(w.ProjectorsByEvent[eventName], projName)
		}
	}

	return w
}

// eventStore is a minimal reader for the event log, avoiding import cycle
// with the store package. We only need Read.
type eventStore struct {
	path string
}

func openEventStore(home string) *eventStore {
	return &eventStore{path: filepath.Join(home, "events.jsonl")}
}

func (s *eventStore) Read() ([]event.Event, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var events []event.Event
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e event.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse event line: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}
