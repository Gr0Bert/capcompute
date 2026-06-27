// Package dispatcher defines the vocabulary a guest host call speaks: a Call
// from the guest, an Outcome (result, yield, or failure) back, and the
// Dispatcher interface that turns one into the other. Authorization carries
// the forward-propagating approval context for replayed external tasks.
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

// Decision is the outcome of an external (human-in-the-loop) task approval.
type Decision string

const (
	Approved  Decision = "approved"
	Completed Decision = "completed"
	Failed    Decision = "failed"
	Denied    Decision = "denied"
	Cancelled Decision = "cancelled"
)

// Authorization is the forward-propagating security context for a replayed
// external task. When the runtime replays an approved task it populates this
// value and passes it to every Dispatch call; on a fresh call it is zero.
type Authorization struct {
	Decision Decision        `json:"decision,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Actor    string          `json:"actor,omitempty"`
	Reason   string          `json:"reason,omitempty"`
}

// Dispatcher owns policy and handler dispatch for guest calls.
type Dispatcher[K any] interface {
	Dispatch(ctx context.Context, guestData K, call Call, auth Authorization) (Outcome, error)
	Capabilities() []Capability
}
