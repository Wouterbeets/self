package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	htmlesc "html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ks/internal/event"
	"ks/internal/kernel"
	"ks/internal/seed"
	"ks/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	home := homeDir()
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = cmdInit(home)
	case "plant":
		if len(args) < 1 {
			err = fmt.Errorf("usage: ks plant <seed-dir>")
		} else {
			err = cmdPlant(home, args[0])
		}
	case "invoke":
		if len(args) < 1 {
			err = fmt.Errorf("usage: ks invoke <command> [args...]")
		} else {
			err = cmdInvoke(home, args[0], args[1:])
		}
	case "project":
		projName := ""
		if len(args) >= 1 {
			projName = args[0]
		}
		err = cmdProject(home, projName)
	case "log":
		err = cmdLog(home)
	case "seeds":
		err = cmdSeeds(home)
	case "think":
		prompt := strings.Join(args, " ")
		err = cmdThink(home, prompt)
	case "serve":
		port := ""
		if len(args) >= 1 {
			port = args[0]
		}
		err = cmdServe(home, port)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "ks: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "ks: %s\n", err)
		os.Exit(1)
	}
}

func homeDir() string {
	if v := os.Getenv("KS_HOME"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".ks")
}

func usage() {
	fmt.Fprint(os.Stderr, `ks — knowledge seed protocol kernel

usage: ks <command> [args]

commands:
  init                    create a fresh ks home (appends kernel.initialized)
  plant <seed-dir>        compile declared commands/projectors, replay seed's events
  invoke <command> [args] run a command, append events, auto-run affected projectors
  project [projector]     replay events through a projector, emit HTML to site/
  log                     show the event log
  seeds                   list planted seeds (from seed.planted events)
  think <prompt>          call the kernel's brain (LLM + garden exploration)
  serve [port]            serve KS_HOME/site over HTTP (see routes below)

http routes (ks serve, default port 7777):
  /                       kernel.html — wiring + projections
  /live/<projector>       re-run projector against current events
  /events                 raw events.jsonl

on disk:
  KS_HOME/site/           materialized HTML projections (current state, written by ks project)

environment:
  KS_HOME        ks home directory (default ~/.ks)
  KS_LLM_URL     llm api base url (overrides the opencode-go default)
  KS_LLM_API_KEY llm api key (not needed for local llama-server)
  KS_LLM_MODEL   llm model name (overrides the opencode-go default)
  KS_LLM_STUB    set to "1" to force stub scripts

By default, ks uses the opencode-go subscription (read from
~/.local/share/opencode/auth.json, endpoint https://opencode.ai/zen/go,
model glm-5.2). If a request hits a quota-exceeded / rate-limit error,
ks falls back to a local llama-server on port 8080 for that call.
Override the primary with KS_LLM_* env vars for another endpoint.
Set KS_LLM_STUB=1 to force stub scripts without calling the LLM.

kernel-known events:
  kernel.initialized   written by ks init
  command.declared     compiled by ks plant AND ks invoke (self-improvement)
  projector.declared   compiled by ks plant AND ks invoke (self-improvement)
  script.compiled      logged by the kernel for every compile; ALSO acted on —
                       a seed or command may emit one to install an exact script
                       verbatim (no LLM), so the loop carries code, not just a
                       spec. Re-emitting an older one from the log is rollback.
  seed.planted         written by ks plant as a receipt
  everything else      comes from seeds or from commands that emit declarations

at compile time and via 'ks think', the LLM gets a read-only bash tool
(cwd=KS_HOME) to explore the garden — existing commands, projectors,
events, wiring, projections — and adapt to the receiver's current state.
commands that need LLM access call 'ks think' instead of making their own
HTTP calls — the kernel is the sole steward of LLM credentials.
`)
}

func cmdInit(home string) error {
	if err := os.MkdirAll(filepath.Join(home, "registry", "commands"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(home, "registry", "projectors"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(home, "site"), 0755); err != nil {
		return err
	}
	st := store.Open(home)
	payload, _ := json.Marshal(map[string]string{
		"version": "ks/v0",
	})
	e := event.New(event.KernelInitialized, payload)
	if err := st.Append(&e); err != nil {
		return err
	}

	fmt.Printf("initialized ks home at %s (seq %d %s)\n", home, e.Seq, e.Name)
	return kernel.RenderHTML(home)
}

func cmdPlant(home string, seedDir string) error {
	manifest, err := seed.Load(seedDir)
	if err != nil {
		return err
	}

	compiler := seed.NewCompiler(home)
	registry := filepath.Join(home, "registry")

	type compiled struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		Script string `json:"script"`
	}
	var compiledScripts []compiled

	// A seed may ship a capability as exact code (a script.compiled event)
	// instead of, or alongside, its declaration. When it does, the declaration
	// still serves as the spec — it puts the capability into kernel.html's
	// wiring/identity — but there's no point asking the LLM to re-derive a
	// binary the seed already carries. Pre-scan the shipped scripts so the
	// compile loops below skip those names; the replay loop installs them.
	shipped := map[string]bool{}
	for _, e := range manifest.Events {
		if e.Name != event.ScriptCompiled {
			continue
		}
		var cs struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if json.Unmarshal(e.Payload, &cs) == nil && cs.Name != "" {
			shipped[cs.Type+"/"+cs.Name] = true
		}
	}

	for _, cmd := range manifest.Commands {
		if shipped["command/"+cmd.Name] {
			fmt.Printf("command %q shipped verbatim — skipping compile\n", cmd.Name)
			continue
		}
		fmt.Printf("compiling command %q...", cmd.Name)
		script, err := compiler.CompileCommand(cmd)
		if err != nil {
			fmt.Printf(" failed\n")
			return fmt.Errorf("command %q: %w", cmd.Name, err)
		}
		if err := seed.WriteCommandScript(registry, cmd.Name, script); err != nil {
			return err
		}
		fmt.Printf(" planted\n")
		compiledScripts = append(compiledScripts, compiled{"command", cmd.Name, script})
	}

	for _, proj := range manifest.Projectors {
		if shipped["projector/"+proj.Name] {
			fmt.Printf("projector %q shipped verbatim — skipping compile\n", proj.Name)
			continue
		}
		fmt.Printf("compiling projector %q...", proj.Name)
		script, err := compiler.CompileProjector(proj)
		if err != nil {
			fmt.Printf(" failed\n")
			return fmt.Errorf("projector %q: %w", proj.Name, err)
		}
		if err := seed.WriteProjectorScript(registry, proj.Name, script); err != nil {
			return err
		}
		fmt.Printf(" planted\n")
		compiledScripts = append(compiledScripts, compiled{"projector", proj.Name, script})
	}

	st := store.Open(home)
	contentCount := 0
	for i := range manifest.Events {
		e := manifest.Events[i]
		isDeclaration := e.Name == event.CommandDeclared || e.Name == event.ProjectorDeclared
		fresh := event.New(e.Name, e.Payload)
		if err := st.Append(&fresh); err != nil {
			return err
		}
		// A seed can ship exact code: a script.compiled event is installed
		// verbatim (no LLM), so a seed isn't limited to specs the compiler must
		// re-derive — it can carry a known-good binary. If the seed also
		// declared the same name above, this overwrites that fresh compile with
		// the shipped script. It's already appended (provenance), so don't
		// re-log it; it's an install, not replayed content.
		if e.Name == event.ScriptCompiled {
			kind, name, iErr := kernel.InstallScript(registry, e.Payload)
			if iErr != nil {
				return iErr
			}
			if kind != "" {
				fmt.Printf("installing %s %q verbatim... done\n", kind, name)
			}
			continue
		}
		if !isDeclaration {
			contentCount++
		}
	}

	for _, cs := range compiledScripts {
		payload, _ := json.Marshal(cs)
		e := event.New(event.ScriptCompiled, payload)
		if err := st.Append(&e); err != nil {
			return err
		}
	}

	receiptPayload, _ := json.Marshal(map[string]any{
		"seed":            manifest.Name,
		"commands":        commandNames(manifest.Commands),
		"projectors":      projectorNames(manifest.Projectors),
		"events_replayed": contentCount,
	})
	receipt := event.New(event.SeedPlanted, receiptPayload)
	if err := st.Append(&receipt); err != nil {
		return err
	}

	fmt.Printf("seed %q planted: %d command(s), %d projector(s), %d event(s) replayed, receipt seq %d\n",
		manifest.Name, len(manifest.Commands), len(manifest.Projectors), contentCount, receipt.Seq)
	return kernel.RenderHTML(home)
}

// invokeCommand runs a command end-to-end: executes the script, appends the
// events it emits, compiles any declarations it produced (the strange loop),
// and auto-runs the projectors that consume those events. Returns the events
// produced. Progress is logged to stderr; callers format their own output.
// Shared by the CLI (cmdInvoke), the HTTP /invoke route, and the brain's
// command tools — one invoke pipeline, three callers.
func invokeCommand(home string, command string, args []string) ([]event.Event, error) {
	scriptPath := filepath.Join(home, "registry", "commands", command)
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("command %q not found (plant a seed that declares it)", command)
	}

	st := store.Open(home)
	current, err := st.Read()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(scriptPath, args...)
	cmd.Env = append(os.Environ(), "KS_HOME="+home)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	feedEvents(stdin, current)

	var newEvents []event.Event
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var partial struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &partial); err != nil {
			return nil, fmt.Errorf("command output parse error: %w", err)
		}
		if partial.Name == "" {
			return nil, fmt.Errorf("command output missing event name: %s", line)
		}
		newEvents = append(newEvents, event.New(partial.Name, partial.Payload))
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("command exited: %w", err)
	}

	for i := range newEvents {
		if err := st.Append(&newEvents[i]); err != nil {
			return nil, err
		}
	}

	// Strange-loop hook: compile any command.declared / projector.declared the
	// command emitted, so a command can plant new capabilities at invoke time.
	compiledCmds, compiledProjs, cErr := kernel.CompileDeclarations(home, newEvents)
	if cErr != nil {
		fmt.Fprintf(os.Stderr, "ks: warning: declaration compile failed: %s\n", cErr)
	}
	if len(compiledCmds) > 0 || len(compiledProjs) > 0 {
		fmt.Fprintf(os.Stderr, "ks: self-improved: %d command(s), %d projector(s) compiled\n",
			len(compiledCmds), len(compiledProjs))
	}

	// Auto-run projectors that consume the new events. The kernel reads its own
	// projection (site/kernel.html) to know which projectors care about which
	// events — burn kernel.html, replay events, it comes back.
	wiring, wErr := kernel.ReadWiring(home)
	if wErr != nil {
		fmt.Fprintf(os.Stderr, "ks: warning: could not read kernel wiring: %s\n", wErr)
		return newEvents, nil
	}
	ran := map[string]bool{}
	for _, e := range newEvents {
		for _, projName := range wiring.ProjectorsForEvent(e.Name) {
			if ran[projName] {
				continue
			}
			ran[projName] = true
			fmt.Fprintf(os.Stderr, "ks: auto-running projector %q\n", projName)
			if pErr := runProjectorToSite(home, projName); pErr != nil {
				fmt.Fprintf(os.Stderr, "ks: projector %q failed: %s\n", projName, pErr)
			}
		}
	}

	return newEvents, nil
}

// cmdInvoke is the CLI wrapper around invokeCommand: it prints each appended
// event to stdout.
func cmdInvoke(home string, command string, args []string) error {
	newEvents, err := invokeCommand(home, command, args)
	if err != nil {
		return err
	}
	for _, e := range newEvents {
		fmt.Printf("%s appended seq %d %s\n", e.ID, e.Seq, e.Name)
	}
	return nil
}

func cmdProject(home string, name string) error {
	if name == "" {
		entries, err := os.ReadDir(filepath.Join(home, "registry", "projectors"))
		if err != nil {
			return fmt.Errorf("no projectors planted")
		}
		if len(entries) == 0 {
			return fmt.Errorf("no projectors planted")
		}
		name = entries[0].Name()
	}

	// runProjector always persists to site/<name>.html and also writes
	// to stdout when showStdout is true.
	return runProjector(home, name, true)
}

// runProjectorToSite runs a projector and persists its HTML to
// KS_HOME/site/<name>.html. Used by the auto-run mechanism after invoke.
func runProjectorToSite(home string, name string) error {
	return runProjector(home, name, false)
}

// runProjector runs a projector script, piping all events as JSONL to stdin.
// It always persists HTML to KS_HOME/site/<name>.html. If showStdout is true,
// it also writes HTML to os.Stdout.
func runProjector(home string, name string, showStdout bool) error {
	scriptPath := filepath.Join(home, "registry", "projectors", name)
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("projector %q not found", name)
	}

	st := store.Open(home)
	events, err := st.Read()
	if err != nil {
		return err
	}

	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	siteFile, err := os.Create(filepath.Join(siteDir, name+".html"))
	if err != nil {
		return err
	}
	defer siteFile.Close()

	cmd := exec.Command(scriptPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if showStdout {
		cmd.Stdout = io.MultiWriter(os.Stdout, siteFile)
	} else {
		cmd.Stdout = siteFile
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	feedEvents(stdin, events)

	return cmd.Wait()
}

// feedEvents writes events as JSONL to a child process's stdin in a goroutine,
// closing stdin when done. Shared by the command and projector pipelines.
func feedEvents(stdin io.WriteCloser, events []event.Event) {
	go func() {
		w := bufio.NewWriter(stdin)
		for _, e := range events {
			line, _ := json.Marshal(e)
			w.Write(line)
			w.WriteByte('\n')
		}
		w.Flush()
		stdin.Close()
	}()
}

// projectHTML runs a projector against the current event log and returns its
// HTML output, without persisting to site/. Used by ks serve to render fresh
// projections on every request.
func projectHTML(home, name string) ([]byte, error) {
	scriptPath := filepath.Join(home, "registry", "projectors", name)
	events, err := store.Open(home).Read()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(scriptPath)
	cmd.Env = append(os.Environ(), "KS_HOME="+home)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	feedEvents(stdin, events)
	return cmd.Output()
}

// autoReloadSnippet polls /version and reloads the page when the event log
// grows, so served projections update hands-free.
const autoReloadSnippet = `<script>(function(){var c=null;function p(){fetch('/version').then(function(r){return r.text()}).then(function(v){if(c===null){c=v}else if(v!==c){location.reload()}}).catch(function(){});setTimeout(p,1000)}p()})();</script>`

// injectAutoReload inserts the auto-reload script before </body> (or appends it
// if there's no body tag), so any served HTML page live-updates.
func injectAutoReload(html []byte) []byte {
	s := string(html)
	if i := strings.LastIndex(s, "</body>"); i >= 0 {
		return []byte(s[:i] + autoReloadSnippet + s[i:])
	}
	return append(html, []byte(autoReloadSnippet)...)
}

// enrichmentCSS is the kernel's one shared stylesheet — the "enrichment layer".
// Projectors emit bare semantic HTML (no styling); the kernel injects this so
// every projection is themed consistently. Classless-first (elements styled
// directly) plus a tiny stable class vocabulary: muted, card, row, stack, tag
// (+accent), msg (+who), num, button.secondary/.danger. Themes = override :root.
const enrichmentCSS = `<style>
:root{--bg:#fbfbfa;--fg:#1f2328;--muted:#6a737d;--border:#e2e4e8;--accent:#2563eb;--accent-fg:#fff;--card:#fff;--danger:#b42318;--radius:6px;--gap:16px;--maxw:880px}
@media (prefers-color-scheme:dark){:root{--bg:#0e1116;--fg:#e6edf3;--muted:#8b949e;--border:#30363d;--accent:#4493f8;--card:#161b22}}
*{box-sizing:border-box}
body{background:var(--bg);color:var(--fg);max-width:var(--maxw);margin:0 auto;padding:28px 24px 64px;line-height:1.55;font-size:16px;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
h1,h2,h3{line-height:1.25;margin:1.5em 0 .5em;font-weight:650}h1{font-size:1.7rem;margin-top:0}h2{font-size:1.25rem}
p{margin:.6em 0}a{color:var(--accent);text-decoration:none}a:hover{text-decoration:underline}
hr{border:0;border-top:1px solid var(--border);margin:28px 0}
nav{display:flex;gap:18px;align-items:baseline;padding-bottom:14px;border-bottom:1px solid var(--border)}
nav .brand{font-weight:700;margin-right:auto;font-family:ui-monospace,Menlo,monospace}
code{font-family:ui-monospace,Menlo,monospace;font-size:.88em;background:var(--card);border:1px solid var(--border);border-radius:4px;padding:1px 5px}
table{border-collapse:collapse;width:100%;margin:12px 0}th,td{text-align:left;padding:8px 12px;border-bottom:1px solid var(--border)}
th{font-size:.74rem;text-transform:uppercase;letter-spacing:.04em;color:var(--muted)}
tbody tr:hover{background:color-mix(in srgb,var(--accent) 7%,transparent)}
td.num,th.num{text-align:right;font-variant-numeric:tabular-nums}tfoot td{font-weight:700;border-top:2px solid var(--fg)}
button,input,select,textarea{font:inherit;color:inherit}
input,textarea,select{background:var(--bg);border:1px solid var(--border);border-radius:var(--radius);padding:8px 11px;width:100%}
input:focus,textarea:focus{outline:2px solid var(--accent);border-color:var(--accent)}
button{background:var(--accent);color:var(--accent-fg);border:1px solid var(--accent);border-radius:var(--radius);padding:8px 16px;cursor:pointer;font-weight:600}
button:hover{filter:brightness(1.08)}button.secondary{background:transparent;color:var(--accent)}button.danger{background:var(--danger);border-color:var(--danger)}
form{display:flex;flex-direction:column;gap:10px;max-width:520px}
.muted{color:var(--muted)}
.card{background:var(--card);border:1px solid var(--border);border-radius:var(--radius);padding:14px 18px;margin:14px 0}
.row{display:flex;gap:var(--gap);align-items:center;flex-wrap:wrap}.stack{display:flex;flex-direction:column;gap:10px}
.tag{display:inline-block;font-size:.72rem;padding:2px 8px;border-radius:999px;border:1px solid var(--border);color:var(--muted)}
.tag.accent{color:var(--accent);border-color:var(--accent)}
.msg{padding:8px 0;border-bottom:1px solid var(--border)}.msg:last-child{border-bottom:0}
.msg .who{font-size:.72rem;color:var(--muted);text-transform:uppercase;letter-spacing:.04em}
</style>`

// navCSS styles the projection sidebar. Injected after the page's own styles
// (so its body offset wins), with fallback colors for pages that don't define
// the enrichment variables (e.g. kernel.html). Collapses to a top bar on narrow
// screens.
const navCSS = `<style>
.ks-nav{position:fixed;left:0;top:0;bottom:0;width:180px;overflow:auto;box-sizing:border-box;
  border-right:1px solid var(--border,#e2e4e8);padding:20px 14px;background:var(--card,#fff);font-size:14px}
.ks-nav a{display:block;padding:5px 8px;border-radius:5px;color:var(--fg,#1f2328);text-decoration:none;
  white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.ks-nav a:hover{background:color-mix(in srgb,var(--accent,#2563eb) 12%,transparent)}
.ks-nav .ks-brand{font-weight:700;font-family:ui-monospace,Menlo,monospace;font-size:1.1rem;margin-bottom:6px}
.ks-nav .ks-label{font-size:11px;text-transform:uppercase;letter-spacing:.04em;opacity:.55;margin:14px 8px 4px}
body{padding-left:210px}
@media (max-width:640px){.ks-nav{position:static;width:auto;height:auto;border-right:0;border-bottom:1px solid var(--border,#e2e4e8)}body{padding-left:0}}
</style>`

// navHTML builds the projection sidebar from the planted projectors. Flat for
// now (nested projections aren't modelled yet — they'd need a recursive walk).
func navHTML(home string) string {
	var b strings.Builder
	b.WriteString(`<aside class="ks-nav"><a class="ks-brand" href="/">ks</a>`)
	b.WriteString(`<div class="ks-label">projections</div>`)
	entries, _ := os.ReadDir(filepath.Join(home, "registry", "projectors"))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		b.WriteString(fmt.Sprintf(`<a href="/%s">%s</a>`, n, htmlesc.EscapeString(n)))
	}
	b.WriteString(`<div class="ks-label">kernel</div><a href="/">wiring &amp; identity</a></aside>`)
	return b.String()
}

// injectNav adds the projection sidebar (and its styles) to a served page —
// universal chrome the kernel provides so projectors stay bare. Used for both
// projector pages and kernel.html.
func injectNav(home string, page []byte) []byte {
	s := string(page)
	if i := strings.Index(s, "</head>"); i >= 0 {
		s = s[:i] + navCSS + s[i:]
	} else {
		s = navCSS + s
	}
	aside := navHTML(home)
	if i := strings.Index(s, "<body"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j >= 0 {
			pos := i + j + 1
			return []byte(s[:pos] + aside + s[pos:])
		}
	}
	return []byte(aside + s)
}

// enrich injects the shared stylesheet into <head>, the projection sidebar, and
// the auto-reload script — so bare semantic projector output renders styled,
// navigable, and live without the projector carrying any of it.
func enrich(home string, page []byte) []byte {
	s := string(page)
	if i := strings.Index(s, "</head>"); i >= 0 {
		s = s[:i] + enrichmentCSS + s[i:]
	} else {
		s = enrichmentCSS + s
	}
	return injectNav(home, injectAutoReload([]byte(s)))
}

func cmdServe(home string, port string) error {
	if port == "" {
		port = "7777"
	}

	os.MkdirAll(filepath.Join(home, "site"), 0755)

	// Rebuild kernel.html
	if err := kernel.RenderHTML(home); err != nil {
		fmt.Fprintf(os.Stderr, "ks: warning: could not rebuild kernel.html: %s\n", err)
	}

	// Rebuild all projectors from the registry
	projDir := filepath.Join(home, "registry", "projectors")
	entries, err := os.ReadDir(projDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				fmt.Fprintf(os.Stderr, "rebuilding projector %q...\n", e.Name())
				if rebuildErr := runProjectorToSite(home, e.Name()); rebuildErr != nil {
					fmt.Fprintf(os.Stderr, "ks: warning: projector %q failed: %s\n", e.Name(), rebuildErr)
				}
			}
		}
	}

	mux := http.NewServeMux()

	// / serves a live view. "/" and "/kernel" re-render the kernel wiring; any
	// path matching a planted projector re-runs it against current events so
	// the page is never stale. Everything else falls back to static site/ files.
	// HTML responses get a small auto-reload script injected: the browser polls
	// /version and reloads when the event log grows, so projections update hands-
	// free as new events land.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.Trim(r.URL.Path, "/"), ".html")

		if name == "" || name == "kernel" {
			if err := kernel.RenderHTML(home); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			data, err := os.ReadFile(filepath.Join(home, "site", "kernel.html"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(injectNav(home, injectAutoReload(data)))
			return
		}

		if _, err := os.Stat(filepath.Join(home, "registry", "projectors", name)); err == nil {
			html, err := projectHTML(home, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(enrich(home, html))
			return
		}

		http.FileServer(http.Dir(filepath.Join(home, "site"))).ServeHTTP(w, r)
	})

	// /version — a cheap change token: the byte size of the append-only event
	// log. Stat is O(1) and catches appends from any writer (including other
	// processes), where reading + parsing the whole log for the last seq is
	// O(n) on every 1s poll. The injected auto-reload script polls this.
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		var size int64
		if fi, err := os.Stat(filepath.Join(home, "events.jsonl")); err == nil {
			size = fi.Size()
		}
		fmt.Fprintf(w, "%d", size)
	})

	// /live/<projector> — re-run projector against current events.jsonl.
	mux.HandleFunc("/live/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/live/")
		if name == "" {
			http.Error(w, "projector name required (e.g. /live/note)", http.StatusBadRequest)
			return
		}
		scriptPath := filepath.Join(home, "registry", "projectors", name)
		if _, err := os.Stat(scriptPath); err != nil {
			http.Error(w, "projector "+name+" not found", http.StatusNotFound)
			return
		}
		st := store.Open(home)
		events, err := st.Read()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c := exec.Command(scriptPath)
		stdin, err := c.StdinPipe()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		c.Stdout = w
		c.Stderr = os.Stderr
		if err := c.Start(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		feedEvents(stdin, events)
		c.Wait()
	})

	// /events — raw events.jsonl.
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(home, "events.jsonl"))
	})

	// POST /invoke/<command> — run a command from the browser. The raw request
	// body is passed as the command's single argument. This makes projections a
	// read+write surface: a projector can emit a form that POSTs here, the
	// command runs through the normal pipeline (append events, strange-loop
	// compile, auto-run projectors), and the auto-reload script then refreshes
	// the page to show the new events.
	mux.HandleFunc("/invoke/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		command := strings.TrimPrefix(r.URL.Path, "/invoke/")
		if command == "" {
			http.Error(w, "command required (e.g. /invoke/chat)", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		// A plain HTML <form> posts application/x-www-form-urlencoded: each
		// field's value becomes a positional command argument, in submission
		// order (which the projector author controls). Any other body (e.g. a
		// raw fetch) is passed as a single argument. Field names are for humans;
		// position is the contract.
		var args []string
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			for _, pair := range strings.Split(string(body), "&") {
				if pair == "" {
					continue
				}
				_, v, _ := strings.Cut(pair, "=")
				v = strings.ReplaceAll(v, "+", " ")
				if dec, err := url.QueryUnescape(v); err == nil {
					v = dec
				}
				args = append(args, v)
			}
		} else if msg := strings.TrimSpace(string(body)); msg != "" {
			args = []string{msg}
		}
		if err := cmdInvoke(home, command, args); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Post/Redirect/Get: send a browser form back to the page it came from
		// so a full reload shows the new state. Zero JavaScript required.
		if ref := r.Header.Get("Referer"); ref != "" {
			http.Redirect(w, r, ref, http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "ok")
	})

	addr := ":" + port
	fmt.Fprintf(os.Stderr, "ks serving %s on http://localhost:%s\n", filepath.Join(home, "site"), port)
	fmt.Fprintf(os.Stderr, "  /              kernel wiring + projections (kernel.html)\n")
	fmt.Fprintf(os.Stderr, "  /live/<name>   re-run projector against current events\n")
	fmt.Fprintf(os.Stderr, "  /events        raw events.jsonl\n")
	return http.ListenAndServe(addr, mux)
}

func cmdLog(home string) error {
	st := store.Open(home)
	events, err := st.Read()
	if err != nil {
		return err
	}
	for _, e := range events {
		fmt.Printf("%d %s %s\n", e.Seq, e.ID, e.Name)
	}
	return nil
}

func cmdSeeds(home string) error {
	st := store.Open(home)
	events, err := st.Read()
	if err != nil {
		return fmt.Errorf("no kernel home (run ks init first)")
	}

	found := false
	for _, e := range events {
		if e.Name != event.SeedPlanted {
			continue
		}
		var rec struct {
			Seed           string   `json:"seed"`
			Commands       []string `json:"commands"`
			Projectors     []string `json:"projectors"`
			EventsReplayed int      `json:"events_replayed"`
		}
		json.Unmarshal(e.Payload, &rec)
		parts := []string{}
		if len(rec.Commands) > 0 {
			parts = append(parts, "commands: "+strings.Join(rec.Commands, ", "))
		}
		if len(rec.Projectors) > 0 {
			parts = append(parts, "projectors: "+strings.Join(rec.Projectors, ", "))
		}
		parts = append(parts, fmt.Sprintf("events replayed: %d", rec.EventsReplayed))
		fmt.Printf("%s — %s (seq %d)\n", rec.Seed, strings.Join(parts, ", "), e.Seq)
		found = true
	}
	if !found {
		fmt.Println("(no seeds planted)")
	}
	return nil
}

func cmdThink(home string, prompt string) error {
	if prompt == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		return fmt.Errorf("usage: ks think <prompt> (or pipe prompt on stdin)")
	}

	compiler := seed.NewCompiler(home)

	// The brain can act by calling planted commands as tools. Guard against
	// recursion (a brain-invoked command may itself call `ks think`): bound the
	// depth via KS_THINK_DEPTH. Past the cap, the brain may still talk and
	// declare but gets no command tools, so it cannot act and cannot recurse.
	depth := 0
	fmt.Sscanf(os.Getenv("KS_THINK_DEPTH"), "%d", &depth)

	var commands []seed.Command
	var invoke seed.CommandInvoker
	if depth < 3 {
		commands = plantedCommands(home)
		os.Setenv("KS_THINK_DEPTH", fmt.Sprintf("%d", depth+1))
		invoke = func(name, args string) (string, error) {
			var argv []string
			if strings.TrimSpace(args) != "" {
				argv = []string{args}
			}
			evs, err := invokeCommand(home, name, argv)
			if err != nil {
				return "", err
			}
			if len(evs) == 0 {
				return fmt.Sprintf("ran %q (no events emitted)", name), nil
			}
			names := make([]string, len(evs))
			for i, e := range evs {
				names[i] = e.Name
			}
			return fmt.Sprintf("ran %q — appended %d event(s): %s", name, len(evs), strings.Join(names, ", ")), nil
		}
	}

	result, err := compiler.CallBrain(prompt, commands, invoke)
	if err != nil {
		return err
	}

	output := map[string]any{
		"response":     result.Response,
		"declarations": result.Declarations,
	}
	if output["declarations"] == nil {
		output["declarations"] = []any{}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// plantedCommands reads the command declarations from the event log, latest
// wins, in first-seen order. The chat command is excluded — the brain calling
// chat would just re-enter itself.
func plantedCommands(home string) []seed.Command {
	events, err := store.Open(home).Read()
	if err != nil {
		return nil
	}
	byName := map[string]seed.Command{}
	var order []string
	for _, e := range events {
		if e.Name != event.CommandDeclared {
			continue
		}
		var cmd seed.Command
		if json.Unmarshal(e.Payload, &cmd) != nil || cmd.Name == "" {
			continue
		}
		if _, seen := byName[cmd.Name]; !seen {
			order = append(order, cmd.Name)
		}
		byName[cmd.Name] = cmd
	}
	var out []seed.Command
	for _, n := range order {
		if n == "chat" {
			continue
		}
		out = append(out, byName[n])
	}
	return out
}

func commandNames(commands []seed.Command) []string {
	names := make([]string, len(commands))
	for i, c := range commands {
		names[i] = c.Name
	}
	return names
}

func projectorNames(projectors []seed.ProjectorDecl) []string {
	names := make([]string, len(projectors))
	for i, p := range projectors {
		names[i] = p.Name
	}
	return names
}
