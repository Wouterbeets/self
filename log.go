package main

// The log. One append-only file is the whole authoritative state of an
// instance; everything else in this program is a replay of it. Nothing here
// interprets an event — that is capability.go's job. This file only guarantees
// that what lands, lands exactly once, in order, with provenance the kernel
// itself witnessed.

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// An Event is the only record type. Two provenance fields sit beside the
// payload:
//
//	Via — the door. The channel the kernel witnessed this event entering
//	      through: cli, hear, kernel, learn:<account>. Stamped from what the
//	      kernel saw, never accepted from a script, a mind, or a record. A
//	      local fact, like Seq.
//	By  — the speaker. Whatever the caller claimed (SELF_CALLER), recorded
//	      verbatim as a claim and never verified. Portable, like OccurredAt:
//	      testimony keeps its speaker when it travels between instances.
type Event struct {
	ID         string          `json:"id"`
	Seq        int             `json:"seq"`
	Name       string          `json:"name"`
	OccurredAt time.Time       `json:"occurred_at"`
	Via        string          `json:"via,omitempty"`
	By         string          `json:"by,omitempty"`
	Payload    json.RawMessage `json:"payload"`
}

// The doors. A door is what the kernel saw, so there are exactly as many as
// there are ways in.
const (
	doorCLI    = "cli"    // a local invocation: run, give
	doorHear   = "hear"   // came back through the pipe
	doorKernel = "kernel" // the kernel's own testimony: receipts, refusals, attestations
	doorLearn  = "learn:" // + the account name: a deposit
)

// eventName is the shape every name entering the log must have. Lowercase
// dotted, so an event is never confused with incidental JSON: this is half of
// what makes content-dispatch (see the wire) safe.
var eventName = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)

func validEventName(s string) bool { return eventName.MatchString(s) }

// callerClaim is the speaker a local invocation claims to be — verbatim, empty
// when nothing was claimed.
func callerClaim() string { return strings.TrimSpace(os.Getenv("SELF_CALLER")) }

func newEvent(name string, payload json.RawMessage) Event {
	b := make([]byte, 16)
	rand.Read(b)
	// An absent payload and an explicit null are the same thing said twice, and
	// only one of them is safe for a view to index into. Normalize, so no view
	// ever has to defend against None.
	if len(payload) == 0 || string(bytes.TrimSpace(payload)) == "null" {
		payload = json.RawMessage(`{}`)
	}
	return Event{ID: hex.EncodeToString(b), Name: name, OccurredAt: time.Now().UTC(), Payload: payload}
}

func homeDir() string {
	if v := os.Getenv("SELF_HOME"); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			return abs
		}
		return v
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
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
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
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

// appendEvents writes a batch under one lock: sequence numbers are assigned and
// the lines land together, so a declaration and its receipt cannot be split by
// a concurrent invocation. It mutates the events in place with their Seq.
func appendEvents(home string, evs []Event) error {
	if len(evs) == 0 {
		return nil
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		return err
	}
	unlock, err := lockLog(home)
	if err != nil {
		return err
	}
	defer unlock()
	return appendLocked(home, evs)
}

// appendLocked is appendEvents' body for callers already holding the lock (a
// hear body, which must resolve declared-ness between its own appends).
func appendLocked(home string, evs []Event) error {
	// Every append passes through here, so this is where the log's two
	// well-formedness rules live: a lowercase dotted name, and a payload that
	// is valid UTF-8. A RawMessage is written through verbatim, so raw bytes
	// would land unchanged and break every view that parses the line.
	for i := range evs {
		if !validEventName(evs[i].Name) {
			return fmt.Errorf("event name %q is not lowercase dotted (see self help)", evs[i].Name)
		}
		if !utf8.Valid(evs[i].Payload) {
			return fmt.Errorf("event %q carries a payload that is not valid UTF-8", evs[i].Name)
		}
	}
	last, err := lastSeq(home)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	for i := range evs {
		last++
		evs[i].Seq = last
		line, err := json.Marshal(evs[i])
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	f, err := os.OpenFile(logPath(home), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// lastSeq reads the highest sequence number by parsing only the log's last
// line, scanning backwards in chunks until a newline bounds it. Appends stay
// O(1) as the log grows, and no sidecar file can drift from it: the log is the
// only record of where it ends. Call under the lock.
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
			continue // trailing blank lines
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

// ───────────────────────────────── the key ──────────────────────────────────

func secretPath(home string) string { return filepath.Join(home, ".secret") }

// secret reads the instance key. It NEVER creates one: reads project, writes
// append, and a stray `self` in some directory must not leave a key file
// behind. Absent means absent — nothing verifies, so the instance has no
// capabilities, which is the truth about a directory that is not an instance.
func secret(home string) []byte {
	data, err := os.ReadFile(secretPath(home))
	if err != nil {
		return nil
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) == 0 {
		return nil
	}
	return key
}

// ensureSecret mints the key if it is missing. Only write paths call it.
func ensureSecret(home string) ([]byte, error) {
	if key := secret(home); key != nil {
		return key, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(secretPath(home), []byte(hex.EncodeToString(key)), 0600); err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "self: new instance %s\n", home)
	return key, nil
}

// ─────────────────────────────── the receipt ────────────────────────────────

// A receipt is the kernel's signature over installed bytes. consumes is signed
// with the script because a view is only a pure function relative to the events
// it is fed: sign one without the other and a re-declaration silently changes
// what the old script sees.
type receipt struct {
	Type     string   `json:"type"`
	Name     string   `json:"name"`
	Script   string   `json:"script"`
	Consumes []string `json:"consumes,omitempty"`
	By       string   `json:"by,omitempty"`
	Sig      string   `json:"sig"`
}

func sign(key []byte, r receipt) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("self.receipt.v3\x00"))
	fields := []string{r.Type, r.Name, r.Script, strings.Join(r.Consumes, "\x00"), r.By}
	for _, f := range fields {
		fmt.Fprintf(m, "%d:", len(f))
		m.Write([]byte(f))
	}
	return hex.EncodeToString(m.Sum(nil))
}

// verifyReceipt is the whole trust gate. A receipt that does not verify under
// this instance's key is inert data in the log, no matter who wrote it.
func verifyReceipt(key []byte, payload json.RawMessage) (receipt, bool) {
	var r receipt
	if key == nil || json.Unmarshal(payload, &r) != nil {
		return r, false
	}
	if r.Sig == "" || r.Script == "" || !validCapability(r.Type, r.Name) {
		return r, false
	}
	return r, hmac.Equal([]byte(sign(key, r)), []byte(r.Sig))
}
