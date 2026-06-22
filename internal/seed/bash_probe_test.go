package seed

import "testing"

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
	}
	for name, cmd := range escapes {
		if _, err := runBash(dir, cmd); err == nil {
			t.Errorf("%s: %q should be blocked but was allowed", name, cmd)
		}
	}
}
