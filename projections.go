package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// runProjection replays the whole log through a projector script and returns
// the HTML it emits. Run it twice, get the same page — a pure function of the log.
func runProjection(home, name string) ([]byte, error) {
	bin, _ := scriptPath(home, "projector", name)
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("projection %q not found", name)
	}
	events, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "SELF_HOME="+home)
	cmd.Dir = home
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	feedEvents(stdin, events)
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("projection %q exited: %w", name, err)
	}
	return out.Bytes(), nil
}

func projectToSite(home, name string) error {
	page, err := runProjection(home, name)
	if err != nil {
		return err
	}
	out := filepath.Join(home, "site", name+".html")
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	return os.WriteFile(out, page, 0644)
}

func refreshProjections(home string) {
	root := filepath.Join(home, "capabilities", "projectors")
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "run" {
			return nil
		}
		name, _ := filepath.Rel(root, filepath.Dir(p)) // the dir is the name; nesting nests
		if err := projectToSite(home, name); err != nil {
			fmt.Fprintf(os.Stderr, "self: projection %q failed: %s\n", name, err)
		}
		return nil
	})
}

// refreshSite writes every kernel-resident view of state and re-runs every
// declared projector. Call this whenever the log changes: it keeps disk in
// lockstep with the log so a brain (or a human, or an external agent) reading
// files under SELF_HOME/site/ sees current state, never a stale view. There is
// no internal state the kernel renders into a brain prompt that is not on disk.
// The brief is written LAST, after the projections, so a brain that reads the
// brief and then follows its pointers to site/*.html always finds pages at
// least as fresh as the brief that sent it there.
func refreshSite(home string) {
	renderKernelHTML(home)
	renderCoreFile(home)
	refreshProjections(home)
	renderBriefFile(home)
}
