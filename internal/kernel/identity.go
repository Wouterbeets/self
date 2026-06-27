package kernel

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A home has TWO keys, for two different trust needs:
//
//   - .secret (HMAC, symmetric — see secret.go): gates *install* of compiled
//     bytes. Only this home can produce a valid script.compiled signature, so
//     you can never install another node's binary. Private by design.
//
//   - .identity (ed25519, asymmetric — here): signs *verification attestations*.
//     A script.verified receipt is a claim — "I ran this binary against these
//     examples and it passed" — that is MEANT to be checked by others. Signed
//     with the home's private key; anyone holding the home's public key can
//     verify it, with no shared secret and without re-running anything.
//
// Install-authority is symmetric and private; a verification claim is asymmetric
// and publicly checkable. That split is what lets sovereign nodes trust each
// other's verified work without a central authority — the math vouches, not a
// platform.

func identityPath(home string) string { return filepath.Join(home, ".identity") }

// loadOrCreateIdentity returns the home's ed25519 private key, minting it (a
// 32-byte seed, stored hex, 0600) on first use.
func loadOrCreateIdentity(home string) (ed25519.PrivateKey, error) {
	p := identityPath(home)
	if data, err := os.ReadFile(p); err == nil {
		if seed, derr := hex.DecodeString(strings.TrimSpace(string(data))); derr == nil && len(seed) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(seed), nil
		}
	}
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	seed := priv.Seed()
	if err := os.WriteFile(p, []byte(hex.EncodeToString(seed)), 0600); err != nil {
		return nil, err
	}
	return priv, nil
}

// InitIdentity mints the signing keypair at `self init` time (idempotent).
func InitIdentity(home string) error {
	_, err := loadOrCreateIdentity(home)
	return err
}

// PublicIdentity returns the home's public key as hex — its shareable identity.
func PublicIdentity(home string) (string, error) {
	priv, err := loadOrCreateIdentity(home)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(priv.Public().(ed25519.PublicKey)), nil
}

// Attestation is the script.verified payload: the verification result, the
// hashes of exactly what was checked (so a third party can confirm the claim
// refers to the binary and examples they hold), and an ed25519 signature over
// all of it plus the signer's public key.
type Attestation struct {
	Type           string   `json:"type"`
	Name           string   `json:"name"`
	Passed         bool     `json:"passed"`
	Ran            int      `json:"ran"`
	PassedCount    int      `json:"passed_count"`
	Failures       []string `json:"failures,omitempty"`
	ScriptSHA256   string   `json:"script_sha256"`
	ExamplesSHA256 string   `json:"examples_sha256"`
	PubKey         string   `json:"pubkey"`
	Sig            string   `json:"sig"`
}

// SHA256Hex is the hex sha256 of s — used to bind an attestation to the exact
// script and examples it concerns.
func SHA256Hex(s []byte) string {
	sum := sha256.Sum256(s)
	return hex.EncodeToString(sum[:])
}

// attestationMessage is the canonical signed bytes: everything that defines the
// claim, except the signature and pubkey themselves. Sign and verify both build
// it the same way, so a tampered field invalidates the signature.
func attestationMessage(a Attestation) []byte {
	return []byte(strings.Join([]string{
		a.Type, a.Name,
		strconv.FormatBool(a.Passed),
		strconv.Itoa(a.Ran), strconv.Itoa(a.PassedCount),
		a.ScriptSHA256, a.ExamplesSHA256,
	}, "\x00"))
}

// SignAttestation fills a's PubKey and Sig with the home's ed25519 identity and
// returns the signed payload as JSON, ready to log as a script.verified event.
func SignAttestation(home string, a Attestation) (json.RawMessage, error) {
	priv, err := loadOrCreateIdentity(home)
	if err != nil {
		return nil, err
	}
	a.PubKey = hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	a.Sig = hex.EncodeToString(ed25519.Sign(priv, attestationMessage(a)))
	return json.Marshal(a)
}

// VerifyAttestation reports whether a script.verified payload carries a valid
// ed25519 signature by the public key it names. It proves the named key signed
// exactly this claim — WHO that key is (and whether you trust them) is the
// reader's separate decision. Requires no secret and no access to the signer.
func VerifyAttestation(payload json.RawMessage) (Attestation, bool, error) {
	var a Attestation
	if err := json.Unmarshal(payload, &a); err != nil {
		return a, false, err
	}
	pub, err := hex.DecodeString(a.PubKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return a, false, fmt.Errorf("bad or missing pubkey")
	}
	sig, err := hex.DecodeString(a.Sig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return a, false, fmt.Errorf("bad or missing signature")
	}
	return a, ed25519.Verify(pub, attestationMessage(a), sig), nil
}
