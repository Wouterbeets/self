// self-serve — HTTP face of a self instance. GET / is brief, /view/… is
// `self view`, /run/… is `self run`. PORT and SELF_BIN select the listen
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

	// ?raw=1 hands over the kernel's own bytes under their real media type:
	// the address to give an image viewer, another tab, or curl.
	if r.URL.Query().Get("raw") != "" {
		w.Header().Set("Content-Type", mediaType(out))
		w.Header().Set("ETag", etagOf(out))
		w.Write(out)
		return
	}

	if notModified(w, r, out) {
		return
	}

	// One question decides both the strip and the listing's own numbers:
	// are the numbers this view printed addressable as pages of it?
	addr, _ := splitFormat(args)
	numbered := lastNumber(addr) < 0 && numbersAddressable(name, addr, numbersIn(out))
	nav := viewNav(name, args, out, numbered)
	var body []byte
	switch {
	case len(strings.TrimSpace(string(out))) == 0:
		body = []byte(page(name, nav+"<p>Nothing. The view is silent, which is its empty state.</p>"))
	case isSVG(out):
		// A drawing is inlined, so it renders in the page that navigates it.
		body = []byte(page(name, nav+"<figure>"+string(out)+"</figure>"))
	case isWAV(out):
		// Sound cannot be inlined; the raw door is its source.
		body = []byte(page(name, nav+`<audio controls src="`+rawPath(r)+`"></audio>`))
	case isHTML(out):
		// A standalone document is its own page; do not dress it.
		body = out
	default:
		body = []byte(page(name, nav+linkifyTextView(string(out), name, args, numbered)))
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

// notModified answers the refresh poll from the kernel's bytes alone, before
// a caller spends anything on rendering or navigation.
func notModified(w http.ResponseWriter, r *http.Request, kernel []byte) bool {
	if r.Header.Get("If-None-Match") != etagOf(kernel) {
		return false
	}
	w.WriteHeader(http.StatusNotModified)
	return true
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

// isSVG is checked before isHTML: a drawing may carry an xml prolog, and it
// is a picture to inline rather than a document to hand over whole.
func isSVG(b []byte) bool {
	s := strings.ToLower(string(bytes.TrimLeft(b, " \t\r\n")))
	if strings.HasPrefix(s, "<svg") {
		return true
	}
	if strings.HasPrefix(s, "<?xml") {
		if i := strings.Index(s, "<svg"); i >= 0 && i < 512 {
			return true
		}
	}
	return false
}

func isWAV(b []byte) bool {
	return len(b) > 12 && bytes.HasPrefix(b, []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WAVE"))
}

// mediaType names what a view printed, for the raw door only. A view that
// prints nothing recognizable is text, which is the honest default.
func mediaType(b []byte) string {
	switch {
	case isSVG(b):
		return "image/svg+xml"
	case isWAV(b):
		return "audio/wav"
	case isHTML(b):
		return "text/html; charset=utf-8"
	}
	return "text/plain; charset=utf-8"
}

func rawPath(r *http.Request) string {
	return r.URL.EscapedPath() + "?raw=1"
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

// sectionOf is one "## heading" block of the brief, up to the next heading.
func sectionOf(brief, heading string) string {
	start := strings.Index(brief, heading)
	if start < 0 {
		return ""
	}
	section := brief[start:]
	if i := strings.Index(section[2:], "\n## "); i >= 0 {
		section = section[:i+2]
	}
	return section
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

// linkifyTextView escapes a view's text and makes the addresses inside it
// clickable: any markdown link it printed, and the "#NUMBER" a listing uses
// to name one of its own pages.
func linkifyTextView(raw, name string, args []string, numbered bool) string {
	esc := html.EscapeString(raw)
	esc = markdownLink.ReplaceAllString(esc, `<a href="$2">$1</a>`)
	body, _ := splitFormat(args)
	if !numbered {
		// Either this is one numbered page already — where a "#NUMBER" is a
		// reference, not a way elsewhere — or the numbers are not its own.
		return "<pre>" + esc + "</pre>"
	}
	esc = indexNumber.ReplaceAllStringFunc(esc, func(m string) string {
		n := indexNumber.FindStringSubmatch(m)[1]
		return `<a href="` + pathFor(name, with(body, n), "") + `">#` + n + `</a>`
	})
	return "<pre>" + esc + "</pre>"
}

var indexNumber = regexp.MustCompile(`(?m)^#(\d+)\b`)
var numeric = regexp.MustCompile(`^\d+$`)
var briefEntry = regexp.MustCompile(`(?m)^- \*\*([^*]+)\*\* — (.*)$`)

// renderFormats are the trailing words a view uses for a rendering of the
// same page. Only ones the view's own description advertises are offered,
// and only after the kernel has accepted them.
var renderFormats = []string{"svg", "html", "wav"}

func isFormat(s string) bool {
	for _, f := range renderFormats {
		if s == f {
			return true
		}
	}
	return false
}

// splitFormat separates the address of a page from a trailing rendering, so
// a format can be swapped without losing where you are.
func splitFormat(args []string) (body []string, format string) {
	if n := len(args); n > 0 && isFormat(args[n-1]) {
		return args[:n-1], args[n-1]
	}
	return args, ""
}

func with(args []string, extra ...string) []string {
	return append(append([]string{}, args...), extra...)
}

func lastNumber(args []string) int {
	for i := len(args) - 1; i >= 0; i-- {
		if numeric.MatchString(args[i]) {
			return i
		}
	}
	return -1
}

// numbersIn scrapes the "#NUMBER" lines a listing prints, in its own order:
// the pages that listing says are addressable.
func numbersIn(b []byte) []string {
	var out []string
	for _, m := range indexNumber.FindAllStringSubmatch(string(b), -1) {
		out = append(out, m[1])
	}
	return out
}

func pathFor(name string, body []string, format string) string {
	p := "/view/" + pathEscapeName(name)
	for _, a := range body {
		p += "/" + url.PathEscape(a)
	}
	if format != "" {
		p += "/" + url.PathEscape(format)
	}
	return p
}

// viewNav is the strip above a rendered view: where you are, the pages either
// side of a numbered one, and the other renderings of this same page. Every
// link is a GET on a read door, so moving through them appends nothing.
func viewNav(name string, args []string, out []byte, numbered bool) string {
	var links []string
	add := func(href, label string) {
		links = append(links, `<a href="`+href+`">`+html.EscapeString(label)+`</a>`)
	}
	body, format := splitFormat(args)

	add("/", "self")
	add("/view/", "views")
	if len(args) > 0 {
		add(pathFor(name, nil, ""), name)
	}

	if i := lastNumber(body); i >= 0 {
		// The same view without the number is this page's index, and the
		// numbers it prints are the pages around this one.
		index := with(body[:i], body[i+1:]...)
		if len(index) > 0 || format != "" {
			// With no argument left the view's own name is already the index.
			add(pathFor(name, index, format), "index")
		}
		if listing, _, code, err := selfOutput(with([]string{"view", name}, index...)...); err == nil && code == 0 {
			nums := numbersIn(listing)
			at := -1
			for j, n := range nums {
				if n == body[i] {
					at = j
				}
			}
			sibling := func(j int, label string) {
				alt := with(body)
				alt[i] = nums[j]
				add(pathFor(name, alt, format), label)
			}
			if at > 0 {
				sibling(at-1, "◀ #"+nums[at-1])
			}
			if at >= 0 && at < len(nums)-1 {
				sibling(at+1, "#"+nums[at+1]+" ▶")
			}
		}
	} else if nums := numbersIn(out); numbered && len(nums) > 0 {
		last := nums[len(nums)-1]
		add(pathFor(name, with(body, last), format), "latest #"+last)
	}

	if format != "" {
		add(pathFor(name, body, ""), "text")
	}
	desc := strings.ToLower(viewDesc(name))
	for _, f := range renderFormats {
		if f == format || !strings.Contains(desc, f) {
			continue
		}
		// Offer a rendering only where the kernel actually accepts it: this
		// view may take that word somewhere else in its arguments.
		if b, _, code, err := selfOutput(with([]string{"view", name}, with(body, f)...)...); err != nil || code != 0 || len(strings.TrimSpace(string(b))) == 0 {
			continue
		}
		add(pathFor(name, body, f), f)
	}
	if isSVG(out) || isWAV(out) || isHTML(out) {
		add(pathFor(name, body, format)+"?raw=1", "raw")
	}
	return "<nav>" + strings.Join(links, " · ") + "</nav>"
}

// numbersAddressable asks the kernel whether the numbers a listing printed
// are pages of this view. A view can print numbers that live under another
// word — a bell lists its rings and its scores — and a link the kernel would
// refuse is worse than no link, so the last one is tried before any is made.
func numbersAddressable(name string, body, nums []string) bool {
	if len(nums) == 0 {
		return false
	}
	_, _, code, err := selfOutput(with([]string{"view", name}, with(body, nums[len(nums)-1])...)...)
	return err == nil && code == 0
}

// viewDesc is the one line the brief gives a view, read only to see which
// renderings it says it has.
func viewDesc(name string) string {
	out, _, code, err := selfOutput("brief")
	if err != nil || code != 0 {
		return ""
	}
	for _, m := range briefEntry.FindAllStringSubmatch(sectionOf(string(out), "## views"), -1) {
		if m[1] == name {
			return m[2]
		}
	}
	return ""
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
		"<style>:root{color-scheme:light dark}body{font:16px/1.55 system-ui,sans-serif;margin:0 auto;max-width:46rem;padding:1.5rem 1rem 3rem}code{font-family:ui-monospace,monospace;font-size:.85rem;background:#8882;padding:.1rem .35rem;border-radius:.3rem}pre{white-space:pre-wrap;font-family:ui-monospace,monospace;font-size:.85rem;background:#8881;padding:.75rem 1rem;border-radius:.5rem}a{color:inherit}nav{font-size:.8rem;margin:0 0 1.25rem;opacity:.85}figure{margin:0}svg{max-width:100%;height:auto}audio{width:100%}</style>" +
		"</head><body><h1>" + html.EscapeString(title) + "</h1>" + body + "</body></html>"
}
