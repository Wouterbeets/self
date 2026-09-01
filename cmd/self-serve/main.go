// self-serve — the stateless HTTP face of a self instance.
//
// It knows nothing about any view and nothing about any command. GET
// /view/<name>[/<arg>…] is `self view <name> [args…]` with the bytes that
// returned; /run/<cmd>[/<arg>…] is `self run <cmd> [args…]`. `/` is `self
// brief`. Every request is a fresh replay, so the server holds no session, no
// cache, no sidecar — it is a pipe over the kernel, and the log stays the
// only state.
//
//	self-serve            listen on 127.0.0.1:8377 (PORT overrides)
//	SELF_BIN              kernel to exec (default: a `self` next to this
//	                      binary, then PATH)
//
// Pages keep themselves current: each rendering is stamped with an etag over
// the kernel's bytes, the page re-asks every two seconds, and a 304 is the
// whole conversation when the log did not move. The stamp is computed per
// request, so the server still holds nothing.
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
	mux.HandleFunc("/", index)
	mux.HandleFunc("/view/", view)
	mux.HandleFunc("/run/", run)
	return mux
}

func index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
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
		// Zero-arg form is the discoverable index, same as `self view`.
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

	name, args := resolveName(segs, knownNames("views"))
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
		// Silence is a view's empty state; say so once, plainly.
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
	name, args := resolveName(segs, knownNames("commands"))
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
	// The etag is a stamp on the kernel's bytes, so it names the log state
	// the page shows — not the wire bytes, which gain a script below.
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

// withRefresh appends the two-second checker. The etag travels inside the
// script, so the page knows what it last saw without any other state.
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

// resolveName picks the longest known capability name that is a prefix of
// segs, so /run/timer/set/x is `self run timer/set x` rather than
// `self run timer set x`. Unknown names fall back to the first segment —
// the kernel is the one that says they do not exist.
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
		ok := true
		for i, s := range nsegs {
			if segs[i] != s {
				ok = false
				break
			}
		}
		if ok && len(nsegs) > bestN {
			bestN, best = len(nsegs), n
		}
	}
	if bestN == 0 {
		return segs[0], segs[1:]
	}
	return best, segs[bestN:]
}

var briefItem = regexp.MustCompile(`(?m)^- \*\*([^*]+)\*\*`)
var viewIndexItem = regexp.MustCompile(`(?m)^- ([A-Za-z0-9_][A-Za-z0-9_./-]*) —`)
var markdownLink = regexp.MustCompile(`\[([^\]\r\n]+)\]\((https?://[^\s<>\)]+)\)`)

func knownNames(kind string) []string {
	out, _, code, err := selfOutput("brief")
	if err != nil || code != 0 {
		return nil
	}
	return namesInSection(string(out), "## "+kind)
}

func namesInSection(brief, heading string) []string {
	start := strings.Index(brief, heading)
	if start < 0 {
		return nil
	}
	section := brief[start:]
	if i := strings.Index(section[2:], "\n## "); i >= 0 {
		section = section[:i+2]
	}
	var names []string
	for _, m := range briefItem.FindAllStringSubmatch(section, -1) {
		names = append(names, m[1])
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

// selfOutput execs the kernel and returns stdout, stderr, and the exit code.
// A command that appends nothing still exits 0; one that is refused exits
// nonzero and says why on stderr — both surfaces come back to the caller.
func selfOutput(argv ...string) (out []byte, errOut string, code int, err error) {
	cmd := exec.Command(lookSelf(), argv...)
	cmd.Env = selfEnv()
	b, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return b, strings.TrimSpace(string(ee.Stderr)), ee.ExitCode(), nil
		}
		return nil, "", 1, err
	}
	return b, "", 0, nil
}

// lookSelf is the kernel this door talks to: SELF_BIN, then a `self` sitting
// next to this binary, then PATH. Same rule browse uses to find self-serve.
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

// selfEnv is the inherited environment with the claim rewritten: whatever
// lands in the log through this door is attributable to the browser.
func selfEnv() []string {
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "SELF_CALLER=") {
			env = append(env, kv)
		}
	}
	return append(env, "SELF_CALLER=browser")
}

func page(title, body string) string {
	return "<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<title>" + html.EscapeString(title) + " — self</title>" +
		"<style>:root{color-scheme:light dark}body{font:16px/1.55 system-ui,sans-serif;margin:0 auto;max-width:46rem;padding:1.5rem 1rem 3rem}code{font-family:ui-monospace,monospace;font-size:.85rem;background:#8882;padding:.1rem .35rem;border-radius:.3rem}pre{white-space:pre-wrap;font-family:ui-monospace,monospace;font-size:.85rem;background:#8881;padding:.75rem 1rem;border-radius:.5rem}a{color:inherit}</style>" +
		"</head><body><h1>" + html.EscapeString(title) + "</h1>" + body + "</body></html>"
}
