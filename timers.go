package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Timers are how an instance acts on its own schedule instead of only when
// spoken to: the weekly digest, the invoice chased after thirty days, the
// Monday list. A timer is not code — it is one event, timer.declared
// {name, every, command, args}, binding an already-installed command to a
// cadence. The serving kernel ticks; nothing else changes: the command runs
// through the same pipe as a form post, its events append the same way, and
// the log shows every firing, because "if it is not an event, it did not
// happen" applies to what the instance does on its own most of all.
//
// The rules, kept few:
//   - the latest timer.declared per name wins; every "off" disables, and
//     capability.retired {type: "timer", name} retires like anything else.
//   - a timer is due when `every` has elapsed since it last fired — or since
//     it was declared, so declaring never fires immediately.
//   - timer.fired appends BEFORE the command runs: a crashing command cannot
//     hot-loop, and a command that errors leaves a timer.failed receipt.
//   - an instance that was off fires a due timer once on the next serve, not
//     once per missed interval. The log's replay stays honest either way.

type timerDecl struct {
	Name    string   `json:"name"`
	Every   string   `json:"every"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// declaredTimers folds the log into the live timer set: latest declaration
// per name wins, "off" and retirement remove, re-declaration revives.
func declaredTimers(events []Event) map[string]timerDecl {
	timers := map[string]timerDecl{}
	for _, e := range events {
		switch e.Name {
		case "timer.declared":
			var d timerDecl
			if json.Unmarshal(e.Payload, &d) != nil || d.Name == "" {
				continue
			}
			if d.Every == "off" || d.Command == "" {
				delete(timers, d.Name)
				continue
			}
			timers[d.Name] = d
		case "capability.retired":
			var r retirement
			if json.Unmarshal(e.Payload, &r) == nil && r.Type == "timer" {
				delete(timers, r.Name)
			}
		}
	}
	return timers
}

// timerEpochs returns, per timer name, the moment its interval counts from:
// the latest firing, or the latest declaration if it has never fired.
func timerEpochs(events []Event) map[string]time.Time {
	epochs := map[string]time.Time{}
	for _, e := range events {
		if e.Name != "timer.declared" && e.Name != "timer.fired" {
			continue
		}
		var p struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(e.Payload, &p) == nil && p.Name != "" {
			epochs[p.Name] = e.OccurredAt
		}
	}
	return epochs
}

// tickTimers fires every due timer once: append the timer.fired receipt,
// then run the bound command through the ordinary pipe. Called periodically
// by the serving kernel; a full-log read per tick keeps it stateless — the
// log alone says what is declared and when it last ran.
func tickTimers(home string, now time.Time) {
	events, err := readEvents(home)
	if err != nil {
		// Never silently: an unreadable log means NO timer will fire, and
		// that must be observable, not a quiet stop.
		fmt.Fprintf(os.Stderr, "self: timers idle — cannot read the log: %s\n", err)
		return
	}
	epochs := timerEpochs(events)
	for name, d := range declaredTimers(events) {
		every, err := time.ParseDuration(d.Every)
		if err != nil || every <= 0 {
			continue // an unparseable cadence never fires; the declaration stays inspectable
		}
		if now.Sub(epochs[name]) < every {
			continue
		}
		payload, _ := json.Marshal(map[string]any{"name": name, "command": d.Command})
		fired := newEvent("timer.fired", payload)
		if err := ingest(home, []Event{fired}); err != nil {
			fmt.Fprintf(os.Stderr, "self: timer %q: %s\n", name, err)
			continue
		}
		if _, err := runCommand(home, d.Command, d.Args); err != nil {
			failed, _ := json.Marshal(map[string]any{"name": name, "command": d.Command, "error": err.Error()})
			e := newEvent("timer.failed", failed)
			ingest(home, []Event{e})
			fmt.Fprintf(os.Stderr, "self: timer %q: %s\n", name, err)
		}
	}
}

// serveTimers is the tick loop the server runs: writes stay routed through
// one process (the same one taking form posts), honoring the single-writer
// posture.
func serveTimers(home string) {
	for {
		tickTimers(home, time.Now().UTC())
		time.Sleep(30 * time.Second)
	}
}
