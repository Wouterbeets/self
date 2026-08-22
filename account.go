package main

// Accounts — the one wire format between instances, and the only thing that
// crosses a boundary. An account is a directory of plain text:
//
//	account/
//	  intent.md      the telling (required)
//	  record.jsonl   the evidence: events verbatim, moments preserved (optional)
//	  manifest.json  the attestation over the record (optional, advisory)
//
// Nothing runnable ever travels. The receiver's own mind reads the intent
// against local state and declares its own capabilities; only the local key
// installs. Giving is cheap, learning is the work — that asymmetry is the
// protocol.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// refused is the frozen set of names a record may never carry raw. It is the
// SOLE gate between a foreign account and the strange loop: replay acts on
// event names, so a deposited command.declared would appear as pending work and
// the next pass would author and sign an attacker-chosen script under the local
// key.
//
// The set is frozen, not derived: it holds every name this kernel acts on plus
// every name any kernel ever acted on. A name may leave the vocabulary; it
// never leaves this set — otherwise retiring a name would quietly make it
// depositable, and yesterday's vocabulary becomes tomorrow's injection.
var refused = map[string]bool{
	// live
	"command.declared":   true,
	"view.declared":      true,
	"script.authored":    true,
	"script.installed":   true,
	"script.rejected":    true,
	"capability.retired": true,
	"intent.declared":    true,
	"lesson.learned":     true,
	"account.given":      true,
	// retired, and refused forever
	"kernel.initialized":            true,
	"projector.declared":            true,
	"script.compiled":               true,
	"self.asked":                    true,
	"self.replied":                  true,
	"self.reflected":                true,
	"learn.orchestrated":            true,
	"capability.revision.requested": true,
}

const lineagePrefix = "lineage."

type manifest struct {
	Events       int    `json:"events"`
	RecordSha256 string `json:"record_sha256"`
	Prefix       string `json:"prefix,omitempty"`     // knowledge flavour: which events were selected
	Capability   string `json:"capability,omitempty"` // capability flavour: command/<n> | view/<n>
}

type account struct {
	Name       string
	Intent     string
	Deposit    []Event
	Manifest   manifest
	RecordHash string // sha256 of the record file as actually read
}

// readAccount reads one account directory. The record is validated whole before
// anything is appended: a refused name means the account deposits nothing at
// all, rather than landing a prefix of itself.
func readAccount(ref string) (*account, error) {
	data, err := os.ReadFile(filepath.Join(ref, "intent.md"))
	if err != nil {
		return nil, fmt.Errorf("an account is a directory with an intent.md: %w", err)
	}
	a := &account{Name: filepath.Base(strings.TrimRight(ref, "/")), Intent: strings.TrimSpace(string(data))}
	if raw, err := os.ReadFile(filepath.Join(ref, "record.jsonl")); err == nil {
		for i, line := range strings.Split(string(raw), "\n") {
			if line = strings.TrimSpace(line); line == "" {
				continue
			}
			var e Event
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				return nil, fmt.Errorf("record.jsonl line %d: %w", i+1, err)
			}
			if refused[e.Name] {
				return nil, fmt.Errorf("record.jsonl line %d carries %q — the kernel's vocabulary never travels; rename it %s%s to carry it as lineage (nothing was deposited)", i+1, e.Name, lineagePrefix, e.Name)
			}
			if !validEventName(e.Name) {
				return nil, fmt.Errorf("record.jsonl line %d: %q is not a lowercase dotted event name (nothing was deposited)", i+1, e.Name)
			}
			a.Deposit = append(a.Deposit, e)
		}
		sum := sha256.Sum256(raw)
		a.RecordHash = hex.EncodeToString(sum[:])
	}
	if raw, err := os.ReadFile(filepath.Join(ref, "manifest.json")); err == nil {
		if err := json.Unmarshal(raw, &a.Manifest); err != nil {
			return nil, fmt.Errorf("parse manifest.json: %w", err)
		}
	}
	return a, nil
}

// cmdLearn is the only way in, and it splits along the seam. The mechanical
// half happens here and needs no mind: the intent is recorded first (someone
// brought this prose here), the record is deposited verbatim next, and the
// attestation lands last — it must be last, because it hashes what actually
// landed. The intelligent half rides the pipe: stdout is the learning prompt.
//
//	self learn account/ | claude -p | self
func cmdLearn(home, ref string, out io.Writer) error {
	a, err := readAccount(ref)
	if err != nil {
		return err
	}
	if _, err := ensureSecret(home); err != nil {
		return err
	}

	batch := make([]Event, 0, len(a.Deposit)+2)

	intent, _ := json.Marshal(map[string]any{"account": a.Name, "intent": a.Intent})
	ie := newEvent("intent.declared", intent)
	ie.Via, ie.By = doorCLI, callerClaim()
	batch = append(batch, ie)

	// Verbatim: this instance's id and seq, the event's own moment and its own
	// speaker. Testimony travels with its time and its voice. The door is
	// re-stamped — whatever via the record carried was another body's fact.
	for _, e := range a.Deposit {
		fresh := newEvent(e.Name, e.Payload)
		if !e.OccurredAt.IsZero() {
			fresh.OccurredAt = e.OccurredAt
		}
		fresh.Via, fresh.By = doorLearn+a.Name, e.By
		batch = append(batch, fresh)
	}

	// The attestation records what was deposited BESIDE what the manifest
	// claimed. A divergence means the account was edited between giving and
	// learning — legitimate curation, visible in both logs forever.
	att := map[string]any{"account": a.Name, "events": len(a.Deposit)}
	if a.RecordHash != "" {
		att["record_sha256"] = a.RecordHash
	}
	if a.Manifest.RecordSha256 != "" {
		att["manifest_sha256"] = a.Manifest.RecordSha256
	}
	ap, _ := json.Marshal(att)
	ae := newEvent("lesson.learned", ap)
	ae.Via = doorKernel // the kernel's own attestation, like a receipt
	batch = append(batch, ae)

	if err := appendEvents(home, batch); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "self: learned %q — %d event(s) deposited; pipe this prompt to a mind:  self learn %s | claude -p | self\n", a.Name, len(a.Deposit), ref)

	st, err := loadState(home)
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, situate(home, st, learnAsk(ref, a)))
	return err
}

// learnAsk frames the work: realize this intent HERE, as this instance's own
// capabilities. The same account learned by two instances yields two
// expressions — that is learning rather than copying.
func learnAsk(ref string, a *account) string {
	ask := fmt.Sprintf("Learn the account %q: decide how its intent should live on THIS instance, declare the capabilities that realize it, and author their scripts in this same answer.\n\nFix the public names the intent fixes; choose everything else yourself against what this instance already has. Do not transplant another instance's design.", a.Name)
	if len(a.Deposit) > 0 {
		abs := ref
		if p, err := filepath.Abs(ref); err == nil {
			abs = p
		}
		ask += fmt.Sprintf("\n\nIts record — %d event(s) — is already in this log, verbatim, through the door learn:%s. Read %s or events.jsonl to ground your declarations in the evidence. lineage.* events are another instance's history: reference material, never yours to re-emit.", len(a.Deposit), a.Name, filepath.Join(abs, "record.jsonl"))
	}
	return ask + "\n\n--- INTENT ---\n" + a.Intent + "\n--- END INTENT ---"
}

// cmdGive writes an account from the live log. Two selectors, one format: an
// event-name prefix gives the knowledge flavour (every matching event,
// verbatim, moments intact); command/<name> or view/<name> gives the capability
// flavour — the declarations and this instance's verified receipts, renamed to
// lineage so they arrive as evidence and can never be installables. Curation is
// the giver's move and it happens in the directory afterwards.
func cmdGive(home, selector, dir string) error {
	st, err := loadState(home)
	if err != nil {
		return err
	}
	var selected []Event
	m := manifest{}

	if typ, name, isCap := strings.Cut(selector, "/"); isCap {
		if !validCapability(typ, name) {
			return fmt.Errorf("a capability selector is command/<name> or view/<name>")
		}
		declName := typ + ".declared"
		for _, e := range st.Events {
			switch e.Name {
			case declName:
				var d decl
				if json.Unmarshal(e.Payload, &d) == nil && d.Name == name {
					selected = append(selected, e)
				}
			case "script.installed":
				if r, ok := verifyReceipt(st.Key, e.Payload); ok && r.Type == typ && r.Name == name {
					selected = append(selected, e)
				}
			}
		}
		if len(selected) == 0 {
			return fmt.Errorf("no declaration for %s/%s in this log — nothing to give", typ, name)
		}
		m.Capability = typ + "/" + name
	} else {
		for _, e := range st.Events {
			if strings.HasPrefix(e.Name, selector) {
				selected = append(selected, e)
			}
		}
		if len(selected) == 0 {
			return fmt.Errorf("no events named %s* in this log — nothing to give", selector)
		}
		m.Prefix = selector
	}

	var record strings.Builder
	enc := json.NewEncoder(&record)
	for _, e := range selected {
		if refused[e.Name] {
			e.Name = lineagePrefix + e.Name
		}
		enc.Encode(e)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	recordBytes := []byte(record.String())
	if err := os.WriteFile(filepath.Join(dir, "record.jsonl"), recordBytes, 0644); err != nil {
		return err
	}
	sum := sha256.Sum256(recordBytes)
	m.Events, m.RecordSha256 = len(selected), hex.EncodeToString(sum[:])
	mb, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(mb, '\n'), 0644); err != nil {
		return err
	}
	// The intent stub is written once and never clobbered: editing it is the
	// giver's moment of curation, and it is the half a receiving mind reads
	// first.
	intentPath := filepath.Join(dir, "intent.md")
	if _, err := os.Stat(intentPath); err != nil {
		if err := os.WriteFile(intentPath, []byte(intentStub(m)), 0644); err != nil {
			return err
		}
	}

	// The giver remembers giving: if it is not an event, it did not happen.
	given, _ := json.Marshal(map[string]any{
		"selector": selector, "events": len(selected), "dir": dir, "record_sha256": m.RecordSha256,
	})
	e := newEvent("account.given", given)
	e.Via, e.By = doorCLI, callerClaim()
	if err := appendEvents(home, []Event{e}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "self: gave %d event(s) to %s — edit %s, then pass the directory on\n", len(selected), dir, intentPath)
	return nil
}

func intentStub(m manifest) string {
	if m.Capability != "" {
		return fmt.Sprintf(`# an account of %s

This account carries evidence, not code: the declarations of %s from the giver's
log and the giver's own signed receipts, renamed lineage.* — inert by type. To
learn it, read the lineage (the latest lineage.script.installed carries the
giver's script as reference), then declare YOUR OWN capability, fitted to this
instance. Never install the reference; re-derive it.

(Giver: edit this file before passing the directory on — say who you are, what
this capability does for you, and what you hope it becomes elsewhere.)
`, m.Capability, m.Capability)
	}
	return fmt.Sprintf(`# an account — %s* events from another instance

This account carries a record, not code: %d event(s) given verbatim from the
giver's log, moments preserved. Learn it and decide how these events should live
here: render them beside what this instance already holds, and where the two
records describe the same things, make the overlap visible — agreements,
contradictions, and what only one side saw. The giver's event names and fields
may not match this instance's; translate in a view, never by rewriting the
deposited events.

(Giver: edit this file before passing the directory on — say who you are, what
these events mean, and what you hope they become elsewhere.)
`, m.Prefix, m.Events)
}
