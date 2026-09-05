package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestRecoveryBeyondOneMegabyte(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "torn", true: "complete"}[complete], func(t *testing.T) {
			h := home(t)
			first := newEvent("first.record", json.RawMessage(`{}`))
			if err := appendEvents(h, []Event{first}); err != nil {
				t.Fatal(err)
			}
			prefix, err := os.ReadFile(logPath(h))
			if err != nil {
				t.Fatal(err)
			}
			large := newEvent("large.record", json.RawMessage(`"`+string(bytes.Repeat([]byte("x"), 2*1024*1024))+`"`))
			large.Seq = 2
			tail, err := json.Marshal(large)
			if err != nil {
				t.Fatal(err)
			}
			if !complete {
				tail = tail[:len(tail)-1]
			}
			if err := os.WriteFile(logPath(h), append(bytes.Clone(prefix), tail...), 0644); err != nil {
				t.Fatal(err)
			}
			if err := appendEvents(h, []Event{newEvent("after.recovery", json.RawMessage(`{}`))}); err != nil {
				t.Fatal(err)
			}
			events, err := readEvents(h)
			want := 2
			if complete {
				want = 3
			}
			if err != nil || len(events) != want || events[want-1].Seq != want {
				t.Fatalf("recovery: count=%d, want=%d, error=%v", len(events), want, err)
			}
			raw, err := os.ReadFile(logPath(h))
			if err != nil || !bytes.HasPrefix(raw, prefix) {
				t.Fatal("recovery changed the committed prefix")
			}
			if complete && !bytes.Equal(events[1].Payload, large.Payload) {
				t.Fatal("recovery changed the complete tail")
			}
		})
	}
}

func TestInvalidAppendDoesNotRepairTail(t *testing.T) {
	h := home(t)
	torn := []byte(`{"name":"torn.record"`)
	if err := os.WriteFile(logPath(h), torn, 0644); err != nil {
		t.Fatal(err)
	}
	if err := appendEvents(h, []Event{newEvent("invalid.payload", json.RawMessage(`{`))}); err == nil {
		t.Fatal("invalid payload succeeded")
	}
	raw, err := os.ReadFile(logPath(h))
	if err != nil || !bytes.Equal(raw, torn) {
		t.Fatal("invalid append modified the log")
	}
}
