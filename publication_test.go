package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentKeyCreationPublishesOneCompleteKey(t *testing.T) {
	h := home(t)
	var wg sync.WaitGroup
	keys := make(chan []byte, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key, err := ensureSecret(h)
			if err != nil {
				t.Errorf("key creation: %v", err)
			}
			keys <- key
		}()
	}
	wg.Wait()
	close(keys)
	for key := range keys {
		if len(key) != 32 || !bytes.Equal(key, secret(h)) {
			t.Fatal("concurrent caller received an incomplete or different key")
		}
	}
}

func TestConcurrentGivePreservesCuration(t *testing.T) {
	h, dir := home(t), t.TempDir()
	heard(t, h, line(t, "note.added", map[string]string{"text": "evidence"}))
	intent := []byte("This is my curated intent.\n")
	if err := os.WriteFile(filepath.Join(dir, "intent.md"), intent, 0644); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 8)
	for range 8 {
		go func() { results <- cmdGive(h, "note.", dir) }()
	}
	wins := 0
	for range 8 {
		if <-results == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("got %d successful exports to one destination", wins)
	}
	a, err := readAccount(dir)
	if err != nil || len(a.Deposit) != 1 || a.RecordHash != a.Manifest.RecordSha256 {
		t.Fatalf("export was incomplete or inconsistent: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "intent.md"))
	if err != nil || !bytes.Equal(got, intent) {
		t.Fatal("export replaced curated intent")
	}
	count := 0
	for _, e := range replayed(t, h).Events {
		if e.Name == "account.given" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("got %d export attestations", count)
	}
}
