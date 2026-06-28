package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	htmlesc "html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"self/internal/event"
	"self/internal/kernel"
	"self/internal/seed"
	"self/internal/store"
)

// demoEventsLog is the shipped demo body (a task board + meal planner), embedded
// so a cold `self` can bring a living garden up with no setup. It is imported
// into a fresh, sovereign home (re-signed under that home's own key — never the
// committed demo key), then populated with a little example content.
//
//go:embed home/events.jsonl
var demoEventsLog []byte

func main() {
	home := homeDir()

	// Bare `self` is the most common action: heal the body from the log, then
	// start the live garden. Rehydrating first means the working tree always
	// matches the one truth — clone a home that is just events.jsonl + .secret
	// and `self` brings the whole body back before serving it.
	if len(os.Args) < 2 {
		if !homeInitialized(home) {
			// First run: bring up a living garden so the cold open is something to
			// explore, not an empty page.
			fmt.Fprintf(os.Stderr, "self: new home %s — bringing up the demo garden…\n", home)
			if err := loadDemo(home); err != nil {
				fmt.Fprintf(os.Stderr, "self: demo: %s\n", err)
			}
		} else if _, _, err := rehydrateFromLog(home); err != nil {
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
	case "brain":
		prompt := strings.Join(args, " ")
		err = cmdBrain(home, prompt)
	case "teach":
		err = cmdTeach(home, args)
	case "demo":
		err = cmdDemo(home)
	case "heartbeat":
		err = cmdHeartbeat(home)
	case "watch":
		err = cmdWatch(home, args)
	case "rehydrate":
		err = cmdRehydrate(home)
	case "selftest":
		err = cmdSelfTest(home)
	case "map":
		err = cmdMap(home)
	case "identity":
		err = cmdIdentity(home)
	case "verify-attestation":
		err = cmdVerifyAttestation(home)
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
  self map                one-glance overview of the garden — commands, projections, recent activity
  self selftest           re-run every installed capability's examples against its binary (regression gate)
  self identity           print this home's public verification key (shareable)
  self verify-attestation check a script.verified attestation piped on stdin (no secret needed)
  self grow <seed>        grow a new capability from a seed
  self run <command> ...  run a capability — append events, refresh projections
  self think "..."        ask the brain — pipe the prompt to the brain process, ingest the events it emits
  self brain "..."        the default brain process itself (prompt in, event JSONL out); swap via $SELF_BRAIN
  self heartbeat          run one self-improvement cycle (brain reflects & grows)
  self watch [secs]       resident loop: react to new activity (or report it, no brain). Ctrl-C to stop
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

// homeInitialized reports whether a home has been minted (its signing key
// exists). A bare `self` on an uninitialized home brings up the demo.
func homeInitialized(home string) bool {
	_, err := os.Stat(filepath.Join(home, ".secret"))
	return err == nil
}

// loadDemo brings up a living garden in a fresh home with no LLM: it initializes
// the home (keys + onboarding), imports the shipped demo's capabilities
// (re-signing their scripts under THIS home's key, so the result is sovereign —
// not a copy of the committed demo key), wires their declarations, then runs a
// few real commands so the board and kitchen open with content to click.
func loadDemo(home string) error {
	if !homeInitialized(home) {
		if err := cmdInit(home); err != nil {
			return err
		}
	}
	st := store.Open(home)

	type script struct{ kind, body string }
	latest := map[string]script{}
	var order []string
	for _, line := range strings.Split(string(demoEventsLog), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		switch e.Name {
		case event.ScriptCompiled:
			// Don't carry the demo's receipts (signed with the demo key); keep the
			// bytes and re-sign them under this home below.
			var cs struct{ Type, Name, Script string }
			if json.Unmarshal(e.Payload, &cs) == nil && cs.Name != "" {
				if _, seen := latest[cs.Name]; !seen {
					order = append(order, cs.Name)
				}
				latest[cs.Name] = script{cs.Type, cs.Script}
			}
		case event.CommandDeclared, event.ProjectorDeclared:
			ev := event.New(e.Name, e.Payload) // declarations wire kernel.html + auto-run
			if err := st.Append(&ev); err != nil {
				return err
			}
			// Skip kernel.initialized, script.verified, seed.planted, and the demo's
			// own content — we regenerate fresh content below.
		}
	}
	for _, name := range order {
		s := latest[name]
		if err := kernel.InstallBuiltin(home, s.kind, name, s.body); err != nil {
			return err
		}
	}

	// Populate with a little real content (no LLM — these are plain commands).
	demo := [][]string{
		{"capture", "Email the contractor about the deck"},
		{"capture", "Draft the Q3 planning doc"},
		{"capture", "Book the dentist"},
		{"move", "1", "This week"},
		{"move", "3", "Done"},
		{"plan", "mon", "Tacos"},
		{"plan", "tue", "Sheet-pan salmon"},
		{"shop", "olive oil"},
		{"shop", "limes"},
	}
	for _, run := range demo {
		_, _ = runCommand(home, run[0], run[1:]) // best-effort; demo content only
	}

	if err := kernel.RenderHTML(home); err != nil {
		return err
	}
	for _, p := range []string{"welcome", "board", "kitchen"} {
		_ = runProjectorToSite(home, p)
	}
	return nil
}

// cmdDemo loads the demo into the current home (use a fresh SELF_HOME if the
// current one is already in use).
func cmdDemo(home string) error {
	if homeInitialized(home) {
		return fmt.Errorf("home %s already exists — point SELF_HOME at a fresh dir to try the demo (e.g. SELF_HOME=$(mktemp -d) self demo)", home)
	}
	if err := loadDemo(home); err != nil {
		return err
	}
	fmt.Printf("demo loaded at %s — run `self` and open http://localhost:7777\n", home)
	return nil
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
	if err := kernel.InitIdentity(home); err != nil {
		return fmt.Errorf("mint identity key: %w", err)
	}
	st := store.Open(home)
	payload, _ := json.Marshal(map[string]string{
		"version": "self/v0",
	})
	e := event.New(event.KernelInitialized, payload)
	if err := st.Append(&e); err != nil {
		return err
	}

	// Install the brain-setup surface (a baby-kernel onboarding seed, signed by
	// the secret just minted) so a fresh user can wire in their LLM from a page,
	// before any LLM exists. See onboarding.go.
	if err := installOnboarding(home); err != nil {
		return fmt.Errorf("install onboarding: %w", err)
	}

	fmt.Printf("initialized self at %s (seq %d %s)\n", home, e.Seq, e.Name)
	fmt.Printf("%s\n", onboardingURLHint(home))
	if err := kernel.RenderHTML(home); err != nil {
		return err
	}
	// Render the onboarding pages so /, /setup and /interview are ready on first serve.
	for _, p := range []string{"welcome", "setup", "interview"} {
		if err := runProjectorToSite(home, p); err != nil {
			return err
		}
	}
	return nil
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
		if ok, vErr := kernel.VerifyAndLog(home, "command", cmd.Name, script, cmd.Examples); vErr != nil {
			fmt.Printf(" verify error\n")
			return fmt.Errorf("verify command %q: %w", cmd.Name, vErr)
		} else if !ok {
			fmt.Printf(" failed verification\n")
			return fmt.Errorf("command %q failed its examples — not installed", cmd.Name)
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
		if ok, vErr := kernel.VerifyAndLog(home, "projector", proj.Name, script, proj.Examples); vErr != nil {
			fmt.Printf(" verify error\n")
			return fmt.Errorf("verify projector %q: %w", proj.Name, vErr)
		} else if !ok {
			fmt.Printf(" failed verification\n")
			return fmt.Errorf("projector %q failed its examples — not installed", proj.Name)
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

	newEvents, err := pipeProcess(home, scriptPath, args)
	if err != nil {
		return nil, err
	}
	if err := ingestEvents(home, newEvents); err != nil {
		return nil, err
	}
	return newEvents, nil
}

// pipeProcess runs an executable as a Unix pipeline node: it sets SELF_HOME and
// cwd-relevant env, feeds the current event log as JSONL on stdin, and parses
// the new events the process emits as JSONL on stdout. This is the one shape the
// kernel uses to talk to *any* outside intelligence — a compiled command, or the
// brain process — so a command and a brain are the same kind of thing: a process
// the kernel pipes events through.
func pipeProcess(home, bin string, argv []string) ([]event.Event, error) {
	st := store.Open(home)
	current, err := st.Read()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, argv...)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home)
	cmd.Dir = home // the process can read the garden directly (ls/cat), no tool round-trip
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
		return nil, fmt.Errorf("start %s: %w", filepath.Base(bin), err)
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
			return nil, fmt.Errorf("%s output parse error: %w", filepath.Base(bin), err)
		}
		if partial.Name == "" {
			return nil, fmt.Errorf("%s output missing event name: %s", filepath.Base(bin), line)
		}
		newEvents = append(newEvents, event.New(partial.Name, partial.Payload))
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("%s exited: %w", filepath.Base(bin), err)
	}
	return newEvents, nil
}

// ingestEvents appends the events a piped process emitted and runs the kernel's
// three reactions over them: the strange-loop compile (command/projector
// declarations), restore (data-only restore.requested), and projector auto-run.
// Shared by command invocation and the brain — whatever emits the events, the
// kernel reacts to them identically.
func ingestEvents(home string, newEvents []event.Event) error {
	st := store.Open(home)

	// No reserve filter: a process may emit a script.compiled, but it can't sign
	// it for this home, so it's inert — Restore verifies before installing. Code
	// reaches capabilities/ only through the kernel's own signed receipts.
	for i := range newEvents {
		if err := st.Append(&newEvents[i]); err != nil {
			return err
		}
	}

	// Strange-loop hook: compile any command.declared / projector.declared the
	// process emitted, so a command — or the brain — can grow new capabilities.
	compiledCmds, compiledProjs, cErr := kernel.CompileDeclarations(home, newEvents)
	if cErr != nil {
		fmt.Fprintf(os.Stderr, "self: warning: declaration compile failed: %s\n", cErr)
	}
	if len(compiledCmds) > 0 || len(compiledProjs) > 0 {
		fmt.Fprintf(os.Stderr, "self: self-improved: %d command(s), %d projector(s) compiled\n",
			len(compiledCmds), len(compiledProjs))
	}

	// Restore hook: a process may emit a data-only restore.requested {name, seq};
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
		return nil
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
	return nil
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

		// The root is the welcome page — the human landing. The dev wiring view
		// lives at /kernel. Fall back to it if welcome isn't installed (an older
		// home that predates onboarding).
		if name == "" {
			if _, err := os.Stat(filepath.Join(home, "capabilities", "projectors", "welcome")); err == nil {
				name = "welcome"
			} else {
				name = "kernel"
			}
		}

		if name == "kernel" {
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

	// POST /teach — the human-is-the-compiler route. The operator authors a
	// capability's script by hand (in the interview page) and the kernel signs +
	// installs it. This is a privileged KERNEL route, distinct from /run: code
	// enters only through a path the person at the keyboard drives, never through
	// a command-emitted event, so the foreign-code line (Slices 4–6) stays drawn.
	mux.HandleFunc("/teach", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var consumes []string
		for _, c := range strings.Split(r.FormValue("consumes"), ",") {
			if c = strings.TrimSpace(c); c != "" {
				consumes = append(consumes, c)
			}
		}
		var examples []seed.Example
		if ex := strings.TrimSpace(r.FormValue("examples")); ex != "" {
			if err := json.Unmarshal([]byte(ex), &examples); err != nil {
				http.Error(w, "examples must be a JSON array: "+err.Error(), http.StatusBadRequest)
				return
			}
		}
		if err := kernel.Teach(home, r.FormValue("kind"), r.FormValue("name"), consumes, r.FormValue("script"), examples); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// If this answered a parked question, close it.
		if id := strings.TrimSpace(r.FormValue("id")); id != "" {
			ans, _ := json.Marshal(map[string]string{"id": id})
			e := event.New("brain.answered", ans)
			_ = store.Open(home).Append(&e)
		}
		if ref := r.Header.Get("Referer"); ref != "" {
			http.Redirect(w, r, ref, http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "taught")
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
	for _, k := range []string{"title", "content", "text", "meal", "item", "issue", "what", "response", "message", "seed", "name", "stage"} {
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

// cmdMap prints a one-glance overview of the whole garden: ways to act
// (commands), ways to see (projections + their live URLs), and recent activity.
// As capabilities grow, this is the human's entry point — the navigable map the
// scaling story needs — and it's a pure read of the log, no kernel, no brain.
func cmdMap(home string) error {
	events, err := store.Open(home).Read()
	if err != nil {
		return err
	}
	if len(events) == 0 {
		fmt.Println("(empty garden — run 'self init')")
		return nil
	}

	type decl struct {
		desc  string
		order int
	}
	cmds := map[string]decl{}
	projs := map[string]decl{}
	noise := map[string]bool{
		event.ScriptCompiled: true, event.ScriptVerified: true,
		event.CommandDeclared: true, event.ProjectorDeclared: true,
		event.SeedPlanted: true, event.KernelInitialized: true,
		event.RestoreRequested: true, "self.heartbeat": true,
	}
	var born string
	var recent []event.Event
	for i, e := range events {
		switch e.Name {
		case event.KernelInitialized:
			if born == "" {
				born = e.OccurredAt.Format("2006-01-02 15:04")
			}
		case event.CommandDeclared:
			var c seed.Command
			if json.Unmarshal(e.Payload, &c) == nil && c.Name != "" {
				cmds[c.Name] = decl{firstSentence(c.Description), i}
			}
		case event.ProjectorDeclared:
			var p seed.ProjectorDecl
			if json.Unmarshal(e.Payload, &p) == nil && p.Name != "" {
				projs[p.Name] = decl{firstSentence(p.Description), i}
			}
		}
		if !noise[e.Name] {
			recent = append(recent, e)
		}
	}

	fmt.Printf("self — a garden born %s, %d events\n\n", born, len(events))

	fmt.Printf("ways to act (%d commands):\n", len(cmds))
	for _, name := range sortedByName(cmds) {
		fmt.Printf("  self run %-10s %s\n", name, cmds[name].desc)
	}
	fmt.Printf("\nways to see (%d projections):\n", len(projs))
	for _, name := range sortedByName(projs) {
		fmt.Printf("  /%-12s %s\n", name, projs[name].desc)
	}

	if len(recent) > 0 {
		fmt.Printf("\nlately:\n")
		start := 0
		if len(recent) > 6 {
			start = len(recent) - 6
		}
		for _, e := range recent[start:] {
			hint := eventHint(e)
			fmt.Printf("  %s  %-16s %s\n", e.OccurredAt.Format("15:04"), e.Name, hint)
		}
	}
	fmt.Printf("\nbrowse it live with 'self' (web), or 'self ls' / 'self where' for paths.\n")
	return nil
}

// firstSentence trims a description to its first sentence (or 80 chars) for a
// compact one-line map entry.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '.'); i > 0 && i < 90 {
		return s[:i]
	}
	return truncateLine(s, 80)
}

func sortedByName[T any](m map[string]T) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
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

// cmdSelfTest re-runs every installed capability's declared examples against the
// binary on disk and reports pass/fail/untested per capability, exiting nonzero
// if any fail. It is the standing regression gate — the projection/output is the
// oracle — that lets the system (and the autonomous heartbeat loop) prove a
// change didn't break a contract before trusting it.
func cmdSelfTest(home string) error {
	results, err := kernel.SelfTest(home)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Println("selftest: no installed capabilities to test")
		return nil
	}
	passed, failed, untested := 0, 0, 0
	for _, r := range results {
		switch {
		case !r.HasExamples:
			untested++
			fmt.Printf("  ?  %-14s (%s)  no examples — untested\n", r.Name, r.Kind)
		case r.Result.OK():
			passed++
			fmt.Printf("  ✓  %-14s (%s)  %s\n", r.Name, r.Kind, r.Result.Summary())
		default:
			failed++
			fmt.Printf("  ✗  %-14s (%s)  %s\n", r.Name, r.Kind, r.Result.Summary())
			for _, f := range r.Result.Failures {
				fmt.Printf("        %s\n", f)
			}
		}
	}
	fmt.Printf("selftest: %d passed, %d failed, %d untested\n", passed, failed, untested)
	if failed > 0 {
		return fmt.Errorf("%d capabilit(ies) failed selftest", failed)
	}
	return nil
}

// cmdIdentity prints the home's public verification key — its shareable
// identity. Other nodes use it to check this home's script.verified attestations
// without re-running anything and without any shared secret.
func cmdIdentity(home string) error {
	pub, err := kernel.PublicIdentity(home)
	if err != nil {
		return err
	}
	fmt.Printf("verification identity (ed25519 public key):\n%s\n", pub)
	fmt.Printf("\nShare this so others can verify your script.verified attestations.\n")
	fmt.Printf("Your private key stays in %s/.identity and never leaves.\n", home)
	return nil
}

// cmdVerifyAttestation reads a script.verified event (or its bare payload) as
// JSON on stdin and reports whether its ed25519 signature is valid — the
// receiver-side check that turns a foreign node's verification claim from
// "trust me" into "the math agrees." It needs no secret and no access to the
// signer; trusting WHO the key belongs to is a separate, human decision.
func cmdVerifyAttestation(home string) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("input is not JSON: %w", err)
	}
	payload := json.RawMessage(data)
	if p, ok := probe["payload"]; ok {
		payload = p // a full event was piped in; check its payload
	}

	att, ok, err := kernel.VerifyAttestation(payload)
	if err != nil {
		return fmt.Errorf("not a verifiable attestation: %w", err)
	}
	if !ok {
		fmt.Println("✗ INVALID — signature does not match (tampered, or wrong key)")
		return fmt.Errorf("attestation failed verification")
	}
	fmt.Printf("✓ VALID signature\n")
	fmt.Printf("  signer pubkey:   %s\n", att.PubKey)
	fmt.Printf("  attests:         %q (%s) passed=%v (%d/%d examples)\n",
		att.Name, att.Type, att.Passed, att.PassedCount, att.Ran)
	fmt.Printf("  of script sha256:   %s\n", att.ScriptSHA256)
	fmt.Printf("  vs examples sha256: %s\n", att.ExamplesSHA256)
	fmt.Printf("\nThe signature is sound. Whether to trust this signer is your call;\n")
	fmt.Printf("recompute the script/examples hashes against the bytes you hold to\n")
	fmt.Printf("confirm the claim is about the same capability.\n")
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

	// Read the log before this beat so we can hand the brain, by default, the
	// events since its last heartbeat — its context for "what changed in the
	// garden, and can I help?" — instead of making it explore from scratch. This
	// is the first step toward a brain that reacts to activity: a heartbeat that
	// already knows what happened since it last looked.
	prior, _ := st.Read()

	hb := event.New("self.heartbeat", json.RawMessage(`{}`))
	if err := st.Append(&hb); err != nil {
		return err
	}

	const basePrompt = `This is a self-improvement heartbeat. Explore your garden — your capabilities, recent events, and projections — and choose ONE small, high-value improvement: a missing capability, a projection that would make your shared state clearer, or a fix for something that has drifted. If it is warranted, declare it (command.declared / projector.declared) so it compiles into a real capability. Adapt to what already exists rather than duplicating it. If nothing is worth changing right now, say so plainly and declare nothing. Keep it minimal.`
	prompt := basePrompt + heartbeatContext(prior)

	commands, invoke := brainTools(home)
	result, err := compiler.CallBrain(prompt, commands, invoke)
	if err != nil {
		return err
	}

	// Apply any declarations the brain produced, through the strange loop.
	applyDeclarations(home, result)
	fmt.Println(result.Response)
	return nil
}

// sinceLastHeartbeat returns the events strictly after the most recent
// self.heartbeat. With no prior heartbeat (the first beat) it returns them all —
// the whole garden is new — and heartbeatContext caps the volume.
func sinceLastHeartbeat(events []event.Event) []event.Event {
	last := -1
	for i, e := range events {
		if e.Name == "self.heartbeat" {
			last = i
		}
	}
	return events[last+1:]
}

// heartbeatContext formats the activity since the last heartbeat into a concise
// block for the brain's prompt — name + a truncated payload per event, so the
// brain sees what changed without re-exploring. Kernel bookkeeping receipts
// (script.compiled / script.verified) are skipped: they are derived, not
// "actions to respond to." Empty string when nothing of note happened (so a
// quiet beat stays quiet). Capped so a busy stretch can't blow up the prompt.
// meaningfulEvents drops kernel bookkeeping receipts (script.compiled /
// script.verified) — derived events, not "actions to respond to."
func meaningfulEvents(events []event.Event) []event.Event {
	var acts []event.Event
	for _, e := range events {
		if e.Name == event.ScriptCompiled || e.Name == event.ScriptVerified {
			continue
		}
		acts = append(acts, e)
	}
	return acts
}

// formatEventContext renders a capped, concise block of events for a brain
// prompt (name + truncated payload per event). Returns "" when there's nothing.
// Shared by the heartbeat (events since last beat) and the watcher (events since
// last reaction).
func formatEventContext(acts []event.Event, intro string) string {
	if len(acts) == 0 {
		return ""
	}
	const capN = 40
	omitted := 0
	if len(acts) > capN {
		omitted = len(acts) - capN
		acts = acts[len(acts)-capN:]
	}
	var b strings.Builder
	b.WriteString(intro)
	if omitted > 0 {
		fmt.Fprintf(&b, "  (… %d earlier events omitted …)\n", omitted)
	}
	for _, e := range acts {
		fmt.Fprintf(&b, "  seq %d  %s  %s\n", e.Seq, e.Name,
			truncateLine(strings.TrimSpace(string(e.Payload)), 140))
	}
	return b.String()
}

func heartbeatContext(events []event.Event) string {
	acts := meaningfulEvents(sinceLastHeartbeat(events))
	body := formatEventContext(acts,
		"\n\nSince your last heartbeat, these things happened in the garden — your context for what changed and whether you can help:\n")
	if body == "" {
		return ""
	}
	return body + "\nResponding to what changed is welcome, but optional — if nothing here warrants action, say so and declare nothing."
}

// applyDeclarations appends any declarations the brain produced and runs them
// through the strange loop (compile). Shared by heartbeat and watch.
func applyDeclarations(home string, result *seed.BrainResult) {
	st := store.Open(home)
	var declEvents []event.Event
	for _, d := range result.Declarations {
		name, _ := d["name"].(string)
		payload, _ := json.Marshal(d["payload"])
		if name == "" || len(payload) == 0 || string(payload) == "null" {
			continue
		}
		e := event.New(name, payload)
		if err := st.Append(&e); err != nil {
			fmt.Fprintf(os.Stderr, "self: append declaration: %s\n", err)
			return
		}
		declEvents = append(declEvents, e)
	}
	if len(declEvents) > 0 {
		cmds, projs, cErr := kernel.CompileDeclarations(home, declEvents)
		if cErr != nil {
			fmt.Fprintf(os.Stderr, "self: compile failed: %s\n", cErr)
		} else {
			fmt.Fprintf(os.Stderr, "self: grew %d command(s), %d projection(s)\n", len(cmds), len(projs))
		}
	}
}

const watchPrompt = `You are watching the garden as a quiet resident. New activity has appeared since you last looked (below). Consider whether you can be genuinely helpful — grow a small capability, surface something, fix a drift. Most of the time the right move is to do nothing: only act if it clearly helps. If you act, keep it minimal; if not, say so briefly and declare nothing.`

// cmdWatch is the resident loop: it tails the event log and, on new activity,
// either REACTS (hands the new events to the brain to see if it can help — the
// reactive brain) or, with no brain configured, simply REPORTS the activity (a
// live monitor). This is the thing a timer/cron can't be: an actually-running
// process. The loop only ticks while it's alive — which is exactly the point we
// found the hard way, that autonomy needs a resident process, not a scheduler
// that fires "when idle." It guards against reacting to its own output by
// advancing its watermark past everything that exists after each reaction.
func cmdWatch(home string, args []string) error {
	interval := 5 * time.Second
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			interval = time.Duration(n) * time.Second
		}
	}
	compiler := seed.NewCompiler(home)
	brainOn := compiler.Available()
	st := store.Open(home)

	events, err := st.Read()
	if err != nil {
		return err
	}
	lastSeq := 0
	if len(events) > 0 {
		lastSeq = events[len(events)-1].Seq
	}
	mode := "observe-only (no brain configured — reporting activity)"
	if brainOn {
		mode = "reactive (brain on)"
	}
	fmt.Fprintf(os.Stderr, "self: watching %s every %s — %s. Ctrl-C to stop.\n", home, interval, mode)

	for {
		time.Sleep(interval)
		events, err := st.Read()
		if err != nil {
			fmt.Fprintf(os.Stderr, "self: watch read error: %s\n", err)
			continue
		}
		var fresh []event.Event
		for _, e := range events {
			if e.Seq > lastSeq && e.Name != "self.heartbeat" {
				fresh = append(fresh, e)
			}
		}
		acts := meaningfulEvents(fresh)
		tail := func() {
			if len(events) > 0 {
				lastSeq = events[len(events)-1].Seq
			}
		}
		if len(acts) == 0 {
			tail() // advance over pure bookkeeping so we don't rescan it
			continue
		}
		if !brainOn {
			for _, e := range acts {
				fmt.Printf("  %s  %-16s %s\n", e.OccurredAt.Format("15:04:05"), e.Name, eventHint(e))
			}
			tail()
			continue
		}
		fmt.Fprintf(os.Stderr, "self: reacting to %d new event(s)…\n", len(acts))
		ctx := formatEventContext(acts,
			"\n\nNew activity in the garden since you last looked — quietly consider whether you can help; staying silent is fine:\n")
		commands, invoke := brainTools(home)
		result, bErr := compiler.CallBrain(watchPrompt+ctx, commands, invoke)
		if bErr != nil {
			fmt.Fprintf(os.Stderr, "self: watch brain error: %s\n", bErr)
		} else {
			applyDeclarations(home, result)
			if strings.TrimSpace(result.Response) != "" {
				fmt.Println(result.Response)
			}
		}
		// Feedback guard: skip everything that now exists — including the brain's
		// own appends — so the watcher never reacts to its own output.
		if after, rErr := st.Read(); rErr == nil && len(after) > 0 {
			lastSeq = after[len(after)-1].Seq
		} else {
			tail()
		}
	}
}

// cmdThink is the brain's call interface for capabilities — backward-compatible
// with every garden ever grown. Its contract is unchanged: read a prompt, return
// {response, declarations} JSON, append nothing (the caller, e.g. the `chat`
// command, owns appending the events). What changed underneath is only the
// plumbing: instead of linking the LLM in-process, it spawns the brain *process*
// ($SELF_BRAIN, default `self brain`), reads the events it emits, and folds them
// back into the same JSON shape the old `self think` produced. So an existing
// `chat` (or `grow-spec`) keeps working untouched, while the brain is now a
// swappable process behind the pipe.
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

	bin, argv := brainCommand(prompt)
	emitted, err := pipeProcess(home, bin, argv) // spawn the brain; parse its events; do NOT append
	if err != nil {
		return fmt.Errorf("brain: %w", err)
	}

	// Fold the brain process's event stream back into the legacy {response,
	// declarations} shape: assistant chat.message(s) → response; everything else
	// (the *.declared events) → declarations, each already a {name, payload} event.
	var responses []string
	declarations := []map[string]any{}
	for _, e := range emitted {
		if e.Name == "chat.message" {
			var p struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}
			if json.Unmarshal(e.Payload, &p) == nil && p.Role == "assistant" {
				responses = append(responses, p.Content)
			}
			continue
		}
		declarations = append(declarations, map[string]any{
			"name":    e.Name,
			"payload": json.RawMessage(e.Payload),
		})
	}

	output := map[string]any{
		"response":     strings.Join(responses, "\n"),
		"declarations": declarations,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// brainCommand resolves the brain process to spawn. $SELF_BRAIN overrides it
// (its first word is the executable, the rest are leading args, then the prompt
// is appended as one final argument); the default is self's own `brain` mode,
// which wraps the in-tree LLM compiler. The prompt is always passed as a single
// argv element so multi-word prompts survive without re-quoting.
func brainCommand(prompt string) (string, []string) {
	if v := strings.TrimSpace(os.Getenv("SELF_BRAIN")); v != "" {
		parts := strings.Fields(v)
		return parts[0], append(parts[1:], prompt)
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "self"
	}
	return exe, []string{"brain", prompt}
}

// cmdBrain is the *default* brain process — the one self ships, behind the same
// stdin/stdout contract any replacement must honor. It wraps the in-tree LLM
// compiler (CallBrain): explore the garden, optionally act by running commands,
// optionally grow by declaring capabilities. Its whole effect is the JSONL event
// stream it writes to stdout: the user's message and the brain's reply as
// chat.message events, then any declarations verbatim. The kernel (cmdThink)
// ingests that stream like any command's output. Because it is just a process,
// it can be replaced wholesale via $SELF_BRAIN.
func cmdBrain(home string, prompt string) error {
	if prompt == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		return fmt.Errorf("usage: self brain <prompt> (or pipe prompt on stdin)")
	}

	// Resolve the page-configured brain (provider/url/model from the log, token
	// from the key file) into SELF_LLM_* — unless those are already set, so an
	// explicit env override still wins. This is what makes the setup page drive
	// the brain.
	provider := loadBrainConfig(home)

	// Human-in-the-loop brain: there is no LLM to call. Park the prompt as a
	// brain.asked event the human answers later via /interview, and reply with a
	// placeholder so the caller's chat shows the question is pending. We append
	// brain.asked DIRECTLY here rather than emitting it on stdout, because the
	// brain is reached through `self think`, which is a pure query (it returns the
	// reply but appends nothing) — so the parked question must persist itself.
	// Async by design: the "thought continues" when the answer event flows
	// through the normal pipeline, not when this process resumes.
	if provider == "human" {
		ask, _ := json.Marshal(map[string]any{"id": newAskID(), "prompt": prompt})
		e := event.New(event_BrainAsked, ask)
		if aErr := store.Open(home).Append(&e); aErr != nil {
			return aErr
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"name": "chat.message", "payload": map[string]any{
			"role": "assistant", "content": "🧑‍💻 parked for the human brain — open /interview to answer",
		}})
	}

	compiler := seed.NewCompiler(home)
	commands, invoke := brainTools(home)
	result, err := compiler.CallBrain(prompt, commands, invoke)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	emit := func(name string, payload any) error {
		return enc.Encode(map[string]any{"name": name, "payload": payload})
	}
	if err := emit("chat.message", map[string]any{"role": "user", "content": prompt}); err != nil {
		return err
	}
	if strings.TrimSpace(result.Response) != "" {
		if err := emit("chat.message", map[string]any{"role": "assistant", "content": result.Response}); err != nil {
			return err
		}
	}
	for _, d := range result.Declarations {
		name, _ := d["name"].(string)
		if name == "" || d["payload"] == nil {
			continue
		}
		if err := emit(name, d["payload"]); err != nil {
			return err
		}
	}
	return nil
}

// cmdTeach installs an operator-authored capability: the human writes the script
// (read from stdin) and the kernel signs + installs it. This is the "human is the
// compiler" path — code enters via a kernel verb the person at the keyboard runs,
// not via the LLM and not via a command-emitted event.
//
//	self teach command  <name>                          < script
//	self teach projector <name> evt.a,evt.b              < script   (consumes for wiring)
//	self teach command  <name> --examples=ex.json        < script   (verified before install)
func cmdTeach(home string, args []string) error {
	// Pull the optional --examples=<file> flag out of the positional args.
	var examplesFile string
	var pos []string
	for _, a := range args {
		if strings.HasPrefix(a, "--examples=") {
			examplesFile = strings.TrimPrefix(a, "--examples=")
			continue
		}
		pos = append(pos, a)
	}
	if len(pos) < 2 {
		return fmt.Errorf("usage: self teach <command|projector> <name> [consumes-csv] [--examples=file]  (script on stdin)")
	}
	kind, name := pos[0], pos[1]
	var consumes []string
	if len(pos) > 2 {
		for _, c := range strings.Split(pos[2], ",") {
			if c = strings.TrimSpace(c); c != "" {
				consumes = append(consumes, c)
			}
		}
	}

	var examples []seed.Example
	if examplesFile != "" {
		raw, err := os.ReadFile(examplesFile)
		if err != nil {
			return fmt.Errorf("read examples: %w", err)
		}
		if err := json.Unmarshal(raw, &examples); err != nil {
			return fmt.Errorf("parse examples (want a JSON array of {note,args,events,expect_contains}): %w", err)
		}
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	if err := kernel.Teach(home, kind, name, consumes, string(data), examples); err != nil {
		return err
	}
	gate := ""
	if len(examples) > 0 {
		gate = fmt.Sprintf(" (passed %d example(s))", len(examples))
	}
	fmt.Printf("taught %s %q — operator-authored, signed + installed%s\n", kind, name, gate)
	return nil
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
