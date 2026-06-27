package event

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

type Event struct {
	ID         string          `json:"id"`
	Seq        int             `json:"seq"`
	Name       string          `json:"name"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}

func New(name string, payload json.RawMessage) Event {
	return Event{
		ID:         NewID(),
		Name:       name,
		OccurredAt: time.Now().UTC(),
		Payload:    payload,
	}
}

func NewID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

const (
	KernelInitialized = "kernel.initialized"
	CommandDeclared   = "command.declared"
	ProjectorDeclared = "projector.declared"
	ScriptCompiled    = "script.compiled"
	SeedPlanted       = "seed.planted"
	// RestoreRequested is a data-only intent ({name, seq}) that the kernel acts
	// on by reinstalling an earlier compiled receipt. It carries no code, so any
	// seed, command, or the CLI may emit it — but the install itself stays the
	// kernel's, reading only its own logged receipts. This is what lets `restore`
	// be an ordinary capability while the privileged install remains kernel-only.
	RestoreRequested = "restore.requested"
)
