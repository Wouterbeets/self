// self-serve — HTTP face of a self instance. GET / is brief, /view/… is
// `self view`, POST /run/… is `self run`. PORT and SELF_BIN select the listen
// address and kernel.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const defaultPort = "8377"

var nameOK = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	fmt.Fprintf(os.Stderr, "self-serve: http://127.0.0.1:%s\n", port)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, handler()))
}

func handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", index)
	mux.HandleFunc("GET /view/", view)
	mux.HandleFunc("POST /run/", run)
	return mux
}

func index(w http.ResponseWriter, r *http.Request) {
	out, errOut, code, err := selfOutput("brief")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if code != 0 {
		http.Error(w, strings.TrimSpace(errOut+"\n"+string(out)), http.StatusBadGateway)
		return
	}
	inner := linkifyBrief(string(out))
	reply(w, r, out, []byte(page("self", inner)))
}

func view(w http.ResponseWriter, r *http.Request) {
	segs, err := pathSegs(r.URL.EscapedPath(), "/view/")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(segs) == 0 {
		out, errOut, code, err := selfOutput("view")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if code != 0 {
			http.Error(w, strings.TrimSpace(errOut+"\n"+string(out)), http.StatusBadGateway)
			return
		}
		inner := linkifyViewIndex(string(out))
		reply(w, r, out, []byte(page("views", inner)))
		return
	}

	name, args := resolveName(segs, knownNames("view"))
	if !nameSegmentsOK(name) {
		http.Error(w, "unknown view: "+name, http.StatusBadRequest)
		return
	}

	argv := append([]string{"view", name}, args...)
	out, errOut, code, err := selfOutput(argv...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if code != 0 {
		msg := strings.TrimSpace(errOut + "\n" + string(out))
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	var body []byte
	switch {
	case len(strings.TrimSpace(string(out))) == 0:
		body = []byte(page(name, "<p>Nothing. The view is silent, which is its empty state.</p>"))
	case isHTML(out):
		body = out
	default:
		body = []byte(page(name, linkifyTextView(string(out))))
	}
	reply(w, r, out, body)
}

func run(w http.ResponseWriter, r *http.Request) {
	segs, err := pathSegs(r.URL.EscapedPath(), "/run/")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(segs) == 0 {
		http.Error(w, "usage: /run/<command> [<args…>]", http.StatusBadRequest)
		return
	}
	name, args := resolveName(segs, knownNames("run"))
	if !nameSegmentsOK(name) {
		http.Error(w, "bad command name", http.StatusBadRequest)
		return
	}
	argv := append([]string{"run", name}, args...)
	out, errOut, code, err := selfOutput(argv...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body := strings.TrimRight(string(out), "\n")
	if errOut != "" {
		body = body + "\n" + strings.TrimRight(errOut, "\n")
	}
	if code != 0 {
		body = "self: exit " + strconv.Itoa(code) + "\n" + body
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if code != 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	fmt.Fprintln(w, body)
}

func reply(w http.ResponseWriter, r *http.Request, kernel, body []byte) {
	etag := etagOf(kernel)
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body = withRefresh(body, etag)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("ETag", etag)
	w.Write(body)
}

func etagOf(b []byte) string {
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func withRefresh(body []byte, etag string) []byte {
	script := `<script>setInterval(function(){fetch(location.pathname,{headers:{"If-None-Match":` +
		strconv.Quote(etag) + `}}).then(function(r){if(r.status===200)location.reload()})},2000)</script>`
	if i := bytes.LastIndex(body, []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(body)+len(script))
		out = append(out, body[:i]...)
		out = append(out, script...)
		out = append(out, body[i:]...)
		return out
	}
	return append(body, []byte(script)...)
}

func isHTML(b []byte) bool {
	s := strings.ToLower(string(bytes.TrimLeft(b, " \t\r\n")))
	return strings.HasPrefix(s, "<!doctype") || strings.HasPrefix(s, "<html") || strings.HasPrefix(s, "<?xml")
}

func pathSegs(escaped, prefix string) ([]string, error) {
	var segs []string
	for _, seg := range strings.Split(strings.TrimPrefix(escaped, prefix), "/") {
		if seg == "" {
			continue
		}
		arg, err := url.PathUnescape(seg)
		if err != nil || arg == "" {
			return nil, fmt.Errorf("bad argument in path")
		}
		segs = append(segs, arg)
	}
	return segs, nil
}

func nameSegmentsOK(name string) bool {
	if name == "" {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if !nameOK.MatchString(seg) {
			return false
		}
	}
	return true
}

// Longest known name prefix of segs: /run/timer/set/x is timer/set, not timer.
func resolveName(segs, names []string) (name string, args []string) {
	if len(segs) == 0 {
		return "", nil
	}
	bestN := 0
	best := ""
	for _, n := range names {
		nsegs := strings.Split(n, "/")
		if len(nsegs) == 0 || len(nsegs) > len(segs) {
			continue
		}
		if len(nsegs) > bestN && slices.Equal(segs[:len(nsegs)], nsegs) {
			bestN, best = len(nsegs), n
		}
	}
	if bestN == 0 {
		return segs[0], segs[1:]
	}
	return best, segs[bestN:]
}

var briefItem = regexp.MustCompile(`(?m)^- (?:\*\*)?([A-Za-z0-9_][A-Za-z0-9_./-]*)(?:\*\*)?`)
var viewIndexItem = regexp.MustCompile(`(?m)^- ([A-Za-z0-9_][A-Za-z0-9_./-]*) —`)
var markdownLink = regexp.MustCompile(`\[([^\]\r\n]+)\]\((https?://[^\s<>\)]+)\)`)

func knownNames(verb string) []string {
	out, _, code, err := selfOutput("__complete", verb, "")
	if err != nil || code != 0 {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name, _, _ := strings.Cut(line, "\t")
		if nameSegmentsOK(name) {
			names = append(names, name)
		}
	}
	return names
}

func linkifyBrief(raw string) string {
	esc := html.EscapeString(raw)
	start := strings.Index(esc, "## views")
	if start < 0 {
		return "<pre>" + boldBriefItems(esc) + "</pre>"
	}
	head, rest := esc[:start], esc[start:]
	views, after := rest, ""
	if i := strings.Index(rest[2:], "\n## "); i >= 0 {
		i += 2
		views, after = rest[:i], rest[i:]
	}
	views = briefItem.ReplaceAllStringFunc(views, func(m string) string {
		sub := briefItem.FindStringSubmatch(m)
		name := sub[1]
		return `- <a href="/view/` + pathEscapeName(name) + `"><strong>` + name + `</strong></a>`
	})
	// Commands stay names, not GET links: a click would append.
	return "<pre>" + boldBriefItems(head) + views + boldBriefItems(after) + "</pre>"
}

func boldBriefItems(s string) string {
	return briefItem.ReplaceAllString(s, `- <strong>$1</strong>`)
}

func linkifyViewIndex(raw string) string {
	esc := html.EscapeString(raw)
	esc = viewIndexItem.ReplaceAllStringFunc(esc, func(m string) string {
		sub := viewIndexItem.FindStringSubmatch(m)
		name := sub[1]
		return `- <a href="/view/` + pathEscapeName(name) + `">` + name + `</a> —`
	})
	return "<pre>" + esc + "</pre>"
}

func linkifyTextView(raw string) string {
	esc := html.EscapeString(raw)
	esc = markdownLink.ReplaceAllString(esc, `<a href="$2">$1</a>`)
	return "<pre>" + esc + "</pre>"
}

func pathEscapeName(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func selfOutput(argv ...string) (out []byte, errOut string, code int, err error) {
	cmd := exec.Command(lookSelf(), argv...)
	cmd.Env = append(os.Environ(), "SELF_CALLER=browser")
	b, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return b, strings.TrimSpace(string(ee.Stderr)), ee.ExitCode(), nil
		}
		return nil, "", 1, err
	}
	return b, "", 0, nil
}

func lookSelf() string {
	if b := os.Getenv("SELF_BIN"); b != "" {
		return b
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "self")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if p, err := exec.LookPath("self"); err == nil {
		return p
	}
	return "self"
}

func page(title, body string) string {
	return "<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<title>" + html.EscapeString(title) + " — self</title>" +
		"<style>:root{color-scheme:light dark}body{font:16px/1.55 system-ui,sans-serif;margin:0 auto;max-width:46rem;padding:1.5rem 1rem 3rem}code{font-family:ui-monospace,monospace;font-size:.85rem;background:#8882;padding:.1rem .35rem;border-radius:.3rem}pre{white-space:pre-wrap;font-family:ui-monospace,monospace;font-size:.85rem;background:#8881;padding:.75rem 1rem;border-radius:.5rem}a{color:inherit}</style>" +
		"</head><body><h1>" + html.EscapeString(title) + "</h1>" + body + "</body></html>"
}
