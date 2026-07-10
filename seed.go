package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func parseDeposit(raw []byte) ([]Event, error) {
	var evs []Event
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse seed.jsonl: %w", err)
		}
		evs = append(evs, e)
	}
	return evs, nil
}

func readSeedSource(ref string) (name, intent string, deposit []Event, err error) {
	data, e := os.ReadFile(filepath.Join(ref, "intent.md"))
	if e != nil {
		return "", "", nil, fmt.Errorf("a seed is a directory with an intent.md: %w", e)
	}
	name, intent = filepath.Base(ref), strings.TrimSpace(string(data))
	if raw, e := os.ReadFile(filepath.Join(ref, "seed.jsonl")); e == nil {
		deposit, err = parseDeposit(raw)
	}
	return name, intent, deposit, err
}

// cmdExport writes a content seed from the live log: every event whose name
// matches the prefix, the file.stored metadata for every blob those events
// reference, the blobs themselves, and an editable intent — a directory
// another instance can grow. This is how lived content travels: capabilities
// cross as declarations (share/adopt); records cross as seeds, with their
// dates and their files intact, and the receiver's brain decides how two
// lives merge on one page. Export selects, the human curates — the intent
// stub is written to be edited before the directory is sent — and the
// sender's log remembers the giving.
// as, when non-empty, renames the matched prefix on the way out — the
// sender-side remap the protocol asks for when two instances speak the same
// vocabulary: Fred and Jake both log matchday.match ours-first, so Fred
// exports his as fred. and Jake's own season stays uncontaminated while the
// derby page consumes both. The rename is recorded in the provenance event;
// planted events are translated in projections, never rewritten after they
// land.
func cmdExport(home, prefix, dir, as string) error {
	if prefix == "" || dir == "" {
		return fmt.Errorf("usage: self export <event-prefix> <dir> [<new-prefix>]")
	}
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	include := map[string]bool{} // event ID → rides in the seed
	hashes := map[string]bool{}  // blob hashes the selected events reference
	n := 0
	for _, e := range events {
		if !strings.HasPrefix(e.Name, prefix) {
			continue
		}
		include[e.ID], n = true, n+1
		for _, h := range payloadHashes(e.Payload) {
			hashes[h] = true
		}
	}
	if n == 0 {
		return fmt.Errorf("no events named %s* in this log — nothing to export", prefix)
	}
	// The metadata for every referenced blob rides along, renamed only when
	// two different blobs claim one human name — grow verifies each pinned
	// sha256 against the bytes, so names in the seed must map one-to-one.
	blobName := map[string]string{}
	taken := map[string]bool{}
	for _, e := range events {
		if e.Name != "file.stored" {
			continue
		}
		var p struct {
			Name   string `json:"name"`
			Sha256 string `json:"sha256"`
		}
		if json.Unmarshal(e.Payload, &p) != nil || !hashes[p.Sha256] || blobName[p.Sha256] != "" {
			continue
		}
		if !fileExists(blobPath(home, p.Sha256)) {
			fmt.Fprintf(os.Stderr, "self: export — blob %s (%s) is missing from files/; left out\n", p.Sha256[:12], p.Name)
			continue
		}
		name := filepath.Base(p.Name)
		if taken[name] {
			name = p.Sha256[:8] + "-" + name
		}
		taken[name], blobName[p.Sha256], include[e.ID] = true, name, true
	}
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0755); err != nil {
		return err
	}
	var seed strings.Builder
	enc := json.NewEncoder(&seed)
	provFields := map[string]any{"prefix": prefix, "events": n, "files": len(blobName)}
	if as != "" {
		provFields["as"] = as
	}
	prov, _ := json.Marshal(provFields)
	enc.Encode(newEvent("seed.provenance", prov))
	for _, e := range events {
		if !include[e.ID] {
			continue
		}
		if as != "" && strings.HasPrefix(e.Name, prefix) {
			e.Name = as + strings.TrimPrefix(e.Name, prefix)
		}
		if e.Name == "file.stored" {
			var p map[string]any
			json.Unmarshal(e.Payload, &p)
			hash, _ := p["sha256"].(string)
			p["name"] = blobName[hash]
			e.Payload, _ = json.Marshal(p)
			if err := copyFile(blobPath(home, hash), filepath.Join(dir, "files", blobName[hash])); err != nil {
				return err
			}
		}
		enc.Encode(e)
	}
	if err := os.WriteFile(filepath.Join(dir, "seed.jsonl"), []byte(seed.String()), 0644); err != nil {
		return err
	}
	// An intent stub, written once and never clobbered: editing it is the
	// sender's moment of curation, and the receiver's brain reads it to
	// decide how two records merge.
	intentPath := filepath.Join(dir, "intent.md")
	if !fileExists(intentPath) {
		stub := fmt.Sprintf(`# planted — %s* events from another instance

This seed carries content, not code: %d event(s) and %d file(s) exported
from the sender's log, dates preserved. Grow it and decide how these events
should live here: render them alongside what this instance already holds,
and where the two records describe the same things, make the overlap
visible — agreements, contradictions, and what only one side saw. The
sender's event names and fields may not match this instance's; translate in
the projection, never by rewriting the events.

(Sender: edit this file before passing the directory on — say who you are,
what these events mean, and what you hope grows from them.)
`, prefix, n, len(blobName))
		if err := os.WriteFile(intentPath, []byte(stub), 0644); err != nil {
			return err
		}
	}
	// The sender remembers giving: if it is not an event, it did not happen.
	given, _ := json.Marshal(map[string]any{"prefix": prefix, "events": n, "files": len(blobName), "dir": dir})
	e := newEvent("seed.exported", given)
	if err := appendEvent(home, &e); err != nil {
		return err
	}
	refreshSite(home)
	fmt.Fprintf(os.Stderr, "self: exported %d event(s) and %d file(s) to %s — edit %s, then send the directory\n",
		n, len(blobName), dir, intentPath)
	return nil
}

// payloadHashes finds the blob hashes a payload references: any 64-hex run,
// wherever it sits in the JSON — commands name files by hash in whatever
// field fits their domain, so a reference is recognized by shape, not by a
// blessed field name.
func payloadHashes(payload json.RawMessage) []string {
	var out []string
	for _, m := range hexRunPattern.FindAllString(string(payload), -1) {
		if len(m) == 64 {
			out = append(out, m)
		}
	}
	return out
}

var hexRunPattern = regexp.MustCompile(`[0-9a-f]{64,}`)

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// depositSeedFile realizes one file.stored deposit at grow time. A seed may
// carry a files/ dir next to seed.jsonl; a deposit event names one of those
// files by its human name, and growing copies the bytes into the instance's
// blob store and completes the payload — hash, size, mime — from the bytes
// themselves. An author-pinned sha256 is verified, never trusted; an omitted
// one is filled in, so authoring a seed never requires computing a hash by
// hand.
func depositSeedFile(home, seedDir string, payload json.RawMessage) (json.RawMessage, error) {
	var p struct {
		Name   string `json:"name"`
		Sha256 string `json:"sha256"`
	}
	if json.Unmarshal(payload, &p) != nil || p.Name == "" {
		return nil, fmt.Errorf(`a file.stored deposit needs {"name": …} naming a file under the seed's files/`)
	}
	base := filepath.Base(p.Name)
	f, err := os.Open(filepath.Join(seedDir, "files", base))
	if err != nil {
		return nil, fmt.Errorf("seed file %q: %w", p.Name, err)
	}
	defer f.Close()
	hash, size, head, err := storeBlob(home, f)
	if err != nil {
		return nil, err
	}
	if p.Sha256 != "" && p.Sha256 != hash {
		return nil, fmt.Errorf("seed file %q hashes to %s, not the declared %s", p.Name, hash, p.Sha256)
	}
	full, _ := json.Marshal(map[string]any{
		"name": base, "mime": blobMime(base, head), "size": size, "sha256": hash,
	})
	return full, nil
}
