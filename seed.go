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
