package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Intent describes a desired outcome, not an execution type. Closing it records a mind's
// judgment; the kernel cannot certify that an arbitrary outcome was achieved.
type intentItem struct {
	Name        string         `json:"name"`
	Summary     string         `json:"summary,omitempty"`
	Description string         `json:"description"`
	Seq         int            `json:"-"`
	Closure     *intentClosure `json:"-"`
}

type intentClosure struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

var intentName = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

func validIntentName(name string) bool { return intentName.MatchString(name) }

func (w *intentItem) declaration() decl {
	return decl{Name: w.Name, Summary: w.Summary, Description: w.Description}
}

func (st *state) intent(name string) *intentItem {
	for _, w := range st.Intents {
		if w.Name == name {
			return w
		}
	}
	return nil
}

func (st *state) applyIntent(e Event) {
	// Defense in depth: imported testimony must not open or close local work.
	if strings.HasPrefix(e.Via, doorLearn) {
		return
	}
	switch e.Name {
	case "intent.declared", "work.declared":
		var w intentItem
		if json.Unmarshal(e.Payload, &w) != nil || !validIntentName(w.Name) || strings.TrimSpace(w.Description) == "" {
			return
		}
		w.Seq = e.Seq
		if old := st.intent(w.Name); old != nil {
			*old = w
		} else {
			st.Intents = append(st.Intents, &w)
		}
	case "intent.closed", "work.closed":
		var c intentClosure
		if json.Unmarshal(e.Payload, &c) != nil || strings.TrimSpace(c.Reason) == "" || (c.Outcome != "completed" && c.Outcome != "dropped") {
			return
		}
		if w := st.intent(c.Name); w != nil {
			w.Closure = &c
		}
	}
}

func (st *state) openIntents() []*intentItem {
	var open []*intentItem
	for _, w := range st.Intents {
		if w.Closure == nil {
			open = append(open, w)
		}
	}
	return open
}

func intentIndex(items []*intentItem) string {
	var b strings.Builder
	for _, w := range items {
		fmt.Fprintf(&b, "- intent/%s — %s\n", w.Name, w.declaration().summary())
	}
	return b.String()
}

func intentBrief(st *state) string {
	open := st.openIntents()
	if len(open) == 0 {
		return ""
	}
	const limit = 12
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Open intents — %d declared outcomes\n\n", len(open))
	b.WriteString("Intentions to consider within the current scope. Read details with `self brief intent/<name>`.\n\n")
	b.WriteString(intentIndex(open[:min(len(open), limit)]))
	if len(open) > limit {
		fmt.Fprintf(&b, "\n%d more; `self brief intent/` lists all open intents.\n", len(open)-limit)
	}
	return b.String()
}

func intentDetail(st *state, name string) (string, error) {
	if name == "" {
		open := st.openIntents()
		if len(open) == 0 {
			return "No open intents.\n", nil
		}
		return intentIndex(open), nil
	}
	w := st.intent(name)
	if w == nil {
		return "", fmt.Errorf("no intent %q in this log — `self brief intent/` lists open intents", name)
	}
	text := fmt.Sprintf("# intent/%s\n\n%s\n\ndeclared at seq %d", w.Name, w.Description, w.Seq)
	if w.Closure == nil {
		text += " · open\n"
	} else {
		text += fmt.Sprintf(" · %s\n\n%s\n", w.Closure.Outcome, w.Closure.Reason)
	}
	return text, nil
}
