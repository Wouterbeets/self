package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ks/internal/event"
)

type Manifest struct {
	Name        string       `json:"-"`
	Description string       `json:"-"`
	Lineage     string       `json:"-"`
	Version     string       `json:"-"`
	SeedDir     string       `json:"-"`
	Trios       []Trio       `json:"-"`
	Events      []event.Event `json:"-"`
}

type Trio struct {
	Name      string          `json:"name"`
	Command   CommandDecl     `json:"command"`
	Event     EventDecl       `json:"event"`
	Projector ProjectorDecl   `json:"projector"`
}

type CommandDecl struct {
	Description string            `json:"description"`
	Params      map[string]string `json:"params"`
}

type EventDecl struct {
	Name   string            `json:"name"`
	Fields map[string]string `json:"fields"`
}

type ProjectorDecl struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Consumes    []string `json:"consumes"`
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

		if e.Name == event.TrioDeclared {
			var trio Trio
			if err := json.Unmarshal(e.Payload, &trio); err != nil {
				return nil, fmt.Errorf("parse trio.declared payload: %w", err)
			}
			m.Trios = append(m.Trios, trio)
		}
	}

	if len(m.Events) == 0 {
		return nil, fmt.Errorf("seed %q has no events", m.Name)
	}

	return m, nil
}
