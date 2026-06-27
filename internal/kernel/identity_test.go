package kernel

import (
	"encoding/json"
	"testing"
)

func sampleAttestation() Attestation {
	return Attestation{
		Type: "projector", Name: "hotspots",
		Passed: true, Ran: 2, PassedCount: 2,
		ScriptSHA256:   SHA256Hex([]byte("the script")),
		ExamplesSHA256: SHA256Hex([]byte("the examples")),
	}
}

func TestAttestationRoundTrip(t *testing.T) {
	home := t.TempDir()
	payload, err := SignAttestation(home, sampleAttestation())
	if err != nil {
		t.Fatalf("SignAttestation: %v", err)
	}
	att, ok, err := VerifyAttestation(payload)
	if err != nil || !ok {
		t.Fatalf("valid attestation should verify: ok=%v err=%v", ok, err)
	}
	if att.Name != "hotspots" || !att.Passed {
		t.Errorf("decoded attestation wrong: %+v", att)
	}
	// The home's public key is what was embedded.
	pub, _ := PublicIdentity(home)
	if att.PubKey != pub {
		t.Errorf("embedded pubkey %q != home pubkey %q", att.PubKey, pub)
	}
}

func TestAttestationVerifiableWithoutSecret(t *testing.T) {
	// A third party verifies using only the payload — no access to the signer's
	// home, key, or any shared secret. This is the whole point.
	signer := t.TempDir()
	payload, _ := SignAttestation(signer, sampleAttestation())

	// Verify in a process that has a *different* home entirely.
	other := t.TempDir()
	_, _ = PublicIdentity(other) // mint a different identity, unrelated
	_, ok, err := VerifyAttestation(payload)
	if err != nil || !ok {
		t.Fatalf("attestation should verify from the payload alone: ok=%v err=%v", ok, err)
	}
}

func TestAttestationTamperFails(t *testing.T) {
	home := t.TempDir()
	payload, _ := SignAttestation(home, sampleAttestation())

	var m map[string]any
	json.Unmarshal(payload, &m)
	m["passed"] = false // flip the verdict, keep the signature
	tampered, _ := json.Marshal(m)

	if _, ok, _ := VerifyAttestation(tampered); ok {
		t.Error("a flipped 'passed' must invalidate the signature")
	}

	// also tamper the script hash
	json.Unmarshal(payload, &m)
	m["script_sha256"] = SHA256Hex([]byte("a different script"))
	tampered2, _ := json.Marshal(m)
	if _, ok, _ := VerifyAttestation(tampered2); ok {
		t.Error("a swapped script hash must invalidate the signature")
	}
}

func TestAttestationWrongKeyFails(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	payload, _ := SignAttestation(a, sampleAttestation())
	bPub, _ := PublicIdentity(b)

	var m map[string]any
	json.Unmarshal(payload, &m)
	m["pubkey"] = bPub // claim a different signer, keep a's signature
	forged, _ := json.Marshal(m)

	if _, ok, _ := VerifyAttestation(forged); ok {
		t.Error("a's signature must not verify under b's public key")
	}
}
