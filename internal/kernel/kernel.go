package kernel

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ks/internal/event"
	"ks/internal/seed"
)

const PipeContract = `command script: receives args as argv, current events as JSONL on stdin, writes new events as JSONL on stdout (one JSON object per line, fields: name, payload). The kernel assigns id, seq, occurred_at.
projector script: receives all events as JSONL on stdin, writes HTML on stdout. The kernel persists the output to KS_HOME/site/<projector_name>.html.
The kernel sets the KS_HOME env var on every script. Commands may write persistent structured state to $KS_HOME/artefacts/<name>.json.
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

// RenderHTML reads the event log, extracts all command.declared,
// projector.declared, and seed.planted events, and writes the kernel
// wiring as HTML to KS_HOME/site/kernel.html. Called at init and plant.
func RenderHTML(home string) error {
	st := openEventStore(home)
	events, err := st.Read()
	if err != nil {
		return err
	}

	var commands []CommandInfo
	var projectors []ProjectorInfo
	var seeds []SeedInfo

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
		}
	}

	html := buildHTML(commands, projectors, seeds)

	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	return os.WriteFile(filepath.Join(siteDir, "kernel.html"), []byte(html), 0644)
}

func buildHTML(commands []CommandInfo, projectors []ProjectorInfo, seeds []SeedInfo) string {
	var b strings.Builder

	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<title>ks kernel</title>\n")
	b.WriteString("<style>\n")
	b.WriteString("body { font-family: -apple-system, sans-serif; margin: 20px; background: #fafafa; color: #222; }\n")
	b.WriteString("h1 { margin-bottom: 4px; }\n")
	b.WriteString("h2 { margin-top: 32px; border-bottom: 1px solid #ddd; padding-bottom: 6px; }\n")
	b.WriteString(".version { color: #888; margin-top: 0; }\n")
	b.WriteString("article { background: white; border: 1px solid #e0e0e0; border-radius: 6px; padding: 12px 16px; margin: 8px 0; }\n")
	b.WriteString("article h3 { margin: 0 0 4px 0; font-family: monospace; }\n")
	b.WriteString("article p { margin: 4px 0; color: #555; }\n")
	b.WriteString("dl { margin: 4px 0; }\n")
	b.WriteString("dt { font-family: monospace; font-weight: bold; color: #333; }\n")
	b.WriteString("dd { margin-left: 16px; color: #666; }\n")
	b.WriteString("ul.consumes { list-style: none; padding: 0; }\n")
	b.WriteString("ul.consumes li { font-family: monospace; font-size: 13px; padding: 2px 0; }\n")
	b.WriteString(".tag { display: inline-block; background: #e8f0fe; border-radius: 3px; padding: 1px 6px; font-size: 12px; font-family: monospace; }\n")
	b.WriteString("pre { background: #f4f4f4; border: 1px solid #e0e0e0; border-radius: 4px; padding: 12px; overflow-x: auto; font-size: 13px; }\n")
	b.WriteString("details { margin: 8px 0; }\n")
	b.WriteString("summary { cursor: pointer; font-weight: bold; color: #555; }\n")
	b.WriteString(".wiring-table { width: 100%; border-collapse: collapse; margin: 8px 0; }\n")
	b.WriteString(".wiring-table th { background: #4a5568; color: white; padding: 6px 10px; text-align: left; }\n")
	b.WriteString(".wiring-table td { border: 1px solid #ddd; padding: 6px 10px; font-family: monospace; font-size: 13px; }\n")
	b.WriteString("</style>\n")
	b.WriteString("</head>\n<body>\n")

	// Identity
	b.WriteString("<h1>ks kernel</h1>\n")
	b.WriteString("<p class=\"version\">ks/v0</p>\n")

	// Known events
	b.WriteString("<section id=\"identity\">\n")
	b.WriteString("<h2>kernel identity</h2>\n")
	b.WriteString("<dl>\n")
	b.WriteString("<dt>kernel.initialized</dt><dd>written by ks init</dd>\n")
	b.WriteString("<dt>command.declared</dt><dd>read by ks plant to compile commands</dd>\n")
	b.WriteString("<dt>projector.declared</dt><dd>read by ks plant to compile projectors</dd>\n")
	b.WriteString("<dt>seed.planted</dt><dd>written by ks plant as a receipt</dd>\n")
	b.WriteString("</dl>\n")

	// Pipe contract
	b.WriteString("<h3>pipe contract</h3>\n")
	b.WriteString("<pre>")
	b.WriteString(html.EscapeString(PipeContract))
	b.WriteString("</pre>\n")

	// System prompts
	b.WriteString("<details class=\"system-prompt\" data-type=\"command\">\n")
	b.WriteString("<summary>command system prompt (used at plant time)</summary>\n")
	b.WriteString("<pre>")
	b.WriteString(html.EscapeString(seed.CommandSystemPrompt))
	b.WriteString("</pre>\n")
	b.WriteString("</details>\n")
	b.WriteString("<details class=\"system-prompt\" data-type=\"projector\">\n")
	b.WriteString("<summary>projector system prompt (used at plant time)</summary>\n")
	b.WriteString("<pre>")
	b.WriteString(html.EscapeString(seed.ProjectorSystemPrompt))
	b.WriteString("</pre>\n")
	b.WriteString("</details>\n")
	b.WriteString("</section>\n")

	// Commands
	b.WriteString("<section id=\"commands\">\n")
	b.WriteString("<h2>commands</h2>\n")
	if len(commands) == 0 {
		b.WriteString("<p>No commands planted yet.</p>\n")
	}
	for _, cmd := range commands {
		b.WriteString(fmt.Sprintf("<article class=\"command\" data-name=%q data-event=%q>\n", cmd.Name, cmd.Event))
		b.WriteString(fmt.Sprintf("<h3>%s</h3>\n", html.EscapeString(cmd.Name)))
		b.WriteString(fmt.Sprintf("<p>%s</p>\n", html.EscapeString(cmd.Description)))
		b.WriteString(fmt.Sprintf("<p>produces event: <span class=\"tag\">%s</span></p>\n", html.EscapeString(cmd.Event)))
		if len(cmd.Params) > 0 {
			b.WriteString("<dl class=\"params\">\n")
			for k, v := range cmd.Params {
				b.WriteString(fmt.Sprintf("<dt>%s</dt><dd>%s</dd>\n", html.EscapeString(k), html.EscapeString(v)))
			}
			b.WriteString("</dl>\n")
		}
		b.WriteString("</article>\n")
	}
	b.WriteString("</section>\n")

	// Projectors
	b.WriteString("<section id=\"projectors\">\n")
	b.WriteString("<h2>projectors</h2>\n")
	if len(projectors) == 0 {
		b.WriteString("<p>No projectors planted yet.</p>\n")
	}
	for _, proj := range projectors {
		b.WriteString(fmt.Sprintf("<article class=\"projector\" data-name=%q>\n", proj.Name))
		b.WriteString(fmt.Sprintf("<h3>%s</h3>\n", html.EscapeString(proj.Name)))
		b.WriteString(fmt.Sprintf("<p>%s</p>\n", html.EscapeString(proj.Description)))
		b.WriteString("<ul class=\"consumes\">\n")
		for _, c := range proj.Consumes {
			b.WriteString(fmt.Sprintf("<li data-event=%q>%s</li>\n", c, html.EscapeString(c)))
		}
		b.WriteString("</ul>\n")
		b.WriteString("</article>\n")
	}
	b.WriteString("</section>\n")

	// Wiring table
	b.WriteString("<section id=\"wiring\">\n")
	b.WriteString("<h2>wiring</h2>\n")
	if len(commands) > 0 && len(projectors) > 0 {
		b.WriteString("<table class=\"wiring-table\">\n")
		b.WriteString("<tr><th>command</th><th>produces event</th><th>consumed by projectors</th></tr>\n")
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
				html.EscapeString(cmd.Name),
				html.EscapeString(cmd.Event),
				html.EscapeString(consumedStr)))
		}
		b.WriteString("</table>\n")
	} else {
		b.WriteString("<p>No wiring yet — plant commands and projectors.</p>\n")
	}
	b.WriteString("</section>\n")

	// Seeds
	b.WriteString("<section id=\"seeds\">\n")
	b.WriteString("<h2>seeds</h2>\n")
	if len(seeds) == 0 {
		b.WriteString("<p>No seeds planted yet.</p>\n")
	}
	for _, s := range seeds {
		b.WriteString(fmt.Sprintf("<article class=\"seed\" data-name=%q data-seq=%q>\n", s.Name, fmt.Sprintf("%d", s.Seq)))
		b.WriteString(fmt.Sprintf("<h3>%s</h3>\n", html.EscapeString(s.Name)))
		if len(s.Commands) > 0 {
			b.WriteString(fmt.Sprintf("<p>commands: %s</p>\n", html.EscapeString(strings.Join(s.Commands, ", "))))
		}
		if len(s.Projectors) > 0 {
			b.WriteString(fmt.Sprintf("<p>projectors: %s</p>\n", html.EscapeString(strings.Join(s.Projectors, ", "))))
		}
		b.WriteString(fmt.Sprintf("<p>planted at seq %d</p>\n", s.Seq))
		b.WriteString("</article>\n")
	}
	b.WriteString("</section>\n")

	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// CompileDeclarations scans events for command.declared and
// projector.declared, compiles any it finds via the LLM compiler,
// writes the scripts to the registry, and re-renders kernel.html.
// Latest declaration wins — a re-declaration overwrites the script.
// Returns the names of commands and projectors compiled.
//
// This is the strange-loop hook: a command (e.g. chat) can emit
// declarations for new commands/projectors, and the kernel compiles
// them on the fly. The event log keeps every version for audit;
// the registry holds only the latest.
func CompileDeclarations(home string, events []event.Event) (commands, projectors []string, err error) {
	compiler := seed.NewCompiler()
	registry := filepath.Join(home, "registry")

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
			if wErr := seed.WriteCommandScript(registry, cmd.Name, script); wErr != nil {
				return nil, nil, wErr
			}
			fmt.Printf(" planted\n")
			commands = append(commands, cmd.Name)

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
			if wErr := seed.WriteProjectorScript(registry, proj.Name, script); wErr != nil {
				return nil, nil, wErr
			}
			fmt.Printf(" planted\n")
			projectors = append(projectors, proj.Name)
		}
	}

	if len(commands) > 0 || len(projectors) > 0 {
		if rErr := RenderHTML(home); rErr != nil {
			return commands, projectors, fmt.Errorf("re-render kernel.html: %w", rErr)
		}
	}
	return commands, projectors, nil
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
