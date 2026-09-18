package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

var completerTimeout = 2 * time.Second

const completerMaxLines = 512

var verbCandidates = []struct{ name, desc string }{
	{"prompt", "print a prompt for an event-producing mind"},
	{"hear", "ingest event JSONL or authored scripts from stdin"},
	{"brief", "show capabilities, pending work, and refusals"},
	{"run", "execute a command capability and append its events"},
	{"view", "replay a pure view; built-in log is always available"},
	{"loop", "run a mind until the log stops changing"},
	{"lease", "atomically manage a goal writer lease"},
	{"checkpoint", "manage durable one-shot action approval"},
	{"reserve", "coordinate repository writers across processes"},
	{"learn", "deposit an account and print its learning prompt"},
	{"give", "write an event or capability account"},
	{"rehydrate", "rebuild derived capability files from the log"},
	{"completion", "print a shell completion script (zsh|bash|fish)"},
	{"help", "print the complete protocol"},
}

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

	var st *state
	switch prev[0] {
	case "view", "run", "brief", "give":
		var err error
		st, err = loadState(home)
		if err != nil {
			return nil
		}
	}

	switch prev[0] {
	case "view", "run":
		typ := kindView
		if prev[0] == "run" {
			typ = kindCommand
		}
		if len(prev) == 1 {
			completeCapNames(st, typ, cur, out)
			return nil
		}
		emitLines(out, runCompleter(home, st, "complete."+prev[1], words))

	case "brief":
		if len(prev) != 1 {
			return nil
		}
		completeDeclNames(st, cur, out)
		for _, w := range st.openIntents() {
			if name := "intent/" + w.Name; strings.HasPrefix(name, cur) {
				fmt.Fprintf(out, "%s\t%s\n", name, w.declaration().summary())
			}
		}

	case "give":
		if len(prev) != 1 {
			return nil
		}
		for _, typ := range []string{kindCommand, kindView} {
			for _, c := range st.list(typ) {
				if sel := typ + "/" + c.Name; strings.HasPrefix(sel, cur) {
					fmt.Fprintf(out, "%s\t%s\n", sel, trunc(c.Decl.summary(), 72))
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
			return nil
		}
		for _, f := range []struct{ name, desc string }{
			{"--guarded", "use isolated planner and worker rails"},
			{"--goal", "goal ID for guarded ownership"},
			{"--repo", "repository for guarded work"},
			{"--branch", "owned goal branch"},
			{"--ask", "the ask; every pass sees it"},
			{"--max-passes", "at most N passes"},
			{"--settle", "quiet passes in a row before the loop stops"},
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

func completeCapNames(st *state, typ, cur string, out io.Writer) {
	for _, c := range st.list(typ) {
		if !strings.HasPrefix(c.Name, cur) {
			continue
		}
		desc := trunc(c.Decl.summary(), 72)
		if c.Receipt == nil {
			desc += " (pending — no script yet)"
		}
		fmt.Fprintf(out, "%s\t%s\n", c.Name, desc)
	}
	if typ == kindView && st.cap(kindView, "log") == nil && strings.HasPrefix("log", cur) {
		fmt.Fprintf(out, "log\tthe last %d events; --all for the whole log (built in)\n", builtinLogTail)
	}
	if typ == kindView && st.cap(kindView, "loop") == nil && strings.HasPrefix("loop", cur) {
		fmt.Fprintln(out, "loop\tguarded pass audit, leases, and checkpoints (built in)")
	}
}

func completeDeclNames(st *state, cur string, out io.Writer) {
	kinds := map[string][]string{}
	var order []string
	for _, typ := range []string{kindCommand, kindView} {
		for _, c := range st.list(typ) {
			if !strings.HasPrefix(c.Name, cur) {
				continue
			}
			if kinds[c.Name] == nil {
				order = append(order, c.Name)
			}
			kinds[c.Name] = append(kinds[c.Name], typ)
		}
	}
	for _, name := range order {
		where := strings.Join(kinds[name], " and ")
		fmt.Fprintf(out, "%s\t%s\n", name, trunc(where+" — "+st.cap(kinds[name][0], name).Decl.summary(), 72))
	}
}

func runCompleter(home string, st *state, name string, args []string) []string {
	c := st.cap(kindView, name)
	if c == nil || c.Receipt == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), completerTimeout)
	defer cancel()
	out, err := executeView(ctx, home, st, name, args, io.Discard, completerTimeout)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(out), "\r", ""), "\n")
	return lines[:min(len(lines), completerMaxLines)]
}

func emitLines(out io.Writer, lines []string) {
	for _, l := range lines {
		if l != "" {
			fmt.Fprintln(out, l)
		}
	}
}

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
