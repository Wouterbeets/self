package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type Event struct {
	ID         string          `json:"id"`
	Seq        int             `json:"seq"`
	Name       string          `json:"name"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

func newEvent(name string, payload json.RawMessage) Event {
	b := make([]byte, 16)
	rand.Read(b)
	return Event{ID: hex.EncodeToString(b), Name: name, OccurredAt: time.Now().UTC(), Payload: payload}
}

func logPath(home string) string { return filepath.Join(home, "events.jsonl") }

func readEvents(home string) ([]Event, error) {
	f, err := os.Open(logPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	// A streaming JSON decoder, not a line scanner: an event line has no fixed
	// size bound — a script.compiled receipt embeds a whole generated script,
	// and the brain seam accepts scripts far larger than one log line used to
	// tolerate. A bufio.Scanner with a fixed max-token cap fails the instant a
	// single line exceeds it, and because every read path (rehydrate included)
	// goes through here, one oversized line would brick the whole instance —
	// including the offline rehydrate that is supposed to be the recovery route.
	// JSONL is valid concatenated JSON, so the decoder reads it with no cap; the
	// only ceiling left is available memory.
	dec := json.NewDecoder(bufio.NewReader(f))
	var events []Event
	for {
		var e Event
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parse event: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}

func appendEvent(home string, e *Event) error {
	if err := os.MkdirAll(home, 0755); err != nil {
		return err
	}
	unlock, err := lockLog(home)
	if err != nil {
		return err
	}
	defer unlock()
	prior, err := readEvents(home)
	if err != nil {
		return err
	}
	e.Seq = 1
	if len(prior) > 0 {
		e.Seq = prior[len(prior)-1].Seq + 1
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(logPath(home), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, string(line))
	return err
}

func lockLog(home string) (func(), error) {
	lf, err := os.OpenFile(logPath(home), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		lf.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
		lf.Close()
	}, nil
}
