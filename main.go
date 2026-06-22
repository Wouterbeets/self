package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
  KS_LLM_URL     llm api base url (auto-detected from opencode-go)
  KS_LLM_API_KEY llm api key (auto-detected from opencode-go)
  KS_LLM_MODEL   llm model name (auto-detected from opencode-go)
  KS_LLM_STUB    set to "1" to force stub scripts

If KS_LLM_* is unset, ks reads opencode-go credentials from
~/.local/share/opencode/auth.json and uses the opencode-go endpoint
(https://opencode.ai/zen/go, model glm-5.2). Set KS_LLM_STUB=1 to
force stub scripts without calling the LLM.

kernel-known events:
  kernel.initialized   written by ks init
  command.declared     compiled by ks plant AND ks invoke (self-improvement)
  projector.declared   compiled by ks plant AND ks invoke (self-improvement)
  script.compiled      written by ks plant/invoke — logs compiled script for rollback
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

	for _, cmd := range manifest.Commands {
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

func cmdInvoke(home string, command string, args []string) error {
	scriptPath := filepath.Join(home, "registry", "commands", command)
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("command %q not found (plant a seed that declares it)", command)
	}

	st := store.Open(home)
	current, err := st.Read()
	if err != nil {
		return err
	}

	cmd := exec.Command(scriptPath, args...)
	cmd.Env = append(os.Environ(), "KS_HOME="+home)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	go func() {
		w := bufio.NewWriter(stdin)
		for _, e := range current {
			line, _ := json.Marshal(e)
			w.Write(line)
			w.WriteByte('\n')
		}
		w.Flush()
		stdin.Close()
	}()

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
			return fmt.Errorf("command output parse error: %w", err)
		}
		if partial.Name == "" {
			return fmt.Errorf("command output missing event name: %s", line)
		}
		e := event.New(partial.Name, partial.Payload)
		newEvents = append(newEvents, e)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command exited: %w", err)
	}

	for i := range newEvents {
		if err := st.Append(&newEvents[i]); err != nil {
			return err
		}
		fmt.Printf("%s appended seq %d %s\n", newEvents[i].ID, newEvents[i].Seq, newEvents[i].Name)
	}

	// Strange-loop hook: if the command emitted any command.declared or
	// projector.declared events, compile them now. This is how chat (or any
	// command) plants new capabilities — including re-declaring itself.
	// Latest declaration wins; the event log keeps every version for audit.
	compiledCmds, compiledProjs, err := kernel.CompileDeclarations(home, newEvents)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ks: warning: declaration compile failed: %s\n", err)
	}
	if len(compiledCmds) > 0 || len(compiledProjs) > 0 {
		fmt.Printf("self-improved: %d command(s), %d projector(s) compiled from invoke\n",
			len(compiledCmds), len(compiledProjs))
	}

	// Auto-run projectors that consume the new events.
	// The kernel reads its own projection (site/kernel.html) to determine
	// which projectors care about which events. Burn kernel.html, replay
	// events, it comes back — the projection IS the aggregate.
	wiring, err := kernel.ReadWiring(home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ks: warning: could not read kernel wiring: %s\n", err)
		return nil
	}
	ran := map[string]bool{}
	for _, e := range newEvents {
		for _, projName := range wiring.ProjectorsForEvent(e.Name) {
			if ran[projName] {
				continue
			}
			ran[projName] = true
			fmt.Printf("auto-running projector %q...\n", projName)
			if err := runProjectorToSite(home, projName); err != nil {
				fmt.Fprintf(os.Stderr, "ks: projector %q failed: %s\n", projName, err)
			}
		}
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

	return cmd.Wait()
}

func cmdServe(home string, port string) error {
	if port == "" {
		port = "7777"
	}

	os.MkdirAll(filepath.Join(home, "site"), 0755)

	mux := http.NewServeMux()

	// / serves kernel.html (the system's self-description and wiring diagram).
	// Other site/ files are served at their paths.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(home, "site", "kernel.html"))
			return
		}
		http.FileServer(http.Dir(filepath.Join(home, "site"))).ServeHTTP(w, r)
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
		go func() {
			bw := bufio.NewWriter(stdin)
			for _, e := range events {
				line, _ := json.Marshal(e)
				bw.Write(line)
				bw.WriteByte('\n')
			}
			bw.Flush()
			stdin.Close()
		}()
		c.Wait()
	})

	// /events — raw events.jsonl.
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(home, "events.jsonl"))
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
	result, err := compiler.CallBrain(prompt)
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
