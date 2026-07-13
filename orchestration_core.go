package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ingest appends the events a process emitted, compiles any declarations among
// them (the strange loop), honors any retirements, and re-renders the
// projections that consume what just landed. Projections are pure replays, so
// re-running any of them is always correct; skipping one whose consumed events
// did not grow is the same page for free.
func ingest(home string, evs []Event) error {
	for i := range evs {
		if err := appendEvent(home, &evs[i]); err != nil {
			return err
		}
	}
	c := newLLM(home)
	if n := compileDeclarations(c, home, evs); n > 0 {
		fmt.Fprintf(os.Stderr, "self: self-improved — %d capabilit(ies) compiled\n", n)
	}
	if n := applyRetirements(home, evs); n > 0 {
		fmt.Fprintf(os.Stderr, "self: retired %d capabilit(ies)\n", n)
	}
	refreshSiteAfter(home, evs)
	return nil
}

// compileDeclarations is the strange-loop hook: every command.declared /
// projector.declared among evs is compiled by the LLM into a script authored
// for this receiver, installed, and logged as a signed receipt. Declaring IS
// creating — this runs at learn time and at run time alike, so a capability (or
// the brain) grows new capabilities just by emitting declarations.
func compileDeclarations(c *llm, home string, evs []Event) int {
	n := 0
	for _, e := range evs {
		var typ, name, script string
		var err error
		switch e.Name {
		case "command.declared":
			var d commandDecl
			if json.Unmarshal(e.Payload, &d) != nil || d.Name == "" {
				continue
			}
			typ, name = "command", d.Name
			fmt.Fprintf(os.Stderr, "self: compiling command %q…\n", name)
			script, err = c.compileCommand(d)
		case "projector.declared":
			var d projectorDecl
			if json.Unmarshal(e.Payload, &d) != nil || d.Name == "" {
				continue
			}
			typ, name = "projector", d.Name
			fmt.Fprintf(os.Stderr, "self: compiling projector %q…\n", name)
			script, err = c.compileProjector(d)
		default:
			continue
		}
		if err == nil {
			err = installScript(home, typ, name, script)
		}
		if err == nil {
			err = appendReceipt(home, typ, name, script, c.identity())
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "self: %s %q failed: %s\n", typ, name, err)
			continue
		}
		n++
	}
	return n
}

// applyRetirements honors capability.retired tombstones on the live path the
// way rehydrate honors them on replay: the installed script and any rendered
// page are removed at once, so disk never drifts from the log. The events all
// stay — a retired capability is one re-declaration away from coming back.
func applyRetirements(home string, evs []Event) int {
	n := 0
	for _, e := range evs {
		if e.Name != "capability.retired" {
			continue
		}
		d, ok := parseRetirement(e.Payload)
		if !ok {
			continue
		}
		p, err := scriptPath(home, d.Type, d.Name)
		if err != nil {
			continue
		}
		os.Remove(p)
		os.Remove(filepath.Dir(p)) // succeeds only when empty — a nested child's dirs survive
		if d.Type == "projector" {
			os.Remove(filepath.Join(home, "site", d.Name+".html"))
		}
		n++
	}
	return n
}

// brainResult carries the brain's response: the text it wrote, any events it
// declared, and (for compile asks) the script it authored.
type brainResult struct {
	Response string
	Events   []map[string]any
	Script   string // a compile ask's answer, from a script.authored event
}

// applyEvents appends events the brain returned and runs any capability
// declarations among them through the strange loop.
func applyEvents(home string, res *brainResult) {
	var evs []Event
	for _, d := range res.Events {
		name, _ := d["name"].(string)
		payload, _ := json.Marshal(d["payload"])
		if name == "" || string(payload) == "null" {
			continue
		}
		e := newEvent(name, payload)
		if err := appendEvent(home, &e); err != nil {
			fmt.Fprintf(os.Stderr, "self: append brain event: %s\n", err)
			return
		}
		evs = append(evs, e)
	}
	if len(evs) > 0 {
		c := newLLM(home)
		c.reasoning = res.Response
		n := compileDeclarations(c, home, evs)
		fmt.Fprintf(os.Stderr, "self: grew %d capabilit(ies)\n", n)
		if r := applyRetirements(home, evs); r > 0 {
			fmt.Fprintf(os.Stderr, "self: retired %d capabilit(ies)\n", r)
		}
		refreshSite(home)
	}
}
