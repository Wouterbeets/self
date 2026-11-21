package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const untouchedAfter = 30 * 24 * time.Hour

func usedDir(home string) string { return filepath.Join(home, "used") }

func stampUse(home, typ, name string) {
	dir := usedDir(home)
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	since := filepath.Join(dir, ".since")
	if _, err := os.Stat(since); errors.Is(err, fs.ErrNotExist) {
		os.WriteFile(since, nil, 0o644)
	}
	p := filepath.Join(dir, typ+"."+name)
	now := time.Now()
	if os.Chtimes(p, now, now) != nil {
		os.WriteFile(p, nil, 0o644)
	}
}

func trackedSince(home string) (time.Time, bool) {
	fi, err := os.Stat(filepath.Join(usedDir(home), ".since"))
	if err != nil {
		return time.Time{}, false
	}
	return fi.ModTime(), true
}

func lastUses(home string, st *state) map[string]time.Time {
	last := map[string]time.Time{}
	prefix := doorCLI + ":"
	for i := range st.Events {
		e := &st.Events[i]
		if e.Name == "capability.kept" {
			var k struct{ Type, Name string }
			if json.Unmarshal(e.Payload, &k) == nil && k.Type != "" && e.OccurredAt.After(last[k.Type+"."+k.Name]) {
				last[k.Type+"."+k.Name] = e.OccurredAt
			}
			continue
		}
		if name, ok := strings.CutPrefix(e.Via, prefix); ok {
			k := kindCommand + "." + name
			if e.OccurredAt.After(last[k]) {
				last[k] = e.OccurredAt
			}
		}
	}
	entries, _ := os.ReadDir(usedDir(home))
	for _, d := range entries {
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			continue
		}
		if fi, err := d.Info(); err == nil && fi.ModTime().After(last[d.Name()]) {
			last[d.Name()] = fi.ModTime()
		}
	}
	return last
}

func lately(st *state, last map[string]time.Time, n int) []string {
	type use struct {
		name string
		at   time.Time
	}
	var uses []use
	for _, c := range st.list(kindCommand) {
		if at, ok := last[c.Type+"."+c.Name]; ok && time.Since(at) < untouchedAfter {
			uses = append(uses, use{c.Name, at})
		}
	}
	sort.SliceStable(uses, func(i, j int) bool { return uses[i].at.After(uses[j].at) })
	var names []string
	for _, u := range uses[:min(len(uses), n)] {
		names = append(names, u.name)
	}
	return names
}

func untouched(home string, st *state, last map[string]time.Time) []string {
	since, ok := trackedSince(home)
	if !ok || time.Since(since) < untouchedAfter {
		return nil
	}
	var names []string
	for _, c := range st.list(kindCommand) {
		if c.Receipt == nil || time.Since(c.RcptAt) < untouchedAfter {
			continue
		}
		if at := last[c.Type+"."+c.Name]; time.Since(at) >= untouchedAfter {
			names = append(names, c.Name)
		}
	}
	return names
}

func eventAt(st *state, seq int) time.Time {
	if i := seq - 1; i >= 0 && i < len(st.Events) && st.Events[i].Seq == seq {
		return st.Events[i].OccurredAt
	}
	for i := range st.Events {
		if st.Events[i].Seq == seq {
			return st.Events[i].OccurredAt
		}
	}
	return time.Time{}
}
