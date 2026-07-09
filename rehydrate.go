package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func rehydrate(home string) error {
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	secret, err := loadSecret(home)
	if err != nil {
		return err
	}
	latest := map[string]receipt{}
	seen := map[string]bool{}
	var order []string
	installed := 0
	for _, e := range events {
		switch e.Name {
		case "script.compiled":
			r, ok := verifiedReceipt(secret, e.Payload)
			if !ok {
				continue
			}
			key := r.Type + "/" + r.Name
			if !seen[key] {
				seen[key] = true
				order = append(order, key)
			}
			latest[key] = r
		case "capability.retired":
			if d, ok := parseRetirement(e.Payload); ok {
				delete(latest, d.Type+"/"+d.Name)
			}
		}
	}
	os.RemoveAll(filepath.Join(home, "capabilities"))
	os.RemoveAll(filepath.Join(home, "site"))
	for _, key := range order {
		r, live := latest[key]
		if !live {
			continue
		}
		if err := installScript(home, r.Type, r.Name, r.Script); err != nil {
			return err
		}
		installed++
	}
	refreshSite(home)
	fmt.Fprintf(os.Stderr, "self: rehydrated %d capabilit(ies) from the log\n", installed)
	return nil
}
