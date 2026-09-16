package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	kindCommand = "command"
	kindView    = "view"
)

type decl struct {
	Name        string   `json:"name"`
	Summary     string   `json:"summary,omitempty"`
	Description string   `json:"description"`
	Consumes    []string `json:"consumes,omitempty"`
}

const summaryBudget = 110

func (d decl) summary() string {
	if s := oneLineOrEmpty(d.Summary); s != "" {
		return clip(s, summaryBudget)
	}
	if s := oneLineOrEmpty(d.Description); s != "" {
		return clip(firstSentence(s), summaryBudget)
	}
	return "(no description)"
}

type capability struct {
	Type    string
	Name    string
	Decl    decl
	DeclSeq int
	Receipt *receipt
	RcptSeq int
	Reject  *rejection
}

func (c *capability) key() string { return c.Type + "/" + c.Name }

func (c *capability) Pending() bool { return c.Receipt == nil || c.RcptSeq < c.DeclSeq }

type rejection struct {
	Seq     int    `json:"-"`
	Type    string `json:"type,omitempty"`
	Name    string `json:"name,omitempty"`
	Reason  string `json:"reason"`
	Excerpt string `json:"excerpt,omitempty"`
}

const excerptCap = 1024

type state struct {
	Events  []Event
	Key     []byte
	Caps    []*capability
	byKey   map[string]*capability
	Reject  []*rejection
	Intents []*intentItem
}

func loadState(home string) (*state, error) {
	events, err := readEvents(home)
	if err != nil {
		return nil, err
	}
	return replay(events, secret(home)), nil
}

func replay(events []Event, key []byte) *state {
	st := &state{Events: events[:0], Key: key, byKey: map[string]*capability{}}
	st.apply(events)
	return st
}

func (st *state) apply(events []Event) {
	st.Events = append(st.Events, events...)
	rejects := map[string]*rejection{}
	for _, r := range st.Reject {
		rejects[r.Type+"/"+r.Name] = r
	}

	forget := func(k string) {
		delete(st.byKey, k)
		delete(rejects, k)
		st.Caps = slices.DeleteFunc(st.Caps, func(c *capability) bool { return c.key() == k })
	}
	live := func(typ, name string) *capability {
		k := typ + "/" + name
		if c, ok := st.byKey[k]; ok {
			return c
		}
		c := &capability{Type: typ, Name: name}
		st.byKey[k] = c
		st.Caps = append(st.Caps, c)
		return c
	}

	for _, e := range events {
		st.applyIntent(e)
		switch e.Name {
		case "command.declared", "view.declared":
			typ := strings.TrimSuffix(e.Name, ".declared")
			var d decl
			if json.Unmarshal(e.Payload, &d) != nil || !validCapability(typ, d.Name) {
				continue
			}
			c := live(typ, d.Name)
			c.Decl, c.DeclSeq = d, e.Seq

		case "script.installed":
			if e.Via != doorKernel {
				continue // a genuine receipt replayed through hear would re-install
			}
			r, ok := verifyReceipt(st.Key, e.Payload)
			if !ok {
				continue
			}
			c := live(r.Type, r.Name)
			c.Receipt, c.RcptSeq = &r, e.Seq
			delete(rejects, r.Type+"/"+r.Name)
			for k, rej := range rejects {
				if !validCapability(rej.Type, rej.Name) {
					delete(rejects, k) // nameless refusals cannot close on their own key
				}
			}

		case "script.rejected":
			if e.Via != doorKernel {
				continue
			}
			var r rejection
			if json.Unmarshal(e.Payload, &r) != nil || r.Reason == "" {
				continue
			}
			r.Seq = e.Seq
			rejects[r.Type+"/"+r.Name] = &r

		case "capability.retired":
			var t struct{ Type, Name string }
			if json.Unmarshal(e.Payload, &t) != nil || !validCapability(t.Type, t.Name) {
				continue
			}
			forget(t.Type + "/" + t.Name)
		}
	}

	for _, c := range st.Caps {
		c.Reject = rejects[c.key()]
	}
	st.Reject = st.Reject[:0]
	for _, r := range rejects {
		st.Reject = append(st.Reject, r)
	}
	sort.Slice(st.Reject, func(i, j int) bool { return st.Reject[i].Seq < st.Reject[j].Seq })
}

func (st *state) cap(typ, name string) *capability { return st.byKey[typ+"/"+name] }

func (st *state) list(typ string) []*capability {
	var out []*capability
	for _, c := range st.Caps {
		if c.Type == typ {
			out = append(out, c)
		}
	}
	return out
}

func (st *state) pending() []*capability {
	var out []*capability
	for _, c := range st.Caps {
		if c.Pending() {
			out = append(out, c)
		}
	}
	return out
}

func (st *state) capabilitiesReady() bool { return len(st.pending()) == 0 && len(st.Reject) == 0 }

func validCapability(typ, name string) bool {
	if typ != kindCommand && typ != kindView {
		return false
	}
	if name == "" || strings.Contains(name, `\`) {
		return false
	}
	if len(name) > 200 {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if len(seg) > 64 {
			return false
		}
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, ".") {
			return false
		}
		if seg == "run" {
			return false
		}
	}
	return true
}

func capDir(home string) string  { return filepath.Join(home, "cap") }
func blobDir(home string) string { return filepath.Join(home, "cap", "blob") }

func blobPath(home, sum string) string { return filepath.Join(blobDir(home), sum) }

func linkPath(home, typ, name string) string {
	return filepath.Join(capDir(home), typ, name, "run")
}

func materialize(home string, st *state, typ, name string) (string, error) {
	c := st.cap(typ, name)
	switch {
	case c == nil:
		other := otherKind(typ)
		if st.cap(other, name) != nil {
			return "", fmt.Errorf("no %s %q in this log — but there is a %s by that name: try `self %s %s`",
				typ, name, other, map[string]string{kindCommand: "run", kindView: "view"}[other], name)
		}
		return "", fmt.Errorf("no %s %q in this log — declare it, or check `self brief`", typ, name)
	case c.Receipt == nil && st.Key == nil && len(st.Events) > 0:
		return "", fmt.Errorf("%s %q: no receipt verifies under this instance's key — is .secret missing next to events.jsonl?", typ, name)
	case c.Receipt == nil:
		return "", fmt.Errorf("%s %q is declared but pending: no script has been authored for it yet", typ, name)
	}

	script := c.Receipt.Script
	sum := sha256.Sum256([]byte(script))
	hexsum := hex.EncodeToString(sum[:])
	blob := blobPath(home, hexsum)

	if have, err := os.ReadFile(blob); err != nil || string(have) != script {
		if err == nil {
			fmt.Fprintf(os.Stderr, "self: %s/%s: blob %s did not match its own hash — restored from the receipt at seq %d\n", typ, name, hexsum[:12], c.RcptSeq)
		}
		if err := writeFileAtomic(blob, []byte(script), 0755, os.Rename); err != nil {
			return "", err
		}
	}
	if err := linkBlob(home, typ, name, hexsum); err != nil {
		return "", err
	}
	return blob, nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode, publish func(string, string) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return publish(tmp.Name(), path)
}

func linkBlob(home, typ, name, sum string) error {
	link := linkPath(home, typ, name)
	rel, err := filepath.Rel(filepath.Dir(link), blobPath(home, sum))
	if err != nil {
		return err
	}
	if have, err := os.Readlink(link); err == nil && have == rel {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(link), ".link-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	staged := filepath.Join(tmp, "run")
	if err := os.Symlink(rel, staged); err != nil {
		return err
	}
	return os.Rename(staged, link)
}

func rehydrate(home string) error {
	unlock, err := lockLog(home)
	if err != nil {
		return err
	}
	defer unlock()
	st, err := loadState(home)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	var failures error
	installed, removed := 0, 0
	for _, c := range st.Caps {
		if c.Receipt == nil {
			continue
		}
		sum := sha256.Sum256([]byte(c.Receipt.Script))
		for _, path := range []string{linkPath(home, c.Type, c.Name), blobPath(home, hex.EncodeToString(sum[:]))} {
			for path != capDir(home) {
				keep[path] = true
				path = filepath.Dir(path)
			}
		}
		if _, err := materialize(home, st, c.Type, c.Name); err != nil {
			failures = errors.Join(failures, fmt.Errorf("%s: %w", c.key(), err))
			continue
		}
		installed++
	}
	for _, kind := range []string{kindCommand, kindView, "blob"} {
		err := filepath.WalkDir(filepath.Join(capDir(home), kind), func(path string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil || keep[path] {
				return err
			}
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			removed++
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		})
		failures = errors.Join(failures, err)
	}
	fmt.Fprintf(os.Stderr, "self: %d capabilit(ies) materialized from the log, %d stale path(s) removed\n", installed, removed)
	return failures
}

func scriptEnv(selfHome, work string) []string {
	env := []string{
		"HOME=" + work,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"TZ=UTC",
		"LC_ALL=C",
		"PYTHONHASHSEED=0", // otherwise set iteration is per-process random
	}
	if selfHome != "" {
		env = append(env, "SELF_HOME="+selfHome)
	}
	for _, kv := range os.Environ() {
		if k, _, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(k, "SELF_") && k != "SELF_HOME" {
			env = append(env, kv)
		}
	}
	return env
}

func feed(w io.WriteCloser, events []Event) {
	go func() {
		enc := json.NewEncoder(w)
		for i := range events {
			if err := enc.Encode(events[i]); err != nil {
				break
			}
		}
		w.Close()
	}()
}

func runCommand(home string, st *state, name string, args []string, via, by string, diag ...io.Writer) ([]Event, error) {
	bin, err := materialize(home, st, kindCommand, name)
	if err != nil {
		return nil, err
	}
	stderr := io.Writer(os.Stderr)
	if len(diag) > 0 && diag[0] != nil {
		stderr = diag[0]
	}
	cmd := exec.Command(bin, args...)
	cmd.Env, cmd.Dir, cmd.Stderr = scriptEnv(home, home), home, stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	feed(stdin, st.Events)

	var out []Event
	var parseErr error
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), lineLimit)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		p, ok := eventLine(line)
		if !ok {
			parseErr = fmt.Errorf("command %q printed a line that is not an event: %s", name, trunc(line, 120))
			continue
		}
		e := newEvent(p.Name, p.Payload)
		e.Via, e.By = via, by
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
		return nil, fmt.Errorf("reading command %q output: %w (nothing appended)", name, err)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("command %q exited: %w (nothing appended)", name, err)
	}
	if parseErr != nil {
		return nil, fmt.Errorf("%w (nothing appended)", parseErr)
	}
	if err := ingest(home, out, nil, nil, by, io.Discard); err != nil {
		return nil, err
	}
	return out, nil
}

func runView(home string, st *state, name string, args ...string) ([]byte, error) {
	return runViewDiag(home, st, name, os.Stderr, args...)
}

func runViewDiag(home string, st *state, name string, diag io.Writer, args ...string) ([]byte, error) {
	if name == "log" && st.cap(kindView, "log") == nil {
		all := false
		for _, a := range args {
			if a != "--all" {
				return nil, fmt.Errorf("built-in view %q takes only --all", name)
			}
			all = true
		}
		return builtinLogView(st, all), nil
	}
	return executeView(context.Background(), home, st, name, args, diag, 0)
}

func executeView(ctx context.Context, home string, st *state, name string, args []string, diag io.Writer, waitDelay time.Duration) ([]byte, error) {
	bin, err := materialize(home, st, kindView, name)
	if err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp("", "self-view-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env, cmd.Dir, cmd.Stderr = scriptEnv("", scratch), scratch, diag
	cmd.WaitDelay = waitDelay
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	feed(stdin, consumed(st.Events, st.cap(kindView, name).Receipt.Consumes))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("view %q exited: %w", name, err)
	}
	return out, nil
}

func consumed(events []Event, consumes []string) []Event {
	if len(consumes) == 0 {
		return events
	}
	want := map[string]bool{}
	for _, c := range consumes {
		if c == "*" {
			return events
		}
		want[c] = true
	}
	var out []Event
	for _, e := range events {
		if want[e.Name] {
			out = append(out, e)
		}
	}
	return out
}

const builtinLogTail = 10

func builtinLogView(st *state, all bool) []byte {
	events := st.Events
	var b strings.Builder
	if !all && len(events) > builtinLogTail {
		fmt.Fprintf(&b, "# last %d of %d events · `self view log --all` for the whole log\n", builtinLogTail, len(events))
		events = events[len(events)-builtinLogTail:]
	}
	for _, e := range events {
		by := e.By
		if by == "" {
			by = "-"
		}
		fmt.Fprintf(&b, "%d\t%s\t%s\tvia=%s\tby=%s\t%s\n",
			e.Seq, e.OccurredAt.Format(time.RFC3339), e.Name, e.Via, by,
			trunc(compact(e.Payload), 200))
	}
	return []byte(b.String())
}
