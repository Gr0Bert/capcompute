package dispatcher

import (
	"context"
)

// Dispatcher owns policy and handler dispatch for new guest calls.
type Dispatcher[K any] interface {
	Dispatch(ctx context.Context, guestData K, call Call) (Outcome, error)
}

// DispatcherFactory creates the dispatcher chain for one play attempt.
type DispatcherFactory[K any] interface {
	NewDispatcher(ctx context.Context, key K) (Dispatcher[K], error)
}
