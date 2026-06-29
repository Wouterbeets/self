package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"self/internal/event"
	"self/internal/kernel"
	"self/internal/store"
)

// The onboarding surface — setup, configure, the human-in-the-loop interview, and
// the welcome page — lives as a real seed (seeds/onboarding/events.jsonl, embedded
// below), NOT as scripts baked into the kernel. It must work before any LLM is
// wired (you pick the brain here), so init installs the seed's reference
// implementations verbatim under a signed receipt rather than compiling them; see
// installOnboarding. Everything else stays in the paradigm: plain inspectable
// files, projections + commands over events, reachable at /setup the moment self
// starts. The same file is an ordinary seed a real brain could `self grow`.

// brainKeyFile is where the API token lives — beside the log like .secret /
// .identity, never inside it. 0600, gitignore-worthy, never replayed.
func brainKeyFile(home string) string { return filepath.Join(home, ".brain-key") }

//go:embed seeds/onboarding/events.jsonl
var onboardingSeed []byte

// installOnboarding plants the onboarding seed into a fresh home — the same thing
// `self grow` would do, except init can't call the compiler (these pages are how
// you wire one in). So for each declaration the kernel installs the seed's
// reference implementation VERBATIM under a signed receipt — the bootstrap
// carve-out, identical in trust to shipping the bytes in Go, only now they live
// in a plain seed file. The declaration it logs carries just the spec
// (implementation stripped), so the log stays lean and the wiring (kernel.html,
// projector auto-run) is byte-for-byte what a grown capability would produce.
// Content events (the initial brain.configured {provider:"none"}) replay as-is.
// Called once from cmdInit, after the secret is minted.
func installOnboarding(home string) error {
	st := store.Open(home)
	for _, line := range strings.Split(string(onboardingSeed), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e event.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return fmt.Errorf("onboarding seed: %w", err)
		}

		if e.Name != event.CommandDeclared && e.Name != event.ProjectorDeclared {
			// Content (e.g. the initial brain.configured) — replay it verbatim.
			ce := event.New(e.Name, e.Payload)
			if err := st.Append(&ce); err != nil {
				return err
			}
			continue
		}

		// Split the declaration: the implementation is the kernel-authored script we
		// install; the rest is the spec we log, exactly like a grown capability's.
		var p map[string]any
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("onboarding declaration: %w", err)
		}
		impl, _ := p["implementation"].(string)
		name, _ := p["name"].(string)
		if impl == "" || name == "" {
			return fmt.Errorf("onboarding declaration %q missing implementation", name)
		}
		delete(p, "implementation")
		spec, _ := json.Marshal(p)
		decl := event.New(e.Name, spec)
		if err := st.Append(&decl); err != nil {
			return err
		}

		kind := "command"
		if e.Name == event.ProjectorDeclared {
			kind = "projector"
		}
		if err := kernel.InstallBuiltin(home, kind, name, impl); err != nil {
			return err
		}
	}
	return nil
}

// event_BrainConfigured is the data-only brain choice event. It deliberately
// carries no secret — only which provider, where, and whether a key is set.
const event_BrainConfigured = "brain.configured"

// The human-in-the-loop brain's two events. A brain.asked is a parked question
// (the prompt self could not answer itself, because the chosen brain is a human);
// a brain.answered marks it resolved. The interview projector renders the open
// ones; the answer command emits the resolution + any capability the human grows.
const (
	event_BrainAsked    = "brain.asked"
	event_BrainAnswered = "brain.answered"
)

// newAskID mints a short opaque id correlating a parked question with its answer.
func newAskID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

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
	case "llamacpp", "ollama", "openai", "custom":
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

// onboardingURLHint is printed after init so the user knows where to go.
func onboardingURLHint(home string) string {
	return fmt.Sprintf("brain not configured — run `self` and open /setup to wire in your LLM (home: %s)", home)
}
