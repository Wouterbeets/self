package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// An Event carries two provenance fields beside its payload. Via is the
// door: the channel the event entered this log through (cli, http:<addr>,
// mind:<id>, learn:<account>, kernel), stamped by the kernel at append time
// and never accepted from a script, a mind, or a record — it states what the
// kernel itself witnessed. By is the speaker: the caller's claimed identity
// (SELF_CALLER locally, the X-Self-Caller header over HTTP), recorded
// verbatim — a claim, not a verified fact, and rendered as one. Via is local
// like Seq: a deposited event gets this log's door. By is portable like
// OccurredAt: testimony keeps its speaker when it travels between bodies.
type Event struct {
	ID         string          `json:"id"`
	Seq        int             `json:"seq"`
	Name       string          `json:"name"`
	OccurredAt time.Time       `json:"occurred_at"`
	Via        string          `json:"via,omitempty"`
	By         string          `json:"by,omitempty"`
	Payload    json.RawMessage `json:"payload"`
}

// callerClaim is the speaker a local invocation claims to be: SELF_CALLER,
// verbatim, empty when nothing was claimed.
func callerClaim() string { return strings.TrimSpace(os.Getenv("SELF_CALLER")) }

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
	var events []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parse event: %w", err)
		}
		events = append(events, e)
	}
	return events, sc.Err()
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
	last, err := lastSeq(home)
	if err != nil {
		return err
	}
	e.Seq = last + 1
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

// lastSeq reads the highest sequence number by parsing only the log's LAST
// line, scanning backwards in chunks until a newline bounds it. Appends stay
// O(1) as the log grows — no full replay, and no sidecar .seq file that could
// drift from the log: the log itself is the only record of where it ends.
// Call under the log lock.
func lastSeq(home string) (int, error) {
	f, err := os.Open(logPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	off := st.Size()
	var tail []byte
	for off > 0 {
		n := int64(64 * 1024)
		if n > off {
			n = off
		}
		off -= n
		chunk := make([]byte, n)
		if _, err := f.ReadAt(chunk, off); err != nil {
			return 0, err
		}
		tail = append(chunk, tail...)
		line := bytes.TrimRight(tail, " \t\r\n")
		if len(line) == 0 {
			continue // trailing blank lines; keep scanning back
		}
		if i := bytes.LastIndexByte(line, '\n'); i >= 0 {
			line = line[i+1:]
		} else if off > 0 {
			continue // the line starts in an earlier chunk
		}
		var e Event
		if err := json.Unmarshal(bytes.TrimSpace(line), &e); err != nil {
			return 0, fmt.Errorf("parse last event: %w", err)
		}
		return e.Seq, nil
	}
	return 0, nil
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
