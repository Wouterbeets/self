package main

import (
	"bytes"
	"strings"
	"testing"
)

func sectionView(t *testing.T, name string, spec *briefSpec, script string) string {
	t.Helper()
	return line(t, "view.declared", decl{Name: name, Description: "a section", Brief: spec}) +
		line(t, "script.authored", authored{Type: "view", Name: name, Script: "#!/bin/sh\ncat >/dev/null\n" + script})
}

// A view that asks for a brief section is read on arrival, under its own
// heading, in declared order; the kernel adds nothing of its own.
func TestBriefSectionsRunInOrderOnBareSelf(t *testing.T) {
	h := home(t)
	heard(t, h, sectionView(t, "later", &briefSpec{Order: 2}, "echo LATER\n")+
		sectionView(t, "sooner", &briefSpec{Order: 1}, "echo SOONER\n")+
		sectionView(t, "quiet", &briefSpec{}, "true\n")+
		sectionView(t, "plain", nil, "echo PLAIN\n"))
	before := stateRevision(replayed(t, h))

	var out bytes.Buffer
	if err := dispatch(h, "", nil, &out); err != nil {
		t.Fatal(err)
	}
	p := out.String()
	s, l := strings.Index(p, "## sooner — `self view sooner`\n\nSOONER"), strings.Index(p, "## later — `self view later`\n\nLATER")
	if s < 0 || l < 0 || s > l {
		t.Fatalf("sections missing or out of order:\n%s", p)
	}
	if strings.Contains(p, "PLAIN") || strings.Contains(p, "## quiet") {
		t.Fatalf("an undeclared or empty section was shown:\n%s", p)
	}
	if after := stateRevision(replayed(t, h)); after != before {
		t.Fatalf("reading the brief appended: %v -> %v", before, after)
	}
}

// A section is bounded: it keeps whole lines within its budget and points at
// the full view.
func TestBriefSectionIsClipped(t *testing.T) {
	h := home(t)
	heard(t, h, sectionView(t, "long", &briefSpec{Bytes: 200}, "i=0; while [ $i -lt 100 ]; do echo line-$i; i=$((i+1)); done\n"))
	got := briefSections(h, replayed(t, h))
	if !strings.Contains(got, "line-0\n") || strings.Contains(got, "line-99") {
		t.Fatalf("section not clipped:\n%s", got)
	}
	if !strings.Contains(got, "`self view long` prints all of it") {
		t.Fatalf("clipped section does not say where the rest is:\n%s", got)
	}
	body := got[strings.Index(got, "\n\n")+2:]
	if len(strings.TrimRight(body, "\n")) > 200 {
		t.Fatalf("section body is %d bytes, budget 200", len(body))
	}
}

// A failing section costs one line, never the brief.
func TestBriefSectionFailureIsOneLine(t *testing.T) {
	h := home(t)
	heard(t, h, sectionView(t, "broken", &briefSpec{Order: 1}, "echo boom >&2; exit 3\n")+
		sectionView(t, "fine", &briefSpec{Order: 2}, "echo FINE\n"))
	got := briefSections(h, replayed(t, h))
	if !strings.Contains(got, "## broken") || !strings.Contains(got, "(unavailable:") || !strings.Contains(got, "FINE") {
		t.Fatalf("a failing section broke the others:\n%s", got)
	}
}

// A declared section without a script is pending, not run.
func TestPendingBriefSectionIsNotRun(t *testing.T) {
	h := home(t)
	heard(t, h, line(t, "view.declared", decl{Name: "soon", Description: "later", Brief: &briefSpec{}}))
	if got := briefSections(h, replayed(t, h)); got != "" {
		t.Fatalf("a pending section rendered: %q", got)
	}
}

// `self brief <name>` says a view is a section and what it may spend.
func TestBriefOneNamesTheSection(t *testing.T) {
	h := home(t)
	heard(t, h, sectionView(t, "s", &briefSpec{Order: 4, Bytes: 99999}, "echo x\n"))
	got, err := briefOne(replayed(t, h), "s")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "brief section: order 4, 16000 bytes") {
		t.Fatalf("section budget not reported or not capped:\n%s", got)
	}
}
