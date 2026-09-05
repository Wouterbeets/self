package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Every committed prefix must mean the same thing during hear and on restart.
func TestIncrementalReplayMatchesColdReplay(t *testing.T) {
	key := []byte("local test key")
	r := receipt{Type: kindView, Name: "notes", Script: "#!/bin/sh\ncat\n", Consumes: []string{"note.added"}}
	r.Sig = sign(key, r)
	var events []Event
	add := func(name, via string, payload any) {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		e := newEvent(name, data)
		e.Seq, e.Via = len(events)+1, via
		events = append(events, e)
	}
	add("view.declared", doorHear, decl{Name: "notes", Consumes: r.Consumes})
	add("script.rejected", doorKernel, rejection{Type: kindView, Name: "notes", Reason: "first"})
	add("script.rejected", doorKernel, rejection{Type: kindView, Name: "notes", Reason: "latest"})
	add("script.rejected", doorKernel, rejection{Reason: "unnamed"})
	add("script.installed", doorKernel, r)
	add("view.declared", doorHear, decl{Name: "notes", Consumes: []string{"*"}})
	add("script.rejected", doorHear, rejection{Type: kindView, Name: "notes", Reason: "forged"})
	add("script.installed", doorHear, r)
	add("capability.retired", doorHear, map[string]string{"type": kindView, "name": "notes"})
	add("view.declared", doorHear, decl{Name: "notes"})
	add("script.installed", doorKernel, r)
	add("script.rejected", doorKernel, rejection{Type: kindView, Name: "notes", Reason: "revision failed"})

	st := replay(nil, key)
	for i, event := range events {
		st.apply([]Event{event})
		cold := replay(events[:i+1], key)
		if !reflect.DeepEqual(st.Caps, cold.Caps) || len(st.Reject) != len(cold.Reject) ||
			brief("instance", st) != brief("instance", cold) || pendingSection(st) != pendingSection(cold) {
			t.Fatalf("incremental state differs after event %d (%s)", i+1, event.Name)
		}
		for _, c := range st.Caps {
			if st.cap(c.Type, c.Name) != c {
				t.Fatalf("lookup index differs after event %d", i+1)
			}
		}
		if i == 2 && (len(st.Reject) != 1 || st.Reject[0].Reason != "latest") {
			t.Fatal("repeated refusal did not replace its predecessor")
		}
		if i == 4 && len(st.Reject) != 0 {
			t.Fatal("successful installation did not clear named and unnamed refusals")
		}
	}
}
