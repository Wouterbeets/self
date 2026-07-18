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

func latestCapabilitySource(home, typ, name string) (decl json.RawMessage, script, receiptID string, err error) {
	events, err := readEvents(home)
	if err != nil {
		return nil, "", "", err
	}
	secret, err := loadSecret(home)
	if err != nil {
		return nil, "", "", err
	}
	for _, e := range events {
		if t, n := declName(e); t == typ && n == name {
			decl = e.Payload
			continue
		}
		if e.Name != "script.compiled" {
			continue
		}
		if r, ok := verifiedReceipt(secret, e.Payload); ok && r.Type == typ && r.Name == name {
			script = r.Script
			receiptID = e.ID
		}
	}
	if decl == nil {
		return nil, "", "", fmt.Errorf("no declaration for %s/%s", typ, name)
	}
	if strings.TrimSpace(script) == "" {
		return nil, "", "", fmt.Errorf("no verified script receipt for %s/%s", typ, name)
	}
	return decl, script, receiptID, nil
}

func receiptCount(home, typ, name string) int {
	secret, err := loadSecret(home)
	if err != nil {
		return 0
	}
	events, err := readEvents(home)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range events {
		if e.Name != "script.compiled" {
			continue
		}
		if r, ok := verifiedReceipt(secret, e.Payload); ok && r.Type == typ && r.Name == name {
			n++
		}
	}
	return n
}

// cmdLearn learns an account: a directory with intent.md (the telling — prose
// intent, not a parts-list), and optionally record.jsonl (events to deposit
// verbatim) and manifest.json (the giver's attestation). The mind reads the
// intent — and the record, with its own tools — against this instance's
// state and declares the decomposition that realizes it here; each piece is
// then compiled with the whole intent woven in. Same account, different
// instance, different expression: learning, not copying.
func cmdLearn(home, ref string) error {
	name, intent, deposit, m, recordHash, err := readAccount(ref)
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{"name": name, "intent": intent})
	ie := newEvent("intent.declared", payload)
	if err := appendEvent(home, &ie); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "self: learning %q from its intent…\n", name)
	res, err := pipeMind(home, "learn", learnPrompt(ref, intent, deposit))
	if err != nil {
		return fmt.Errorf("learn %q: %w (learning needs a mind — %s)", name, err, mindHint)
	}
	c := newLLM(home)
	c.intent = intent

	// The orchestrator's stated reasoning is provenance: log it, so the chain
	// from intent to script survives in the log, and weave it into each
	// compile of this learn so every piece is authored with the plan in
	// view — in-band continuity, never a session store.
	if r := strings.TrimSpace(res.Response); r != "" {
		c.reasoning = r
		rp, _ := json.Marshal(map[string]any{"lesson": name, "reasoning": r})
		re := newEvent("learn.orchestrated", rp)
		if err := appendEvent(home, &re); err != nil {
			return err
		}
	}

	var declEvents []Event
	for _, d := range res.Events {
		n, _ := d["name"].(string)
		p, _ := json.Marshal(d["payload"])
		if (n != "command.declared" && n != "projector.declared") || string(p) == "null" {
			continue
		}
		e := newEvent(n, p)
		if err := appendEvent(home, &e); err != nil {
			return err
		}
		declEvents = append(declEvents, e)
	}
	if len(declEvents) == 0 && len(deposit) == 0 {
		return fmt.Errorf("nothing to learn from %q: the mind declared no capability and the account carries no record", name)
	}
	compiled := compileDeclarations(c, home, declEvents)
	if compiled != len(declEvents) {
		refreshSite(home)
		return fmt.Errorf("recorded %q in the log, but %d of %d declared capabilities compiled; no lesson.learned receipt was written", name, compiled, len(declEvents))
	}

	// The record lands verbatim: this instance's id and seq, the event's own
	// moment. Deposited events never route through the mind — the model only
	// ever writes the disposable part, never the part that accumulates.
	for _, e := range deposit {
		fresh := newEvent(e.Name, e.Payload)
		if !e.OccurredAt.IsZero() {
			fresh.OccurredAt = e.OccurredAt
		}
		if err := appendEvent(home, &fresh); err != nil {
			return err
		}
	}

	// The receipt attests to what was actually deposited, beside what the
	// manifest claimed: a mismatch means the account was edited between
	// giving and learning — an intervention, visible forever in both logs.
	receipt := map[string]any{"lesson": name, "capabilities": compiled, "events": len(deposit)}
	if recordHash != "" {
		receipt["record_sha256"] = recordHash
	}
	if m.RecordSha256 != "" {
		receipt["manifest_sha256"] = m.RecordSha256
	}
	rp, _ := json.Marshal(receipt)
	se := newEvent("lesson.learned", rp)
	if err := appendEvent(home, &se); err != nil {
		return err
	}
	refreshSite(home)
	fmt.Printf("learned %q: %d capabilit(ies), %d event(s) deposited — %s\n", name, compiled, len(deposit), res.Response)
	return nil
}

// learnPrompt frames the orchestration ask: decompose the intent into declared
// capabilities, and hand them back the one way the kernel accepts them. When
// the account carries a record, the mind is pointed at it — evidence is for
// reading, and the file is right there for its tools.
func learnPrompt(ref, intent string, deposit []Event) string {
	prompt := "Learn this account: declare the capabilities that realize its intent here by emitting command.declared / projector.declared events, then summarize in one line."
	if len(deposit) > 0 {
		if abs, err := filepath.Abs(ref); err == nil {
			ref = abs
		}
		prompt += fmt.Sprintf("\n\nThe account carries a record of %d event(s) that will be deposited in the log, verbatim, right after you answer: read %s to ground your declarations in the evidence (lineage.* events are another instance's history — reference material, never yours to re-emit).", len(deposit), filepath.Join(ref, "record.jsonl"))
	}
	return prompt + "\n\n" + mindAnswerContract + "\n\n--- INTENT ---\n" + intent + "\n--- END INTENT ---"
}

func cmdThink(home, prompt string) error {
	if prompt == "" {
		data, _ := io.ReadAll(os.Stdin)
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		return fmt.Errorf("usage: self think <prompt> (or pipe it on stdin)")
	}
	res, err := pipeMind(home, "think", thinkPrompt(prompt))
	if err != nil {
		return fmt.Errorf("mind: %w", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"response": res.Response, "events": res.Events, "declarations": res.Events})
}

// thinkPrompt wraps a think ask with the shared answer contract plus a
// report-only rider. A think returns mind output to the caller and does not
// append; without the contract a tool-capable mind wastes its session trying
// to persist work itself (edit the log, run the CLI) and gets denied.
func thinkPrompt(prompt string) string {
	return prompt + "\n\n" + mindAnswerContract + "\n\n" + mindThinkContract
}

func cmdReflect(home string) error {
	prior, _ := readEvents(home)
	hb := newEvent("self.reflected", json.RawMessage(`{}`))
	if err := appendEvent(home, &hb); err != nil {
		return err
	}
	prompt := `This is a self-improvement reflection. Explore your instance — capabilities, recent events, projections — and choose ONE small, high-value improvement: a missing capability, a clearer projection, a drift to fix. If warranted, declare it (emit command.declared / projector.declared); if nothing is worth changing, say so plainly and declare nothing. Keep it minimal.` +
		"\n\n" + mindAnswerContract + reflectionContext(prior)
	res, err := pipeMind(home, "reflect", prompt)
	if err != nil {
		return err
	}
	applyEvents(home, res)
	fmt.Println(res.Response)
	return nil
}

// reflectionContext hands the mind the events since its last reflection —
// capped, minus kernel bookkeeping receipts — so a reflection reacts to what
// changed instead of exploring from scratch.
func reflectionContext(events []Event) string {
	last := -1
	for i, e := range events {
		if e.Name == "self.reflected" {
			last = i
		}
	}
	var acts []Event
	for _, e := range events[last+1:] {
		if e.Name == "script.compiled" {
			continue
		}
		acts = append(acts, e)
	}
	if len(acts) == 0 {
		return ""
	}
	if len(acts) > 40 {
		acts = acts[len(acts)-40:]
	}
	var b strings.Builder
	b.WriteString("\n\nSince your last reflection, these things happened in this instance:\n")
	for _, e := range acts {
		payload := strings.TrimSpace(string(e.Payload))
		if len(payload) > 140 {
			payload = payload[:140] + "…"
		}
		fmt.Fprintf(&b, "  seq %d  %s  %s\n", e.Seq, e.Name, payload)
	}
	b.WriteString("\nResponding to what changed is welcome, but optional.")
	return b.String()
}

func cmdRun(home, command string, args []string) error {
	if p, _ := scriptPath(home, "command", command); !fileExists(p) {
		return fmt.Errorf("command %q not found (learn a lesson that declares it)", command)
	}
	evs, err := runCommand(home, command, args)
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

func cmdRevise(home, target string, words []string) error {
	typ, name, err := parseCapabilityTarget(target)
	if err != nil {
		return err
	}
	request := strings.TrimSpace(strings.Join(words, " "))
	if request == "" {
		return fmt.Errorf("usage: self revise %s/%s <change request>", typ, name)
	}
	declPayload, script, receiptID, err := latestCapabilitySource(home, typ, name)
	if err != nil {
		return err
	}
	var decl map[string]any
	if err := json.Unmarshal(declPayload, &decl); err != nil {
		return fmt.Errorf("latest declaration for %s/%s is not an object: %w", typ, name, err)
	}
	decl["implementation"] = script
	decl["revision"] = map[string]any{"request": request, "from_receipt": receiptID}
	updatedDecl, _ := json.Marshal(decl)
	revisionPayload, _ := json.Marshal(map[string]any{"type": typ, "name": name, "request": request, "from_receipt": receiptID})
	before := receiptCount(home, typ, name)
	if err := ingest(home, []Event{
		newEvent("capability.revision.requested", revisionPayload),
		newEvent(typ+".declared", updatedDecl),
	}); err != nil {
		return err
	}
	if receiptCount(home, typ, name) <= before {
		return fmt.Errorf("revision for %s/%s was recorded, but the compile produced no signed receipt", typ, name)
	}
	fmt.Printf("revised %s/%s — compiled a fresh signed receipt\n", typ, name)
	return nil
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
	if err := ingest(home, []Event{newEvent("capability.retired", payload)}); err != nil {
		return err
	}
	fmt.Printf("retired %s/%s — the log keeps its history; re-declare to revive\n", typ, name)
	return nil
}
