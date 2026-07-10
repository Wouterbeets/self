package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
