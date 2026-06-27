package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"self/internal/event"
)

type Manifest struct {
	Name       string          `json:"-"`
	SeedDir    string          `json:"-"`
	Commands   []Command       `json:"-"`
	Projectors []ProjectorDecl `json:"-"`
	Events     []event.Event   `json:"-"`
}

type Command struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Params      map[string]string `json:"params"`
	Event       EventDecl         `json:"event"`
	// Implementation is an optional reference implementation. The compiler does
	// NOT install it as-is — it hands it to the LLM as a strong starting point to
	// verify against the pipe contract and adapt to the receiver's garden. So a
	// seed can carry precise, complex code while the loop still produces a binary
	// authored for this receiver (adaptation preserved, no foreign code run).
	Implementation string `json:"implementation,omitempty"`
}

type EventDecl struct {
	Name   string            `json:"name"`
	Fields map[string]string `json:"fields"`
}

type ProjectorDecl struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Consumes    []string `json:"consumes"`
	// Implementation: same contract as Command.Implementation — a reference the
	// compiler verifies and adapts, never installs verbatim.
	Implementation string `json:"implementation,omitempty"`
}

func Load(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read events.jsonl: %w", err)
	}

	m := &Manifest{
		SeedDir: dir,
		Name:    filepath.Base(dir),
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e event.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse event line: %w", err)
		}
		m.Events = append(m.Events, e)

		switch e.Name {
		case event.CommandDeclared:
			var cmd Command
			if err := json.Unmarshal(e.Payload, &cmd); err != nil {
				return nil, fmt.Errorf("parse command.declared payload: %w", err)
			}
			m.Commands = append(m.Commands, cmd)
		case event.ProjectorDeclared:
			var proj ProjectorDecl
			if err := json.Unmarshal(e.Payload, &proj); err != nil {
				return nil, fmt.Errorf("parse projector.declared payload: %w", err)
			}
			m.Projectors = append(m.Projectors, proj)
		}
	}

	if len(m.Events) == 0 {
		return nil, fmt.Errorf("seed %q has no events", m.Name)
	}

	return m, nil
}
