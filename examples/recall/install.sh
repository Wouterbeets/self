#!/bin/sh
# Install recall into a self instance and wire it into Claude Code or opencode.
#
# usage: examples/recall/install.sh [settings.json]
#        examples/recall/install.sh --opencode [plugin-dir]
#        examples/recall/install.sh --pi [extension-dir]
#
# Declares and authors the recall view in $SELF_HOME. For Claude Code, merges
# hooks for every event claude-hook handles into the settings file (default
# ~/.claude/settings.json; use .claude/settings.json for one project); rerunning
# replaces the earlier recall hooks instead of adding more. For opencode, writes
# recall.js into the plugin directory (default ~/.config/opencode/plugins; use
# .opencode/plugins for one project). For pi, writes self-recall.ts into the
# extension directory (default ~/.pi/agent/extensions; .pi/extensions for one
# project). SELF_HOME, SELF_BIN and any SELF_RECALL_* set now are written into
# the hook command, the plugin or the extension.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
plugin= src= dest=
case "${1:-}" in
--opencode) plugin=${2:-$HOME/.config/opencode/plugins} src=opencode.js dest=recall.js ;;
--pi) plugin=${2:-$HOME/.pi/agent/extensions} src=pi.ts dest=self-recall.ts ;;
esac
settings=${1:-$HOME/.claude/settings.json}
bin=${SELF_BIN:-$(command -v self || true)}
[ -n "$bin" ] || { echo "install: no self on PATH; set SELF_BIN" >&2; exit 1; }
[ -n "${SELF_HOME:-}" ] || { echo "install: set SELF_HOME to the instance recall should use" >&2; exit 1; }
command -v jq >/dev/null || { echo "install: needs jq" >&2; exit 1; }
mkdir -p "$SELF_HOME"
s() { SELF_CALLER=${SELF_CALLER:-install} "$bin" "$@"; }

c='["chat.message","recall.node","recall.merged"]'
jq -nc --argjson c "$c" '{name:"view.declared",payload:{name:"recall",summary:"Every earlier conversation as one memory: recent lines detailed, old ones coarse; zoom with <first>+<count>",description:"usage: self view recall [<first>+<count>] — see examples/recall/view.py",consumes:$c}}' | s hear >/dev/null
jq -nc --rawfile v "$here/view.py" '{name:"script.authored",payload:{type:"view",name:"recall",script:$v}}' | s hear >/dev/null

if [ -n "$plugin" ]; then
	mkdir -p "$plugin"
	env=$(env | sed -n 's/^\(SELF_RECALL_[A-Z]*\)=.*/\1/p' | sort | jq -R . | jq -sc --arg h "$SELF_HOME" --arg b "$bin" \
		'reduce .[] as $k ({SELF_HOME: $h, SELF_BIN: $b}; .[$k] = env[$k])')
	tmp=$plugin/.$dest.$$
	lit() { printf %s "$1" | sed 's/[|&\\]/\\&/g'; }
	sed -e "s|^const HOOK = .*|const HOOK = $(lit "$(jq -nc --arg h "$here/claude-hook" '$h')")|" \
		-e "s|^const ENV = {}|const ENV = $(lit "$env")|" "$here/$src" >"$tmp"
	mv "$tmp" "$plugin/$dest"
	echo "recall: view installed in $SELF_HOME; written to $plugin/$dest"
	exit 0
fi

q() { printf "'%s'" "$(printf %s "$1" | sed "s/'/'\\\\''/g")"; }
cmd="SELF_HOME=$(q "$SELF_HOME") SELF_BIN=$(q "$bin")"
for v in $(env | sed -n 's/^\(SELF_RECALL_[A-Z]*\)=.*/\1/p' | sort); do
	cmd="$cmd $v=$(eval q "\"\$$v\"")"
done
cmd="$cmd $(q "$here/claude-hook") # self-recall"

mkdir -p "$(dirname "$settings")"
[ -s "$settings" ] || echo '{}' >"$settings"
tmp=$settings.recall.$$
jq --arg cmd "$cmd" '
	def ours: any(.hooks[]?; (.command // "") | endswith("# self-recall"));
	def entry($m): ({hooks: [{type: "command", command: $cmd, timeout: 30}]} + (if $m then {matcher: $m} else {} end));
	.hooks = (.hooks // {})
	| reduce (["SessionStart", null], ["UserPromptSubmit", null], ["PostToolUse", "*"],
	          ["PostToolUseFailure", "*"], ["SubagentStop", null], ["Stop", null]) as [$ev, $m]
	    (.; .hooks[$ev] = ([(.hooks[$ev] // [])[] | select(ours | not)] + [entry($m)]))
' "$settings" >"$tmp" && mv "$tmp" "$settings"
echo "recall: view installed in $SELF_HOME; hooks written to $settings"
