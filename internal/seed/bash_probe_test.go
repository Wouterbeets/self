package seed

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression tests for the sandbox escapes found while probing the original
// denylist. Each of these slipped through the denylist; the allowlist must
// block them all. (See runBash / allowedInspectors in bash.go.)
func TestBashBlocksKnownEscapes(t *testing.T) {
	dir := t.TempDir()
	escapes := map[string]string{
		"awk system()":          `awk 'BEGIN{system("id")}'`,
		"find -delete":          `find . -maxdepth 0 -delete`,
		"find -exec":            `find . -maxdepth 0 -exec id {} ;`,
		"command substitution":  `echo $(id)`,
		"backtick substitution": "echo `id`",
		"interpreter via path":  `/usr/bin/python3 -c 'import os;os.system("id")'`,
		"editor shell-out":      `vim -h`,
		"xargs arbitrary":       `echo id | xargs sh -c`,
		"env var-run":           `env X=1 id`,
		// File-write escapes through tools that are otherwise read-only: the
		// allowlist's whole point is "look but not touch", so a write primitive
		// in an allowlisted tool is as much an escape as a denied command.
		"find -fprint0":  `find . -maxdepth 0 -fprint0 pwned`,
		"sort -o":        `sort -o pwned /etc/hostname`,
		"sort --output":  `sort --output=pwned /etc/hostname`,
		"sort bundled o": `sort -uo pwned /etc/hostname`,
		"uniq OUT":       `uniq /etc/hostname pwned`,
		// xxd's second positional operand is an OUTPUT file, and `xxd -r -p`
		// reverts hex to arbitrary bytes — the same write shape as `uniq IN OUT`.
		// It's off the allowlist entirely (od covers hex inspection), so it must
		// be denied as an unknown inspector even where xxd is installed.
		"xxd outfile":  `xxd /etc/hostname pwned`,
		"xxd -r write": `echo 23212f62696e2f7368 | xxd -r -p - pwned`,
	}
	for name, cmd := range escapes {
		if _, err := runBash(dir, cmd); err == nil {
			t.Errorf("%s: %q should be blocked but was allowed", name, cmd)
		}
	}
}

// The write-escape fixes must not break the read-only forms of the same tools —
// these stay allowed (output goes to stdout, no file operand).
func TestBashAllowsReadOnlySortUniqFind(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("b\na\nb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ok := []string{
		"sort data.txt",
		"sort -u data.txt",
		"sort data.txt | uniq -c",
		"uniq -c data.txt",
		"uniq -f 1 data.txt",
		"find . -maxdepth 1 -name '*.txt' -printf '%p\\n'",
	}
	for _, cmd := range ok {
		if _, err := runBash(dir, cmd); err != nil {
			t.Errorf("%q should be allowed: %v", cmd, err)
		}
	}
}
