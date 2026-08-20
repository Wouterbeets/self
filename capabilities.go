package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// feedEvents writes events to a process's stdin as JSONL and closes it.
func feedEvents(stdin io.WriteCloser, events []Event) {
	go func() {
		enc := json.NewEncoder(stdin)
		for i := range events {
			enc.Encode(events[i])
		}
		stdin.Close()
	}()
}

// declaredCaps replays the log into the currently declared commands and
// projectors, each in first-declared order. The shared walk behind both the
// orientation brief and the kernel index — the log is the only source, so both
// see exactly the same capabilities in the same order. A capability.retired
// tombstone delists its target; a declaration after the tombstone lists it
// again, freshly ordered.
func declaredCaps(events []Event) (commands map[string]commandDecl, cmdOrder []string, projectors map[string]projectorDecl, projOrder []string) {
	commands = map[string]commandDecl{}
	projectors = map[string]projectorDecl{}
	drop := func(order []string, name string) []string {
		out := order[:0]
		for _, n := range order {
			if n != name {
				out = append(out, n)
			}
		}
		return out
	}
	for _, e := range events {
		switch e.Name {
		case "command.declared":
			var d commandDecl
			if json.Unmarshal(e.Payload, &d) == nil && d.Name != "" {
				if _, ok := commands[d.Name]; !ok {
					cmdOrder = append(cmdOrder, d.Name)
				}
				commands[d.Name] = d
			}
		case "projector.declared":
			var d projectorDecl
			if json.Unmarshal(e.Payload, &d) == nil && d.Name != "" {
				if _, ok := projectors[d.Name]; !ok {
					projOrder = append(projOrder, d.Name)
				}
				projectors[d.Name] = d
			}
		case "capability.retired":
			d, ok := parseRetirement(e.Payload)
			if !ok {
				continue
			}
			switch d.Type {
			case "command":
				delete(commands, d.Name)
				cmdOrder = drop(cmdOrder, d.Name)
			case "projector":
				delete(projectors, d.Name)
				projOrder = drop(projOrder, d.Name)
			}
		}
	}
	return commands, cmdOrder, projectors, projOrder
}

// stateBrief is the kernel's wake-up card for a mind: Layer 0 orientation —
// mechanism + a generated catalog of what exists — not a log digest and not
// philosophy. It tells the mind where it is, how write/extend work, what
// commands and projections are installed, and where depth lives on disk.
// Values and "open when" guidance never live here; they appear only if this
// instance has learned projections that surface them. It opens every prompt
// the ask face of the pipe emits.
//
// A consequence: a mind that cannot inspect files under SELF_HOME — a bare
// stdin/stdout text transform with no tools — cannot do the whole job. The
// seam is a shell pipe; the tool loop is the mind's own concern, never the
// kernel's. The kernel does not sandbox or supply tools.
//
// The kernel materializes the brief to SELF_HOME/site/brief.md (see
// renderBriefFile) so it is explorable on disk like every other piece of
// state. Markdown on purpose — readable as plain text to a mind, to `cat`, and
// served verbatim as text/plain like any other .md file under site/.
func stateBrief(home string) string {
	events, err := readEvents(home)
	if err != nil {
		// a corrupt log is the kernel's failure, not the mind's; surface it
		return fmt.Sprintf("# self — orientation brief\n\nInstance: `%s`\n\n**ERROR reading the log:** %s\n", home, err)
	}
	commands, cmdOrder, projectors, projOrder := declaredCaps(events)

	oneLine := func(s string) string {
		return strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	}
	fmtMap := func(m map[string]string) string {
		if len(m) == 0 {
			return ""
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+":"+m[k])
		}
		return strings.Join(parts, ", ")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# self — orientation brief\n\n")
	fmt.Fprintf(&b, "Instance: `%s`\n", home)
	fmt.Fprintf(&b, "Log: %d events.\n\n", len(events))

	b.WriteString("## How you act\n\n")
	b.WriteString("State that survives is only what lands in `events.jsonl`. The log is append-only.\n\n")
	b.WriteString("- **Read** — `self show <projection>` renders any projection to stdout from the current log; the same pages live at `site/*.html` and over HTTP when serving, so a mind and a human always read the identical surface. `events.jsonl` is the authoritative log; `capabilities/` holds the installed scripts. Reads go through projections. Do not implement a query as a command: a command's output is appended to the log, so routing reads through commands fills the log with render artifacts. If the view you need is missing, author a projector — it is a short script that receives events as JSONL on stdin and prints near-plain HTML.\n")
	b.WriteString("- **Write (commands)** — prefer installed verbs: `self run <command> …` (or HTTP `POST /run/<command>` when serving). Args follow each command below.\n")
	b.WriteString("- **Write** — use installed verbs: `self run <command> …`. A mind's stdout is prose only; the final `self` records it as one `self.replied`. Pure event JSONL is reserved for explicit low-level capability authoring. Do not edit `events.jsonl` yourself.\n")
	b.WriteString("- **Extend** — emit `command.declared` / `projector.declared` the same way, and each script as `script.authored`. Only the kernel installs, under a receipt it signs.\n")
	b.WriteString("- **Summarize** — when a thread's event history stops informing decisions, record an authored summary of the current state through an installed command, so projections can lead with it and readers stop paying for superseded detail. The log keeps everything; summaries are for the readers, not the record.\n")
	b.WriteString("- **The loop** — `echo \"<ask>\" | self | <mind> | self`: prose becomes a situated prompt; the mind uses commands for durable work; the final self records its prose summary. `self protocol` prints the contracts.\n\n")

	if len(events) == 0 {
		b.WriteString("## Empty log\n\n")
		b.WriteString("Nothing installed yet. Learn an account: `self learn lessons/journal | claude -p | self`.\n")
		return b.String()
	}

	if len(cmdOrder) > 0 {
		b.WriteString("## Commands\n\n")
		for _, n := range cmdOrder {
			d := commands[n]
			fmt.Fprintf(&b, "- `%s` — %s\n", n, oneLine(d.Description))
			fmt.Fprintf(&b, "  - run: `self run %s …`\n", n)
			if d.Event.Name != "" {
				fields := fmtMap(d.Event.Fields)
				if fields != "" {
					fmt.Fprintf(&b, "  - emits: `%s` — fields: %s\n", d.Event.Name, fields)
				} else {
					fmt.Fprintf(&b, "  - emits: `%s`\n", d.Event.Name)
				}
			}
			if params := fmtMap(d.Params); params != "" {
				fmt.Fprintf(&b, "  - params: %s\n", params)
			}
		}
		b.WriteString("\n")
	}

	if len(projOrder) > 0 {
		b.WriteString("## Projections\n\n")
		for _, n := range projOrder {
			d := projectors[n]
			consumes := strings.Join(d.Consumes, ", ")
			if consumes == "" {
				consumes = "—"
			}
			fmt.Fprintf(&b, "- `/%s` — %s → `site/%s.html` (consumes %s)\n",
				n, oneLine(d.Description), n, consumes)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Depth (optional)\n\n")
	b.WriteString("- `events.jsonl` — append-only log (authoritative)\n")
	b.WriteString("- `capabilities/` — installed command and projector scripts\n")
	b.WriteString("- `site/kernel.html` — full index, compiled-capability pipe contract, lifecycle events\n")
	b.WriteString("- Account exchange: `self give` / `self learn` (Account Protocol) — not required for ordinary asks\n")
	b.WriteString("- Reconstruction: `self rehydrate` rebuilds `capabilities/` + `site/` from the log + `.secret` (no mind)\n")
	return b.String()
}

// renderBriefFile writes the orientation brief to SELF_HOME/site/brief.md,
// the kernel-resident surface a mind reads. Called alongside renderKernelHTML
// whenever the log changes, and re-run immediately before every ask prompt
// (see freshBrief) so a mind never reads stale orientation. Served verbatim
// as text/plain like any other .md file under site/.
func renderBriefFile(home string) {
	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	os.WriteFile(filepath.Join(siteDir, "brief.md"), []byte(stateBrief(home)), 0644)
}

// freshBrief writes the orientation brief to disk and returns the exact bytes
// the kernel just wrote. Used by the ask face so a prompt always carries the
// current state of the instance — never a cached file that could grow stale if
// the log changed outside the normal refresh path (e.g. a CLI `run` between a
// serve request and an ask). The disk is the source; the mind can read the
// same file itself to explore. Write then read back would be redundant —
// stateBrief is deterministic, so the bytes written are the bytes returned.
func freshBrief(home string) string {
	brief := stateBrief(home)
	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	os.WriteFile(filepath.Join(siteDir, "brief.md"), []byte(brief), 0644)
	return brief
}

// pipeProcess runs an executable as a Unix pipeline node — the one shape the
// kernel uses to talk to any outside process, a compiled command or the mind.
func pipeProcess(home, bin string, argv []string) ([]Event, error) {
	current, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, argv...)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home)
	cmd.Dir = home
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

	var out []Event
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Only name and payload are read from a process's output: a script
		// that emits via/by is claiming a door, and doors are not claimable —
		// the caller of runCommand stamps what the kernel witnessed.
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
		out = append(out, newEvent(partial.Name, partial.Payload))
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("%s exited: %w", filepath.Base(bin), err)
	}
	return out, nil
}

// verifyInstalledScript keeps live execution behind the same trust gate as
// rehydrate: derived bytes must exactly match the latest live, locally signed
// receipt for this capability.
func verifyInstalledScript(home, typ, name string) (string, error) {
	events, err := readEvents(home)
	if err != nil {
		return "", err
	}
	secret, err := loadSecret(home)
	if err != nil {
		return "", err
	}
	var trusted string
	for _, e := range events {
		switch e.Name {
		case "script.compiled":
			if r, ok := verifiedReceipt(secret, e.Payload); ok && r.Type == typ && r.Name == name {
				trusted = r.Script
			}
		case "capability.retired":
			if r, ok := parseRetirement(e.Payload); ok && r.Type == typ && r.Name == name {
				trusted = ""
			}
		}
	}
	if trusted == "" {
		return "", fmt.Errorf("%s %q has no live verified script receipt", typ, name)
	}
	bin, err := scriptPath(home, typ, name)
	if err != nil {
		return "", err
	}
	installed, err := os.ReadFile(bin)
	if err != nil {
		return "", fmt.Errorf("%s %q not found: %w", typ, name, err)
	}
	if string(installed) != trusted {
		return "", fmt.Errorf("%s %q does not match its latest verified receipt. To restore the verified script, run self rehydrate. To change the script intentionally, pipe a script.authored event into self: {\"name\":\"script.authored\",\"payload\":{\"type\":\"%s\",\"name\":\"%s\",\"script\":\"<full script>\"}} — direct file edits are never trusted", typ, name, typ, name)
	}
	return bin, nil
}

// runCommand executes an installed command and ingests what it emits. via
// and by are the invocation's provenance — the door the kernel witnessed and
// the caller's claim — stamped onto every emitted event before it lands: a
// script's own output cannot set them.
func runCommand(home, command string, args []string, via, by string) ([]Event, error) {
	bin, err := verifyInstalledScript(home, "command", command)
	if err != nil {
		return nil, err
	}
	evs, err := pipeProcess(home, bin, args)
	if err != nil {
		return nil, err
	}
	for i := range evs {
		evs[i].Via, evs[i].By = via, by
	}
	return evs, ingest(home, evs)
}

type commandDecl struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Params      map[string]string `json:"params"`
	Event       struct {
		Name   string            `json:"name"`
		Fields map[string]string `json:"fields"`
	} `json:"event"`
	// Implementation is an optional reference the compiler verifies and adapts —
	// never installed as-is, so precision from the giver and receiver
	// adaptation both survive.
	Implementation string `json:"implementation,omitempty"`
	Revision       struct {
		Request     string `json:"request,omitempty"`
		FromReceipt string `json:"from_receipt,omitempty"`
	} `json:"revision,omitempty"`
}

type projectorDecl struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Consumes       []string `json:"consumes"`
	Implementation string   `json:"implementation,omitempty"`
	Revision       struct {
		Request     string `json:"request,omitempty"`
		FromReceipt string `json:"from_receipt,omitempty"`
	} `json:"revision,omitempty"`
}
