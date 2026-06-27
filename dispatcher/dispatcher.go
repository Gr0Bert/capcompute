// Package dispatcher defines the vocabulary a guest host call speaks: a Call
// from the guest, an Outcome (result, yield, or failure) back, and the
// Dispatcher interface that turns one into the other. Capabilities are part of
// the Dispatcher contract; concrete dispatchers own their capability metadata.
// This package owns no capability behavior, persistence, or replay policy —
// those live in concrete dispatchers and the replay decorators above it.
package dispatcher

import (
	"context"
	"encoding/json"
)

type Capability struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Dispatcher owns policy and handler dispatch for new guest calls.
type Dispatcher[K any] interface {
	Dispatch(ctx context.Context, guestData K, call Call) (Outcome, error)
	Capabilities() []Capability
}
