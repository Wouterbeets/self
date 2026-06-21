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
	TrioDeclared      = "trio.declared"
	SeedPlanted       = "seed.planted"
)
