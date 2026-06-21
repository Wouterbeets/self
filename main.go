package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ks/internal/event"
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
  plant <seed-dir>        compile trio.declared events, replay seed's events
  invoke <command> [args] run a registered command, append its events
  project [projector]     replay events through a projector, emit HTML
  log                     show the event log
  seeds                   list planted seeds (from seed.planted events)

environment:
  KS_HOME        ks home directory (default ~/.ks)
  KS_LLM_URL     llm api base url (default http://127.0.0.1:8080)
  KS_LLM_API_KEY llm api key (if unset, uses stub scripts)
  KS_LLM_MODEL   llm model name (default "local")

kernel-known events:
  kernel.initialized   written by ks init
  trio.declared        read by ks plant to know what to compile
  seed.planted         written by ks plant as a receipt
  everything else      comes from seeds
`)
}

func cmdInit(home string) error {
	if err := os.MkdirAll(filepath.Join(home, "registry", "commands"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(home, "registry", "projectors"), 0755); err != nil {
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
	return nil
}

func cmdPlant(home string, seedDir string) error {
	manifest, err := seed.Load(seedDir)
	if err != nil {
		return err
	}

	compiler := seed.NewCompiler()
	registry := filepath.Join(home, "registry")

	for _, trio := range manifest.Trios {
		fmt.Printf("compiling trio %q...", trio.Name)
		compiled, err := compiler.Compile(trio)
		if err != nil {
			fmt.Printf(" failed\n")
			return fmt.Errorf("trio %q: %w", trio.Name, err)
		}
		if err := seed.WriteScripts(registry, trio.Name, compiled); err != nil {
			return err
		}
		fmt.Printf(" planted\n")
	}

	st := store.Open(home)
	contentCount := 0
	for i := range manifest.Events {
		e := manifest.Events[i]
		if e.Name == event.TrioDeclared {
			continue
		}
		fresh := event.New(e.Name, e.Payload)
		if err := st.Append(&fresh); err != nil {
			return err
		}
		contentCount++
	}

	receiptPayload, _ := json.Marshal(map[string]any{
		"seed":           manifest.Name,
		"trios":          trioNames(manifest.Trios),
		"events_replayed": contentCount,
	})
	receipt := event.New(event.SeedPlanted, receiptPayload)
	if err := st.Append(&receipt); err != nil {
		return err
	}

	fmt.Printf("seed %q planted: %d trio(s), %d event(s) replayed, receipt seq %d\n",
		manifest.Name, len(manifest.Trios), contentCount, receipt.Seq)
	return nil
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

	scriptPath := filepath.Join(home, "registry", "projectors", name)
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("projector %q not found", name)
	}

	st := store.Open(home)
	events, err := st.Read()
	if err != nil {
		return err
	}

	cmd := exec.Command(scriptPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
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
			Trios          []string `json:"trios"`
			EventsReplayed int      `json:"events_replayed"`
		}
		json.Unmarshal(e.Payload, &rec)
		fmt.Printf("%s — trios: %s, events replayed: %d (seq %d)\n",
			rec.Seed, strings.Join(rec.Trios, ", "), rec.EventsReplayed, e.Seq)
		found = true
	}
	if !found {
		fmt.Println("(no seeds planted)")
	}
	return nil
}

func trioNames(trios []seed.Trio) []string {
	names := make([]string, len(trios))
	for i, t := range trios {
		names[i] = t.Name
	}
	return names
}
