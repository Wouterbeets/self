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

// feedText writes a plain-text string to a process's stdin and closes it. Used
// for the mind, which receives an orientation brief — not the raw log — so it
// reads where to look, then explores SELF_HOME itself with its own tools
// instead of being force-fed a firehose of events.
func feedText(stdin io.WriteCloser, text string) {
	go func() {
		io.WriteString(stdin, text)
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
// instance has learned projections that surface them.
//
// A consequence: a mind that cannot inspect files under SELF_HOME — a plain
// stdin/stdout API adapter with no tools — cannot do the job. The kernel's
// process seam is still a pipe (brief on stdin; stdout parsed on exit), but a
// real mind needs a tool loop on its side of it. How an adapter turns tools or
// APIs into that process stdout is adapter-local (see examples/). The kernel
// does not sandbox or supply tools.
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
	fmt.Fprintf(&b, "Log: %d events. Set `SELF_MIND_ID` when you author — receipts record it.\n\n", len(events))

	b.WriteString("## How you act\n\n")
	b.WriteString("State that survives is only what lands in `events.jsonl`. The log is append-only.\n\n")
	b.WriteString("- **Read** — open files under this instance: `site/*.html` (rendered state a human sees), `events.jsonl` (authoritative log), `capabilities/` (installed scripts).\n")
	b.WriteString("- **Write (commands)** — prefer installed verbs: `self run <command> …` (or HTTP `POST /run/<command>` when serving). Args follow each command below.\n")
	b.WriteString("- **Write (events)** — when this ask expects you to persist directly, emit domain events as this process's stdout (one compact JSON object per line: `{\"name\":\"…\",\"payload\":{…}}`). Do not edit `events.jsonl` yourself; do not install scripts yourself.\n")
	b.WriteString("- **Extend** — emit `command.declared` / `projector.declared` the same way when this ask is learn/reflect (or declare is warranted). The kernel compiles and signs; you only author.\n")
	b.WriteString("- **Query** — `think` is report-only: the kernel returns your reply and does not append from it.\n\n")
	b.WriteString("When the kernel spawned you, it reads **only this process's stdout** after you exit. How your adapter turns tools or API calls into that stdout is adapter-local — see `self protocol` and `examples/`.\n\n")

	if len(events) == 0 {
		b.WriteString("## Empty log\n\n")
		b.WriteString("Nothing installed yet. Learn an account: `self learn <account>` (try `lessons/journal`).\n")
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
	b.WriteString("- Account exchange: `self give` / `self learn` (Account Protocol) — not required for ordinary run/think\n")
	b.WriteString("- Reconstruction: `self rehydrate` rebuilds `capabilities/` + `site/` from the log + `.secret` (no mind)\n")
	return b.String()
}

// renderBriefFile writes the orientation brief to SELF_HOME/site/brief.md,
// the kernel-resident surface a mind reads. Called alongside renderKernelHTML
// whenever the log changes, and re-run immediately before every mind ask (see
// freshBrief) so a mind never reads stale orientation. Served verbatim as
// text/plain like any other .md file under site/.
func renderBriefFile(home string) {
	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	os.WriteFile(filepath.Join(siteDir, "brief.md"), []byte(stateBrief(home)), 0644)
}

// freshBrief writes the orientation brief to disk and returns the exact bytes
// the kernel just wrote. Used by pipeMind so the mind is always fed the
// current state of the instance — never a cached file that could grow stale if
// the log changed outside the normal refresh path (e.g. a CLI `run` between a
// serve request and a mind call). The disk is the source; the mind can read
// the same file itself to explore. Write then read back would be redundant —
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
		return "", fmt.Errorf("%s %q does not match its latest verified receipt; run self rehydrate", typ, name)
	}
	return bin, nil
}

func runCommand(home, command string, args []string) ([]Event, error) {
	bin, err := verifyInstalledScript(home, "command", command)
	if err != nil {
		return nil, err
	}
	evs, err := pipeProcess(home, bin, args)
	if err != nil {
		return nil, err
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
