package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

type Event struct {
	ID         string          `json:"id"`
	Origin     string          `json:"origin,omitempty"` // foreign identity, never authority
	Seq        int             `json:"seq"`
	Name       string          `json:"name"`
	OccurredAt time.Time       `json:"occurred_at"`
	Via        string          `json:"via,omitempty"`
	By         string          `json:"by,omitempty"`
	Payload    json.RawMessage `json:"payload"`
}

const (
	doorCLI    = "cli"
	doorHear   = "hear"
	doorKernel = "kernel"
	doorLearn  = "learn:"
)

var eventName = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)

func validEventName(s string) bool { return eventName.MatchString(s) }

func callerClaim() string { return os.Getenv("SELF_CALLER") }

func newEvent(name string, payload json.RawMessage) Event {
	b := make([]byte, 16)
	rand.Read(b)
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
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			if !terminated {
				fmt.Fprintf(os.Stderr, "self: ignoring an unterminated, incomplete final line in events.jsonl (%d bytes) — it was never committed\n", len(line))
				break
			}
			return nil, fmt.Errorf("events.jsonl line %d is not a readable event: %w — the log is authoritative, so fix that line (the rest of the file is intact)", n+1, err)
		}
		events = append(events, e)
	}
	return events, nil
}

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

func appendLocked(home string, evs []Event) error {
	if len(evs) == 0 {
		return nil
	}
	for i := range evs {
		if !validEventName(evs[i].Name) {
			return fmt.Errorf("event name %q is not lowercase dotted (see self help)", evs[i].Name)
		}
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
	if err := dropFragment(home); err != nil {
		return err
	}
	f, err := os.OpenFile(logPath(home), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		return errors.Join(err, f.Close())
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		return errors.Join(err, f.Truncate(st.Size()), f.Close())
	}
	return f.Close()
}

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
	size := st.Size()
	var tail []byte
	var keep int64
	for window := int64(64 * 1024); ; window *= 4 {
		window = min(window, size)
		buf := make([]byte, window)
		if _, err := f.ReadAt(buf, size-window); err != nil {
			return err
		}
		i := bytes.LastIndexByte(buf, '\n')
		if i == len(buf)-1 {
			return nil
		}
		if i >= 0 || window == size {
			tail, keep = buf[i+1:], size-window+int64(i)+1
			break
		}
	}

	var e Event
	if json.Unmarshal(bytes.TrimSpace(tail), &e) == nil {
		_, err := f.WriteAt([]byte("\n"), size)
		return err
	}

	// keep == 0 means the scan concluded the entire file is one unterminated
	// fragment, so the truncate below would discard every record in it. That is
	// only ever right for a log that genuinely holds no complete line; if a
	// windowing or short-read fault reports it for a log full of events,
	// truncating destroys the whole body. The log is the only durable thing a
	// self has, so re-read and refuse rather than take the scan on faith.
	if keep == 0 {
		whole := make([]byte, size)
		if n, rerr := f.ReadAt(whole, 0); rerr != nil && int64(n) < size {
			return rerr
		}
		if bytes.IndexByte(whole, '\n') >= 0 {
			return fmt.Errorf("refusing to truncate events.jsonl to zero: the trailing-fragment scan found no newline in %d bytes, but the file holds complete lines", size)
		}
	}

	fmt.Fprintf(os.Stderr, "self: dropping %d incomplete byte(s) from the end of events.jsonl — they parse as no event, so they were never a record\n", size-keep)
	return f.Truncate(keep)
}

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
		// Highest seq in the window, not the last line: a hand-appended
		// out-of-order record would otherwise reuse a number.
		best, found := 0, false
		for i := len(lines) - 1; i >= 0; i-- {
			line := bytes.TrimSpace(lines[i])
			if len(line) == 0 {
				continue
			}
			if i == 0 && window < st.Size() {
				break
			}
			var e Event
			if json.Unmarshal(line, &e) != nil {
				continue
			}
			found = true
			if e.Seq > best {
				best = e.Seq
			}
		}
		if found {
			return best, nil
		}
		if window >= st.Size() {
			return 0, nil
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

func secretPath(home string) string { return filepath.Join(home, ".secret") }

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

func ensureSecret(home string) ([]byte, error) {
	if key := secret(home); key != nil {
		return key, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := writeFileAtomic(secretPath(home), []byte(hex.EncodeToString(key)), 0600, os.Link); err != nil {
		if os.IsExist(err) {
			if existing := secret(home); existing != nil {
				return existing, nil
			}
		}
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "self: new instance %s\n", home)
	return key, nil
}

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
