package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// An account is the one wire format between instances — a directory a mind
// gives and another mind learns from:
//
//	account/
//	  intent.md      the telling: who this is from, what it means, what
//	                 might grow from it — read by the human first, the
//	                 mind second (required)
//	  record.jsonl   the evidence: events verbatim, moments preserved
//	                 (optional; intent alone is a bare lesson)
//	  manifest.json  the attestation: event count + sha256 of the record,
//	                 so the receiver can see whether what arrived is what
//	                 was given (optional)
//
// Nothing runnable ever rides in an account. The receiver's own mind reads
// the intent against the receiver's own state and declares its own
// capabilities; only the local kernel's signature installs anything. Giving
// is cheap; learning is the work — that asymmetry is the protocol.

type manifest struct {
	Events       int    `json:"events"`
	RecordSha256 string `json:"record_sha256"`
	Prefix       string `json:"prefix,omitempty"`     // knowledge flavor: which events were selected
	Capability   string `json:"capability,omitempty"` // capability flavor: command/<n> or projector/<n>
}

// kernelVocabulary is the set of event names the kernel itself acts on. These
// never travel raw: give renames them to lineage.<name> on the way out, and
// learn refuses them in a record — so a foreign account can carry its history
// as evidence, but can never speak in this kernel's voice.
var kernelVocabulary = map[string]bool{
	"kernel.initialized":            true,
	"intent.declared":               true,
	"learn.orchestrated":            true,
	"lesson.learned":                true,
	"account.given":                 true,
	"command.declared":              true,
	"projector.declared":            true,
	"script.compiled":               true,
	"capability.retired":            true,
	"capability.revision.requested": true,
	"self.reflected":                true,
	"mind.routed":                   true,
	"compile.escalated":             true,
	"mind.refused":                  true,
	"review.rejected":               true,
}

const lineagePrefix = "lineage."

func parseDeposit(raw []byte) ([]Event, error) {
	var evs []Event
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse record.jsonl: %w", err)
		}
		if kernelVocabulary[e.Name] {
			return nil, fmt.Errorf("the record carries %q — the kernel's vocabulary never travels; rename it %s%s to carry it as lineage", e.Name, lineagePrefix, e.Name)
		}
		evs = append(evs, e)
	}
	return evs, nil
}

// readAccount reads one account directory: the intent (required), the record
// (optional, validated), and the manifest (optional). recordHash is the
// sha256 of the record file actually read — what learn will attest to having
// planted, beside whatever the manifest claims.
func readAccount(ref string) (name, intent string, deposit []Event, m manifest, recordHash string, err error) {
	data, e := os.ReadFile(filepath.Join(ref, "intent.md"))
	if e != nil {
		return "", "", nil, m, "", fmt.Errorf("an account is a directory with an intent.md: %w", e)
	}
	name, intent = filepath.Base(ref), strings.TrimSpace(string(data))
	if raw, e := os.ReadFile(filepath.Join(ref, "record.jsonl")); e == nil {
		if deposit, err = parseDeposit(raw); err != nil {
			return "", "", nil, m, "", err
		}
		sum := sha256.Sum256(raw)
		recordHash = hex.EncodeToString(sum[:])
	}
	if raw, e := os.ReadFile(filepath.Join(ref, "manifest.json")); e == nil {
		if err := json.Unmarshal(raw, &m); err != nil {
			return "", "", nil, m, "", fmt.Errorf("parse manifest.json: %w", err)
		}
	}
	return name, intent, deposit, m, recordHash, nil
}

// cmdGive writes an account from the live log. Two selectors, one format:
// an event-name prefix ("note.") gives the knowledge flavor — every matching
// event, verbatim, moments intact; "command/<name>" or "projector/<name>"
// gives the capability flavor — the declarations and locally-verified
// receipts, renamed to lineage.* so they arrive as evidence, never as
// installables. Curation is the giver's move and it happens in the
// directory: edit the intent, delete lines from the record, then send. The
// giving itself is remembered in the log.
func cmdGive(home, selector, dir string) error {
	if selector == "" || dir == "" {
		return fmt.Errorf("usage: self give <event-prefix | command/<name> | projector/<name>> <dir>")
	}
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	var selected []Event
	m := manifest{}
	if strings.Contains(selector, "/") {
		typ, name, err := parseCapabilityTarget(selector)
		if err != nil {
			return err
		}
		secret, err := loadSecret(home)
		if err != nil {
			return err
		}
		for _, e := range events {
			if t, n := declName(e); t == typ && n == name {
				selected = append(selected, e)
			} else if e.Name == "script.compiled" {
				if r, ok := verifiedReceipt(secret, e.Payload); ok && r.Type == typ && r.Name == name {
					selected = append(selected, e)
				}
			}
		}
		if len(selected) == 0 {
			return fmt.Errorf("no declaration for %s/%s in this log — nothing to give", typ, name)
		}
		m.Capability = typ + "/" + name
	} else {
		for _, e := range events {
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
		if kernelVocabulary[e.Name] {
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
	// giver's moment of curation, and the receiver's mind reads it to decide
	// how this record should live there.
	intentPath := filepath.Join(dir, "intent.md")
	if !fileExists(intentPath) {
		if err := os.WriteFile(intentPath, []byte(giveIntentStub(m)), 0644); err != nil {
			return err
		}
	}

	// The giver remembers giving: if it is not an event, it did not happen.
	given, _ := json.Marshal(map[string]any{
		"selector": selector, "events": len(selected), "dir": dir, "record_sha256": m.RecordSha256,
	})
	e := newEvent("account.given", given)
	if err := appendEvent(home, &e); err != nil {
		return err
	}
	refreshSite(home)
	fmt.Fprintf(os.Stderr, "self: gave %d event(s) to %s — edit %s, then pass the directory on\n",
		len(selected), dir, intentPath)
	return nil
}

func giveIntentStub(m manifest) string {
	if m.Capability != "" {
		return fmt.Sprintf(`# an account of %s

This account carries evidence, not code: the declarations of %s from the
giver's log and the giver's signed receipts, all renamed lineage.* — inert
by type. To learn it, read the lineage (the latest lineage.script.compiled
carries the giver's script as reference), then declare your own capability
fitted to this instance. Never install the reference; re-derive it.

(Giver: edit this file before passing the directory on — say who you are,
what this capability does for you, and what you hope it becomes elsewhere.)
`, m.Capability, m.Capability)
	}
	return fmt.Sprintf(`# an account — %s* events from another instance

This account carries a record, not code: %d event(s) given verbatim from
the giver's log, moments preserved. Learn it and decide how these events
should live here: render them beside what this instance already holds, and
where the two records describe the same things, make the overlap visible —
agreements, contradictions, and what only one side saw. The giver's event
names and fields may not match this instance's; translate in the
projection, never by rewriting the planted events.

(Giver: edit this file before passing the directory on — say who you are,
what these events mean, and what you hope grows from them.)
`, m.Prefix, m.Events)
}
