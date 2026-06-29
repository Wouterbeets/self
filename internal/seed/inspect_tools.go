package seed

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	inspectMaxReadBytes   = 20000
	inspectMaxSearchFiles = 500
)

func inspectToolDefs() []map[string]any {
	return []map[string]any{
		toolDef("tree", "Show a bounded directory tree for progressive unfolding of the current state. Counts entries and reports omissions.", map[string]any{
			"path":  map[string]any{"type": "string", "description": "Path under SELF_HOME, e.g. ., site, capabilities, capabilities/commands"},
			"depth": map[string]any{"type": "integer", "description": "Directory depth to expand, like tree -L. Default 2, max 5."},
			"limit": map[string]any{"type": "integer", "description": "Maximum entries to show total. Default 120, max 500."},
		}, []string{"path"}),
		toolDef("list", "List one directory with total/shown/omitted counts and file metadata.", map[string]any{
			"path":  map[string]any{"type": "string", "description": "Directory path under SELF_HOME"},
			"limit": map[string]any{"type": "integer", "description": "Maximum entries to show. Default 100, max 500."},
		}, []string{"path"}),
		toolDef("read", "Read a bounded slice of an allowed file. Use this for site/*.html, events.jsonl, scripts, and seed materialized state.", map[string]any{
			"path":   map[string]any{"type": "string", "description": "File path under SELF_HOME"},
			"offset": map[string]any{"type": "integer", "description": "Byte offset. Default 0."},
			"limit":  map[string]any{"type": "integer", "description": "Maximum bytes to return. Default 8000, max 20000."},
		}, []string{"path"}),
		toolDef("search", "Regex search over allowed state files. Use this to discover capabilities, routes, event names, and conventions.", map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Go regular expression"},
			"path":    map[string]any{"type": "string", "description": "File or directory under SELF_HOME. Default ."},
			"limit":   map[string]any{"type": "integer", "description": "Maximum matching lines. Default 80, max 300."},
		}, []string{"pattern"}),
		toolDef("events", "Structured event-log reader with filtering and limits. Prefer this over raw-reading large events.jsonl.", map[string]any{
			"names":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Event names to include. Empty means all."},
			"since_seq": map[string]any{"type": "integer", "description": "Only events with seq greater than this. Default 0."},
			"limit":     map[string]any{"type": "integer", "description": "Maximum events. Default 30, max 200."},
		}, nil),
		toolDef("event_names", "Compact event vocabulary summary: counts and latest seq per event name.", map[string]any{
			"limit": map[string]any{"type": "integer", "description": "Maximum names to show. Default 50, max 200."},
		}, nil),
		toolDef("latest_state", "Tiny snapshot of current state: latest seq, event names, top-level tree counts, rendered site pages, and brain provider without secrets.", map[string]any{}, nil),
	}
}

func isInspectToolName(name string) bool {
	switch name {
	case "tree", "list", "read", "search", "events", "event_names", "latest_state":
		return true
	}
	return false
}

func toolDef(name, description string, properties map[string]any, required []string) map[string]any {
	params := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		params["required"] = required
	}
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": description,
			"parameters":  params,
		},
	}
}

var doneTool = toolDef("done", "Finish the current brain/orchestration task with a short summary. Use when you are done exploring/declaring.", map[string]any{
	"summary": map[string]any{"type": "string", "description": "Short final summary"},
}, []string{"summary"})

func (c *Compiler) executeInspectTool(name, args string) string {
	switch name {
	case "tree":
		var a struct {
			Path  string `json:"path"`
			Depth int    `json:"depth"`
			Limit int    `json:"limit"`
		}
		json.Unmarshal([]byte(args), &a)
		return c.inspectTree(a.Path, a.Depth, a.Limit)
	case "list":
		var a struct {
			Path  string `json:"path"`
			Limit int    `json:"limit"`
		}
		json.Unmarshal([]byte(args), &a)
		return c.inspectList(a.Path, a.Limit)
	case "read":
		var a struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		json.Unmarshal([]byte(args), &a)
		return c.inspectRead(a.Path, a.Offset, a.Limit)
	case "search":
		var a struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
			Limit   int    `json:"limit"`
		}
		json.Unmarshal([]byte(args), &a)
		return c.inspectSearch(a.Pattern, a.Path, a.Limit)
	case "events":
		var a struct {
			Names    []string `json:"names"`
			SinceSeq int      `json:"since_seq"`
			Limit    int      `json:"limit"`
		}
		json.Unmarshal([]byte(args), &a)
		return c.inspectEvents(a.Names, a.SinceSeq, a.Limit)
	case "event_names":
		var a struct {
			Limit int `json:"limit"`
		}
		json.Unmarshal([]byte(args), &a)
		return c.inspectEventNames(a.Limit)
	case "latest_state":
		return c.inspectLatestState()
	default:
		return fmt.Sprintf("error: unknown inspection tool %q", name)
	}
}

func (c *Compiler) inspectPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	path = filepath.Clean(strings.TrimSpace(path))
	if filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, "../") {
		return "", fmt.Errorf("path must stay under SELF_HOME")
	}
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		switch part {
		case ".brain-key", ".secret", ".identity", ".git":
			return "", fmt.Errorf("private kernel files are not visible to the LLM")
		}
	}
	full := filepath.Join(c.Home, path)
	rel, err := filepath.Rel(c.Home, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must stay under SELF_HOME")
	}
	return full, nil
}

func cleanInspectRel(home, full string) string {
	rel, err := filepath.Rel(home, full)
	if err != nil || rel == "." {
		return "."
	}
	return filepath.ToSlash(rel)
}

func isPrivateInspectName(name string) bool {
	switch name {
	case ".brain-key", ".secret", ".identity", ".git":
		return true
	}
	return false
}

func clampInt(v, def, max int) int {
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}

func (c *Compiler) inspectList(path string, limit int) string {
	limit = clampInt(limit, 100, 500)
	full, err := c.inspectPath(path)
	if err != nil {
		return "error: " + err.Error()
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return "error: " + err.Error()
	}
	entries = visibleEntries(entries)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%s total=%d shown=%d omitted=%d\n", cleanInspectRel(c.Home, full), len(entries), min(len(entries), limit), max(0, len(entries)-limit))
	for i, e := range entries {
		if i >= limit {
			break
		}
		info, _ := e.Info()
		fmt.Fprintf(&b, "%s\n", inspectEntryLine(e.Name(), e.IsDir(), info))
	}
	return b.String()
}

func (c *Compiler) inspectTree(path string, depth, limit int) string {
	depth = clampInt(depth, 2, 5)
	limit = clampInt(limit, 120, 500)
	full, err := c.inspectPath(path)
	if err != nil {
		return "error: " + err.Error()
	}
	shown := 0
	var b strings.Builder
	var walk func(string, string, int)
	walk = func(dir, prefix string, remaining int) {
		if shown >= limit {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Fprintf(&b, "%s(error: %s)\n", prefix, err)
			return
		}
		entries = visibleEntries(entries)
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir()
			}
			return entries[i].Name() < entries[j].Name()
		})
		fmt.Fprintf(&b, "%s%s/ (%d entries)\n", prefix, filepath.Base(dir), len(entries))
		if remaining <= 0 {
			return
		}
		for i, e := range entries {
			if shown >= limit {
				fmt.Fprintf(&b, "%s... %d more (limit reached)\n", prefix, len(entries)-i)
				return
			}
			info, _ := e.Info()
			shown++
			if e.IsDir() {
				walk(filepath.Join(dir, e.Name()), prefix+"  ", remaining-1)
			} else {
				fmt.Fprintf(&b, "%s  %s\n", prefix, inspectEntryLine(e.Name(), false, info))
			}
		}
	}
	walk(full, "", depth)
	fmt.Fprintf(&b, "shown=%d limit=%d\n", shown, limit)
	return b.String()
}

func visibleEntries(entries []fs.DirEntry) []fs.DirEntry {
	visible := entries[:0]
	for _, e := range entries {
		if isPrivateInspectName(e.Name()) {
			continue
		}
		visible = append(visible, e)
	}
	return visible
}

func inspectEntryLine(name string, isDir bool, info fs.FileInfo) string {
	if isDir {
		return name + "/"
	}
	size := int64(0)
	mode := ""
	if info != nil {
		size = info.Size()
		if info.Mode()&0111 != 0 {
			mode = " executable"
		}
	}
	return fmt.Sprintf("%s (%d bytes%s)", name, size, mode)
}

func (c *Compiler) inspectRead(path string, offset, limit int) string {
	limit = clampInt(limit, 8000, inspectMaxReadBytes)
	if offset < 0 {
		offset = 0
	}
	full, err := c.inspectPath(path)
	if err != nil {
		return "error: " + err.Error()
	}
	info, err := os.Stat(full)
	if err != nil {
		return "error: " + err.Error()
	}
	if info.IsDir() {
		return "error: read expects a file; use list/tree for directories"
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "error: " + err.Error()
	}
	if offset > len(data) {
		offset = len(data)
	}
	end := min(len(data), offset+limit)
	omitted := len(data) - end
	return fmt.Sprintf("%s bytes=%d offset=%d returned=%d omitted_after=%d\n%s", cleanInspectRel(c.Home, full), len(data), offset, end-offset, omitted, string(data[offset:end]))
}

func (c *Compiler) inspectSearch(pattern, path string, limit int) string {
	limit = clampInt(limit, 80, 300)
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "error: bad regex: " + err.Error()
	}
	full, err := c.inspectPath(path)
	if err != nil {
		return "error: " + err.Error()
	}
	matches, files := 0, 0
	var b strings.Builder
	visit := func(file string) bool {
		if matches >= limit || files >= inspectMaxSearchFiles {
			return false
		}
		info, err := os.Stat(file)
		if err != nil || info.IsDir() || info.Size() > 512*1024 {
			return true
		}
		files++
		f, err := os.Open(file)
		if err != nil {
			return true
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			if re.MatchString(line) {
				matches++
				fmt.Fprintf(&b, "%s:%d: %s\n", cleanInspectRel(c.Home, file), lineNo, truncate(line, 240))
				if matches >= limit {
					break
				}
			}
		}
		return matches < limit
	}
	info, err := os.Stat(full)
	if err != nil {
		return "error: " + err.Error()
	}
	if !info.IsDir() {
		visit(full)
	} else {
		filepath.WalkDir(full, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && isPrivateInspectName(d.Name()) {
				return filepath.SkipDir
			}
			if !d.IsDir() && !isPrivateInspectName(d.Name()) && !visit(p) {
				return filepath.SkipAll
			}
			return nil
		})
	}
	fmt.Fprintf(&b, "matches=%d limit=%d files_scanned=%d\n", matches, limit, files)
	return b.String()
}

func (c *Compiler) inspectEvents(names []string, sinceSeq, limit int) string {
	limit = clampInt(limit, 30, 200)
	nameSet := map[string]bool{}
	for _, n := range names {
		if n != "" {
			nameSet[n] = true
		}
	}
	file, err := os.Open(filepath.Join(c.Home, "events.jsonl"))
	if err != nil {
		return "error: " + err.Error()
	}
	defer file.Close()
	var b strings.Builder
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	shown := 0
	for sc.Scan() {
		var e struct {
			Seq     int             `json:"seq"`
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Seq <= sinceSeq || (len(nameSet) > 0 && !nameSet[e.Name]) {
			continue
		}
		fmt.Fprintf(&b, "%d %s %s\n", e.Seq, e.Name, truncate(string(e.Payload), 500))
		shown++
		if shown >= limit {
			break
		}
	}
	fmt.Fprintf(&b, "shown=%d limit=%d\n", shown, limit)
	return b.String()
}

func (c *Compiler) inspectEventNames(limit int) string {
	limit = clampInt(limit, 50, 200)
	file, err := os.Open(filepath.Join(c.Home, "events.jsonl"))
	if err != nil {
		return "error: " + err.Error()
	}
	defer file.Close()
	type stat struct {
		Name          string
		Count, Latest int
	}
	stats := map[string]*stat{}
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		var e struct {
			Seq  int    `json:"seq"`
			Name string `json:"name"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Name == "" {
			continue
		}
		if stats[e.Name] == nil {
			stats[e.Name] = &stat{Name: e.Name}
		}
		stats[e.Name].Count++
		stats[e.Name].Latest = e.Seq
	}
	list := make([]*stat, 0, len(stats))
	for _, s := range stats {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		return list[i].Name < list[j].Name
	})
	var b strings.Builder
	for i, s := range list {
		if i >= limit {
			break
		}
		fmt.Fprintf(&b, "%s count=%d latest_seq=%d\n", s.Name, s.Count, s.Latest)
	}
	fmt.Fprintf(&b, "names=%d shown=%d\n", len(list), min(len(list), limit))
	return b.String()
}

func (c *Compiler) inspectLatestState() string {
	return strings.Join([]string{
		"EVENT NAMES:\n" + c.inspectEventNames(20),
		"SITE:\n" + c.inspectList("site", 30),
		"TOP LEVEL:\n" + c.inspectTree(".", 1, 80),
	}, "\n")
}
