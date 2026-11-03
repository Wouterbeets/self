#!/bin/sh
# Declare and install practice, practices, check.go-errors and pr.push into $SELF_HOME,
# then seed the go/errors practice. Idempotent: unchanged declarations and practices append nothing new
# worth reading (declarations re-sign the same bytes).
set -eu
here=$(cd "$(dirname "$0")/.." && pwd)
: "${SELF_HOME:?set SELF_HOME}"
export SELF_CALLER="${SELF_CALLER:-installer}"

desc() { python3 -c 'import ast,sys; print(ast.get_docstring(ast.parse(open(sys.argv[1]).read())))' "$1"; }

declare() { # type name summary file consumes-json [extra-json]
  extra=${6:-}
  [ -n "$extra" ] || extra='{}'
  jq -nc --arg t "$1" --arg n "$2" --arg s "$3" --arg d "$(desc "$4")" --argjson c "$5" --argjson x "$extra" \
    '{name:($t+".declared"),payload:({name:$n,summary:$s,description:$d,consumes:$c}+$x)}' | self hear >/dev/null
  jq -nc --arg t "$1" --arg n "$2" --rawfile s "$4" '{name:"script.authored",payload:{type:$t,name:$n,script:$s}}' | self hear >/dev/null
}

declare command practice "Declare how work is done here: a rule, the files it applies to, where it is shown or enforced" \
  "$here/practice/command.py" '["practice.saved","practice.retired"]'
declare view practices "Live practices: id, insertion points, check, title; an id prints one in full" \
  "$here/practice/view.py" '["practice.saved","practice.retired"]' '{"brief":{"order":40,"bytes":800}}'
declare command check.go-errors "Judge the Go lines a branch adds against go/errors: bare returns, %v of err, 'failed to'" \
  "$here/check-go-errors/command.py" '["practice.saved"]'
declare command pr.push "The one way to push: check the practices at push, push, open or update the PR, bind it to the goal" \
  "$here/pr-push/command.py" '["goal.worked"]'

self run practice set go/errors \
  --title "Wrap every returned error with what this code was doing" \
  --rule 'Return fmt.Errorf("<operation>: %w", err), never a bare `return err`: the chain must say where it went wrong. Lowercase, name the operation ("load order 42: %w"), never "failed to". Always %w for an err argument, never %v. A caller that branches on it gets a package sentinel (ErrX) wrapped with %w. Passing through unchanged is the rare exception: mark it //nowrap: <reason>.' \
  --when '**/*.go,!**/*_test.go' --at work,push --check check.go-errors >/dev/null
