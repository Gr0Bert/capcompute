package extism2

import "context"

// Decision is the policy result for a call.
type Decision struct {
	Allowed bool
	Reason  string
}

// Policy decides whether a new call may reach handlers.
type Policy[K comparable] interface {
	Decide(ctx context.Context, key K, call Call) Decision
}

// PolicyFunc adapts a function into a Policy.
type PolicyFunc[K comparable] func(context.Context, K, Call) Decision

func (f PolicyFunc[K]) Decide(ctx context.Context, key K, call Call) Decision {
	return f(ctx, key, call)
}

// Handlers execute named host functions requested by a guest.
type Handlers[K comparable] interface {
	Execute(ctx context.Context, key K, call Call) (Outcome, error)
}

// HandlerFunc adapts a function into Handlers.
type HandlerFunc[K comparable] func(context.Context, K, Call) (Outcome, error)

func (f HandlerFunc[K]) Execute(ctx context.Context, key K, call Call) (Outcome, error) {
	return f(ctx, key, call)
}

// Dispatcher owns policy and handler dispatch for new guest calls.
type Dispatcher[K comparable] interface {
	Dispatch(ctx context.Context, key K, call Call) (Outcome, error)
}

// DefaultDispatcher authorizes and executes new calls.
type DefaultDispatcher[K comparable] struct {
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
