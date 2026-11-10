package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

//go:embed seed/entry
var seedEntry string

//go:embed seed/journal
var seedJournal string

const doorSeed = "seed"

var seedEnabled = true

type seedCap struct {
	typ    string
	decl   decl
	script string
}

var seedCaps = []seedCap{
	{kindCommand, decl{
		Name:        "entry",
		Summary:     "record one journal entry",
		Description: "usage: entry <text…>\n\nAppends one journal.entry {text}. Read entries back with the journal view. This command and that view are the worked example every instance starts with: `self view log --all` shows the four events that declared and installed them, which is the shape to follow when authoring your own (`self help`).",
	}, seedEntry},
	{kindView, decl{
		Name:        "journal",
		Summary:     "every journal entry, newest first",
		Description: "usage: journal\n\nPrints one line per journal.entry, newest first: timestamp, tab, text. An empty journal prints nothing.",
		Consumes:    []string{"journal.entry"},
	}, seedJournal},
}

func (st *state) previewSeed() {
	if !seedEnabled || len(st.Events) > 0 {
		return
	}
	st.Preview = true
	for _, s := range seedCaps {
		c := &capability{Type: s.typ, Name: s.decl.Name, Decl: s.decl,
			Receipt: &receipt{Type: s.typ, Name: s.decl.Name, Script: s.script, Consumes: s.decl.Consumes, By: doorSeed}}
		st.byKey[c.key()] = c
		st.Caps = append(st.Caps, c)
	}
}

func seedLocked(home string, key []byte) error {
	if !seedEnabled {
		return nil
	}
	last, err := lastSeq(home)
	if err != nil {
		return fmt.Errorf("check for an empty log before seeding: %w", err)
	}
	if last != 0 {
		return nil
	}
	var evs []Event
	var scripts []authored
	for _, s := range seedCaps {
		payload, err := json.Marshal(s.decl)
		if err != nil {
			return fmt.Errorf("encode the seed declaration of %s: %w", s.decl.Name, err)
		}
		e := newEvent(s.typ+".declared", payload)
		e.Via = doorSeed
		evs = append(evs, e)
		scripts = append(scripts, authored{Type: s.typ, Name: s.decl.Name, Script: s.script})
	}
	if err := ingestLocked(home, key, evs, scripts, nil, doorSeed, io.Discard); err != nil {
		return fmt.Errorf("seed the first write: %w", err)
	}
	fmt.Fprintf(os.Stderr, "self: first write — seeded command/entry and view/journal as a worked example (seq 1-%d)\n", 2*len(seedCaps))
	return nil
}

func seedNow(home string) error {
	key, err := ensureSecret(home)
	if err != nil {
		return fmt.Errorf("create the instance key before seeding: %w", err)
	}
	unlock, err := lockLog(home)
	if err != nil {
		return fmt.Errorf("lock the log before seeding: %w", err)
	}
	defer unlock()
	return seedLocked(home, key)
}
