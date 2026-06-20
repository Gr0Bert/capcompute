package dispatcher

import (
	"context"
	"encoding/json"
)

// Dispatcher owns policy and handler dispatch for new guest calls.
type Dispatcher[K any] interface {
	Dispatch(ctx context.Context, guestData K, call Call) (Outcome, error)
}

// Capability describes one guest-callable operation exposed by a dispatcher.
type Capability struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// CapabilityProvider optionally exposes the operations accepted by a dispatcher.
type CapabilityProvider interface {
	Capabilities() []Capability
}

// Capabilities returns a defensive copy of capabilities exposed by dispatcher.
func Capabilities[K any](value Dispatcher[K]) []Capability {
	provider, ok := value.(CapabilityProvider)
	if !ok {
		return nil
	}
	return cloneCapabilities(provider.Capabilities())
}

// DispatcherFactory creates the dispatcher chain for one play attempt.
type DispatcherFactory[K any] interface {
	NewDispatcher(ctx context.Context, key K) (Dispatcher[K], error)
}
