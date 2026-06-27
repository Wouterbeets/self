package kernel

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// The kernel signs every compiled script with a per-home secret, so provenance
// is intrinsic to the receipt rather than enforced by where it was written. Only
// the kernel knows the secret, so only kernel-authored script.compiled receipts
// verify: a seed or command may append a script.compiled to the log, but an
// unsigned (or wrong-key) one never installs. That replaces the ingress-filter
// "reserve" — script.compiled is now an ordinary logged event whose *power*
// comes from a signature only the kernel can produce.
//
// The secret lives in SELF_HOME (like an ssh host key), never in the event log,
// and is per-home: signatures are meaningful only on the receiver that produced
// them. That's exactly right for the thesis — you can't import another node's
// binaries, only its declarations, which your kernel recompiles and re-signs.

func secretPath(home string) string { return filepath.Join(home, ".secret") }

// loadOrCreateSecret returns the home's signing key, minting it (32 random
// bytes, 0600) on first use.
func loadOrCreateSecret(home string) ([]byte, error) {
	p := secretPath(home)
	if data, err := os.ReadFile(p); err == nil {
		if key, derr := hex.DecodeString(strings.TrimSpace(string(data))); derr == nil && len(key) > 0 {
			return key, nil
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, []byte(hex.EncodeToString(key)), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

// InitSecret mints the signing key at `self init` time (idempotent).
func InitSecret(home string) error {
	_, err := loadOrCreateSecret(home)
	return err
}

// signScript returns the hex HMAC over (type, name, script). All three are
// bound, so a valid receipt can't be relabeled to install one capability's
// bytes under another's name.
func signScript(secret []byte, typ, name, script string) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(typ))
	m.Write([]byte{0})
	m.Write([]byte(name))
	m.Write([]byte{0})
	m.Write([]byte(script))
	return hex.EncodeToString(m.Sum(nil))
}

// compiledReceipt is the script.compiled payload: the compiled bytes plus a
// signature proving the kernel authored them.
type compiledReceipt struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Script string `json:"script"`
	Sig    string `json:"sig"`
}

// SignedReceipt builds a signed script.compiled payload for home.
func SignedReceipt(home, typ, name, script string) (json.RawMessage, error) {
	secret, err := loadOrCreateSecret(home)
	if err != nil {
		return nil, err
	}
	return json.Marshal(compiledReceipt{typ, name, script, signScript(secret, typ, name, script)})
}

// verifyReceipt reports whether payload is a script.compiled this home's kernel
// actually signed. A constant-time compare guards the (academic, for a PoC)
// timing channel.
func verifyReceipt(secret []byte, payload json.RawMessage) bool {
	var r compiledReceipt
	if json.Unmarshal(payload, &r) != nil || r.Sig == "" || r.Script == "" {
		return false
	}
	want := signScript(secret, r.Type, r.Name, r.Script)
	return hmac.Equal([]byte(want), []byte(r.Sig))
}
