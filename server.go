package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// cmdServe serves the instance: every page provably current against the log,
// every affordance a plain HTML form. The injected shell layers feel on top —
// a skin and a nav — but carries no state and grants no power: strip it and
// every page still works, because the forms do.
func cmdServe(home string) error {
	refreshSite(home)
	mux := serveMux(home)

	// Loopback by default: the write path (/run/<command>) has no auth, and
	// local-first means local. SELF_BIND is the whole bind address, host or
	// host:port (default 127.0.0.1:7777) — 0.0.0.0 opens it to the network
	// for anyone who knowingly wants that.
	addr := envOr("SELF_BIND", "127.0.0.1")
	if !strings.Contains(addr, ":") {
		addr += ":7777"
	}
	fmt.Fprintf(os.Stderr, "self: serving at http://%s (home %s)\n", addr, home)
	fmt.Fprintf(os.Stderr, "  /                      my identity — capabilities, paths, contract\n")
	fmt.Fprintf(os.Stderr, "  /<projection>          a projection, current against the log\n")
	fmt.Fprintf(os.Stderr, "  /run/<command>         run a capability (plain HTML forms)\n")
	fmt.Fprintf(os.Stderr, "  /events                the raw event log\n")
	return http.ListenAndServe(addr, mux)
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
			renderKernelHTML(home)
			renderBriefFile(home)
			page, err := os.ReadFile(filepath.Join(home, "site", "kernel.html"))
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
		// text/plain. A mind (or human, or external agent) can reach any
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
		case strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
			body, _ := io.ReadAll(r.Body)
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
			body, _ := io.ReadAll(r.Body)
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
