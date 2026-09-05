package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRehydrateProtectsLiveBlobWhenLinkFails(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	link := linkPath(h, kindCommand, "entry")
	blob, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(link, 0755); err != nil {
		t.Fatal(err)
	}
	if err := rehydrate(h); err == nil {
		t.Fatal("reconstruction claimed success with an unreplaceable link")
	}
	if _, err := os.Stat(blob); err != nil {
		t.Fatalf("failed link installation cost the live blob: %v", err)
	}
}

func TestRehydrateReportsCleanupFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	h := home(t)
	growJournal(t, h)
	parent := filepath.Join(capDir(h), kindCommand)
	stale := filepath.Join(parent, "stale")
	if err := os.WriteFile(stale, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0755) })
	if err := rehydrate(h); err == nil {
		t.Fatal("reconstruction ignored cleanup permission failure")
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("fixture did not prevent removal: %v", err)
	}
}

func TestRehydrateDoesNotFollowStaleSymlink(t *testing.T) {
	h := home(t)
	growJournal(t, h)
	outside := t.TempDir()
	file := filepath.Join(outside, "keep")
	if err := os.WriteFile(file, []byte("outside the instance"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(blobDir(h), "stale")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := rehydrate(h); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("stale symlink survived: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("cleanup followed the symlink: %v", err)
	}
}
