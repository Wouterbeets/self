package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// The blob store: SELF_HOME/files/<sha256>, content-addressed. The log carries
// only file METADATA — a small file.stored event {name, mime, size, sha256} —
// and the bytes live here, outside the log, so events.jsonl stays lean text
// that scanners, seeds, and rehydrate can chew through. The kernel appends
// file.stored itself whenever bytes enter the store (a multipart upload, an
// @path CLI arg, a seed's files/), because the store is kernel infrastructure
// and "if it is not an event, it did not happen". Same hash, same file: dedup
// is free, and a blob is immutable once written — a renamed or superseded file
// is a later event, never a rewrite.
//
// Blobs are user content, not kernel-derived state: rehydrate rebuilds
// capabilities/ and site/ from the log alone, but never blobs. An instance is
// events.jsonl + .secret + files/ — the first two rebuild everything else,
// the third must be backed up alongside them.

func blobsDir(home string) string { return filepath.Join(home, "files") }

func blobPath(home, hash string) string { return filepath.Join(blobsDir(home), hash) }

// validFileHash accepts exactly one spelling — 64 lowercase hex — so a hash is
// safe to join into a path and unambiguous to compare.
func validFileHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// storeBlob streams r into the store: hash while writing to a temp file, then
// rename into place under the content address. Returns the hash, the byte
// count, and the first bytes for mime sniffing. If the blob already exists the
// temp copy is discarded — same content, same address, nothing to do.
func storeBlob(home string, r io.Reader) (hash string, size int64, head []byte, err error) {
	dir := blobsDir(home)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", 0, nil, err
	}
	tmp, err := os.CreateTemp(dir, ".incoming-*")
	if err != nil {
		return "", 0, nil, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	size, err = io.Copy(io.MultiWriter(h, tmp, headWriter{&head}), r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", 0, nil, err
	}
	hash = hex.EncodeToString(h.Sum(nil))
	dst := blobPath(home, hash)
	if fileExists(dst) {
		return hash, size, head, nil
	}
	return hash, size, head, os.Rename(tmp.Name(), dst)
}

// headWriter keeps the first 512 bytes of a stream — what
// http.DetectContentType wants — without buffering the rest.
type headWriter struct{ buf *[]byte }

func (w headWriter) Write(p []byte) (int, error) {
	if room := 512 - len(*w.buf); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		*w.buf = append(*w.buf, p[:room]...)
	}
	return len(p), nil
}

// blobMime names a stored file's type: the extension when it is known (a .stl
// or .gcode sniffs as octet-stream, but its name says what it is), content
// sniffing otherwise.
func blobMime(name string, head []byte) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		return t
	}
	return http.DetectContentType(head)
}

// storeFile stores one named stream and returns the file.stored event that
// records it: {name, mime, size, sha256}. This is the one ingress every path
// shares — a browser upload, an @path CLI arg, a seed's files/ — so the log
// remembers every deposit the same way. The caller ingests the event (before
// running any command that receives the hash, so the command's stdin log
// already carries the file's metadata).
func storeFile(home, name string, r io.Reader) (string, Event, error) {
	hash, size, head, err := storeBlob(home, r)
	if err != nil {
		return "", Event{}, err
	}
	payload, _ := json.Marshal(map[string]any{
		"name": filepath.Base(name), "mime": blobMime(name, head), "size": size, "sha256": hash,
	})
	return hash, newEvent("file.stored", payload), nil
}

// storeFileArgs resolves a command's CLI args the way a multipart form
// resolves file inputs: an arg spelled @<path> deposits that file into the
// store and becomes its sha256, so `self run add-photo @sunset.jpg "golden
// hour"` and the browser form travel the same road. Everything else passes
// through untouched. The returned deposits must be ingested before the
// command runs.
func storeFileArgs(home string, args []string) (resolved []string, deposits []Event, err error) {
	resolved = make([]string, len(args))
	for i, a := range args {
		if !strings.HasPrefix(a, "@") || len(a) == 1 {
			resolved[i] = a
			continue
		}
		f, err := os.Open(a[1:])
		if err != nil {
			return nil, nil, fmt.Errorf("file arg %s: %w", a, err)
		}
		hash, e, serr := storeFile(home, filepath.Base(a[1:]), f)
		f.Close()
		if serr != nil {
			return nil, nil, serr
		}
		deposits = append(deposits, e)
		resolved[i] = hash
	}
	return resolved, deposits, nil
}

// depositCommandFile realizes a file.stored event a command emitted — the
// fourth ingress, the one that makes commands producers and not just
// recorders. A command that derives a file (an export, an invoice, a booklet)
// writes the bytes wherever it likes and emits file.stored
// {"name": …, "path": …}; the kernel — keeper of the store and the only
// hasher — copies the bytes in content-addressed, completes the payload
// {name, mime, size, sha256} from the bytes themselves, and drops the path
// (transport, never truth). A relative path resolves under SELF_HOME. A
// payload carrying a sha256 is verified against the bytes, so a command can
// neither mislabel a blob nor claim one that does not exist: with no path,
// the sha256 must already name a blob in the store, which is re-hashed rather
// than believed. The source file is never deleted — the kernel copies, the
// command owns its scratch.
func depositCommandFile(home string, payload json.RawMessage) (json.RawMessage, error) {
	var p struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		Sha256 string `json:"sha256"`
	}
	if json.Unmarshal(payload, &p) != nil || p.Name == "" {
		return nil, fmt.Errorf(`a file.stored event from a command needs {"name": …} and a "path" to the bytes (or the "sha256" of a blob already in the store)`)
	}
	src := p.Path
	switch {
	case src == "":
		if !validFileHash(p.Sha256) {
			return nil, fmt.Errorf("file.stored %q: give a path to the bytes, or the sha256 of a blob already in the store", p.Name)
		}
		src = blobPath(home, p.Sha256)
	case !filepath.IsAbs(src):
		src = filepath.Join(home, src)
	}
	f, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("file.stored %q: %w", p.Name, err)
	}
	defer f.Close()
	hash, size, head, err := storeBlob(home, f)
	if err != nil {
		return nil, err
	}
	if p.Sha256 != "" && p.Sha256 != hash {
		return nil, fmt.Errorf("file.stored %q: bytes hash to %s, not the declared %s", p.Name, hash, p.Sha256)
	}
	base := filepath.Base(p.Name)
	full, _ := json.Marshal(map[string]any{
		"name": base, "mime": blobMime(base, head), "size": size, "sha256": hash,
	})
	return full, nil
}

// danglingFiles scans file.stored events for blobs missing from the store —
// a log restored without its files/ dir. A warning, never a failure: the
// kernel rebuilds itself fine, only the referenced bytes are gone.
func danglingFiles(home string, events []Event) []string {
	var missing []string
	seen := map[string]bool{}
	for _, e := range events {
		if e.Name != "file.stored" {
			continue
		}
		var p struct {
			Name   string `json:"name"`
			Sha256 string `json:"sha256"`
		}
		if json.Unmarshal(e.Payload, &p) != nil || !validFileHash(p.Sha256) || seen[p.Sha256] {
			continue
		}
		seen[p.Sha256] = true
		if !fileExists(blobPath(home, p.Sha256)) {
			missing = append(missing, fmt.Sprintf("%s (%s)", p.Sha256[:12], p.Name))
		}
	}
	return missing
}
