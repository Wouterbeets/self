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

// cmdServe serves the instance: every page re-rendered against current events,
// every affordance a plain HTML form. The injected shell layers feel on top —
// pending turns, live re-replay, theme — but carries no state and grants no
// power: strip it and every page still works, because the forms do.
func cmdServe(home string) error {
	refreshSite(home)

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
			page, err := runProjection(home, name)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
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

	// GET /orchestration_core → the orchestration_core.go source
	// Allows the brain (or any agent) to understand how the system orchestrates itself
	mux.HandleFunc("/orchestration_core", func(w http.ResponseWriter, r *http.Request) {
		source, err := os.ReadFile(filepath.Join(os.Getenv("SELF_GOPATH"), "orchestration_core.go"))
		if err != nil {
			// Fallback: try current directory or common locations
			for _, p := range []string{
				"orchestration_core.go",
				filepath.Join(os.Getenv("SELF_HOME"), "orchestration_core.go"),
			} {
				if s, err := os.ReadFile(p); err == nil {
					source = s
					break
				}
			}
		}
		if source == nil {
			http.Error(w, "orchestration_core.go not found", 404)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(source)
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
		body, _ := io.ReadAll(r.Body)
		var args []string
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
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
		} else if msg := strings.TrimSpace(string(body)); msg != "" {
			args = []string{msg}
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
	fmt.Fprintf(os.Stderr, "  /<projection>          a projection, re-rendered live\n")
	fmt.Fprintf(os.Stderr, "  /run/<command>         run a capability (plain HTML forms)\n")
	fmt.Fprintf(os.Stderr, "  /events                the raw event log\n")
	fmt.Fprintf(os.Stderr, "  /orchestration_core    how self orchestrates itself (for agents)\n")
	return http.ListenAndServe(addr, mux)
}
