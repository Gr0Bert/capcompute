package extism2

import "context"

// Decision is the policy result for a call.
type Decision struct {
	Allowed bool
	Reason  string
}

// Policy decides whether a new call may reach handlers.
type Policy[K any] interface {
	Decide(ctx context.Context, key K, call Call) Decision
}

// PolicyFunc adapts a function into a Policy.
type PolicyFunc[K any] func(context.Context, K, Call) Decision

func (f PolicyFunc[K]) Decide(ctx context.Context, key K, call Call) Decision {
	return f(ctx, key, call)
}

// Handlers execute named host functions requested by a guest.
type Handlers[K any] interface {
	Execute(ctx context.Context, key K, call Call) (Outcome, error)
}

// HandlerFunc adapts a function into Handlers.
type HandlerFunc[K any] func(context.Context, K, Call) (Outcome, error)

func (f HandlerFunc[K]) Execute(ctx context.Context, key K, call Call) (Outcome, error) {
	return f(ctx, key, call)
}

// Dispatcher owns policy and handler dispatch for new guest calls.
type Dispatcher[K any] interface {
	Dispatch(ctx context.Context, key K, call Call) (Outcome, error)
}

// DispatcherFunc adapts a function into a Dispatcher.
type DispatcherFunc[K any] func(context.Context, K, Call) (Outcome, error)

func (f DispatcherFunc[K]) Dispatch(ctx context.Context, key K, call Call) (Outcome, error) {
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

func (d *DefaultDispatcher[K]) Dispatch(ctx context.Context, key K, call Call) (Outcome, error) {
	if d.Policy == nil {
		return Outcome{}, ErrPolicyRequired
	}
	decision := d.Policy.Decide(ctx, key, call)
	if !decision.Allowed {
		return Failed(decision.Reason), nil
	}

	if d.Handlers == nil {
		return Outcome{}, ErrHandlersRequired
	}
	outcome, err := d.Handlers.Execute(ctx, key, call)
	if err != nil {
		return Outcome{}, err
	}
	if !validOutcome(outcome) {
		return Outcome{}, ErrOutcomeRequired
	}
	return outcome, nil
}
