package dispatcher

import (
	"capcompute/internal/runtime/extism2"
	"context"
)

// Decision is the policy result for a call.
type Decision struct {
	Allowed bool
	Reason  string
}

// Policy decides whether a new call may reach handlers.
type Policy[K any] interface {
	Decide(ctx context.Context, key K, call extism2.Call) Decision
}

// PolicyFunc adapts a function into a Policy.
type PolicyFunc[K any] func(context.Context, K, extism2.Call) Decision

func (f PolicyFunc[K]) Decide(ctx context.Context, key K, call extism2.Call) Decision {
	return f(ctx, key, call)
}

// Handlers execute named host functions requested by a guest.
type Handlers[K any] interface {
	Execute(ctx context.Context, key K, call extism2.Call) (extism2.Outcome, error)
}

// HandlerFunc adapts a function into Handlers.
type HandlerFunc[K any] func(context.Context, K, extism2.Call) (extism2.Outcome, error)

func (f HandlerFunc[K]) Execute(ctx context.Context, key K, call extism2.Call) (extism2.Outcome, error) {
	return f(ctx, key, call)
}

// Dispatcher owns policy and handler dispatch for new guest calls.
type Dispatcher[K any] interface {
	Dispatch(ctx context.Context, key K, call extism2.Call) (extism2.Outcome, error)
}

// DispatcherFunc adapts a function into a Dispatcher.
type DispatcherFunc[K any] func(context.Context, K, extism2.Call) (extism2.Outcome, error)

func (f DispatcherFunc[K]) Dispatch(ctx context.Context, key K, call extism2.Call) (extism2.Outcome, error) {
	return f(ctx, key, call)
}

// DispatcherFactory creates the dispatcher chain for one play attempt.
type DispatcherFactory[K any] interface {
	NewDispatcher(ctx context.Context, key K) (Dispatcher[K], error)
}

// DispatcherFactoryFunc adapts a function into a DispatcherFactory.
type DispatcherFactoryFunc[K any] func(context.Context, K) (Dispatcher[K], error)

func (f DispatcherFactoryFunc[K]) NewDispatcher(ctx context.Context, key K) (Dispatcher[K], error) {
	return f(ctx, key)
}

// DefaultDispatcher authorizes and executes new calls.
type DefaultDispatcher[K any] struct {
	Policy   Policy[K]
	Handlers Handlers[K]
}

func (d *DefaultDispatcher[K]) Dispatch(ctx context.Context, key K, call extism2.Call) (extism2.Outcome, error) {
	if d.Policy == nil {
		return extism2.Outcome{}, extism2.ErrPolicyRequired
	}
	decision := d.Policy.Decide(ctx, key, call)
	if !decision.Allowed {
		return extism2.Failed(decision.Reason), nil
	}

	if d.Handlers == nil {
		return extism2.Outcome{}, extism2.ErrHandlersRequired
	}
	outcome, err := d.Handlers.Execute(ctx, key, call)
	if err != nil {
		return extism2.Outcome{}, err
	}
	if !extism2.validOutcome(outcome) {
		return extism2.Outcome{}, extism2.ErrOutcomeRequired
	}
	return outcome, nil
}
