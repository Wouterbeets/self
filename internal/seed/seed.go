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
	// Examples are receiver-checkable conformance tests: input → output-must-contain
	// assertions that define what the capability MUST do, independent of how it is
	// implemented. The kernel runs the freshly compiled binary against them before
	// installing it. A seed's examples are a portable contract — a receiver that
	// recompiles the seed to its own vocabulary must still satisfy them, which is
	// what makes cross-node sharing "verify the result" rather than "trust the
	// compiler."
	Examples []Example `json:"examples,omitempty"`
}

// Example is one conformance test for a compiled script. The script is run with
// Args as argv and Events (as JSONL) on stdin; its stdout must contain every
// string in ExpectContains. Commands emit JSONL events on stdout and projectors
// emit HTML, so the substring contract works uniformly for both.
type Example struct {
	Note           string            `json:"note,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Events         []json.RawMessage `json:"events,omitempty"`
	ExpectContains []string          `json:"expect_contains,omitempty"`
	// ExpectOrder asserts these substrings appear in stdout in this order (each
	// present, and each no earlier than the previous). It proves a *ranking* or
	// sequence, which mere presence (ExpectContains) cannot — e.g. that a hotspots
	// projector lists the worst place first, not just that both places appear.
	ExpectOrder []string `json:"expect_order,omitempty"`
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
	// Examples: same contract as Command.Examples — input → output-must-contain
	// conformance tests the compiled projector must pass before it installs.
	Examples []Example `json:"examples,omitempty"`
}

// IntentSeed is the genotype: the prose intent for a product surface plus its
// invariants (the fitness function). It carries no decomposition — the
// orchestrator grows commands/projectors from the intent against the garden.
type IntentSeed struct {
	Name       string
	Intent     string
	Invariants []Invariant
}

// Invariant is one must-hold for a grown surface, written in the receiver's
// vocabulary. Machine-checkable ones run like Examples against the grown binary;
// Brain ones depend on the brain (non-deterministic output) and are checked live
// during growth rather than by static replay, with Asserts stating the contract.
type Invariant struct {
	Name           string            `json:"name,omitempty"`
	Capability     string            `json:"capability,omitempty"` // the surface name it falls on
	Kind           string            `json:"kind,omitempty"`       // command | projector
	Args           []string          `json:"args,omitempty"`
	Events         []json.RawMessage `json:"events,omitempty"`
	ExpectContains []string          `json:"expect_contains,omitempty"`
	ExpectOrder    []string          `json:"expect_order,omitempty"`
	Brain          bool              `json:"brain,omitempty"`
	Asserts        string            `json:"asserts,omitempty"`
	Note           string            `json:"note,omitempty"`
}

// Example returns the machine-checkable form of an invariant, or nil if it is
// brain-dependent (checked live, not by static replay).
func (iv Invariant) Example() *Example {
	if iv.Brain {
		return nil
	}
	return &Example{Note: iv.Note, Args: iv.Args, Events: iv.Events,
		ExpectContains: iv.ExpectContains, ExpectOrder: iv.ExpectOrder}
}

// HasIntent reports whether dir is an intent seed (a genotype) rather than a
// legacy events.jsonl seed (a pre-decomposed parts list).
func HasIntent(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "intent.md"))
	return err == nil
}

// LoadIntent reads an intent seed: intent.md (the genotype) + optional
// invariants.jsonl (the fitness function).
func LoadIntent(dir string) (*IntentSeed, error) {
	data, err := os.ReadFile(filepath.Join(dir, "intent.md"))
	if err != nil {
		return nil, fmt.Errorf("read intent.md: %w", err)
	}
	s := &IntentSeed{Name: filepath.Base(dir), Intent: strings.TrimSpace(string(data))}
	if raw, err := os.ReadFile(filepath.Join(dir, "invariants.jsonl")); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var iv Invariant
			if err := json.Unmarshal([]byte(line), &iv); err != nil {
				return nil, fmt.Errorf("parse invariant: %w", err)
			}
			s.Invariants = append(s.Invariants, iv)
		}
	}
	return s, nil
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
