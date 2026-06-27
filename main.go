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

	"self/internal/event"
	"self/internal/kernel"
	"self/internal/seed"
	"self/internal/store"
)

func main() {
	home := homeDir()

	// Bare `self` is the most common action: heal the body from the log, then
	// start the live garden. Rehydrating first means the working tree always
	// matches the one truth — clone a home that is just events.jsonl + .secret
	// and `self` brings the whole body back before serving it.
	if len(os.Args) < 2 {
		if _, _, err := rehydrateFromLog(home); err != nil {
			fmt.Fprintf(os.Stderr, "self: rehydrate: %s\n", err)
		}
		if err := cmdServe(home, ""); err != nil {
			fmt.Fprintf(os.Stderr, "self: %s\n", err)
			os.Exit(1)
		}
		return
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = cmdInit(home)
	case "grow":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self grow <seed-dir>")
		} else {
			err = cmdGrow(home, args[0])
		}
	case "run":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self run <command> [args...]")
		} else {
			err = cmdRun(home, args[0], args[1:])
		}
	case "think":
		prompt := strings.Join(args, " ")
		err = cmdThink(home, prompt)
	case "heartbeat":
		err = cmdHeartbeat(home)
	case "rehydrate":
		err = cmdRehydrate(home)
	case "restore":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self restore <name> [seq]")
		} else {
			err = cmdRestore(home, args)
		}
	case "show":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self show <projection>")
		} else {
			err = cmdShow(home, args[0])
		}
	case "live":
		port := ""
		if len(args) >= 1 {
			port = args[0]
		}
		err = cmdServe(home, port)
	case "history":
		err = cmdHistory(home, args)
	case "ls":
		err = cmdLs(home, args)
	case "where":
		err = cmdWhere(home)
	case "which":
		if len(args) < 1 {
			err = fmt.Errorf("usage: self which <name>")
		} else {
			err = cmdWhich(home, args[0])
		}
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "self: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "self: %s\n", err)
		os.Exit(1)
	}
}

func homeDir() string {
	if v := os.Getenv("SELF_HOME"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".self")
}

func usage() {
	fmt.Fprint(os.Stderr, `self — a sovereign, self-improving capability system

One append-only event log + shared projections that you and your agent see
identically. A minimal kernel; everything else grows as seeds through the
strange loop.

usage: self [command] [args]

  self                    rehydrate the body from the log, then start the live garden (the default)
  self init               initialize the baby kernel
  self rehydrate          rebuild capabilities/ + site/ from the log's signed receipts (no LLM)
  self grow <seed>        grow a new capability from a seed
  self run <command> ...  run a capability — append events, refresh projections
  self think "..."        ask the brain (LLM + garden exploration)
  self heartbeat          run one self-improvement cycle (brain reflects & grows)
  self restore <name> [seq]   roll a capability back to an earlier compiled version
  self show <name>        render a projection (piped: HTML to stdout; else open in browser)
  self live [port]        start the live garden explicitly (default port 7777)
  self history [-n N] [--raw]   recent events, newest last
  self ls [commands|projectors|seeds]   list what exists (with paths)
  self where              show SELF_HOME and every important path
  self which <name>       show the full path to a command or projector

live garden routes (self live, default port 7777):
  /                       my identity page — capabilities, paths, wiring
  /<projection>           a projection, re-rendered live
  /live/<projection>      re-run a projection against current events
  /run/<command>          run a capability from the browser (plain HTML forms)
  /events                 the raw event log (events.jsonl)

on disk (all open and inspectable — see 'self where'):
  SELF_HOME/events.jsonl              the only source of truth (append-only)
  SELF_HOME/capabilities/commands/    compiled command scripts
  SELF_HOME/capabilities/projectors/  compiled projector scripts
  SELF_HOME/site/                     materialized HTML projections

environment:
  SELF_HOME        my home directory (default ~/.self)
  SELF_LLM_URL     llm api base url (overrides the opencode-go default)
  SELF_LLM_API_KEY llm api key (not needed for local llama-server)
  SELF_LLM_MODEL   llm model name (overrides the opencode-go default)
  SELF_LLM_STUB    set to "1" to force stub scripts (no LLM)

By default, self uses the opencode-go subscription (read from
~/.local/share/opencode/auth.json, endpoint https://opencode.ai/zen/go,
model glm-5.2). On a quota / rate-limit error it falls back to a local
llama-server on port 8080 for that call. Override with SELF_LLM_* env vars.
Set SELF_LLM_STUB=1 to force stub scripts without calling the LLM.

events the kernel acts on:
  kernel.initialized   written by 'self init'
  command.declared     compiled into a command by 'self grow' AND 'self run'
  projector.declared   compiled into a projection by 'self grow' AND 'self run'
  restore.requested    DATA-ONLY rollback intent {name, seq} — any seed,
                       command, or the CLI may emit it; the kernel reinstalls an
                       earlier receipt. Carries no code, so it adds no surface.
  script.compiled      a compile receipt, SIGNED with the home's secret
                       (SELF_HOME/.secret). Anyone may append one, but only a
                       kernel-signed receipt ever installs — provenance is in the
                       signature, not in who wrote it. restore reads these.
  seed.planted         written by 'self grow' as a receipt
  everything else      comes from seeds, or commands that emit declarations

The loop carries SPECS, not code: the LLM is always the compiler, so every
binary is authored for this receiver and adaptation is never skipped. Each
compile is logged as a script.compiled receipt signed with the home's secret;
install verifies the signature, so only kernel-authored code reaches
capabilities/. A seed may carry a reference implementation (an "implementation"
field on a declaration) — the compiler verifies it against the pipe contract and
adapts it; it is never installed as-is. At compile time and via 'self think', the brain gets a
read-only bash tool (cwd=SELF_HOME) to explore the garden and adapt to my
current state. The kernel is the sole steward of LLM credentials.
`)
}

func cmdInit(home string) error {
	if err := os.MkdirAll(filepath.Join(home, "capabilities", "commands"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(home, "capabilities", "projectors"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(home, "site"), 0755); err != nil {
		return err
	}
	if err := kernel.InitSecret(home); err != nil {
		return fmt.Errorf("mint signing key: %w", err)
	}
	st := store.Open(home)
	payload, _ := json.Marshal(map[string]string{
		"version": "self/v0",
	})
	e := event.New(event.KernelInitialized, payload)
	if err := st.Append(&e); err != nil {
		return err
	}

	fmt.Printf("initialized self at %s (seq %d %s)\n", home, e.Seq, e.Name)
	fmt.Printf("a baby kernel — grow it: self grow <seed>\n")
	return kernel.RenderHTML(home)
}

func cmdGrow(home string, seedDir string) error {
	manifest, err := seed.Load(seedDir)
	if err != nil {
		return err
	}

	compiler := seed.NewCompiler(home)
	capDir := filepath.Join(home, "capabilities")

	type compiled struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		Script string `json:"script"`
	}
	var compiledScripts []compiled

	for _, cmd := range manifest.Commands {
		fmt.Printf("compiling command %q...", cmd.Name)
		script, err := compiler.CompileCommand(cmd)
		if err != nil {
			fmt.Printf(" failed\n")
			return fmt.Errorf("command %q: %w", cmd.Name, err)
		}
		if err := seed.WriteCommandScript(capDir, cmd.Name, script); err != nil {
			return err
		}
		fmt.Printf(" compiled\n")
		compiledScripts = append(compiledScripts, compiled{"command", cmd.Name, script})
	}

	for _, proj := range manifest.Projectors {
		fmt.Printf("compiling projector %q...", proj.Name)
		script, err := compiler.CompileProjector(proj)
		if err != nil {
			fmt.Printf(" failed\n")
			return fmt.Errorf("projector %q: %w", proj.Name, err)
		}
		if err := seed.WriteProjectorScript(capDir, proj.Name, script); err != nil {
			return err
		}
		fmt.Printf(" compiled\n")
		compiledScripts = append(compiledScripts, compiled{"projector", proj.Name, script})
	}

	st := store.Open(home)
	contentCount := 0
	for i := range manifest.Events {
		e := manifest.Events[i]
		// No special-casing of script.compiled anymore: a seed can append one,
		// but it carries no valid signature for this home, so it can never install
		// (Restore verifies). It's inert data, like any other event.
		isDeclaration := e.Name == event.CommandDeclared || e.Name == event.ProjectorDeclared
		fresh := event.New(e.Name, e.Payload)
		if err := st.Append(&fresh); err != nil {
			return err
		}
		if !isDeclaration {
			contentCount++
		}
	}

	// Log a signed receipt for each script we just compiled. The signature is
	// what gives a script.compiled power — only the kernel can produce it — so
	// these (and only these) are restorable later.
	for _, cs := range compiledScripts {
		payload, sErr := kernel.SignedReceipt(home, cs.Type, cs.Name, cs.Script)
		if sErr != nil {
			return sErr
		}
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

	fmt.Printf("grew %q: %d command(s), %d projector(s), %d event(s) replayed, receipt seq %d\n",
		manifest.Name, len(manifest.Commands), len(manifest.Projectors), contentCount, receipt.Seq)
	return kernel.RenderHTML(home)
}

// runCommand runs a capability end-to-end: executes the script, appends the
// events it emits, compiles any declarations it produced (the strange loop),
// and auto-runs the projectors that consume those events. Returns the events
// produced. Progress is logged to stderr; callers format their own output.
// Shared by the CLI (cmdRun), the HTTP /run route, and the brain's command
// tools — one run pipeline, three callers.
func runCommand(home string, command string, args []string) ([]event.Event, error) {
	scriptPath := filepath.Join(home, "capabilities", "commands", command)
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("command %q not found (grow a seed that declares it)", command)
	}

	st := store.Open(home)
	current, err := st.Read()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(scriptPath, args...)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home)
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

	// No reserve filter: a command may emit a script.compiled, but it can't sign
	// it for this home, so it's inert — Restore verifies before installing. Code
	// reaches capabilities/ only through the kernel's own signed receipts.
	for i := range newEvents {
		if err := st.Append(&newEvents[i]); err != nil {
			return nil, err
		}
	}

	// Strange-loop hook: compile any command.declared / projector.declared the
	// command emitted, so a command can grow new capabilities at run time.
	compiledCmds, compiledProjs, cErr := kernel.CompileDeclarations(home, newEvents)
	if cErr != nil {
		fmt.Fprintf(os.Stderr, "self: warning: declaration compile failed: %s\n", cErr)
	}
	if len(compiledCmds) > 0 || len(compiledProjs) > 0 {
		fmt.Fprintf(os.Stderr, "self: self-improved: %d command(s), %d projector(s) compiled\n",
			len(compiledCmds), len(compiledProjs))
	}

	// Restore hook: a command may emit a data-only restore.requested {name, seq};
	// the kernel acts on it by reinstalling its own earlier receipt. This is how
	// the `restore` capability works — an ordinary command, no special kernel verb.
	if restored, rErr := kernel.ApplyRestores(home, newEvents); rErr != nil {
		fmt.Fprintf(os.Stderr, "self: warning: restore failed: %s\n", rErr)
	} else if len(restored) > 0 {
		fmt.Fprintf(os.Stderr, "self: restored %s\n", strings.Join(restored, ", "))
	}

	// Auto-run projectors that consume the new events. The kernel reads its own
	// projection (site/kernel.html) to know which projectors care about which
	// events — burn kernel.html, replay events, it comes back.
	wiring, wErr := kernel.ReadWiring(home)
	if wErr != nil {
		fmt.Fprintf(os.Stderr, "self: warning: could not read kernel wiring: %s\n", wErr)
		return newEvents, nil
	}
	ran := map[string]bool{}
	for _, e := range newEvents {
		for _, projName := range wiring.ProjectorsForEvent(e.Name) {
			if ran[projName] {
				continue
			}
			ran[projName] = true
			fmt.Fprintf(os.Stderr, "self: auto-running projector %q\n", projName)
			if pErr := runProjectorToSite(home, projName); pErr != nil {
				fmt.Fprintf(os.Stderr, "self: projector %q failed: %s\n", projName, pErr)
			}
		}
	}

	return newEvents, nil
}

// cmdRun is the CLI wrapper around runCommand: it prints each appended
// event to stdout.
func cmdRun(home string, command string, args []string) error {
	newEvents, err := runCommand(home, command, args)
	if err != nil {
		return err
	}
	for _, e := range newEvents {
		fmt.Printf("%s appended seq %d %s\n", e.ID, e.Seq, e.Name)
	}
	return nil
}

// cmdRestore is the always-on, built-in trigger for a rollback — a thin
// convenience so the safety net exists even on a bare kernel with no `restore`
// seed grown. It does the same thing the `restore` capability does: log a
// data-only restore.requested intent, then let the kernel act on it. (The
// install itself is the kernel's; the CLI only supplies a name and seq.)
func cmdRestore(home string, args []string) error {
	name := args[0]
	seq := 0
	if len(args) >= 2 {
		if _, e := fmt.Sscanf(args[1], "%d", &seq); e != nil || seq <= 0 {
			return fmt.Errorf("seq must be a positive integer from 'self history', got %q", args[1])
		}
	}
	payload, _ := json.Marshal(map[string]any{"name": name, "seq": seq})
	intent := event.New(event.RestoreRequested, payload)
	if err := store.Open(home).Append(&intent); err != nil {
		return err
	}
	_, err := kernel.ApplyRestores(home, []event.Event{intent})
	return err
}

// cmdShow renders a projection. Unix-native: when stdout is piped it writes the
// raw projection HTML to stdout (compose it with other tools); when run from a
// terminal it writes a styled, self-contained copy and opens it in a browser.
// Either way it refreshes the canonical bare projection in site/ that agents
// read directly.
func cmdShow(home string, name string) error {
	scriptPath := filepath.Join(home, "capabilities", "projectors", name)
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("projection %q not found — try 'self ls projectors'", name)
	}
	html, err := projectHTML(home, name)
	if err != nil {
		return err
	}

	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	if err := os.WriteFile(filepath.Join(siteDir, name+".html"), html, 0644); err != nil {
		return err
	}

	if !isTerminal(os.Stdout) {
		_, err := os.Stdout.Write(html)
		return err
	}

	// Interactive: a styled, self-contained page (enrichment CSS inlined) so it
	// looks right opened straight from disk, no server required.
	preview := injectStyle(html)
	tmp := filepath.Join(os.TempDir(), "self-show-"+name+".html")
	if err := os.WriteFile(tmp, preview, 0644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "self: rendered %q — opening in your browser\n", name)
	if err := openInBrowser("file://" + tmp); err != nil {
		fmt.Fprintf(os.Stderr, "self: couldn't open a browser (%v)\n", err)
		fmt.Fprintf(os.Stderr, "self: the page is at %s — or run 'self live' and visit http://localhost:7777/%s\n", tmp, name)
	}
	return nil
}

// isTerminal reports whether f is a terminal (character device) rather than a
// pipe or file — used to decide between human and machine output.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// injectStyle inlines the kernel's enrichment stylesheet into a projection so a
// bare projector page renders styled when opened from disk (no server, so no
// nav or auto-reload — those are serve-time concerns).
func injectStyle(html []byte) []byte {
	s := string(html)
	if i := strings.Index(s, "</head>"); i >= 0 {
		return []byte(s[:i] + enrichmentCSS + s[i:])
	}
	return append([]byte(enrichmentCSS), html...)
}

// openInBrowser opens target with the platform's default opener.
func openInBrowser(target string) error {
	for _, bin := range []string{"xdg-open", "open", "sensible-browser", "x-www-browser"} {
		if path, err := exec.LookPath(bin); err == nil {
			return exec.Command(path, target).Start()
		}
	}
	return fmt.Errorf("no opener (xdg-open/open) found on PATH")
}

// runProjectorToSite runs a projector and persists its HTML to
// SELF_HOME/site/<name>.html. Used by the auto-run mechanism after invoke.
func runProjectorToSite(home string, name string) error {
	return runProjector(home, name, false)
}

// runProjector runs a projector script, piping all events as JSONL to stdin.
// It always persists HTML to SELF_HOME/site/<name>.html. If showStdout is true,
// it also writes HTML to os.Stdout.
func runProjector(home string, name string, showStdout bool) error {
	scriptPath := filepath.Join(home, "capabilities", "projectors", name)
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
// HTML output, without persisting to site/. Used by self live to render fresh
// projections on every request.
func projectHTML(home, name string) ([]byte, error) {
	scriptPath := filepath.Join(home, "capabilities", "projectors", name)
	events, err := store.Open(home).Read()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(scriptPath)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home)
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
.self-nav{position:fixed;left:0;top:0;bottom:0;width:180px;overflow:auto;box-sizing:border-box;
  border-right:1px solid var(--border,#e2e4e8);padding:20px 14px;background:var(--card,#fff);font-size:14px}
.self-nav a{display:block;padding:5px 8px;border-radius:5px;color:var(--fg,#1f2328);text-decoration:none;
  white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.self-nav a:hover{background:color-mix(in srgb,var(--accent,#2563eb) 12%,transparent)}
.self-nav .self-brand{font-weight:700;font-family:ui-monospace,Menlo,monospace;font-size:1.1rem;margin-bottom:6px}
.self-nav .self-label{font-size:11px;text-transform:uppercase;letter-spacing:.04em;opacity:.55;margin:14px 8px 4px}
body{padding-left:210px}
@media (max-width:640px){.self-nav{position:static;width:auto;height:auto;border-right:0;border-bottom:1px solid var(--border,#e2e4e8)}body{padding-left:0}}
</style>`

// navHTML builds the projection sidebar from the planted projectors. Flat for
// now (nested projections aren't modelled yet — they'd need a recursive walk).
func navHTML(home string) string {
	var b strings.Builder
	b.WriteString(`<aside class="self-nav"><a class="self-brand" href="/">self</a>`)
	b.WriteString(`<div class="self-label">projections</div>`)
	entries, _ := os.ReadDir(filepath.Join(home, "capabilities", "projectors"))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		b.WriteString(fmt.Sprintf(`<a href="/%s">%s</a>`, n, htmlesc.EscapeString(n)))
	}
	b.WriteString(`<div class="self-label">kernel</div><a href="/">wiring &amp; identity</a></aside>`)
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
		fmt.Fprintf(os.Stderr, "self: warning: could not rebuild kernel.html: %s\n", err)
	}

	// Rebuild all projectors from the registry
	projDir := filepath.Join(home, "capabilities", "projectors")
	entries, err := os.ReadDir(projDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				fmt.Fprintf(os.Stderr, "rebuilding projector %q...\n", e.Name())
				if rebuildErr := runProjectorToSite(home, e.Name()); rebuildErr != nil {
					fmt.Fprintf(os.Stderr, "self: warning: projector %q failed: %s\n", e.Name(), rebuildErr)
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

		if _, err := os.Stat(filepath.Join(home, "capabilities", "projectors", name)); err == nil {
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
		scriptPath := filepath.Join(home, "capabilities", "projectors", name)
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

	// POST /run/<command> — run a capability from the browser. The raw request
	// body is passed as the command's single argument. This makes projections a
	// read+write surface: a projector can emit a form that POSTs here, the
	// command runs through the normal pipeline (append events, strange-loop
	// compile, auto-run projectors), and the auto-reload script then refreshes
	// the page to show the new events.
	mux.HandleFunc("/run/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		command := strings.TrimPrefix(r.URL.Path, "/run/")
		if command == "" {
			http.Error(w, "command required (e.g. /run/chat)", http.StatusBadRequest)
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
		if err := cmdRun(home, command, args); err != nil {
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
	fmt.Fprintf(os.Stderr, "self: the living garden is at http://localhost:%s\n", port)
	fmt.Fprintf(os.Stderr, "  /              my identity — capabilities, paths, wiring\n")
	fmt.Fprintf(os.Stderr, "  /<projection>  a projection, re-rendered live\n")
	fmt.Fprintf(os.Stderr, "  /run/<command> run a capability (plain HTML forms)\n")
	fmt.Fprintf(os.Stderr, "  /events        the raw event log\n")
	fmt.Fprintf(os.Stderr, "self: home is %s (run 'self where' for all paths)\n", home)
	return http.ListenAndServe(addr, mux)
}

// cmdHistory shows recent events newest-last. Human-readable by default
// (seq, time, name, a short payload hint); --raw dumps the JSONL; -n N bounds
// the count (default 20), --all shows everything.
func cmdHistory(home string, args []string) error {
	n := 20
	raw := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--raw":
			raw = true
		case "--all":
			n = -1
		case "-n":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &n)
				i++
			}
		}
	}
	events, err := store.Open(home).Read()
	if err != nil {
		return err
	}
	if len(events) == 0 {
		fmt.Println("(no events yet — run 'self init')")
		return nil
	}
	start := 0
	if n >= 0 && len(events) > n {
		start = len(events) - n
	}
	for _, e := range events[start:] {
		if raw {
			line, _ := json.Marshal(e)
			fmt.Println(string(line))
			continue
		}
		fmt.Printf("%4d  %s  %-20s  %s\n", e.Seq, e.OccurredAt.Format("15:04:05"), e.Name, eventHint(e))
	}
	return nil
}

// eventHint pulls a short, human-friendly summary from common payload shapes so
// 'self history' reads like a story rather than a wall of JSON.
func eventHint(e event.Event) string {
	var p map[string]any
	if json.Unmarshal(e.Payload, &p) != nil {
		return ""
	}
	for _, k := range []string{"title", "content", "text", "response", "message", "seed", "name", "stage"} {
		if v, ok := p[k].(string); ok && v != "" {
			return truncateLine(v, 64)
		}
	}
	return ""
}

func truncateLine(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// cmdLs lists what exists. With no argument it prints an overview; with
// commands/projectors it prints names with their full file paths (so the
// filesystem is obvious); with seeds it lists what's been grown.
func cmdLs(home string, args []string) error {
	what := ""
	if len(args) > 0 {
		what = args[0]
	}
	switch what {
	case "commands":
		return listDir(filepath.Join(home, "capabilities", "commands"), "command")
	case "projectors":
		return listDir(filepath.Join(home, "capabilities", "projectors"), "projection")
	case "seeds":
		return listSeeds(home)
	case "":
		cmds := dirEntries(filepath.Join(home, "capabilities", "commands"))
		projs := dirEntries(filepath.Join(home, "capabilities", "projectors"))
		fmt.Printf("self at %s\n\n", home)
		fmt.Printf("commands (%d):    %s\n", len(cmds), strings.Join(cmds, ", "))
		fmt.Printf("projections (%d): %s\n", len(projs), strings.Join(projs, ", "))
		fmt.Println("\nmore: self ls commands | self ls projectors | self ls seeds | self where")
		return nil
	default:
		return fmt.Errorf("unknown list %q — try: commands, projectors, seeds (or just 'self ls')", what)
	}
}

func dirEntries(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func listDir(dir, label string) error {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		fmt.Printf("(no %ss yet — grow a seed: self grow <seed>)\n", label)
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fmt.Printf("%-22s %s\n", e.Name(), filepath.Join(dir, e.Name()))
	}
	return nil
}

func listSeeds(home string) error {
	events, err := store.Open(home).Read()
	if err != nil {
		return fmt.Errorf("no self home yet (run 'self init')")
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
			parts = append(parts, "projections: "+strings.Join(rec.Projectors, ", "))
		}
		parts = append(parts, fmt.Sprintf("events replayed: %d", rec.EventsReplayed))
		fmt.Printf("%s — %s (seq %d)\n", rec.Seed, strings.Join(parts, ", "), e.Seq)
		found = true
	}
	if !found {
		fmt.Println("(no seeds grown yet — self grow <seed>)")
	}
	return nil
}

// cmdWhere prints SELF_HOME and every important path, marking what exists, so
// the filesystem never feels hidden.
func cmdWhere(home string) error {
	fmt.Printf("SELF_HOME=%s\n\n", home)
	for _, p := range []struct{ label, path string }{
		{"event log", filepath.Join(home, "events.jsonl")},
		{"signing key", filepath.Join(home, ".secret")},
		{"commands", filepath.Join(home, "capabilities", "commands")},
		{"projections", filepath.Join(home, "capabilities", "projectors")},
		{"site (HTML)", filepath.Join(home, "site")},
		{"identity page", filepath.Join(home, "site", "kernel.html")},
	} {
		mark := "—"
		if _, err := os.Stat(p.path); err == nil {
			mark = "✓"
		}
		fmt.Printf("  %s  %-14s %s\n", mark, p.label, p.path)
	}
	fmt.Println("\neverything here is plain files — read, grep, and inspect freely.")
	return nil
}

// cmdWhich prints the full path to a command or projection by name.
func cmdWhich(home string, name string) error {
	found := false
	for _, kind := range []struct{ dir, label string }{
		{"commands", "command"},
		{"projectors", "projection"},
	} {
		p := filepath.Join(home, "capabilities", kind.dir, name)
		if _, err := os.Stat(p); err == nil {
			fmt.Printf("%-11s %s\n", kind.label, p)
			found = true
		}
	}
	if !found {
		return fmt.Errorf("%q is neither a command nor a projection — try 'self ls'", name)
	}
	return nil
}

// rehydrateFromLog rebuilds the on-disk body — capabilities/ and the rendered
// site/ — from the event log's signed receipts, so the working tree is a pure
// materialization of the one truth. No LLM, no network; safe to run repeatedly
// (it reinstalls identical bytes). This is what makes events.jsonl (+ the home's
// .secret, which verifies the receipts) a sufficient artifact: drop those two
// files into a fresh home and the whole body comes back.
func rehydrateFromLog(home string) (commands, projectors []string, err error) {
	commands, projectors, err = kernel.Rehydrate(home)
	if err != nil {
		return nil, nil, err
	}
	if len(commands)+len(projectors) == 0 {
		return commands, projectors, nil // uninitialized, or no signed receipts to install
	}
	if rErr := kernel.RenderHTML(home); rErr != nil {
		return commands, projectors, rErr
	}
	for _, name := range projectors {
		if rErr := runProjectorToSite(home, name); rErr != nil {
			fmt.Fprintf(os.Stderr, "self: rehydrate render %q: %s\n", name, rErr)
		}
	}
	return commands, projectors, nil
}

// cmdRehydrate explicitly rebuilds the body from the log and reports what it
// reinstated. Bare `self` does this automatically before serving.
func cmdRehydrate(home string) error {
	commands, projectors, err := rehydrateFromLog(home)
	if err != nil {
		return err
	}
	if len(commands)+len(projectors) == 0 {
		fmt.Println("rehydrate: nothing signed to reinstall.")
		fmt.Println("  Either this home is uninitialized, or its .secret did not sign the log's")
		fmt.Println("  receipts (a fresh key can't verify another home's bytes). Copy the home's")
		fmt.Println("  .secret alongside events.jsonl, or re-grow from the log's declarations.")
		return nil
	}
	fmt.Printf("rehydrated from the log: %d command(s), %d projection(s) reinstalled and rendered\n",
		len(commands), len(projectors))
	fmt.Printf("  commands:    %s\n", strings.Join(commands, ", "))
	fmt.Printf("  projectors:  %s\n", strings.Join(projectors, ", "))
	return nil
}

// cmdHeartbeat runs one self-improvement cycle: the brain reflects on the
// garden, may act (call capabilities) and grow (declare new ones). Declarations
// it produces are applied through the strange loop, so the cycle can leave self
// with a genuinely new capability.
func cmdHeartbeat(home string) error {
	compiler := seed.NewCompiler(home)
	if !compiler.Available() {
		return fmt.Errorf("heartbeat needs the brain (an LLM) — set SELF_LLM_* or run a local llama-server")
	}
	st := store.Open(home)
	hb := event.New("self.heartbeat", json.RawMessage(`{}`))
	if err := st.Append(&hb); err != nil {
		return err
	}

	const prompt = `This is a self-improvement heartbeat. Explore your garden — your capabilities, recent events, and projections — and choose ONE small, high-value improvement: a missing capability, a projection that would make your shared state clearer, or a fix for something that has drifted. If it is warranted, declare it (command.declared / projector.declared) so it compiles into a real capability. Adapt to what already exists rather than duplicating it. If nothing is worth changing right now, say so plainly and declare nothing. Keep it minimal.`

	commands, invoke := brainTools(home)
	result, err := compiler.CallBrain(prompt, commands, invoke)
	if err != nil {
		return err
	}

	// Apply any declarations the brain produced, through the strange loop.
	var declEvents []event.Event
	for _, d := range result.Declarations {
		name, _ := d["name"].(string)
		payload, _ := json.Marshal(d["payload"])
		if name == "" || len(payload) == 0 || string(payload) == "null" {
			continue
		}
		e := event.New(name, payload)
		if err := st.Append(&e); err != nil {
			return err
		}
		declEvents = append(declEvents, e)
	}
	if len(declEvents) > 0 {
		cmds, projs, cErr := kernel.CompileDeclarations(home, declEvents)
		if cErr != nil {
			fmt.Fprintf(os.Stderr, "self: heartbeat compile failed: %s\n", cErr)
		} else {
			fmt.Fprintf(os.Stderr, "self: heartbeat grew %d command(s), %d projection(s)\n", len(cmds), len(projs))
		}
	}
	fmt.Println(result.Response)
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
		return fmt.Errorf("usage: self think <prompt> (or pipe prompt on stdin)")
	}

	compiler := seed.NewCompiler(home)

	commands, invoke := brainTools(home)
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

// brainTools returns the capabilities the brain may call as tools and an
// invoker that runs them, honoring SELF_THINK_DEPTH so a brain-invoked command
// that itself calls the brain can't recurse without bound. Past the cap it
// returns no tools — the brain can still talk and declare, but not act.
// Shared by 'self think' and 'self heartbeat'.
func brainTools(home string) ([]seed.Command, seed.CommandInvoker) {
	depth := 0
	fmt.Sscanf(os.Getenv("SELF_THINK_DEPTH"), "%d", &depth)
	if depth >= 3 {
		return nil, nil
	}
	os.Setenv("SELF_THINK_DEPTH", fmt.Sprintf("%d", depth+1))
	commands := plantedCommands(home)
	invoke := func(name, args string) (string, error) {
		var argv []string
		if strings.TrimSpace(args) != "" {
			argv = []string{args}
		}
		evs, err := runCommand(home, name, argv)
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
	return commands, invoke
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
