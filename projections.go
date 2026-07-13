package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// A projection is a pure replay: events in, HTML out, same input same bytes.
// That contract is the thing this file protects as logs grow long. Instead of
// incremental protocols, snapshots, or per-projector cache files — state the
// kernel would have to keep in sync with the log — replays are kept cheap two
// stateless ways: a projector is fed only the events its declaration consumes
// (so a narrow page never pays for the whole log), and a projection is re-run
// only when the log grew events it consumes (so an unrelated append never
// re-renders it). The projector script itself stays dumb forever.

// consumesMatch reports whether an event belongs on a projector's stdin. An
// empty consumes list — or "*" — means everything: the projector asked for the
// whole log and gets it, exactly as before consumes became operative.
func consumesMatch(consumes []string, eventName string) bool {
	if len(consumes) == 0 {
		return true
	}
	for _, c := range consumes {
		if c == "*" || c == eventName {
			return true
		}
	}
	return false
}

func matchingEvents(events []Event, consumes []string) []Event {
	if len(consumes) == 0 {
		return events
	}
	var out []Event
	for _, e := range events {
		if consumesMatch(consumes, e.Name) {
			out = append(out, e)
		}
	}
	return out
}

// projectionPage runs one projector script over the events matching its
// declaration and returns the HTML it emits. The projector contract is
// unchanged — stdin is event JSONL, stdout is HTML — the kernel just stopped
// feeding it events it declared no interest in.
func projectionPage(home, name string, d projectorDecl, events []Event) ([]byte, error) {
	bin, err := verifyInstalledScript(home, "projector", name)
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
	feedEvents(stdin, matchingEvents(events, d.Consumes))
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("projection %q exited: %w", name, err)
	}
	return out.Bytes(), nil
}

// runProjection replays the log through a projector script and returns the
// HTML it emits. Run it twice, get the same page — a pure function of the log.
func runProjection(home, name string) ([]byte, error) {
	events, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	_, _, projectors, _ := declaredCaps(events)
	return projectionPage(home, name, projectors[name], events)
}

func sitePagePath(home, name string) string {
	return filepath.Join(home, "site", name+".html")
}

// writeSitePage persists a rendered projection atomically (temp + rename), so
// a reader — the server's fast path, a mind, cat — never sees a half page.
func writeSitePage(home, name string, page []byte) error {
	out := sitePagePath(home, name)
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(out), "."+filepath.Base(out)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(page); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), out)
}

// freshSitePage returns the materialized page when it is provably current —
// its mtime is after the log's, so it was rendered (or verified unchanged)
// after the last append — and nil when it must be re-replayed. A pure
// filesystem check: no cursor files, no render bookkeeping to drift. Any
// append path that forgets to refresh just falls back to a live replay.
func freshSitePage(home, name string) []byte {
	st, err := os.Stat(sitePagePath(home, name))
	if err != nil {
		return nil
	}
	if log, err := os.Stat(logPath(home)); err == nil && !st.ModTime().After(log.ModTime()) {
		return nil
	}
	page, err := os.ReadFile(sitePagePath(home, name))
	if err != nil {
		return nil
	}
	return page
}

// refreshProjections re-renders the declared projections. fresh is the slice
// of just-appended events driving this refresh: a projector consuming none of
// them is skipped — its page cannot have changed — and its mtime is bumped so
// freshSitePage keeps trusting the file. fresh == nil replays everything
// (serve start, rehydrate, learn). One log read serves every projector.
func refreshProjections(home string, fresh []Event) {
	events, err := readEvents(home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "self: refresh projections: %s\n", err)
		return
	}
	_, _, projectors, projOrder := declaredCaps(events)
	for _, name := range projOrder {
		d := projectors[name]
		out := sitePagePath(home, name)
		if fresh != nil && fileExists(out) && len(matchingEvents(fresh, d.Consumes)) == 0 {
			now := time.Now()
			os.Chtimes(out, now, now)
			continue
		}
		page, err := projectionPage(home, name, d, events)
		if err == nil {
			err = writeSitePage(home, name, page)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "self: projection %q failed: %s\n", name, err)
		}
	}
}

// touchesCapabilities reports whether any fresh event is capability
// lifecycle — a declaration, a recompile, a retirement. Those refresh
// everything (a declaration's compile also appends receipts that never pass
// through here); ordinary domain events refresh only their consumers.
func touchesCapabilities(fresh []Event) bool {
	for _, e := range fresh {
		switch e.Name {
		case "command.declared", "projector.declared", "script.compiled", "capability.retired":
			return true
		}
	}
	return false
}

// refreshSite writes every kernel-resident view of state and re-runs every
// declared projector. Call this whenever the log changes: it keeps disk in
// lockstep with the log so a mind (or a human, or an external agent) reading
// files under SELF_HOME/site/ sees current state, never a stale view. There is
// no internal state the kernel renders into a mind prompt that is not on disk.
// The brief is written LAST, after the projections, so a mind that reads the
// brief and then follows its pointers to site/*.html always finds pages at
// least as fresh as the brief that sent it there.
func refreshSite(home string) {
	refreshSiteAfter(home, nil)
}

// refreshSiteAfter is refreshSite scoped to what just happened: given the
// events this ingest appended, only the projections consuming them re-render.
// The kernel's own pages (kernel.html, brief.md) are cheap in-process renders
// and always refresh; projector subprocesses are the cost worth skipping.
func refreshSiteAfter(home string, fresh []Event) {
	if touchesCapabilities(fresh) {
		fresh = nil
	}
	renderKernelHTML(home)
	refreshProjections(home, fresh)
	renderBriefFile(home)
}
