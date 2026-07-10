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

// maxEventLine bounds one log line everywhere bytes enter or leave the log —
// the same limit on the append path, the read path, and every pipe scanner —
// so an oversized record is refused up front instead of poisoning every
// future read of the log.
const maxEventLine = 8 * 1024 * 1024

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
	// A line that fails to parse is fatal corruption — unless it is the log's
	// final line, where it is the partial write of a crashed append: never an
	// acknowledged event, so replays drop it (with a warning) and the next
	// append repairs it (see appendEvent).
	var torn string
	var tornErr error
	lastSeq := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), maxEventLine)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if torn != "" {
			return nil, fmt.Errorf("parse event: %w (not the final line — the log needs manual repair)", tornErr)
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			torn, tornErr = line, err
			continue
		}
		if e.Seq <= lastSeq {
			// The accepted cross-process write race, made audible when it fires.
			fmt.Fprintf(os.Stderr, "self: warning — event %s has seq %d after seq %d (two writers raced an append?)\n", e.ID, e.Seq, lastSeq)
		}
		lastSeq = e.Seq
		events = append(events, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read log: %w", err)
	}
	if torn != "" {
		fmt.Fprintf(os.Stderr, "self: warning — dropping a torn final log line (%d byte(s), likely a crashed append): %s\n", len(torn), tornErr)
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
	last, torn, err := lastSeq(home)
	if err != nil {
		return err
	}
	if torn >= 0 {
		// A torn final line is a crashed append's partial write — never an
		// acknowledged event. Finishing that failed write by removing it is
		// repair, not deletion: the log returns to its last consistent record.
		fmt.Fprintf(os.Stderr, "self: repairing a torn final log line (truncating the partial bytes of a crashed append)\n")
		if err := os.Truncate(logPath(home), torn); err != nil {
			return err
		}
	}
	e.Seq = last + 1
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if len(line) > maxEventLine {
		return fmt.Errorf("event %s is %d bytes — larger than the %d-byte line limit a replay can read back", e.Name, len(line), maxEventLine)
	}
	f, err := os.OpenFile(logPath(home), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, string(line)); err != nil {
		f.Close()
		return err
	}
	// Durability is the log's whole promise: the append is acknowledged only
	// once the bytes are synced, so a crash cannot drop an event a caller was
	// told is in the record.
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// lastSeq reads the highest sequence number by parsing only the log's LAST
// line, scanning backwards in chunks until a newline bounds it. Appends stay
// O(1) as the log grows — no full replay, and no sidecar .seq file that could
// drift from the log: the log itself is the only record of where it ends.
// A final line that does not parse is a torn append: its byte offset comes
// back as torn (−1 when the tail is intact) and the previous line supplies
// the seq, so one crash never wedges the write path. Call under the log lock.
func lastSeq(home string) (seq int, torn int64, err error) {
	f, err := os.Open(logPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, -1, nil
		}
		return 0, -1, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, -1, err
	}
	torn = -1
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
			return 0, -1, err
		}
		tail = append(chunk, tail...)
		for {
			line := bytes.TrimRight(tail, " \t\r\n")
			if len(line) == 0 {
				tail = tail[:0]
				break // nothing but blanks in hand; keep scanning back
			}
			i := bytes.LastIndexByte(line, '\n')
			if i < 0 && off > 0 {
				break // the line starts in an earlier chunk; read more
			}
			var e Event
			if json.Unmarshal(bytes.TrimSpace(line[i+1:]), &e) == nil {
				return e.Seq, torn, nil
			}
			if torn >= 0 {
				return 0, -1, fmt.Errorf("parse last event: two unparseable final lines — the log needs manual repair")
			}
			torn = off + int64(i) + 1 // where the torn line's bytes begin
			if i < 0 {
				return 0, torn, nil // the whole log is one torn line
			}
			tail = tail[:i+1]
		}
	}
	return 0, torn, nil
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
