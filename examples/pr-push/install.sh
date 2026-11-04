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
declare command check.no-comments "Judge the comment lines a branch adds to Go, PHP and TS/JS: directives and annotations pass" \
  "$here/check-no-comments/command.py" '["practice.saved"]'
declare command check.migrations-ordered "A branch's new Atlas migration must sort after master's newest and ship its atlas.sum" \
  "$here/check-migrations-ordered/command.py" '["practice.saved"]'
declare command check.workflow-scope "Stop a push touching .github/workflows/ when the GitHub token lacks the workflow scope" \
  "$here/check-workflow-scope/command.py" '["practice.saved"]'
declare command check.pr-title "The PR title must be a Conventional Commit with a lowercase subject" \
  "$here/check-pr-title/command.py" '["practice.saved"]'
declare command practice.backtest "Replay the practice checks on merged history or anyone's open PR: calibrate each check, list gated escapes" \
  "$here/practice-backtest/command.py" '["practice.saved","practice.retired","pr.opened","pr.updated","pr.checked"]'
declare command feedback "Carry reviewers' comments on a goal's PR to the agent that owns it; recurring ones become practices" \
  "$here/feedback/command.py" '["ship.tracked","agent.started","goal.worked","goal.updated","goal.removed","feedback.received","feedback.delivered"]'
declare command preprod "Test a merged change on preprod as Wouter: log in, screenshot desktop+mobile, record a verdict on the goal and PR" \
  "$here/preprod/command.py" '["ship.tracked"]'
declare view ship "Ship pipeline per goal: stage, PR, announce, preprod verification; actions prints the driver's TSV to-do list" \
  "$here/ship/view.py" '["*"]'
declare command pr.push "The one way to push: check the practices at push, push, open or update the PR, bind it to the goal" \
  "$here/pr-push/command.py" '["goal.worked"]'

self run practice set go/errors \
  --title "Wrap every returned error with what this code was doing" \
  --rule 'Return fmt.Errorf("<operation>: %w", err), never a bare `return err`: the chain must say where it went wrong. Lowercase, name the operation ("load order 42: %w"), never "failed to". Always %w for an err argument, never %v. A caller that branches on it gets a package sentinel (ErrX) wrapped with %w. Passing through unchanged is the rare exception: mark it //nowrap: <reason>.' \
  --when '**/*.go,!**/*_test.go' --at work,push --check check.go-errors >/dev/null

self run practice set code/no-comments \
  --title "Agents add no code comments" \
  --rule 'Add no comments to code: names and structure carry the meaning, the PR body carries the why. Wouter permits a specific comment explicitly; reviewers ask to drop the rest. Directives (//go:, //nolint, @ts-expect-error, eslint-disable), PHP annotations (@var, @param…) and generated files are fine; comments already in the file stay.' \
  --when '**/*.go,**/*.php,**/*.ts,**/*.tsx,**/*.js,**/*.jsx,**/*.mjs,**/*.cjs,!**/node_modules/**,!**/vendor/**' \
  --at work,push --check check.no-comments --remote MYM-Tech/ >/dev/null

self run practice set migrations/ordered \
  --title "A new Atlas migration sorts after master's newest and ships its atlas.sum" \
  --rule 'Atlas applies a directory linearly: a file dated before one already on master fails the preprod deploy at "db migration" with "added out of order". If master gained a newer migration while your PR was open, rename yours past it and rehash atlas.sum (atlas migrate hash) before pushing.' \
  --when '**/migrations/**/*.sql' --at work,push --check check.migrations-ordered --remote MYM-Tech/ >/dev/null

self run practice set github/workflow-scope \
  --title "Workflow files need a token with the workflow scope" \
  --rule 'GitHub refuses a push touching .github/workflows/* unless the token has the workflow scope, and the work token has none. Have Wouter run gh auth refresh -h github.com -s workflow, or split the workflow change into its own PR for devops (.github is DevOps-owned: .github/AGENTS.md).' \
  --when '.github/workflows/**' --at push --check check.workflow-scope >/dev/null

self run practice set pr/title \
  --title "PR title is a Conventional Commit with a lowercase subject" \
  --rule 'The squash merge makes the PR title the commit subject, gated by pr-title.yml: <type>(<project>): <subject>, type one of feat|fix|docs|chore|refactor|test|ci|build|perf|revert|security, subject starting lowercase. Pass it as self run pr.push --title "…".' \
  --when '**' --at work,pr --check check.pr-title --remote MYM-Tech/monorepo >/dev/null

self run practice set preprod/frontend-screenshots \
  --title "A frontend change is verified on preprod with desktop and mobile screenshots" \
  --rule 'Shoot every screen the PR touches with self run preprod shot <path> <name> (desktop 1440 + mobile 390), logged in as the role that sees it. Read every PNG before the verdict: screenshots have caught bugs assertions missed (#2353 lowercased greeting, #2358 clipped tooltip). Attach them with --shot so the verdict comment shows them on the PR.' \
  --when 'frontends/**,monolith/core/templates/**,monolith/core/assets/**,!**/*.test.*,!**/*.spec.*' \
  --at verify --remote MYM-Tech/monorepo >/dev/null
