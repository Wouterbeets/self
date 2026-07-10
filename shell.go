package main

import (
	"bytes"
	"embed"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ─────────────────────────────── kernel.html ────────────────────────────────

// renderKernelHTML writes the kernel's self-description — capabilities, paths,
// the pipe contract — to site/kernel.html: the page a human lands on and the
// first context a brain reads. Like everything in site/, it is a replay of the log.
func renderKernelHTML(home string) {
	events, err := readEvents(home)
	if err != nil {
		return
	}
	commands, cmdOrder, projectors, projOrder := declaredCaps(events)
	// grownBy is provenance: the latest kernel-signed receipt's By per capability.
	// Verified, not merely read — an unsigned or forged by-line never renders.
	grownBy := map[string]string{}
	if secret, _ := loadSecret(home); secret != nil {
		for _, e := range events {
			if e.Name != "script.compiled" {
				continue
			}
			if r, ok := verifiedReceipt(secret, e.Payload); ok && r.By != "" {
				grownBy[r.Type+"/"+r.Name] = r.By
			}
		}
	}

	esc := html.EscapeString
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>self</title></head><body>\n")
	b.WriteString("<h1>self</h1>\n")
	b.WriteString("<p class=\"muted\">a local-first, event-sourced runtime with LLM-generated capabilities</p>\n")
	b.WriteString("<p>One append-only event log is the only state. Everything here — the capabilities, the projections, this page — is a deterministic replay of that log; humans and agents read the same rendered result. Every path below is a plain file.</p>\n")
	b.WriteString(orientationHTML)

	b.WriteString("<h2>commands</h2>\n")
	if len(cmdOrder) == 0 {
		b.WriteString("<p class=\"muted\">None yet — grow a seed: <code>self grow seeds/chat</code>.</p>\n")
	}
	for _, n := range cmdOrder {
		d := commands[n]
		b.WriteString("<article class=\"card\"><h3>" + esc(d.Name) + "</h3><p>" + esc(d.Description) + "</p>")
		b.WriteString("<p class=\"muted\">produces <code>" + esc(d.Event.Name) + "</code>")
		if len(d.Params) > 0 {
			b.WriteString(" · args " + esc(jsonRepr(d.Params)))
		}
		b.WriteString(" · <code>self run " + esc(d.Name) + " …</code>")
		if by := grownBy["command/"+d.Name]; by != "" {
			b.WriteString(" · grown by " + esc(by))
		}
		b.WriteString("</p></article>\n")
	}

	b.WriteString("<h2>projections</h2>\n")
	if len(projOrder) == 0 {
		b.WriteString("<p class=\"muted\">None yet.</p>\n")
	}
	for _, n := range projOrder {
		d := projectors[n]
		b.WriteString("<article class=\"card\"><h3><a href=\"/" + esc(d.Name) + "\">/" + esc(d.Name) + "</a></h3><p>" + esc(d.Description) + "</p>")
		b.WriteString("<p class=\"muted\">consumes <code>" + esc(strings.Join(d.Consumes, ", ")) + "</code>")
		if by := grownBy["projector/"+d.Name]; by != "" {
			b.WriteString(" · grown by " + esc(by))
		}
		b.WriteString("</p></article>\n")
	}

	b.WriteString("<h2>where I live</h2>\n<table><tr><th>what</th><th>path</th></tr>")
	for _, row := range [][2]string{
		{"the only truth", filepath.Join(home, "events.jsonl")},
		{"compiled commands", filepath.Join(home, "capabilities", "commands")},
		{"compiled projectors", filepath.Join(home, "capabilities", "projectors")},
		{"materialized HTML", filepath.Join(home, "site")},
		{"stored files, content-addressed", filepath.Join(home, "files")},
	} {
		b.WriteString("<tr><td>" + esc(row[0]) + "</td><td><code>" + esc(row[1]) + "</code></td></tr>")
	}
	b.WriteString("</table>\n")

	b.WriteString("<h2>the pipe contract</h2>\n<pre>" + esc(pipeContract) + "</pre>\n")
	b.WriteString("<h2>the events I act on</h2>\n<p><code>command.declared</code> / <code>projector.declared</code> compile into capabilities (the strange loop, at grow time and run time). <code>script.compiled</code> is a compile receipt signed with my <code>.secret</code> — anyone may append one, but only a kernel-signed receipt ever installs; <code>self rehydrate</code> rebuilds my whole instance from them. <code>capability.retired</code> takes a capability off the derived surface — script and page — while every event stays; a later re-declaration revives it. <code>file.stored</code> records a file entering my content-addressed store (an upload, an <code>@path</code> arg, a seed's <code>files/</code>): the bytes live at <code>files/&lt;sha256&gt;</code> and serve at <code>/files/&lt;sha256&gt;</code>; the log carries only the metadata.</p>\n")
	b.WriteString("</body></html>\n")

	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	os.WriteFile(filepath.Join(siteDir, "kernel.html"), []byte(b.String()), 0644)
}

// ─────────────────────────────── the surface ────────────────────────────────

// The shell is the one shared enrichment the kernel injects at serve time —
// theme and feel layered over projections that stay bare semantic HTML on
// disk. The split of responsibilities is the design system: the log is the
// truth, the projection is the state, the shell is the feel. The shell knows
// the class vocabulary, never the events; strip it (self show, curl, lynx,
// rehydrate) and every page still works, because every affordance underneath
// is a plain HTML form.
//
// The feel is swappable. What is fixed is the class vocabulary and the
// structural rules below — the contract the projections and shellScript are
// written against. A *theme* changes none of that: it is only a skin, a set of
// CSS custom properties (palette, fonts, radii, border weight, shadow) that the
// structural layer reads through var(). So switching designs never renames a
// class or rewrites a rule; every projection keeps working unchanged and only
// the feel moves. Themes are picked at serve time and carry no state into the
// log — presentation, like prefers-color-scheme; the bare HTML on disk stays
// theme-agnostic, so rehydrate and self show are untouched.

const shellMeta = `<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark">`

const defaultTheme = "grove"

// A theme is a skin: CSS custom properties (palette, fonts, radii, borders) the
// structural layer reads through var(), plus—optionally—a few extra rules for a
// feel variables alone can't carry. It is injected AFTER the structural CSS, so
// its variables resolve and its rules layer on top; it never renames a class or
// changes what a projection emits.
type theme struct {
	label string
	css   string
}

//go:embed themes/*.css
var themeFS embed.FS

// themes and themeOrder are built once from the embedded themes/*.css files —
// the design set ships inside the binary, so serving needs no files on disk and
// the offline guarantees hold. Each file's base name is the theme id; the
// default (grove) lists first, the rest alphabetically. Add a design by dropping
// a .css into themes/ and rebuilding — nothing structural changes.
var themes, themeOrder = loadThemes()

func loadThemes() (map[string]theme, []string) {
	m := map[string]theme{}
	var extra []string
	entries, _ := themeFS.ReadDir("themes")
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".css") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".css")
		data, err := themeFS.ReadFile("themes/" + e.Name())
		if err != nil || name == "" {
			continue
		}
		m[name] = theme{label: themeLabel(name), css: string(data)}
		if name != defaultTheme {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	return m, append([]string{defaultTheme}, extra...)
}

// themeLabel is the picker's display name for a theme id: its capitalized form.
func themeLabel(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// structuralCSS is the fixed half of the shell: the class vocabulary and every
// layout rule, written entirely against var()s a theme supplies. It never
// mentions a literal color, font, or radius — that is what makes the embedded
// themes/*.css skins interchangeable.
//
//go:embed shell/structural.css
var structuralCSS string

// validTheme reports whether name is a known design; selection paths accept
// only known names, so the injected picker links can never smuggle
// arbitrary CSS in.
func validTheme(name string) bool { _, ok := themes[name]; return ok }

// themeCSS assembles the full <style> for one design: the shared structural
// rules, then the theme's CSS (its variables, and any extra rules that layer on
// top). Unknown names fall back to the default.
func themeCSS(name string) string {
	t, ok := themes[name]
	if !ok {
		t = themes[defaultTheme]
	}
	return shellMeta + "<style>" + structuralCSS + "\n" + t.css + "\n</style>"
}

// pickTheme resolves the design for one request: an explicit ?theme= wins,
// then the SELF_THEME instance default, then the built-in default. Two
// mechanisms, no remembered state — a theme is presentation for one request,
// like prefers-color-scheme, never something the server holds for you.
func pickTheme(r *http.Request) string {
	if t := r.URL.Query().Get("theme"); validTheme(t) {
		return t
	}
	if t := strings.TrimSpace(os.Getenv("SELF_THEME")); validTheme(t) {
		return t
	}
	return defaultTheme
}

// themePicker is the one bit of DOM the shell adds to the body: a small fixed
// switcher of plain links. It works with no JS (each link is a GET that the
// server themes and remembers), and it is styled by the active theme itself, so
// it always matches the page it sits on.
func themePicker(current string) string {
	var b strings.Builder
	b.WriteString(`<nav class="self-themes" aria-label="page design">`)
	for _, name := range themeOrder {
		if name == current {
			b.WriteString(`<a href="?theme=` + name + `" aria-current="true">` + themes[name].label + `</a>`)
		} else {
			b.WriteString(`<a href="?theme=` + name + `">` + themes[name].label + `</a>`)
		}
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// shellScript is the reactive half of the shell: progressive enhancement
// only, injected at serve time and never persisted. The state machine is
// untouched — every interaction is still form → command → events → replay;
// the script changes how the round-trip FEELS, not what it is. It may show
// intent in flight (a pending turn, a thinking brain) but never claims
// state: when the round-trip lands, the page is re-fetched and the log's
// replay wins. Liveness is the same idea watched from outside — the byte
// length of /events is the cursor; when the log grows, re-replay.
//
//go:embed shell/shell.js
var shellScriptBody string

// orientationHTML is the kernel index's static "if you are an LLM" briefing —
// self-description copy, kept as data beside the shell it renders with.
//
//go:embed shell/orientation.html
var orientationHTML string

// shellScript wraps the embedded progressive-enhancement JS as an injectable
// <script> element.
var shellScript = "<script>" + shellScriptBody + "</script>"

// siteNav is the human way around an instance: one bar of plain links,
// injected by the kernel on every served page, listing every declared
// projection plus the kernel's own surfaces (brief, events). Projectors stay
// bare — explorability is chrome, so it belongs to the shell, not to every
// script. Like everything served, it is a replay of the log: declared
// projections in declaration order.
func siteNav(home, current string) string {
	events, err := readEvents(home)
	if err != nil {
		return ""
	}
	_, _, _, projOrder := declaredCaps(events)
	link := func(href, label string) string {
		esc := html.EscapeString(label)
		// a nested page (finances/bills) marks its top-level entry (finances)
		if label == current || strings.HasPrefix(current, label+"/") {
			return `<a href="` + href + `" aria-current="true">` + esc + `</a>`
		}
		return `<a href="` + href + `">` + esc + `</a>`
	}
	var b strings.Builder
	b.WriteString(`<nav class="self-nav" aria-label="instance"><a class="self-brand" href="/">self</a>`)
	for _, n := range projOrder {
		if strings.Contains(n, "/") {
			continue // nested pages unfold from their parents, not the nav
		}
		b.WriteString(link("/"+n, n))
	}
	b.WriteString(link("/brief", "brief"))
	b.WriteString(link("/events", "events"))
	b.WriteString(`</nav>`)
	return b.String()
}

// siteFile resolves a path under SELF_HOME/site/ to a file by name, looking for
// <name>.html, <name>.md, and <name>.txt in order. It returns the file path and
// the matched extension, or "" if no such file. Used by the server and by
// `self show` so a brain (or human, or external agent) can reach any on-disk
// artifact by bare name.
func siteFile(home, name string) (path, ext string) {
	for _, e := range []string{".html", ".md", ".txt"} {
		p := filepath.Join(home, "site", name+e)
		if fileExists(p) {
			return p, e
		}
	}
	return "", ""
}

// serveSiteFile writes a site file to an HTTP response, dispatching by
// extension: .html goes through the shell (themed, progressive-enhanced), while
// .md and .txt are served verbatim as text/plain — plain text is honest about
// what it is, and the kernel renders no markup it did not itself emit.
func serveSiteFile(w http.ResponseWriter, r *http.Request, home, current, path, ext string) {
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if ext == ".html" {
		writePage(w, r, home, current, data)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

func injectShell(page []byte, theme, nav string) []byte {
	head := themeCSS(theme) + shellScript
	if i := bytes.Index(page, []byte("<head>")); i >= 0 {
		i += len("<head>")
		page = append(page[:i:i], append([]byte(head), page[i:]...)...)
	} else {
		page = append([]byte(head), page...)
	}
	if nav != "" {
		if i := bytes.Index(page, []byte("<body>")); i >= 0 {
			i += len("<body>")
			page = append(page[:i:i], append([]byte(nav), page[i:]...)...)
		} else {
			page = append([]byte(nav), page...)
		}
	}
	picker := themePicker(theme)
	if j := bytes.LastIndex(page, []byte("</body>")); j >= 0 {
		return append(page[:j:j], append([]byte(picker), page[j:]...)...)
	}
	return append(page, []byte(picker)...)
}

// writePage sends an on-disk projection through the shell for one request:
// resolve the design and inject theme + script + nav + picker. This is the
// only place a theme touches a response; nothing is written back to the log,
// to disk, or to the client. current names the page being served so the nav
// can mark it.
func writePage(w http.ResponseWriter, r *http.Request, home, current string, page []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(injectShell(page, pickTheme(r), siteNav(home, current)))
}
