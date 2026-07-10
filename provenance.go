package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func loadSecret(home string) ([]byte, error) {
	p := filepath.Join(home, ".secret")
	data, err := os.ReadFile(p)
	if err == nil {
		key, derr := hex.DecodeString(strings.TrimSpace(string(data)))
		if derr != nil || len(key) == 0 {
			// Never rotate over a key that exists but does not decode: every
			// signed receipt verifies only under the original bytes, so
			// replacing them would orphan the instance's whole capability
			// history. Corruption is the user's to repair, loudly.
			return nil, fmt.Errorf("%s exists but is not a hex key — refusing to replace it (all signed receipts verify only under it); restore it from backup", p)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, []byte(hex.EncodeToString(key)), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

type receipt struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Script string `json:"script"`
	By     string `json:"by,omitempty"`
	Sig    string `json:"sig"`
}

func sign(secret []byte, typ, name, script, by string) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("self.receipt.v2\x00"))
	for _, field := range []string{typ, name, script, by} {
		fmt.Fprintf(m, "%d:", len(field))
		m.Write([]byte(field))
	}
	return hex.EncodeToString(m.Sum(nil))
}

func appendReceipt(home, typ, name, script, by string) error {
	secret, err := loadSecret(home)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(receipt{typ, name, script, by, sign(secret, typ, name, script, by)})
	e := newEvent("script.compiled", payload)
	return appendEvent(home, &e)
}

func verifiedReceipt(secret []byte, payload json.RawMessage) (receipt, bool) {
	var r receipt
	if json.Unmarshal(payload, &r) != nil || r.Sig == "" || r.Script == "" || r.Name == "" {
		return r, false
	}
	return r, hmac.Equal([]byte(sign(secret, r.Type, r.Name, r.Script, r.By)), []byte(r.Sig))
}

// compiledReceipt returns the verified receipt an event carries, if it is a
// kernel-signed script.compiled record; anything else in the log is inert data.
func compiledReceipt(secret []byte, e Event) (receipt, bool) {
	if e.Name != "script.compiled" {
		return receipt{}, false
	}
	return verifiedReceipt(secret, e.Payload)
}

// forEachVerifiedReceipt replays the log's signed receipts in order. An
// unreadable log or secret yields no receipts, like every other replay of an
// empty instance.
func forEachVerifiedReceipt(home string, fn func(e Event, r receipt)) {
	events, err := readEvents(home)
	if err != nil {
		return
	}
	secret, err := loadSecret(home)
	if err != nil {
		return
	}
	for _, e := range events {
		if r, ok := compiledReceipt(secret, e); ok {
			fn(e, r)
		}
	}
}

func scriptPath(home, typ, name string) (string, error) {
	switch typ {
	case "command":
		return filepath.Join(home, "capabilities", "commands", name, "run"), nil
	case "projector":
		return filepath.Join(home, "capabilities", "projectors", name, "run"), nil
	}
	return "", fmt.Errorf("unknown capability type %q", typ)
}

func validCapabilityName(name string) bool {
	if name == "" || strings.Contains(name, `\`) {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, ".") {
			return false
		}
	}
	return true
}

func installScript(home, typ, name, script string) error {
	if !validCapabilityName(name) {
		return fmt.Errorf("unsafe capability name %q", name)
	}
	p, err := scriptPath(home, typ, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(script), 0755)
}

type retirement struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func parseRetirement(payload json.RawMessage) (retirement, bool) {
	var d retirement
	if json.Unmarshal(payload, &d) != nil {
		return d, false
	}
	if d.Type != "command" && d.Type != "projector" {
		return d, false
	}
	if !validCapabilityName(d.Name) {
		return d, false
	}
	return d, true
}
