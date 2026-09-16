package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Work carries intent, not an execution type. Closing it records a mind's
// judgment; the kernel cannot certify that an arbitrary outcome was achieved.
type workItem struct {
	Name        string       `json:"name"`
	Summary     string       `json:"summary,omitempty"`
	Description string       `json:"description"`
	Seq         int          `json:"-"`
	Closure     *workClosure `json:"-"`
}

type workClosure struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

func validWorkName(name string) bool {
	if name == "" || len(name) > 200 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-/", c)) {
			return false
		}
	}
	return true
}

func (w *workItem) declaration() decl {
	return decl{Name: w.Name, Summary: w.Summary, Description: w.Description}
}

func (st *state) work(name string) *workItem {
	for _, w := range st.Work {
		if w.Name == name {
			return w
		}
	}
	return nil
}

func (st *state) applyWork(e Event) {
	// Defense in depth: imported testimony must not open or close local work.
	if strings.HasPrefix(e.Via, doorLearn) {
		return
	}
	switch e.Name {
	case "work.declared":
		var w workItem
		if json.Unmarshal(e.Payload, &w) != nil || !validWorkName(w.Name) || strings.TrimSpace(w.Description) == "" {
			return
		}
		w.Seq = e.Seq
		if old := st.work(w.Name); old != nil {
			*old = w
		} else {
			st.Work = append(st.Work, &w)
		}
	case "work.closed":
		var c workClosure
		if json.Unmarshal(e.Payload, &c) != nil || strings.TrimSpace(c.Reason) == "" || (c.Outcome != "completed" && c.Outcome != "dropped") {
			return
		}
		if w := st.work(c.Name); w != nil {
			w.Closure = &c
		}
	}
}

func (st *state) openWork() []*workItem {
	var open []*workItem
	for _, w := range st.Work {
		if w.Closure == nil {
			open = append(open, w)
		}
	}
	return open
}

func workIndex(items []*workItem) string {
	var b strings.Builder
	for _, w := range items {
		fmt.Fprintf(&b, "- work/%s — %s\n", w.Name, w.declaration().summary())
	}
	return b.String()
}

func workBrief(st *state) string {
	open := st.openWork()
	if len(open) == 0 {
		return ""
	}
	const limit = 12
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Open work — %d declared outcomes\n\n", len(open))
	b.WriteString("Available work, not an assignment. Read details with `self brief work/<name>`.\n\n")
	shown := open
	if len(shown) > limit {
		shown = shown[:limit]
	}
	b.WriteString(workIndex(shown))
	if len(open) > limit {
		fmt.Fprintf(&b, "\n%d more; `self brief work/` lists all open work.\n", len(open)-limit)
	}
	return b.String()
}

func workDetail(st *state, name string) (string, error) {
	if name == "" {
		if len(st.openWork()) == 0 {
			return "No open work.\n", nil
		}
		return workIndex(st.openWork()), nil
	}
	w := st.work(name)
	if w == nil {
		return "", fmt.Errorf("no work %q in this log — `self brief work/` lists open work", name)
	}
	text := fmt.Sprintf("# work/%s\n\n%s\n\ndeclared at seq %d", w.Name, w.Description, w.Seq)
	if w.Closure == nil {
		text += " · open\n"
	} else {
		text += fmt.Sprintf(" · %s\n\n%s\n", w.Closure.Outcome, w.Closure.Reason)
	}
	return text, nil
}
