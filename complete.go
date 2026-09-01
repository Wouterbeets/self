// Shell completion. The kernel can finish what it already knows — verbs, and
// the capability names its replayed state holds — but an argument like a goal
// name is domain state, and the kernel holds no domain model. So argument
// completion is delegated to an ordinary view named `complete.<name>`: a pure
// function of the log, authored through the same growth loop as everything
// else. The instance grows its own autocomplete; the kernel only provides the
// seam. PROTOCOL.md documents the convention.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// completerTimeout bounds a delegated completer. Completion runs on a
// tab-press: a completer that hangs must degrade to silence, never to a
// frozen shell. A var so tests can shorten it.
var completerTimeout = 2 * time.Second

// completerMaxLines bounds what a delegated completer can hand a shell.
const completerMaxLines = 512

var verbCandidates = []struct{ name, desc string }{
	{"hear", "ingest event JSONL or authored scripts from stdin"},
	{"brief", "show capabilities, pending work, and refusals"},
	{"run", "execute a command capability and append its events"},
	{"view", "replay a pure view; built-in log is always available"},
	{"loop", "run situated turns to an unchanged-state fixed point"},
	{"learn", "deposit an account and print its learning prompt"},
	{"give", "write an event or capability account"},
	{"rehydrate", "rebuild derived capability files from the log"},
	{"completion", "print a shell completion script (zsh|bash|fish)"},
	{"help", "print the complete protocol"},
}

// cmdComplete answers `self __complete <words…>`: the words after `self`, the
// last being the partial word under the cursor (possibly empty). It prints one
// candidate per line, optionally `candidate\tdescription`. Completion is
// best-effort by construction: anything wrong degrades to no output and exit
// 0, because stderr on this path lands in someone's prompt line.
func cmdComplete(home string, words []string, out io.Writer) error {
	if len(words) == 0 {
		words = []string{""}
	}
	cur := words[len(words)-1]
	prev := words[:len(words)-1]

	if len(prev) == 0 {
		for _, v := range verbCandidates {
			if strings.HasPrefix(v.name, cur) {
				fmt.Fprintf(out, "%s\t%s\n", v.name, v.desc)
			}
		}
		return nil
	}

	switch prev[0] {
	case "view", "run":
		st, err := loadState(home)
		if err != nil {
			return nil
		}
		typ := kindView
		if prev[0] == "run" {
			typ = kindCommand
		}
		if len(prev) == 1 {
			completeCapNames(st, typ, cur, out)
			return nil
		}
		// An argument position. The kernel does not know what a goal or a
		// task is; a view named complete.<name> might, and it is a pure
		// replay, so running it on a tab-press reads and never writes the
		// log. It receives the same words this verb did.
		emitLines(out, runCompleter(home, st, "complete."+prev[1], words))

	case "give":
		if len(prev) != 1 {
			return nil // the second argument is a directory: the shim falls back to files
		}
		st, err := loadState(home)
		if err != nil {
			return nil
		}
		for _, typ := range []string{kindCommand, kindView} {
			for _, c := range st.list(typ) {
				if sel := typ + "/" + c.Name; strings.HasPrefix(sel, cur) {
					fmt.Fprintf(out, "%s\t%s\n", sel, trunc(oneLine(c.Decl.Description), 72))
				}
			}
		}
		seen := map[string]bool{}
		for _, e := range st.Events {
			if !seen[e.Name] && strings.HasPrefix(e.Name, cur) {
				seen[e.Name] = true
				fmt.Fprintf(out, "%s\tevents by this name\n", e.Name)
			}
		}

	case "completion":
		if len(prev) != 1 {
			return nil
		}
		for _, sh := range []string{"zsh", "bash", "fish"} {
			if strings.HasPrefix(sh, cur) {
				fmt.Fprintln(out, sh)
			}
		}

	case "loop":
		if !strings.HasPrefix(cur, "-") {
			return nil // after the flags comes `-- <mind…>`: the shim falls back to files
		}
		for _, f := range []struct{ name, desc string }{
			{"--ask", "explicit objective for pass one; later passes are naked"},
			{"--max-passes", "fail if state changes for N consecutive passes"},
			{"--timeout", "fail when one mind process exceeds this duration"},
			{"--help", "print the complete loop invocation"},
		} {
			if strings.HasPrefix(f.name, cur) {
				fmt.Fprintf(out, "%s\t%s\n", f.name, f.desc)
			}
		}
	}
	return nil
}

// completeCapNames prints the names the replayed state actually knows,
// annotated from their declarations. A pending capability is offered too:
// completing to it and running it yields the kernel's honest "declared but
// pending" answer, so the tab key doubles as a status surface.
func completeCapNames(st *state, typ, cur string, out io.Writer) {
	for _, c := range st.list(typ) {
		if !strings.HasPrefix(c.Name, cur) {
			continue
		}
		desc := trunc(oneLine(c.Decl.Description), 72)
		if c.Receipt == nil {
			desc += " (pending — no script yet)"
		}
		fmt.Fprintf(out, "%s\t%s\n", c.Name, desc)
	}
	if typ == kindView && st.cap(kindView, "log") == nil && strings.HasPrefix("log", cur) {
		fmt.Fprintln(out, "log\tevery event, one line each (built in)")
	}
}

// runCompleter replays the view that owns an argument position. It is runView
// with the shell in mind: a deadline instead of patience, silence instead of
// stderr, and nil instead of any error.
func runCompleter(home string, st *state, name string, args []string) []string {
	c := st.cap(kindView, name)
	if c == nil || c.Receipt == nil {
		return nil
	}
	bin, err := materialize(home, st, kindView, name)
	if err != nil {
		return nil
	}
	scratch, err := os.MkdirTemp("", "self-complete-")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(scratch)

	ctx, cancel := context.WithTimeout(context.Background(), completerTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env, cmd.Dir = scriptEnv("", scratch), scratch
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}
	var buf strings.Builder
	cmd.Stdout = &buf
	if cmd.Start() != nil {
		return nil
	}
	feed(stdin, consumed(st.Events, c.Receipt.Consumes))
	if cmd.Wait() != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(buf.String(), "\r", ""), "\n")
	if len(lines) > completerMaxLines {
		lines = lines[:completerMaxLines]
	}
	return lines
}

func emitLines(out io.Writer, lines []string) {
	for _, l := range lines {
		if l != "" {
			fmt.Fprintln(out, l)
		}
	}
}

// ─────────────────────────────── the shims ──────────────────────────────────
//
// The shims are deliberately dumb and stable: every candidate comes from
// `self __complete`, which replays the instance's own log, so a capability —
// or a completer — grown after the shim was installed appears without
// reinstalling anything.

func completionScript(shell string) (string, error) {
	switch shell {
	case "zsh":
		return zshShim, nil
	case "bash":
		return bashShim, nil
	case "fish":
		return fishShim, nil
	default:
		return "", fmt.Errorf("no completion for %q — shells: zsh bash fish", shell)
	}
}

const zshShim = `#compdef self
# zsh completion for self.
#
# For the current shell (compinit must already have run):
#   source <(self completion zsh)
# Or install as an autoloaded function:
#   self completion zsh > "${fpath[1]}/_self"

_self() {
  local -a lines specs
  local line cand desc
  lines=(${(f)"$(command self __complete "${(@)words[2,CURRENT]}" 2>/dev/null)"})
  if (( ${#lines[@]} == 0 )); then
    _default
    return
  fi
  for line in "${lines[@]}"; do
    cand=${line%%$'\t'*}
    desc=${line#*$'\t'}
    if [[ "$desc" == "$line" ]]; then
      specs+=("${cand//:/\\:}")
    else
      specs+=("${cand//:/\\:}:${desc}")
    fi
  done
  _describe 'self' specs
}

if [[ "${zsh_eval_context[-1]}" == "loadautofunc" ]]; then
  _self "$@"
else
  compdef _self self
fi
`

const bashShim = `# bash completion for self.
#
# For the current shell:
#   source <(self completion bash)
# Or install:
#   self completion bash > /usr/local/etc/bash_completion.d/self

_self_complete() {
  local cur=${COMP_WORDS[COMP_CWORD]}
  # Copy the slice before narrowing IFS: bash 3.2 (macOS) joins a quoted
  # array slice into one word when IFS holds no space.
  local -a words=("${COMP_WORDS[@]:1:COMP_CWORD}")
  local IFS=$'\n'
  COMPREPLY=($(compgen -W "$(self __complete "${words[@]}" 2>/dev/null | cut -f1)" -- "$cur"))
}
complete -o default -F _self_complete self
`

const fishShim = `# fish completion for self.
#
# For the current shell:
#   self completion fish | source
# Or install:
#   self completion fish > ~/.config/fish/completions/self.fish

function __self_complete
    set -l words (commandline -opc) (commandline -ct)
    self __complete $words[2..-1] 2>/dev/null
end
complete -c self -f -a '(__self_complete)'
`
