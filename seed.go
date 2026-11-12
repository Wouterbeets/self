package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const doorSeed = "seed"

var seedEnabled = true

type seedCap struct {
	typ  string
	decl decl
}

var seedCaps = []seedCap{
	{kindCommand, decl{
		Name:        "note",
		Summary:     "record one note",
		Description: "usage: note <text…>\n\nAppends one note.added {text}: the words given, joined by single spaces. With no text it prints the usage on stderr, exits 2 and appends nothing.\n\nThe kernel declared this on the first write and left the script to the first mind here. Write it for an interpreter this machine has; `self help` shows how a script is authored.",
	}},
	{kindView, decl{
		Name:        "notes",
		Summary:     "every note, newest first",
		Description: "usage: notes\n\nPrints one line per note.added, newest first: occurred_at, a tab, then the text with newlines shown as spaces. No notes prints nothing.\n\nThe kernel declared this on the first write and left the script to the first mind here.",
		Consumes:    []string{"note.added"},
	}},
}

func (st *state) previewSeed() {
	if !seedEnabled || len(st.Events) > 0 {
		return
	}
	st.Preview = true
	for _, s := range seedCaps {
		c := &capability{Type: s.typ, Name: s.decl.Name, Decl: s.decl}
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
	for _, s := range seedCaps {
		payload, err := json.Marshal(s.decl)
		if err != nil {
			return fmt.Errorf("encode the seed declaration of %s: %w", s.decl.Name, err)
		}
		e := newEvent(s.typ+".declared", payload)
		e.Via = doorSeed
		evs = append(evs, e)
	}
	if err := ingestLocked(home, key, evs, nil, nil, doorSeed, io.Discard); err != nil {
		return fmt.Errorf("record the seed declarations: %w", err)
	}
	fmt.Fprintf(os.Stderr, "self: first write — declared command/note and view/notes (seq 1-%d); they are pending until a mind authors their scripts\n", len(seedCaps))
	return nil
}
