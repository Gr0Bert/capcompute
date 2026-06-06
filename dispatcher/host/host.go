package host

import (
	"capcompute/dispatcher"
	"context"
	"errors"
)

var (
	ErrOutcomeRequired  = errors.New("error outcome required")
	ErrHandlersRequired = errors.New("error handlers required")
)

// Handlers execute named host functions requested by a guest.
type Handlers[K any] interface {
	Execute(ctx context.Context, guestData K, call dispatcher.Call) (dispatcher.Outcome, error)
}

type Dispatcher[K any] struct {
	Handlers Handlers[K]
}

func (d *Dispatcher[K]) Dispatch(ctx context.Context, key K, call dispatcher.Call) (dispatcher.Outcome, error) {
	if d.Handlers == nil {
		return dispatcher.Outcome{}, ErrHandlersRequired
	}
	outcome, err := d.Handlers.Execute(ctx, key, call)
	if err != nil {
		return dispatcher.Outcome{}, err
	}

	switch outcome.Kind() {
	case dispatcher.OutcomeResult, dispatcher.OutcomeYield, dispatcher.OutcomeFailed:
		return outcome, nil
	default:
		return dispatcher.Outcome{}, ErrOutcomeRequired
	}
}
