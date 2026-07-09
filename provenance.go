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
	if data, err := os.ReadFile(p); err == nil {
		if key, err := hex.DecodeString(strings.TrimSpace(string(data))); err == nil && len(key) > 0 {
			return key, nil
		}
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
