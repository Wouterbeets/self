// self-serve — the stateless HTTP face of a self instance.
//
// It knows nothing about any view and nothing about any command. GET
// /view/<name> is `self view <name>` with the bytes that returned; /run/<cmd>
// [<args…>] is `self run <cmd> <args…>`. Every request is a fresh replay, so
// the server holds no session, no cache, no sidecar — it is a pipe over the
// kernel, and the log stays the only state.
//
//	self-serve            listen on 127.0.0.1:8377 (PORT overrides)
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
	mux := http.NewServeMux()
	mux.HandleFunc("/", index)
	mux.HandleFunc("/view/", view)
	mux.HandleFunc("/run/", run)
	fmt.Fprintf(os.Stderr, "self-serve: http://127.0.0.1:%s\n", port)
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, mux))
}

func index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, page("self", `<ul>
<li><code>GET /view/&lt;name&gt;</code> — a view replayed from the log</li>
<li><code>/run/&lt;command&gt;/&lt;arg&gt;…</code> — a command, appended to the log</li>
</ul><p>Whatever the instance declares, this page shows it. The server holds nothing of its own.</p>`))
}

func view(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/view/"), "/")
	if !nameOK.MatchString(name) {
		http.Error(w, "unknown view: "+name, http.StatusBadRequest)
		return
	}
	out, _, _, err := selfOutput("view", name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		body = []byte(page(name, "<pre>"+html.EscapeString(string(out))+"</pre>"))
	}

	// The etag is a stamp on the kernel's bytes, so it names the log state
	// the page shows — not the wire bytes, which gain a script below.
	etag := etagOf(body)
	if match := r.Header.Get("If-None-Match"); match == etag || match == "*" {
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

func run(w http.ResponseWriter, r *http.Request) {
	argv := []string{}
	for _, seg := range strings.Split(strings.TrimPrefix(r.URL.EscapedPath(), "/run/"), "/") {
		if seg == "" {
			continue
		}
		arg, err := url.PathUnescape(seg)
		if err != nil || arg == "" {
			http.Error(w, "bad argument in path", http.StatusBadRequest)
			return
		}
		argv = append(argv, arg)
	}
	if len(argv) == 0 {
		http.Error(w, "usage: /run/<command> [<args…>]", http.StatusBadRequest)
		return
	}
	out, errOut, code, err := selfOutput(append([]string{"run"}, argv...)...)
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

func isHTML(b []byte) bool {
	s := strings.ToLower(string(bytes.TrimLeft(b, " \t\r\n")))
	return strings.HasPrefix(s, "<!doctype") || strings.HasPrefix(s, "<html") || strings.HasPrefix(s, "<?xml")
}

// selfOutput execs the kernel and returns stdout, stderr, and the exit code.
// A command that appends nothing still exits 0; one that is refused exits
// nonzero and says why on stderr — both surfaces come back to the caller.
func selfOutput(argv ...string) (out []byte, errOut string, code int, err error) {
	cmd := exec.Command("self", argv...)
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
		"<style>:root{color-scheme:light dark}body{font:16px/1.55 system-ui,sans-serif;margin:0 auto;max-width:46rem;padding:1.5rem 1rem 3rem}code{font-family:ui-monospace,monospace;font-size:.85rem;background:#8882;padding:.1rem .35rem;border-radius:.3rem}pre{white-space:pre-wrap;font-family:ui-monospace,monospace;font-size:.85rem;background:#8881;padding:.75rem 1rem;border-radius:.5rem}</style>" +
		"</head><body><h1>" + html.EscapeString(title) + "</h1>" + body + "</body></html>"
}
