package main

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// The orchestration source travels in the binary so /orchestration_core
// answers from any install, not just a dev checkout.
//
//go:embed orchestration_core.go
var orchestrationCoreSource []byte

// cmdServe serves the instance: every page provably current against the log,
// every affordance a plain HTML form. The injected shell layers feel on top —
// pending turns, live re-replay, theme — but carries no state and grants no
// power: strip it and every page still works, because the forms do.
func cmdServe(home string) error {
	refreshSite(home)
	mux := serveMux(home)
	go serveTimers(home)

	// Loopback by default: the write path (/run/<command>) has no auth, and
	// local-first means local. SELF_BIND is the whole bind address, host or
	// host:port (default 127.0.0.1:7777) — 0.0.0.0 opens it to the network
	// for anyone who knowingly wants that.
	addr := os.Getenv("SELF_BIND")
	if addr == "" {
		addr = "127.0.0.1"
	}
	if !strings.Contains(addr, ":") {
		addr += ":7777"
	}
	fmt.Fprintf(os.Stderr, "self: serving at http://%s (home %s)\n", addr, home)
	fmt.Fprintf(os.Stderr, "  /                      my identity — capabilities, paths, contract\n")
	fmt.Fprintf(os.Stderr, "  /<projection>          a projection, current against the log\n")
	fmt.Fprintf(os.Stderr, "  /run/<command>         run a capability (plain HTML forms; file inputs upload)\n")
	fmt.Fprintf(os.Stderr, "  /files/<sha256>        a stored file, content-addressed\n")
	fmt.Fprintf(os.Stderr, "  /events                the raw event log\n")
	fmt.Fprintf(os.Stderr, "  /orchestration_core    how self orchestrates itself (for agents)\n")
	return http.ListenAndServe(addr, mux)
}

// readBounded reads a request body (or form field) whole, refusing anything
// past the log's own line limit and failing loudly on a broken read — a
// client that disconnected mid-body must never run a command with a silently
// truncated argument. File uploads are exempt: they stream to the blob store,
// never through memory.
func readBounded(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxEventLine+1))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if len(data) > maxEventLine {
		return nil, fmt.Errorf("request body exceeds %d bytes", maxEventLine)
	}
	return data, nil
}

// serveMux is the instance's whole HTTP surface, separable from the listener
// so tests exercise the real routes.
func serveMux(home string) *http.ServeMux {
	mux := http.NewServeMux()

	// GET /            → kernel.html (my identity), or a welcome projection if grown
	// GET /<name>      → that projection, re-run live
	// anything else    → static site/ files
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.Trim(r.URL.Path, "/"), ".html")
		if name != "" && !validCapabilityName(name) {
			http.Error(w, "not found", 404)
			return
		}
		if name == "" {
			if p, _ := scriptPath(home, "projector", "welcome"); fileExists(p) {
				name = "welcome"
			} else {
				name = "kernel"
			}
		}
		if name == "kernel" {
			page, err := kernelPage(home)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writePage(w, r, home, name, page)
			return
		}
		if p, _ := scriptPath(home, "projector", name); fileExists(p) {
			// The materialized page serves when it is provably current (its
			// mtime postdates the log's last append); otherwise re-replay live
			// and write the result through, so the disk heals to fresh.
			if page := freshSitePage(home, name); page != nil {
				writePage(w, r, home, name, page)
				return
			}
			page, err := runProjection(home, name)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			writeSitePage(home, name, page)
			writePage(w, r, home, name, page)
			return
		}
		// Any on-disk site artifact by bare name: brief, kernel, etc.
		// .html goes through the shell; .md and .txt are served verbatim as
		// text/plain. A brain (or human, or external agent) can reach any
		// kernel-resident surface by name.
		if p, ext := siteFile(home, name); p != "" {
			if name == "brief" {
				renderBriefFile(home) // always fresh when served
				p, ext = siteFile(home, "brief")
			}
			serveSiteFile(w, r, home, name, p, ext)
			return
		}
		http.FileServer(http.Dir(filepath.Join(home, "site"))).ServeHTTP(w, r)
	})

	// GET /events → the raw log
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, logPath(home))
	})

	// GET /files/<sha256>            → a stored blob, content-addressed
	// GET /files/<sha256>/<name>     → the same blob wearing a human name —
	// the name is presentation (download filename, mime hint, readable URL),
	// never resolution; the hash alone decides which bytes serve. Content
	// addressing makes the response immutable, so it caches forever.
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		hash, display, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/files/"), "/")
		if !validFileHash(hash) {
			http.Error(w, "not found — /files/<sha256>[/<name>]", 404)
			return
		}
		f, err := os.Open(blobPath(home, hash))
		if err != nil {
			http.Error(w, "no such file in the store", 404)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		head := make([]byte, 512)
		n, _ := io.ReadFull(f, head)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		ct := blobMime(display, head[:n])
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		// A blob is user content on the instance's own origin — the origin
		// that also serves the unauthenticated write path. nosniff pins the
		// declared type, and any type a browser would execute (HTML, SVG,
		// anything XML-flavored) renders sandboxed: visible, never scripting.
		// Images, PDFs, and downloads are untouched.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.Contains(ct, "html") || strings.Contains(ct, "xml") {
			w.Header().Set("Content-Security-Policy", "sandbox")
		}
		if display != "" {
			safe := strings.Map(func(c rune) rune {
				if c < 32 || c == '"' || c == '\\' {
					return '_'
				}
				return c
			}, display)
			w.Header().Set("Content-Disposition", `inline; filename="`+safe+`"`)
		}
		http.ServeContent(w, r, "", st.ModTime(), f)
	})

	// GET /orchestration_core → the orchestration_core.go source, embedded at
	// build time, so the brain (or any agent) can read how the system
	// orchestrates itself.
	mux.HandleFunc("/orchestration_core", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(orchestrationCoreSource)
	})

	// POST /run/<command> → run a capability from the browser. A form's field
	// values become positional args in document order (names are for humans;
	// order is the contract); then Post/Redirect/Get back to the page.
	mux.HandleFunc("/run/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		command := strings.TrimPrefix(r.URL.Path, "/run/")
		if !validCapabilityName(command) {
			http.Error(w, "not found", 404)
			return
		}
		if p, _ := scriptPath(home, "command", command); !fileExists(p) {
			http.Error(w, fmt.Sprintf("command %q not found", command), 404)
			return
		}
		var args []string
		ct := r.Header.Get("Content-Type")
		switch {
		case strings.HasPrefix(ct, "multipart/form-data"):
			// A form with a file input. Parts stay positional args in document
			// order; each file part is stored content-addressed and its arg is
			// the sha256 — the command resolves SELF_HOME/files/<sha256> itself.
			// The file.stored events ingest BEFORE the command runs, so its
			// stdin log already names, sizes, and types what it was handed.
			mr, err := r.MultipartReader()
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			var deposits []Event
			for {
				part, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				if fn := part.FileName(); fn != "" {
					hash, e, err := storeFile(home, fn, part)
					if err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
					deposits = append(deposits, e)
					args = append(args, hash)
				} else {
					data, err := readBounded(part)
					if err != nil {
						http.Error(w, err.Error(), 400)
						return
					}
					args = append(args, string(data))
				}
			}
			if len(deposits) > 0 {
				if err := ingest(home, deposits); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
			}
		case strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
			body, err := readBounded(r.Body)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			for _, pair := range strings.Split(string(body), "&") {
				if pair == "" {
					continue
				}
				_, v, _ := strings.Cut(pair, "=")
				v = strings.ReplaceAll(v, "+", " ")
				if dec, err := url.QueryUnescape(v); err == nil {
					v = dec
				}
				args = append(args, v)
			}
		default:
			body, err := readBounded(r.Body)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if msg := strings.TrimSpace(string(body)); msg != "" {
				args = []string{msg}
			}
		}
		if _, err := runCommand(home, command, args); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if ref := r.Header.Get("Referer"); ref != "" {
			http.Redirect(w, r, ref, http.StatusSeeOther)
			return
		}
		fmt.Fprint(w, "ok")
	})

	return mux
}
