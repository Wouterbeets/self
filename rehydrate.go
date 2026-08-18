package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func rehydrate(home string) error {
	return rehydrateOpt(home, false)
}

func rehydrateOpt(home string, quietUnlessChanged bool) error {
	before := derivedFingerprint(home)
	events, err := readEvents(home)
	if err != nil {
		return err
	}
	var secret []byte
	if len(events) > 0 {
		secret, err = loadSecret(home)
		if err != nil {
			return err
		}
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
	parent := filepath.Dir(home)
	stage, err := os.MkdirTemp(parent, ".self-rehydrate-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if len(events) > 0 {
		if err := copyDerivedFile(logPath(home), logPath(stage), 0644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(stage, ".secret"), []byte(fmt.Sprintf("%x", secret)), 0600); err != nil {
			return err
		}
	}
	for _, key := range order {
		r, live := latest[key]
		if !live {
			continue
		}
		if err := installScript(stage, r.Type, r.Name, r.Script); err != nil {
			return err
		}
		installed++
	}
	if err := renderRehydratedSite(stage, events); err != nil {
		return err
	}
	if err := replaceDerived(home, stage); err != nil {
		return err
	}
	// Kernel-authored pages name the real home, not the temporary staging path.
	renderKernelHTML(home)
	renderBriefFile(home)
	after := derivedFingerprint(home)
	if !quietUnlessChanged || after != before {
		fmt.Fprintf(os.Stderr, "self: rehydrated %d capabilit(ies) from the log\n", installed)
	}
	return nil
}

// derivedFingerprint is content of capabilities/ + site/, so a no-op rebuild
// can stay silent. Paths are relative; order is Walk's.
func derivedFingerprint(home string) string {
	h := sha256.New()
	for _, dir := range []string{"capabilities", "site"} {
		root := filepath.Join(home, dir)
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(home, path)
			if err != nil {
				return nil
			}
			io.WriteString(h, rel)
			h.Write([]byte{0})
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			h.Write(data)
			h.Write([]byte{0})
			return nil
		})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func copyDerivedFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func renderRehydratedSite(stage string, events []Event) error {
	renderKernelHTML(stage)
	_, _, projectors, order := declaredCaps(events)
	for _, name := range order {
		if !fileExists(filepath.Join(stage, "capabilities", "projectors", name, "run")) {
			continue // a declaration whose compile failed has no derived projector
		}
		page, err := projectionPage(stage, name, projectors[name], events)
		if err != nil {
			return err
		}
		if err := writeSitePage(stage, name, page); err != nil {
			return err
		}
	}
	renderBriefFile(stage)
	return nil
}

// replaceDerived swaps a fully reconstructed pair into place and restores the
// previous pair if either rename fails. The log is untouched.
func replaceDerived(home, stage string) error {
	backup, err := os.MkdirTemp(filepath.Dir(home), ".self-derived-backup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(backup)
	names := []string{"capabilities", "site"}
	var movedOld []string
	for _, name := range names {
		if err := os.MkdirAll(filepath.Join(stage, name), 0755); err != nil {
			return err
		}
		old := filepath.Join(home, name)
		if fileExists(old) {
			if err := os.Rename(old, filepath.Join(backup, name)); err != nil {
				for i := len(movedOld) - 1; i >= 0; i-- {
					prior := movedOld[i]
					if fileExists(filepath.Join(backup, prior)) {
						os.Rename(filepath.Join(backup, prior), filepath.Join(home, prior))
					}
				}
				return err
			}
			movedOld = append(movedOld, name)
		}
	}
	installed := 0
	for _, name := range names {
		if err := os.Rename(filepath.Join(stage, name), filepath.Join(home, name)); err != nil {
			for i := installed - 1; i >= 0; i-- {
				os.RemoveAll(filepath.Join(home, names[i]))
			}
			for _, restore := range names {
				old := filepath.Join(backup, restore)
				if fileExists(old) {
					os.Rename(old, filepath.Join(home, restore))
				}
			}
			return err
		}
		installed++
	}
	return nil
}
