package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// ingest appends the events a process emitted, honors any retirements among
// them, and re-renders the projections that consume what just landed. The
// kernel compiles nothing here — a declaration without a script is pending
// work, surfaced by the ask face of the pipe until a mind authors it (the
// strange loop, one shell pass at a time). Projections are pure replays, so
// re-running any of them is always correct; skipping one whose consumed
// events did not grow is the same page for free.
func ingest(home string, evs []Event) error {
	for i := range evs {
		if err := appendEvent(home, &evs[i]); err != nil {
			return err
		}
	}
	if n := applyRetirements(home, evs); n > 0 {
		fmt.Fprintf(os.Stderr, "self: retired %d capabilit(ies)\n", n)
	}
	refreshSiteAfter(home, evs)
	return nil
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
