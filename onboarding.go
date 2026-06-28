package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"self/internal/event"
	"self/internal/kernel"
	"self/internal/store"
)

// The brain-setup surface — a baby-kernel onboarding seed installed by `self
// init`. It is NOT an LLM-compiled capability: configuring the brain has to work
// before any LLM is wired, so the kernel ships these two scripts and signs them
// itself at init (see kernel.InstallBuiltin). A `setup` projector renders an HTML
// page where the user picks a brain and pastes provider details + token; a
// `configure` command persists the choice — provider/url/model to the log (an
// ordinary, portable brain.configured event), the token to a non-log key file so
// it never leaks when a garden is shared.
//
// This keeps the whole "wire in your LLM" flow inside the paradigm: it's just a
// projection + a command over events, reachable at /setup the moment self starts.

// brainKeyFile is where the API token lives — beside the log like .secret /
// .identity, never inside it. 0600, gitignore-worthy, never replayed.
func brainKeyFile(home string) string { return filepath.Join(home, ".brain-key") }

// installOnboarding wires the brain-setup surface into a fresh home: it declares
// the configure command + setup projector (so kernel.html records the wiring and
// the projector auto-runs on brain.configured), installs their shipped scripts
// with signed receipts, and seeds an initial brain.configured so the page renders
// an "unconfigured" state. Called once from cmdInit, after the secret is minted.
func installOnboarding(home string) error {
	st := store.Open(home)

	cmdDecl, _ := json.Marshal(map[string]any{
		"name":        "configure",
		"description": "Configure self's brain: which LLM provider drives think/grow. Writes provider/url/model to the log and the token to a non-log key file.",
		"params": map[string]string{
			"provider": "string — human | llamacpp | ollama | openai | opencode | anthropic | custom",
			"base_url": "string — OpenAI-compatible base URL (e.g. http://localhost:11434/v1)",
			"model":    "string — model name",
			"key":      "string — API token (blank keeps the current one); stored outside the log",
		},
		"event": map[string]any{
			"name":   event_BrainConfigured,
			"fields": map[string]string{"provider": "string", "base_url": "string", "model": "string", "key_set": "bool"},
		},
	})
	projDecl, _ := json.Marshal(map[string]any{
		"name":        "setup",
		"description": "Brain configuration page: pick a provider and enter LLM details + token. Renders the latest brain.configured.",
		"consumes":    []string{event_BrainConfigured},
	})
	for _, d := range []struct {
		name    string
		payload json.RawMessage
	}{
		{event.CommandDeclared, cmdDecl},
		{event.ProjectorDeclared, projDecl},
	} {
		e := event.New(d.name, d.payload)
		if err := st.Append(&e); err != nil {
			return err
		}
	}

	if err := kernel.InstallBuiltin(home, "command", "configure", configureScript); err != nil {
		return err
	}
	if err := kernel.InstallBuiltin(home, "projector", "setup", setupScript); err != nil {
		return err
	}

	// Seed an initial unconfigured state so the page has something to render.
	initial, _ := json.Marshal(map[string]any{"provider": "none", "base_url": "", "model": "", "key_set": false})
	e := event.New(event_BrainConfigured, initial)
	return st.Append(&e)
}

// event_BrainConfigured is the data-only brain choice event. It deliberately
// carries no secret — only which provider, where, and whether a key is set.
const event_BrainConfigured = "brain.configured"

// loadBrainConfig reads the latest brain.configured event and, if the user has
// chosen an OpenAI-compatible provider, exports SELF_LLM_URL / MODEL / API_KEY
// for the brain process — UNLESS those env vars are already set, so an explicit
// env override always wins. The token is read from the key file, never the log.
// Called by `self brain` so the page-driven choice actually drives the LLM.
// Returns the resolved provider (for messaging), or "" if nothing is configured.
func loadBrainConfig(home string) string {
	events, err := store.Open(home).Read()
	if err != nil {
		return ""
	}
	var provider, baseURL, model string
	for _, e := range events {
		if e.Name != event_BrainConfigured {
			continue
		}
		var c struct {
			Provider string `json:"provider"`
			BaseURL  string `json:"base_url"`
			Model    string `json:"model"`
		}
		if json.Unmarshal(e.Payload, &c) == nil {
			provider, baseURL, model = c.Provider, c.BaseURL, c.Model
		}
	}
	if provider == "" || provider == "none" {
		return provider
	}
	setIfUnset := func(k, v string) {
		if v != "" && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	// human/opencode are resolved elsewhere (the bridge / opencode auth); for the
	// OpenAI-compatible providers we fill the SELF_LLM_* the compiler already reads.
	switch provider {
	case "llamacpp", "ollama", "openai", "custom", "anthropic":
		setIfUnset("SELF_LLM_URL", baseURL)
		setIfUnset("SELF_LLM_MODEL", model)
		if key, err := os.ReadFile(brainKeyFile(home)); err == nil {
			if k := strings.TrimSpace(string(key)); k != "" {
				setIfUnset("SELF_LLM_API_KEY", k)
			}
		}
	}
	return provider
}

// brainConfigured reports whether a real brain has been chosen yet (a
// brain.configured with a provider other than the initial "none"). Used by the
// serve root to land a fresh user on the setup page. Read-only — no env effects.
func brainConfigured(home string) bool {
	events, err := store.Open(home).Read()
	if err != nil {
		return false
	}
	provider := "none"
	for _, e := range events {
		if e.Name != event_BrainConfigured {
			continue
		}
		var c struct {
			Provider string `json:"provider"`
		}
		if json.Unmarshal(e.Payload, &c) == nil && c.Provider != "" {
			provider = c.Provider
		}
	}
	return provider != "none"
}

// configureScript: the `configure` command. Reads provider/base_url/model/key as
// positional argv (the order the setup form's inputs appear), writes the token to
// the key file (blank = keep current), and emits a brain.configured event that
// records everything EXCEPT the token (only key_set: bool).
const configureScript = `#!/usr/bin/env python3
import sys, json, os

provider = sys.argv[1] if len(sys.argv) > 1 else "none"
base_url = sys.argv[2] if len(sys.argv) > 2 else ""
model    = sys.argv[3] if len(sys.argv) > 3 else ""
key      = sys.argv[4] if len(sys.argv) > 4 else ""

home = os.environ.get("SELF_HOME", ".")
keyfile = os.path.join(home, ".brain-key")

# A token is a secret: it goes to a file beside the log, never into an event.
# Blank key = keep whatever is already there (so re-saving other fields is safe).
if key.strip():
    with open(keyfile, "w") as f:
        f.write(key.strip())
    os.chmod(keyfile, 0o600)

key_set = os.path.exists(keyfile) and os.path.getsize(keyfile) > 0

print(json.dumps({"name": "brain.configured", "payload": {
    "provider": provider, "base_url": base_url, "model": model, "key_set": key_set,
}}))
`

// setupScript: the `setup` projector. Renders the latest brain.configured as a
// configuration page — a provider picker + fields posting to /run/configure.
// Bare semantic HTML only (the kernel injects the shared stylesheet); no JS. The
// token is never in the events, so this page physically cannot render it.
const setupScript = `#!/usr/bin/env python3
import sys, json
from html import escape

cfg = {"provider": "none", "base_url": "", "model": "", "key_set": False}
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    e = json.loads(line)
    if e.get("name") == "brain.configured":
        cfg = e.get("payload", cfg)

provider = cfg.get("provider", "none")
base_url = cfg.get("base_url", "") or ""
model    = cfg.get("model", "") or ""
key_set  = cfg.get("key_set", False)

PRESETS = [
    ("human",    "Human-in-the-loop — you write the replies (run brain/bridge.py)"),
    ("llamacpp", "llama.cpp server — http://localhost:8080"),
    ("ollama",   "Ollama — http://localhost:11434"),
    ("openai",   "OpenAI — https://api.openai.com"),
    ("opencode", "opencode-go — uses your opencode auth"),
    ("anthropic","Anthropic — adapter pending (use a proxy for now)"),
    ("custom",   "Custom OpenAI-compatible endpoint"),
]
# base URLs WITHOUT the /v1 suffix — the kernel appends /v1/chat/completions.
DEFAULT_URL = {"llamacpp":"http://localhost:8080","ollama":"http://localhost:11434","openai":"https://api.openai.com"}

print("<!DOCTYPE html>")
print("<html><head><title>brain setup</title></head><body>")
print("<h1>configure your brain</h1>")
print("<p class=\"muted\">self's brain is the LLM that powers <code>think</code>, <code>chat</code> and growing new capabilities. Pick a provider and save. The token is stored outside the event log, so it is never shared when you share your garden.</p>")

if provider == "none":
    print("<p class=\"tag\">not configured yet</p>")
else:
    keymsg = "key set ✓" if key_set else "no key"
    print("<div class=\"card\"><h3>current</h3>")
    print("<p>provider <strong>%s</strong> &middot; %s</p>" % (escape(provider), keymsg))
    if base_url: print("<p class=\"muted\">%s &middot; %s</p>" % (escape(base_url), escape(model or "(no model)")))
    print("</div>")

print("<form method=\"post\" action=\"/run/configure\">")
print("<label>provider</label>")
print("<select name=\"provider\">")
for val, label in PRESETS:
    sel = " selected" if val == provider else ""
    print("  <option value=\"%s\"%s>%s</option>" % (val, sel, escape(label)))
print("</select>")
print("<label>base URL</label>")
print("<input name=\"base_url\" value=\"%s\" placeholder=\"http://localhost:11434 (no /v1 suffix)\">" % escape(base_url))
print("<label>model</label>")
print("<input name=\"model\" value=\"%s\" placeholder=\"e.g. llama3.2 / gpt-4o-mini\">" % escape(model))
print("<label>API token</label>")
print("<input name=\"key\" type=\"password\" placeholder=\"%s\">" % ("leave blank to keep current key" if key_set else "paste token (local providers need none)"))
print("<button>save brain</button>")
print("</form>")

print("<hr><h3>preset endpoints</h3><table><tr><th>provider</th><th>base URL</th></tr>")
for k, u in DEFAULT_URL.items():
    print("<tr><td>%s</td><td><code>%s</code></td></tr>" % (k, u))
print("</table>")
print("<p class=\"muted\">Most providers (llama.cpp, Ollama, OpenAI, vLLM) speak the OpenAI-compatible API and work as-is. Anthropic needs a dedicated adapter (pending) or an OpenAI-compatible proxy.</p>")
print("</body></html>")
`

// onboardingURLHint is printed after init so the user knows where to go.
func onboardingURLHint(home string) string {
	return fmt.Sprintf("brain not configured — run `self` and open /setup to wire in your LLM (home: %s)", home)
}
