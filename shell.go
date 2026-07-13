package main

import (
	"bytes"
	_ "embed"
	"html"
	"net/http"
	"os"
	"path/filepath"
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
	b.WriteString("<p class=\"muted\">software that grows to fit the person using it — on one record that person owns</p>\n")
	b.WriteString("<p>One append-only event log is the authoritative state. Everything here — the capabilities, the projections, this page — is a deterministic replay of that log. Humans and agents read the same rendered result. Every path below is a plain file.</p>\n")
	b.WriteString(orientationHTML)

	b.WriteString("<h2>commands</h2>\n")
	if len(cmdOrder) == 0 {
		b.WriteString("<p class=\"muted\">None yet — learn a lesson: <code>self learn lessons/chat</code>.</p>\n")
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
	} {
		b.WriteString("<tr><td>" + esc(row[0]) + "</td><td><code>" + esc(row[1]) + "</code></td></tr>")
	}
	b.WriteString("</table>\n")

	b.WriteString("<h2>the pipe contract</h2>\n<pre>" + esc(pipeContract) + "</pre>\n")
	b.WriteString("<h2>the events I act on</h2>\n<p><code>command.declared</code> / <code>projector.declared</code> compile into capabilities (the strange loop, at learn time and run time alike). <code>script.compiled</code> is a compile receipt signed with my <code>.secret</code> — anyone may append one, but only a kernel-signed receipt ever installs; <code>self rehydrate</code> rebuilds my whole instance from them. <code>capability.retired</code> takes a capability off the derived surface — script and page — while every event stays; a later re-declaration revives it.</p>\n")
	b.WriteString("</body></html>\n")

	siteDir := filepath.Join(home, "site")
	os.MkdirAll(siteDir, 0755)
	os.WriteFile(filepath.Join(siteDir, "kernel.html"), []byte(b.String()), 0644)
}

// ─────────────────────────────── the surface ────────────────────────────────

// The shell is the one shared enrichment the kernel injects at serve time — a
// skin and a nav layered over projections that stay bare semantic HTML on
// disk. The split of responsibilities is the design system: the log is the
// truth, the projection is the state, the shell is the feel. The shell knows
// the class vocabulary, never the events; strip it (self show, curl, lynx,
// rehydrate) and every page still works, because every affordance underneath
// is a plain HTML form.

const shellMeta = `<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark">`

// shellCSS is the whole feel: the class vocabulary's layout rules and the one
// skin, embedded so serving needs no files on disk and the offline guarantees
// hold.
//
//go:embed shell/shell.css
var shellCSS string

// orientationHTML is the kernel index's static "if you are an LLM" briefing —
// self-description copy, kept as data beside the shell it renders with.
//
//go:embed shell/orientation.html
var orientationHTML string

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
// extension: .html goes through the shell, while .md and .txt are served
// verbatim as text/plain — plain text is honest about what it is, and the
// kernel renders no markup it did not itself emit.
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

func injectShell(page []byte, nav string) []byte {
	head := shellMeta + "<style>" + shellCSS + "</style>"
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
	return page
}

// writePage sends an on-disk projection through the shell for one request:
// inject the skin and the nav. Nothing is written back to the log, to disk, or
// to the client. current names the page being served so the nav can mark it.
func writePage(w http.ResponseWriter, r *http.Request, home, current string, page []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(injectShell(page, siteNav(home, current)))
}
