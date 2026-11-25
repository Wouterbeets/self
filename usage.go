package main

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const untouchedAfter = 30 * 24 * time.Hour

func lastUses(st *state) map[string]time.Time {
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
	return last
}

func viaCounts(st *state) map[string]int {
	counts := map[string]int{}
	for i := range st.Events {
		if v := st.Events[i].Via; v != "" {
			counts[v]++
		}
	}
	return counts
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
	names := make([]string, 0, min(len(uses), n))
	for _, u := range uses[:min(len(uses), n)] {
		names = append(names, u.name)
	}
	return names
}

func untouched(st *state, last map[string]time.Time) []string {
	if len(st.Events) == 0 || time.Since(st.Events[0].OccurredAt) < untouchedAfter {
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
