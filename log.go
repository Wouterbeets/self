package main

// The log. One append-only file is the whole authoritative state of an
// instance; everything else in this program is a replay of it. Nothing here
// interprets an event — that is capability.go's job. This file only guarantees
// that what lands, lands exactly once, in order, with provenance the kernel
// itself witnessed.

import (
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

// readEvents replays the log from disk.
//
// A record is a line TERMINATED BY A NEWLINE. A trailing fragment with no
// newline was never durably committed — a crash, a full disk, a kill mid-write,
// or simply another process appending this instant — so it is not a record and
// is skipped. Without that rule a single short write bricked the instance
// permanently, rehydrate included, and every read raced every append.
//
// A malformed line in the MIDDLE is different: that is real corruption, and it
// is an error naming the line, because silently skipping it would change the
// instance's state without saying so.
func readEvents(home string) ([]Event, error) {
	data, err := os.ReadFile(logPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var events []Event
	for n, line := range strings.SplitAfter(string(data), "\n") {
		terminated := strings.HasSuffix(line, "\n")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !terminated {
			// The tail of the file, mid-write. Not a record.
			fmt.Fprintf(os.Stderr, "self: ignoring an unterminated final line in events.jsonl (%d bytes) — it was never committed\n", len(line))
			break
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("events.jsonl line %d is not a readable event: %w — the log is authoritative, so fix that line (the rest of the file is intact)", n+1, err)
		}
		events = append(events, e)
	}
	return events, nil
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
		// script.authored is a wire message, not an event. The protocol says it
		// never lands in the log, so no door may land it — a command that
		// emitted one would otherwise leave a lie there.
		if evs[i].Name == "script.authored" {
			return fmt.Errorf("script.authored is a wire message, not an event: it is heard by `self hear`, never appended")
		}
		if !utf8.Valid(evs[i].Payload) {
			return fmt.Errorf("event %q carries a payload that is not valid UTF-8", evs[i].Name)
		}
	}
	last, err := lastSeq(home)
	if err != nil {
		return err
	}
	// If the file does not end in a newline, its tail is an uncommitted
	// fragment. Drop it before appending.
	if err := dropFragment(home); err != nil {
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
	// A batch is all or nothing. On a short write — a full disk is the usual
	// way — roll the file back to where it started, still under the lock, so a
	// torn tail never exists. dropFragment is the backstop for the writes that
	// never reach here at all (a kill, a power cut), not the plan.
	base := int64(0)
	if st, serr := f.Stat(); serr == nil {
		base = st.Size()
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Truncate(base)
		f.Close()
		return err
	}
	return f.Close()
}

// dropFragment removes an unterminated final line before the next append.
//
// This is the one place the log shrinks, and it does not violate append-only: a
// record is a line terminated by a newline, so those bytes were never a record.
// The alternatives are both worse. Appending straight on top glues this batch to
// the fragment and destroys the committed record above it. Merely closing the
// fragment with a newline promotes uncommitted bytes into a permanently
// unreadable middle line — which bricks every later read, as it did before this
// was written this way.
//
// Call under the lock.
func dropFragment(home string) error {
	f, err := os.OpenFile(logPath(home), os.O_RDWR, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return err
	}
	// How many trailing bytes follow the last newline?
	const window = 1024 * 1024
	size := st.Size()
	read := int64(window)
	if read > size {
		read = size
	}
	buf := make([]byte, read)
	if _, err := f.ReadAt(buf, size-read); err != nil {
		return err
	}
	i := bytes.LastIndexByte(buf, '\n')
	if i == int(read)-1 {
		return nil // already ends on a record boundary
	}
	var keep int64
	if i < 0 {
		if read < size {
			return fmt.Errorf("events.jsonl has no newline in its last %d bytes; refusing to guess where the last record ends", window)
		}
		keep = 0 // the whole file is one unterminated fragment
	} else {
		keep = size - read + int64(i) + 1
	}
	fmt.Fprintf(os.Stderr, "self: dropping %d uncommitted byte(s) from the end of events.jsonl — a record is a terminated line, so those bytes were never one\n", size-keep)
	return f.Truncate(keep)
}

// lastSeq reads the sequence number to continue from, by parsing only the tail
// of the log — appends stay O(1) as the log grows, and no sidecar file can drift
// from it: the log is the only record of where it ends.
//
// It applies readEvents' rule. A final line with no newline was never committed,
// so the batch about to be written must not inherit its sequence; the same goes
// for a tail that is terminated but unreadable. Walk back to the last line that
// is genuinely a record. Call under the lock.
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

	// Grow the window backwards until a readable record turns up or the whole
	// file has been read. In the normal case the first 64KB is one line.
	for window := int64(64 * 1024); ; window *= 4 {
		if window > st.Size() {
			window = st.Size()
		}
		buf := make([]byte, window)
		if window > 0 {
			if _, err := f.ReadAt(buf, st.Size()-window); err != nil {
				return 0, err
			}
		}
		lines := bytes.Split(buf, []byte{'\n'})
		// The final element is whatever followed the last newline: an
		// uncommitted fragment, or empty. Never a record.
		lines = lines[:len(lines)-1]
		for i := len(lines) - 1; i >= 0; i-- {
			line := bytes.TrimSpace(lines[i])
			if len(line) == 0 {
				continue
			}
			if i == 0 && window < st.Size() {
				break // this line may start before the window
			}
			var e Event
			if json.Unmarshal(line, &e) != nil {
				continue // not a record; keep walking back
			}
			return e.Seq, nil
		}
		if window >= st.Size() {
			return 0, nil // nothing readable anywhere: start from one
		}
	}
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
	// O_EXCL, so exactly one of two selves racing on a fresh home creates the
	// key and the other reads it. Losing that race silently used to mean
	// signing receipts under a key the instance would then throw away.
	f, err := os.OpenFile(secretPath(home), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			if existing := secret(home); existing != nil {
				return existing, nil
			}
		}
		return nil, err
	}
	if _, err := f.WriteString(hex.EncodeToString(key)); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
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

// sign is the trust gate's arithmetic. Every field is length-prefixed so the
// preimage is injective: two different receipts can never share one.
//
// consumes needs its COUNT prefixed as well as each element. Joining the list
// into a single field left the partition unsigned, so a two-element list and one
// element holding the same bytes with a separator inside hashed identically —
// and that single element matches no event name, which means a validly-signed
// receipt whose view is fed nothing.
func sign(key []byte, r receipt) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("self.receipt.v4\x00"))
	field := func(s string) {
		fmt.Fprintf(m, "%d:", len(s))
		m.Write([]byte(s))
	}
	field(r.Type)
	field(r.Name)
	field(r.Script)
	field(r.By)
	fmt.Fprintf(m, "consumes=%d:", len(r.Consumes))
	for _, c := range r.Consumes {
		field(c)
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
