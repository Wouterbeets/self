package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// A view may ask to be read on arrival: its declaration carries a brief
// section, and the orienting surfaces (bare `self`, `self prompt`) run it and
// splice its output under its own heading. The kernel knows nothing of what
// the view says; it only orders, bounds and isolates it. A section is a read:
// it appends nothing, and a view that fails or overruns its time costs one
// line, never the brief.
type briefSpec struct {
	Order int `json:"order,omitempty"`
	Bytes int `json:"bytes,omitempty"`
}

const (
	sectionBytes    = 2000
	sectionBytesMax = 16000
	sectionTimeout  = 3 * time.Second
)

func (s briefSpec) budget() int {
	switch {
	case s.Bytes <= 0:
		return sectionBytes
	case s.Bytes > sectionBytesMax:
		return sectionBytesMax
	}
	return s.Bytes
}

func sectionViews(st *state) []*capability {
	var out []*capability
	for _, c := range st.list(kindView) {
		if c.Decl.Brief != nil && c.Receipt != nil {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := out[i].Decl.Brief.Order, out[j].Decl.Brief.Order; a != b {
			return a < b
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// briefSections renders every section view in order. Views run concurrently
// so the slowest one, not their sum, bounds the wait.
func briefSections(home string, st *state) string {
	views := sectionViews(st)
	if len(views) == 0 {
		return ""
	}
	pages := make([]string, len(views))
	done := make(chan struct{}, len(views))
	for i, c := range views {
		go func() {
			defer func() { done <- struct{}{} }()
			pages[i] = section(home, st, c)
		}()
	}
	for range views {
		<-done
	}
	return strings.Join(pages, "")
}

func section(home string, st *state, c *capability) string {
	head := fmt.Sprintf("\n## %s — `self view %s`\n\n", c.Name, c.Name)
	ctx, cancel := context.WithTimeout(context.Background(), sectionTimeout)
	defer cancel()
	out, err := executeView(ctx, home, st, c.Name, nil, io.Discard, 100*time.Millisecond)
	if ctx.Err() != nil {
		return head + fmt.Sprintf("(unavailable: no answer within %s)\n", sectionTimeout)
	}
	if err != nil {
		return head + fmt.Sprintf("(unavailable: %s)\n", oneLine(err.Error()))
	}
	body := strings.TrimRight(string(out), "\n")
	if strings.TrimSpace(body) == "" {
		return ""
	}
	return head + clipLines(body, c.Decl.Brief.budget(), c.Name) + "\n"
}

// clipLines keeps whole lines within n bytes and says where the rest is.
func clipLines(s string, n int, name string) string {
	if len(s) <= n {
		return s
	}
	more := fmt.Sprintf("… clipped at %d bytes; `self view %s` prints all of it", n, name)
	keep := n - len(more) - 1
	if keep < 0 {
		keep = 0
	}
	cut := strings.LastIndexByte(s[:keep], '\n')
	if cut < 0 {
		return trunc(s, keep) + "\n" + more
	}
	return s[:cut] + "\n" + more
}
