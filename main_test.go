package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets the test binary serve as the playpen's child half: the jail
// re-execs /proc/self/exe, and under `go test` that is this binary, not self.
// Without this dispatch the probe would recurse into the test suite itself.
func TestMain(m *testing.M) {
	if len(os.Args) == 4 && os.Args[1] == "__jail" {
		if err := cmdJail(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "jail: %s\n", err)
			os.Exit(125)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLogAppendRead(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		e := newEvent(name, json.RawMessage(`{"x":1}`))
		if err := appendEvent(home, &e); err != nil {
			t.Fatal(err)
		}
	}
	events, err := readEvents(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	for i, e := range events {
		if e.Seq != i+1 {
			t.Errorf("event %d has seq %d", i, e.Seq)
		}
		if e.ID == "" || e.OccurredAt.IsZero() {
			t.Errorf("event %d missing id or timestamp", i)
		}
	}
}

// TestStrangeLoop drives the whole kernel loop offline: a declaration arrives
// as an event, the (stub) compiler turns it into an installed script with a
// signed receipt, running the command appends its event, and the projection
// re-renders to site/ showing it. This is the spirit in one test.
func TestStrangeLoop(t *testing.T) {
	t.Setenv("SELF_LLM_STUB", "1")
	home := t.TempDir()

	decls := []Event{
		newEvent("command.declared", json.RawMessage(
			`{"name":"note","description":"take a note","params":{"text":"string"},"event":{"name":"note.taken","fields":{"title":"string"}}}`)),
		newEvent("projector.declared", json.RawMessage(
			`{"name":"board","description":"all notes","consumes":["note.taken"]}`)),
	}
	if err := ingest(home, decls); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		filepath.Join(home, "capabilities", "commands", "note"),
		filepath.Join(home, "capabilities", "projectors", "board"),
	} {
		if !fileExists(p) {
			t.Fatalf("strange loop did not install %s", p)
		}
	}

	// Each compile logged a receipt this home's kernel signed.
	secret, err := loadSecret(home)
	if err != nil {
		t.Fatal(err)
	}
	events, _ := readEvents(home)
	receipts := 0
	for _, e := range events {
		if e.Name != "script.compiled" {
			continue
		}
		if _, ok := verifiedReceipt(secret, e.Payload); !ok {
			t.Errorf("seq %d: receipt does not verify", e.Seq)
		}
		receipts++
	}
	if receipts != 2 {
		t.Fatalf("got %d signed receipts, want 2", receipts)
	}

	// Run the grown command; its event must land on the log and in the view.
	if _, err := runCommand(home, "note", []string{"water", "the", "garden"}); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(home, "site", "board.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "water the garden") {
		t.Fatalf("board.html does not show the note:\n%s", page)
	}
}

// TestForgedReceiptIsInert pins the trust model: anything may append a
// script.compiled, but only a kernel-signed receipt ever installs.
func TestForgedReceiptIsInert(t *testing.T) {
	home := t.TempDir()
	if _, err := loadSecret(home); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(receipt{"command", "evil", "#!/bin/sh\necho pwned", "a liar about who wrote this", "deadbeef"})
	e := newEvent("script.compiled", payload)
	if err := appendEvent(home, &e); err != nil {
		t.Fatal(err)
	}
	if err := rehydrate(home); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(home, "capabilities", "commands", "evil")) {
		t.Fatal("a forged receipt installed")
	}
}

// TestGardenRehydrates resurrects the committed body in garden/ — a real
// organism stored as just events.jsonl + .secret — and checks that every organ
// it grew comes back from the log alone. If this passes, the minimal kernel
// still carries the spirit.
func TestGardenRehydrates(t *testing.T) {
	home := t.TempDir()
	for _, f := range []string{"events.jsonl", ".secret"} {
		data, err := os.ReadFile(filepath.Join("garden", f))
		if err != nil {
			t.Fatalf("the garden body is missing: %s", err)
		}
		if err := os.WriteFile(filepath.Join(home, f), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := rehydrate(home); err != nil {
		t.Fatal(err)
	}
	organs := map[string][]string{
		"commands":   {"note", "claim", "verify", "bequeath", "awaken", "wonder", "resolve", "weigh"},
		"projectors": {"chronicle", "pulse", "notes", "ledger", "inheritance", "lineage", "questions", "toll"},
	}
	for kind, names := range organs {
		for _, name := range names {
			if !fileExists(filepath.Join(home, "capabilities", kind, name)) {
				t.Errorf("garden %s %q did not rehydrate", strings.TrimSuffix(kind, "s"), name)
			}
		}
	}
	// The projections replayed too — the previous mind's letter is readable.
	page, err := os.ReadFile(filepath.Join(home, "site", "inheritance.html"))
	if err != nil {
		t.Fatalf("inheritance did not render: %s", err)
	}
	if len(page) == 0 {
		t.Fatal("inheritance rendered empty")
	}
}

// TestPlaypen pins the containment contract of the brain's full-bash jail:
// real execution inside, with the signing key absent, writes confined, and
// the network dark. Where the platform cannot jail, the kernel must fall
// back to the fail-closed read-only allowlist — never fail open.
func TestPlaypen(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "events.jsonl"),
		[]byte(`{"seq":1,"name":"kernel.initialized","payload":{}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".secret"), []byte("sacred"), 0600); err != nil {
		t.Fatal(err)
	}

	// the fallback must refuse writes regardless of platform support
	t.Setenv("SELF_SANDBOX", "0")
	if p := newPlaypen(home); p != nil {
		t.Fatal("SELF_SANDBOX=0 must disable the playpen")
	}
	if out := readOnlyBash(home, "rm events.jsonl"); !strings.Contains(out, "not on the read-only allowlist") {
		t.Fatalf("fallback failed open: %q", out)
	}
	t.Setenv("SELF_SANDBOX", "")

	p := newPlaypen(home)
	if p == nil {
		t.Skip("no user-namespace support here — playpen unavailable, fallback covered above")
	}
	defer p.close()

	// full bash, real execution, state that persists across calls
	p.run("echo tested-by-execution > proof.txt")
	if out := p.run("cat proof.txt"); !strings.Contains(out, "tested-by-execution") {
		t.Fatalf("playpen state did not persist: %q", out)
	}

	// the body copy is real and the log is readable
	if out := p.run("grep -c kernel.initialized events.jsonl"); !strings.Contains(out, "1") {
		t.Fatalf("body copy missing the log: %q", out)
	}

	// the one file that must never enter, never enters
	if out := p.run("ls -a /body"); strings.Contains(out, ".secret") {
		t.Fatalf("the signing key entered the playpen: %q", out)
	}

	// writes cannot leave the jail
	if out := p.run("touch /usr/escaped 2>/dev/null && echo ESCAPED || echo confined"); !strings.Contains(out, "confined") {
		t.Fatalf("write escaped the jail: %q", out)
	}

	// the network namespace is dark
	if out := p.run("(echo x > /dev/tcp/1.1.1.1/80) 2>/dev/null && echo ONLINE || echo dark"); !strings.Contains(out, "dark") {
		t.Fatalf("the playpen reached the network: %q", out)
	}

	// and nothing done inside touched the real body
	if _, err := os.Stat(filepath.Join(home, "proof.txt")); err == nil {
		t.Fatal("playpen write leaked into the real home")
	}
	if data, _ := os.ReadFile(filepath.Join(home, ".secret")); string(data) != "sacred" {
		t.Fatal("the real secret was disturbed")
	}
}

// TestReceiptProvenance pins the by-line: authorship is inside the signature,
// so it can no more be forged, stripped, or moved than the script itself —
// while receipts minted before provenance existed still verify.
func TestReceiptProvenance(t *testing.T) {
	home := t.TempDir()
	secret, err := loadSecret(home)
	if err != nil {
		t.Fatal(err)
	}

	mint := func(r receipt) json.RawMessage {
		p, _ := json.Marshal(r)
		return p
	}

	// a signed by-line verifies, and survives the round trip
	good := receipt{"command", "graze", "#!/bin/sh\necho hi", "the ninth mind, a Claude", ""}
	good.Sig = sign(secret, good.Type, good.Name, good.Script, good.By)
	if r, ok := verifiedReceipt(secret, mint(good)); !ok || r.By != good.By {
		t.Fatal("signed provenance did not verify")
	}

	// legacy receipts (no by) still verify by the old formula
	legacy := receipt{"command", "note", "#!/bin/sh\necho old", "", ""}
	legacy.Sig = sign(secret, legacy.Type, legacy.Name, legacy.Script, "")
	if _, ok := verifiedReceipt(secret, mint(legacy)); !ok {
		t.Fatal("legacy receipt no longer verifies — old bodies would not rehydrate")
	}

	// authorship cannot be relabeled
	relabeled := good
	relabeled.By = "some other mind"
	if _, ok := verifiedReceipt(secret, mint(relabeled)); ok {
		t.Fatal("relabeled authorship verified — provenance is forgeable")
	}

	// authorship cannot be stripped by folding it into the script (the
	// concatenation attack the v2 domain separation exists to kill)
	folded := receipt{good.Type, good.Name, good.Script + "\x00" + good.By, "", good.Sig}
	if _, ok := verifiedReceipt(secret, mint(folded)); ok {
		t.Fatal("by-line folded into script verified — field boundaries are ambiguous")
	}

	// and a receipt the kernel mints carries the brain's identity
	c := &llm{stub: true, home: home}
	if got := c.identity(); got != "stub (no LLM)" {
		t.Fatalf("stub identity = %q", got)
	}
	t.Setenv("SELF_BRAIN_ID", "a mind in its own words")
	if got := c.identity(); got != "a mind in its own words" {
		t.Fatalf("SELF_BRAIN_ID override = %q", got)
	}
}
