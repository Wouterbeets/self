package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// declName returns the capability a declaration event declares, or "".
func declName(e Event) (typ, name string) {
	if e.Name != "command.declared" && e.Name != "projector.declared" {
		return "", ""
	}
	var d struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(e.Payload, &d) != nil {
		return "", ""
	}
	return strings.TrimSuffix(e.Name, ".declared"), d.Name
}

func parseCapabilityTarget(target string) (typ, name string, err error) {
	typ, name, ok := strings.Cut(strings.TrimSpace(target), "/")
	if !ok || name == "" {
		return "", "", fmt.Errorf("target must be command/<name> or projector/<name>")
	}
	if typ != "command" && typ != "projector" {
		return "", "", fmt.Errorf("target type must be command or projector")
	}
	if !validCapabilityName(name) {
		return "", "", fmt.Errorf("unsafe capability name %q", name)
	}
	return typ, name, nil
}

// cmdLearn learns an account mechanically — no mind in sight. The account's
// intent is recorded, its record is deposited verbatim (moments and speakers
// preserved), a lesson.learned receipt attests to what actually landed, and
// the intelligent half rides the pipe: stdout is the learning prompt, ready
// for whatever mind the shell supplies:
//
//	self learn lessons/journal | claude -p | self
//
// Same account, two instances, two expressions — the receiving mind declares
// its own capabilities against local state, and the kernel installs only
// what comes back signed. Dropping the prompt on the floor is legitimate too:
// the record is already in, and the intent stays in the log for a later pass.
func cmdLearn(home, ref string) error {
	name, intent, deposit, m, recordHash, err := readAccount(ref)
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{"name": name, "intent": intent})
	ie := newEvent("intent.declared", payload)
	ie.Via, ie.By = "cli", callerClaim()
	if err := appendEvent(home, &ie); err != nil {
		return err
	}

	// The record lands verbatim: this instance's id and seq, the event's own
	// moment and speaker. Deposited events never route through a mind — a
	// model only ever writes the disposable part, never the part that
	// accumulates. The door is this log's own: whatever via the record
	// carried was another body's fact, and this body's fact is the learn.
	for _, e := range deposit {
		fresh := newEvent(e.Name, e.Payload)
		if !e.OccurredAt.IsZero() {
			fresh.OccurredAt = e.OccurredAt
		}
		fresh.Via, fresh.By = "learn:"+name, e.By
		if err := appendEvent(home, &fresh); err != nil {
			return err
		}
	}

	// The receipt attests to what was actually deposited, beside what the
	// manifest claimed: a mismatch means the account was edited between
	// giving and learning — an intervention, visible forever in both logs.
	receipt := map[string]any{"lesson": name, "events": len(deposit)}
	if recordHash != "" {
		receipt["record_sha256"] = recordHash
	}
	if m.RecordSha256 != "" {
		receipt["manifest_sha256"] = m.RecordSha256
	}
	rp, _ := json.Marshal(receipt)
	se := newEvent("lesson.learned", rp)
	se.Via = "kernel"
	if err := appendEvent(home, &se); err != nil {
		return err
	}
	refreshSite(home)

	fmt.Fprintf(os.Stderr, "self: learned %q — %d event(s) deposited; pipe this prompt to a mind to grow its capabilities:  self learn %s | claude -p | self\n", name, len(deposit), ref)
	_, err = io.WriteString(os.Stdout, situatedPrompt(home, learnAsk(ref, intent, len(deposit))))
	return err
}

// learnAsk frames the learning ask the prompt carries: declare the
// capabilities that realize the intent here, author their scripts, and hand
// everything back the one way the pipe accepts it. When the account carries a
// record, the mind is pointed at the deposited events — evidence is for
// reading, and the log is right there for its tools.
func learnAsk(ref, intent string, deposited int) string {
	ask := "Learn this account: declare the capabilities that realize its intent on this instance (command.declared / projector.declared), author each script (script.authored), then reply in one line (self.replied)."
	if deposited > 0 {
		if abs, err := filepath.Abs(ref); err == nil {
			ref = abs
		}
		ask += fmt.Sprintf("\n\nIts record — %d event(s) — is already deposited in this log, verbatim (door learn:*). Read %s or events.jsonl to ground your declarations in the evidence (lineage.* events are another instance's history — reference material, never yours to re-emit).", deposited, filepath.Join(ref, "record.jsonl"))
	}
	return ask + "\n\n--- INTENT ---\n" + intent + "\n--- END INTENT ---"
}

func cmdRun(home, command string, args []string) error {
	if p, _ := scriptPath(home, "command", command); !fileExists(p) {
		return fmt.Errorf("command %q not found (learn a lesson that declares it)", command)
	}
	evs, err := runCommand(home, command, args, "cli", callerClaim())
	if err != nil {
		return err
	}
	for _, e := range evs {
		fmt.Printf("appended seq %d %s\n", e.Seq, e.Name)
	}
	return nil
}

func cmdShow(home, name string) error {
	if name == "kernel" {
		renderKernelHTML(home)
		renderBriefFile(home)
		page, err := os.ReadFile(filepath.Join(home, "site", "kernel.html"))
		if err != nil {
			return err
		}
		os.Stdout.Write(page)
		return nil
	}
	if name == "brief" {
		renderBriefFile(home)
		data, err := os.ReadFile(filepath.Join(home, "site", "brief.md"))
		if err != nil {
			return err
		}
		os.Stdout.Write(data)
		return nil
	}
	// a live projector takes precedence over a stale on-disk file of the
	// same name — projectors are the log's pure replay, re-run live.
	if p, _ := scriptPath(home, "projector", name); fileExists(p) {
		page, err := runProjection(home, name)
		if err != nil {
			return err
		}
		os.Stdout.Write(page)
		return nil
	}
	// bare name → on-disk artifact (.html, .md, .txt) under site/, if present
	if p, _ := siteFile(home, name); p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		// Write verbatim — the same bytes the server serves. .md and .txt are
		// plain text; .html is the projection's own markup.
		os.Stdout.Write(data)
		return nil
	}
	return fmt.Errorf("projection %q not found", name)
}

func cmdRetire(home, target string) error {
	typ, name, err := parseCapabilityTarget(target)
	if err != nil {
		return err
	}
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	commands, _, projectors, _ := declaredCaps(events)
	declared := false
	switch typ {
	case "command":
		_, declared = commands[name]
	case "projector":
		_, declared = projectors[name]
	}
	if !declared {
		return fmt.Errorf("nothing to retire: %s/%s is not currently declared", typ, name)
	}
	payload, _ := json.Marshal(retirement{Type: typ, Name: name})
	tomb := newEvent("capability.retired", payload)
	tomb.Via, tomb.By = "cli", callerClaim()
	if err := ingest(home, []Event{tomb}); err != nil {
		return err
	}
	fmt.Printf("retired %s/%s — the log keeps its history; re-declare to revive\n", typ, name)
	return nil
}
