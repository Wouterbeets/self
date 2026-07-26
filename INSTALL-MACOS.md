# Installing self on a Mac, with Codex as the mind

A start-to-finish setup for macOS, using the OpenAI **Codex CLI** as the
instance's mind — the coding agent a ChatGPT Plus subscription already pays
for. Works the same on an Apple-silicon Mac Pro and an Intel one; where the
two differ, the step says so.

You will end up with three things:

- `self` on your PATH — one Go binary, no services, no daemons;
- an **instance** at `~/.self`: a directory holding `events.jsonl` (the log)
  and `.secret` (a signing key that never leaves the machine);
- **Codex** plugged in as the mind, so the instance can grow capabilities you
  ask for in plain language.

Budget about twenty minutes, most of it waiting on downloads.

## 0. Prerequisites

macOS 12 or newer, and a terminal. macOS ships zsh, so the shell-rc file in
this guide is `~/.zshrc`; if you have switched to bash, use `~/.bash_profile`
throughout.

```sh
xcode-select --install                 # git and the toolchain (skip if installed)
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

Homebrew puts itself in `/opt/homebrew` on Apple silicon and `/usr/local` on
Intel; its installer prints the one line to add to `~/.zshrc` if your PATH
needs it. Follow that before continuing, then:

```sh
brew install go
```

Go 1.24.1 or newer is required (`go version`). The Codex adapter in step 3 is
a Python 3 script using nothing outside the standard library; the `python3`
that comes with the command line tools above is enough (`python3 -V`).

## 1. Get self and see it work with no model at all

```sh
mkdir -p ~/src && cd ~/src
git clone https://github.com/wouterbeets/self && cd self
./demo.sh
```

`demo.sh` runs the entire loop offline — no API key, no network, no model —
using `examples/mind-stub` as a deterministic stand-in mind. In about ten
seconds you watch a lesson become declarations, declarations compile into
scripts, a command append an event, a projection render it, and the whole
instance rebuild from `events.jsonl` + `.secret` byte for byte. If that
prints `OK`, the machinery is sound on this machine and everything after
this is configuration.

Now install the binary:

```sh
go install .                           # builds ~/go/bin/self
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
command -v self && self -h | head -1
```

Keep the clone. It is where the `lessons/` live, and `self learn` reads them
from disk.

<details>
<summary>No Go? Use a prebuilt binary instead.</summary>

```sh
mkdir -p ~/bin && cd ~/bin
arch=arm64; [ "$(uname -m)" = "x86_64" ] && arch=amd64      # Apple silicon vs Intel
curl -fL -o self "https://github.com/Wouterbeets/self/releases/latest/download/self-darwin-$arch"
chmod +x self
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
```

You still want the clone from above for `lessons/` and
`examples/mind-codex`. If macOS refuses to run the binary ("cannot be opened
because the developer cannot be verified"), clear the quarantine flag the
download put on it: `xattr -dr com.apple.quarantine ~/bin/self`.
</details>

## 2. Install Codex and sign in with your ChatGPT subscription

```sh
brew install --cask codex              # or: npm install -g @openai/codex
codex login                            # opens the browser: "Sign in with ChatGPT"
codex login status
codex doctor                           # auth, reachability, install — all green
```

Sign in with the ChatGPT account that carries the Plus subscription. No API
key, no billing setup: Codex usage is included, within Plus rate limits. A
single `self learn` runs several compiles back to back, so it is a real —
though not extravagant — slice of your weekly allowance.

## 3. Plug Codex in as the mind

The kernel holds no model. Every ask for intelligence — `think`, `reflect`,
`learn`, and each compile — is handed to whatever executable `SELF_MIND`
names, which explores the instance with its own tools and answers on stdout.

`codex exec` is non-interactive and already tool-capable, but it prints a
human session log on stdout — banner, reasoning, tool calls, token count —
and the kernel reads stdout as the answer. So point `SELF_MIND` at the
adapter in this repo, which asks Codex for its final message alone and sends
the session log to stderr where you can watch it:

```sh
cat >> ~/.zshrc <<'EOF'

# self — one shared instance, Codex as its mind
export SELF_HOME="$HOME/.self"
export SELF_MIND="$HOME/src/self/examples/mind-codex"
export SELF_MIND_ID="codex"
EOF
source ~/.zshrc
```

`SELF_HOME` pins one instance no matter which directory you run `self` from;
without it, the current working directory is the instance. `SELF_MIND_ID` is
the author string signed into every receipt — it is how the log remembers
which mind wrote which script.

Check the seam before asking it for anything real:

```sh
mkdir -p "$SELF_HOME"
self think "In one sentence: what is in this instance right now?"
```

Codex's session log scrolls past on stderr, then `self` prints a JSON object
whose `response` is the mind's answer. That is the whole contract working:
brief in, prose out, nothing appended.

The adapter takes four optional knobs, all with sane defaults:

| variable | meaning |
|---|---|
| `SELF_CODEX` | the codex executable (default `codex`) |
| `SELF_CODEX_MODEL` | model passed to `--model` (default: Codex's own) |
| `SELF_CODEX_SANDBOX` | `read-only` \| `workspace-write` \| `danger-full-access` (default `workspace-write`) |
| `SELF_CODEX_ARGS` | extra arguments, split like a shell line |

`workspace-write` is the default for a reason: the mind must be able to read
`$SELF_HOME` and *run* a candidate script before handing it back. Dropping to
`read-only` gives you a mind that can look but never test what it writes.

## 4. Grow the first real capability

```sh
self learn ~/src/self/lessons/chat     # a conversational front door
self                                   # rebuild from the log, then serve
```

Open <http://127.0.0.1:7777>. Codex reads the lesson's `intent.md`, declares
the commands and projections it describes, and the kernel compiles each one
through Codex and installs it under a locally-signed receipt. Expect a few
minutes and a lot of stderr; every capability that lands leaves a
`script.compiled` receipt in the log naming `codex` as its author.

From there, ask the chat page for something the instance cannot do yet and
watch it declare, compile, and install the capability mid-conversation.

Other lessons worth learning next, in the same way:

- `lessons/memory` — durable facts a stateless mind can orient from
- `lessons/journal` — the smallest example: one command, one projection
- `lessons/timers` — scheduled intentions, with the clock kept outside

## 5. Everyday use

```sh
self                       rebuild from the log, then serve at :7777
self run <cmd> [args…]     run a command: append its events, re-render
self show <projection>     render a view to stdout
self think "…"             ask the mind; report-only, appends nothing
self learn <account-dir>   learn capabilities from an intent
self revise command/<name> "<change>"     recompile one capability
self rehydrate             rebuild everything from events.jsonl + .secret
```

Everything derived — `capabilities/`, `site/` — can be deleted and rebuilt.
The two files that matter are `~/.self/events.jsonl` and `~/.self/.secret`;
back those up and the instance is portable to any machine, offline, with no
model involved.

## 6. Before you trust it

Codex writes the scripts; the kernel signs and runs them without a sandbox.
That is a real trust decision, not a formality:

```sh
ls ~/.self/capabilities/commands/                    # what exists
cat ~/.self/capabilities/commands/<name>/run         # what it actually does
```

They are short, plain-text scripts — read the ones you install, especially
before learning an account someone else gave you. The full statement of what
this design does and does not protect against is in the README under
[Limits and threat model](README.md#limits-and-threat-model). The server has
no authentication and binds loopback for that reason; leave `SELF_BIND`
alone unless you know why you are changing it.

## Troubleshooting

**`no mind is plugged in`** — `SELF_MIND` is unset in this shell. `echo
$SELF_MIND`; if it is empty, `source ~/.zshrc` (or you added the exports to
the wrong rc file for your shell).

**`mind mind-codex exited: exit status 1`** — Codex itself failed, and its
error is in the stderr just above. Usually authentication: run `codex login
status`, then `codex doctor`.

**Rate-limit errors partway through a `learn`** — Plus limits are per
window; what compiled before the failure is already installed and signed.
Wait, then `self learn` the same lesson again: the already-declared
capabilities recompile, and nothing in the log is lost.

**The mind answers but writes nothing, or reports permission denied** —
Codex was sandboxed to read-only. Confirm `SELF_CODEX_SANDBOX` is unset or
`workspace-write`.

**`self: command not found`** — `~/go/bin` (or `~/bin`) is not on your PATH
in this shell. Re-run the `echo 'export PATH=…' >> ~/.zshrc && source
~/.zshrc` line from step 1.

**Port 7777 already in use** — something else has it: `lsof -i :7777`. Move
the instance instead with `SELF_BIND=127.0.0.1:7788 self`.

**A compile hangs for minutes** — that is usually Codex working, not a
freeze; its session log on stderr shows the tool calls as they happen.
Ctrl-C is safe: the log only ever gains events that already completed.
