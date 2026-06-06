package replay

import (
	"capcompute/internal/runtime/extism2/dispatcher"
	"context"
	"fmt"
)

// CompletionChecker validates per-play dispatcher state after the guest returns.
type CompletionChecker interface {
	CheckCompleted() error
}

// Tape owns replay cursor state and decides how newly observed outcomes are stored.
type Tape interface {
	Next(call dispatcher.Call) (dispatcher.Outcome, bool, error)
	Record(call dispatcher.Call, outcome dispatcher.Outcome) error
	Reset()
	Remaining() int
}

// Dispatcher serves recorded outcomes before delegating new calls.
type Dispatcher[K any] struct {
	tape Tape
	next dispatcher.Dispatcher[K]
}

func (d *Dispatcher[K]) Dispatch(ctx context.Context, key K, call dispatcher.Call) (dispatcher.Outcome, error) {
	outcome, replayed, err := d.tape.Next(call)
	if err != nil || replayed {
		return outcome, err
	}
	outcome, err = d.next.Dispatch(ctx, key, call)
	if err != nil {
		return dispatcher.Outcome{}, err
	}
	if outcome.Kind() == dispatcher.OutcomeYield {
		d.tape.Reset()
		return outcome, nil
	}
	if err := d.tape.Record(call, outcome); err != nil {
		return dispatcher.Outcome{}, err
	}
	return outcome, nil
}

func (d *Dispatcher[K]) Remaining() int {
	return d.tape.Remaining()
}

func (d *Dispatcher[K]) CheckCompleted() error {
	if remaining := d.Remaining(); remaining > 0 {
		return IncompleteError{Remaining: remaining}
	}
	return nil
}

// DivergedError means the guest requested a different call than history recorded.
type DivergedError struct {
	Index int
	Want  dispatcher.Call
	Got   dispatcher.Call
}

func (e DivergedError) Error() string {
	return fmt.Sprintf("replay diverged at call %d: want %q got %q", e.Index, e.Want.Name, e.Got.Name)
}

// IncompleteError means the guest completed before replaying all recorded calls.
type IncompleteError struct {
	Remaining int
}

func (e IncompleteError) Error() string {
	return fmt.Sprintf("replay incomplete: %d recorded calls were not consumed", e.Remaining)
}
