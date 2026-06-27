package kernel

import (
	"encoding/json"
	"os"
	"path/filepath"

	"self/internal/event"
	"self/internal/seed"
)

// CapTestResult is one capability's selftest outcome.
type CapTestResult struct {
	Name        string
	Kind        string // "command" or "projector"
	HasExamples bool
	Result      seed.VerifyResult
}

// OK reports whether the capability passed. A capability with no examples is
// untested, not failed — selftest reports it so coverage is visible.
func (r CapTestResult) OK() bool {
	if !r.HasExamples {
		return true
	}
	return r.Result.OK()
}

// SelfTest re-runs every INSTALLED capability's declared examples against the
// binary currently on disk, and returns a result per capability. Where the
// compile-time gate (VerifyAndLog) checks a freshly compiled script, SelfTest
// checks what is actually installed right now — so it catches drift: a
// hand-edited script, a rehydrated body, or a regression introduced anywhere
// between compile and now. It is the projection/output-as-oracle made into a
// standing health check: feed each capability its example inputs, read its
// output, assert. Pure (no state mutation); safe to run any time.
func SelfTest(home string) ([]CapTestResult, error) {
	events, err := openEventStore(home).Read()
	if err != nil {
		return nil, err
	}

	// Latest declared examples per capability name.
	cmdEx := map[string][]seed.Example{}
	projEx := map[string][]seed.Example{}
	for _, e := range events {
		switch e.Name {
		case event.CommandDeclared:
			var c seed.Command
			if json.Unmarshal(e.Payload, &c) == nil && c.Name != "" {
				cmdEx[c.Name] = c.Examples
			}
		case event.ProjectorDeclared:
			var p seed.ProjectorDecl
			if json.Unmarshal(e.Payload, &p) == nil && p.Name != "" {
				projEx[p.Name] = p.Examples
			}
		}
	}

	var results []CapTestResult
	run := func(kind, dir string, exmap map[string][]seed.Example) error {
		entries, _ := os.ReadDir(filepath.Join(home, "capabilities", dir))
		for _, ent := range entries {
			if ent.IsDir() {
				continue
			}
			name := ent.Name()
			script, rErr := os.ReadFile(filepath.Join(home, "capabilities", dir, name))
			if rErr != nil {
				continue
			}
			ex := exmap[name]
			res, vErr := seed.VerifyScript(string(script), kind, ex)
			if vErr != nil {
				return vErr
			}
			results = append(results, CapTestResult{
				Name: name, Kind: kind, HasExamples: len(ex) > 0, Result: res,
			})
		}
		return nil
	}

	if err := run("command", "commands", cmdEx); err != nil {
		return results, err
	}
	if err := run("projector", "projectors", projEx); err != nil {
		return results, err
	}
	return results, nil
}
