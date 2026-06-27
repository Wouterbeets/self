package seed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBashReadOnlyCommand(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0644)

	out, err := runBash(dir, "cat test.txt")
	if err != nil {
		t.Fatalf("cat failed: %v", err)
	}
	if strings.TrimSpace(out) != "hello world" {
		t.Errorf("cat output = %q, want %q", out, "hello world")
	}
}

func TestBashListCommands(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "capabilities", "commands"), 0755)
	os.WriteFile(filepath.Join(dir, "capabilities", "commands", "note"), []byte("#!/usr/bin/env python3\n"), 0755)

	out, err := runBash(dir, "ls capabilities/commands/")
	if err != nil {
		t.Fatalf("ls failed: %v", err)
	}
	if !strings.Contains(out, "note") {
		t.Errorf("ls output missing note: %q", out)
	}
}

func TestBashGrepEvents(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(`{"name":"note.captured","payload":{"title":"hello"}}
{"name":"note.captured","payload":{"title":"world"}}
`), 0644)

	out, err := runBash(dir, "grep -c note.captured events.jsonl")
	if err != nil {
		t.Fatalf("grep failed: %v", err)
	}
	if strings.TrimSpace(out) != "2" {
		t.Errorf("grep -c output = %q, want 2", out)
	}
}

func TestBashBlocksDestructiveCommands(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0644)

	blocked := []string{
		"rm test.txt",
		"mv test.txt other.txt",
		"cp test.txt other.txt",
		"mkdir newdir",
		"chmod 777 test.txt",
		"touch newfile",
		"tee test.txt",
		"dd if=/dev/zero of=test.txt",
	}

	for _, cmd := range blocked {
		_, err := runBash(dir, cmd)
		if err == nil {
			t.Errorf("command %q should be blocked but wasn't", cmd)
		}
	}
}

func TestBashBlocksNetworkCommands(t *testing.T) {
	dir := t.TempDir()

	blocked := []string{
		"curl http://example.com",
		"wget http://example.com",
		"nc localhost 8080",
		"ssh localhost",
		"scp file localhost:/tmp",
	}

	for _, cmd := range blocked {
		_, err := runBash(dir, cmd)
		if err == nil {
			t.Errorf("command %q should be blocked but wasn't", cmd)
		}
	}
}

func TestBashBlocksInterpreters(t *testing.T) {
	dir := t.TempDir()

	blocked := []string{
		"python3 -c 'print(1)'",
		"python -c 'print(1)'",
		"node -e 'console.log(1)'",
		"perl -e 'print 1'",
		"ruby -e 'puts 1'",
	}

	for _, cmd := range blocked {
		_, err := runBash(dir, cmd)
		if err == nil {
			t.Errorf("command %q should be blocked but wasn't", cmd)
		}
	}
}

func TestBashBlocksRedirection(t *testing.T) {
	dir := t.TempDir()

	blocked := []string{
		"echo hello > test.txt",
		"echo hello >> test.txt",
		"cat < test.txt",
	}

	for _, cmd := range blocked {
		_, err := runBash(dir, cmd)
		if err == nil {
			t.Errorf("command %q should be blocked but wasn't", cmd)
		}
	}
}

func TestBashBlocksSedInPlace(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

	_, err := runBash(dir, "sed -i 's/hello/world/' test.txt")
	if err == nil {
		t.Error("sed -i should be blocked")
	}
}

func TestBashBlocksSed(t *testing.T) {
	// sed is no longer allowlisted: its w/W (write-to-file) commands and the
	// GNU `e` flag (execute) are shell-exec/file-write escapes. Read-only line
	// selection is covered by head/tail/grep/cut, which are allowlisted.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0644)

	if _, err := runBash(dir, "sed -n '1p' test.txt"); err == nil {
		t.Error("sed should be blocked (not on the read-only allowlist)")
	}
}

func TestBashAllowsInspectionPipelines(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "events.jsonl"),
		[]byte("a note.captured\nb other\nc note.captured\n"), 0644)

	ok := []string{
		"ls",
		"head -2 events.jsonl",
		"cat events.jsonl | grep note.captured | wc -l",
		"ls; echo done",
		"find . -maxdepth 1 -name '*.jsonl'",
	}
	for _, cmd := range ok {
		if _, err := runBash(dir, cmd); err != nil {
			t.Errorf("command %q should be allowed: %v", cmd, err)
		}
	}
}

func TestBashOutputTruncation(t *testing.T) {
	dir := t.TempDir()
	_big := strings.Repeat("x", bashMaxOutput+1000)
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(_big), 0644)

	out, err := runBash(dir, "cat big.txt")
	if err != nil {
		t.Fatalf("cat big.txt failed: %v", err)
	}
	if !strings.Contains(out, "truncated") {
		t.Error("output should be truncated")
	}
}
