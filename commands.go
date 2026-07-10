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
		if r, ok := compiledReceipt(secret, e); ok && r.Type == typ && r.Name == name {
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
	n := 0
	forEachVerifiedReceipt(home, func(_ Event, r receipt) {
		if r.Type == typ && r.Name == name {
			n++
		}
	})
	return n
}

// cmdGrow grows a seed: a directory with intent.md (the genotype — prose
// intent, not a parts-list) and optionally seed.jsonl (initial content events,
// the initial deposit). The orchestrator reads the intent, explores the
// instance, and declares the decomposition that realizes it here; each piece is
// then compiled with the whole intent woven in. Same intent, different instance,
// different decomposition.
func cmdGrow(home, ref string) error {
	name, intent, deposit, err := readSeedSource(ref)
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{"name": name, "intent": intent})
	ie := newEvent("intent.declared", payload)
	if err := appendEvent(home, &ie); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "self: orchestrating %q from intent…\n", name)
	res, err := pipeBrain(home, "grow", growPrompt(intent))
	if err != nil {
		return fmt.Errorf("orchestrate %q: %w (growing needs a brain — %s)", name, err, brainHint)
	}
	c := newLLM(home)
	c.intent = intent
	if len(res.Events) == 0 {
		return fmt.Errorf("the orchestrator declared nothing for %q", name)
	}

	// The orchestrator's stated reasoning is provenance: log it, so the chain
	// from intent to script survives in the log (and in any seed sharing it),
	// and weave it into each compile of this grow so every piece is authored
	// with the plan in view — in-band continuity, never a session store.
	if r := strings.TrimSpace(res.Response); r != "" {
		c.reasoning = r
		rp, _ := json.Marshal(map[string]any{"seed": name, "reasoning": r})
		re := newEvent("grow.orchestrated", rp)
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
	grown := compileDeclarations(c, home, declEvents)

	// The initial deposit: content laid once, so the surface has
	// something to render from the first moment. A file.stored deposit also
	// carries its bytes — from the seed's files/ dir into the blob store.
	for _, e := range deposit {
		payload := e.Payload
		if e.Name == "file.stored" {
			if payload, err = depositSeedFile(home, ref, e.Payload); err != nil {
				return err
			}
		}
		fresh := newEvent(e.Name, payload)
		// A deposit that carries its own moment keeps it: an exported record
		// planted here still says when it happened, not when it arrived. The
		// id and seq are this instance's; the time is the event's.
		if !e.OccurredAt.IsZero() {
			fresh.OccurredAt = e.OccurredAt
		}
		if err := appendEvent(home, &fresh); err != nil {
			return err
		}
	}

	rp, _ := json.Marshal(map[string]any{"seed": name, "capabilities": grown})
	se := newEvent("seed.planted", rp)
	if err := appendEvent(home, &se); err != nil {
		return err
	}
	refreshSite(home)
	fmt.Printf("grew %q: %d capabilit(ies) from intent — %s\n", name, grown, res.Response)
	return nil
}

// growPrompt frames the orchestration ask: decompose the intent into declared
// capabilities, and hand them back the one way the kernel accepts them.
func growPrompt(intent string) string {
	return "Grow the capabilities that realize this product: declare each one by emitting a command.declared / projector.declared event, then summarize in one line.\n\n" +
		brainAnswerContract + "\n\n--- INTENT ---\n" + intent + "\n--- END INTENT ---"
}

func cmdThink(home, prompt string) error {
	if prompt == "" {
		data, _ := io.ReadAll(os.Stdin)
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		return fmt.Errorf("usage: self think <prompt> (or pipe it on stdin)")
	}
	res, err := pipeBrain(home, "think", thinkPrompt(prompt))
	if err != nil {
		return fmt.Errorf("brain: %w", err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"response": res.Response, "declarations": res.Events})
}

// thinkPrompt wraps a think ask with the answer contract. A think is
// report-only — the kernel returns brain-authored events to the caller instead
// of ingesting them — but the brain still needs to know its stdout is the only
// channel: without the contract, a tool-capable brain wastes its session trying
// to persist its work itself (edit the log, run the CLI) and gets denied. Every
// event-expecting ask carries the same guidance; this was the one naked ask left.
func thinkPrompt(prompt string) string {
	return prompt + "\n\n" + brainAnswerContract
}

func cmdHeartbeat(home string) error {
	prior, _ := readEvents(home)
	hb := newEvent("self.heartbeat", json.RawMessage(`{}`))
	if err := appendEvent(home, &hb); err != nil {
		return err
	}
	prompt := `This is a self-improvement heartbeat. Explore your instance — capabilities, recent events, projections — and choose ONE small, high-value improvement: a missing capability, a clearer projection, a drift to fix. If warranted, declare it (emit command.declared / projector.declared); if nothing is worth changing, say so plainly and declare nothing. Keep it minimal.` +
		"\n\n" + brainAnswerContract + heartbeatContext(prior)
	res, err := pipeBrain(home, "heartbeat", prompt)
	if err != nil {
		return err
	}
	applyEvents(home, res)
	fmt.Println(res.Response)
	return nil
}

// heartbeatContext hands the brain the events since its last beat — capped,
// minus kernel bookkeeping receipts — so a beat reacts to what changed instead
// of exploring from scratch.
func heartbeatContext(events []Event) string {
	last := -1
	for i, e := range events {
		if e.Name == "self.heartbeat" {
			last = i
		}
	}
	var acts []Event
	for _, e := range events[last+1:] {
		if e.Name == "script.compiled" || e.Name == "script.verified" {
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
	b.WriteString("\n\nSince your last heartbeat, these things happened in this instance:\n")
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
		return fmt.Errorf("command %q not found (grow a seed that declares it)", command)
	}
	args, deposits, err := storeFileArgs(home, args)
	if err != nil {
		return err
	}
	if len(deposits) > 0 {
		if err := ingest(home, deposits); err != nil {
			return err
		}
		for _, e := range deposits {
			fmt.Printf("stored %s as files/%s\n", jsonField(e.Payload, "name"), jsonField(e.Payload, "sha256"))
		}
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
		page, err := kernelPage(home)
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

func cmdAdopt(home, path string) error {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	var seed []Event
	var typ, name string
	var declPayload json.RawMessage
	reference := ""
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var e Event
		if json.Unmarshal([]byte(line), &e) != nil || e.Name == "" {
			return fmt.Errorf("not a seed — want event JSONL, one {name, payload} per line")
		}
		seed = append(seed, e)
		if t, n := declName(e); n != "" { // the latest declaration is what grows here
			typ, name, declPayload = t, n, e.Payload
		}
		if e.Name == "script.compiled" { // the latest script is reference, never install
			var r receipt
			if json.Unmarshal(e.Payload, &r) == nil && r.Script != "" {
				reference = r.Script
			}
		}
	}
	if declPayload == nil {
		return fmt.Errorf("the seed carries no declaration — nothing can grow from it")
	}
	if err := ensureHome(home); err != nil {
		return err
	}
	if reference != "" {
		var m map[string]any
		if err := json.Unmarshal(declPayload, &m); err != nil {
			return fmt.Errorf("the seed's declaration is not an object: %w", err)
		}
		m["implementation"] = reference
		declPayload, _ = json.Marshal(m)
	}
	ap, _ := json.Marshal(map[string]any{"type": typ, "name": name, "seed": seed})
	before := receiptCount(home, typ, name)
	if err := ingest(home, []Event{
		newEvent("capability.adopted", ap),
		newEvent(typ+".declared", declPayload),
	}); err != nil {
		return err
	}
	// A stale script from an earlier receipt can outlive a failed recompile,
	// so "the file exists" proves nothing. The honest signal is the log: this
	// adopt succeeded only if it minted a fresh signed receipt.
	if receiptCount(home, typ, name) <= before {
		return fmt.Errorf("adopted %q into the log, but the compile produced no signed receipt (any script on disk is from an earlier receipt) — wire a brain and declare it again", name)
	}
	fmt.Printf("adopted %q — re-authored by this instance's own compiler, signed by its own key\n", name)
	return nil
}

func cmdShare(home, name string) error {
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	secret, err := loadSecret(home)
	if err != nil {
		return err
	}
	var seed []Event
	hasDecl := false
	for _, e := range events {
		if _, n := declName(e); n == name {
			seed, hasDecl = append(seed, e), true
		} else if r, ok := compiledReceipt(secret, e); ok && r.Name == name {
			seed = append(seed, e)
		}
	}
	if !hasDecl {
		return fmt.Errorf("no declaration for %q in this log — nothing to share (code never crosses; the declaration is what does)", name)
	}
	enc := json.NewEncoder(os.Stdout)
	for i := range seed {
		enc.Encode(seed[i])
	}
	// The sender remembers giving: if it is not an event, it did not happen.
	payload, _ := json.Marshal(map[string]any{"name": name, "events": len(seed)})
	e := newEvent("capability.shared", payload)
	if err := appendEvent(home, &e); err != nil {
		return err
	}
	refreshSite(home)
	fmt.Fprintf(os.Stderr, "self: shared %q — %d event(s) of intent and evidence\n", name, len(seed))
	return nil
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
