package replay

import (
	"capcompute/internal/runtime/extism2"
	"capcompute/internal/runtime/extism2/dispatcher"
	"context"
	"fmt"
)

// CompletionChecker validates per-play dispatcher state after the guest returns.
type CompletionChecker interface {
	CheckCompleted() error
}

// NewReplayDispatcher creates one per-play replay decorator.
func NewReplayDispatcher[K any](tape Tape, next dispatcher.Dispatcher[K]) (*ReplayDispatcher[K], error) {
	if next == nil {
		return nil, extism2.ErrDispatcherRequired
	}
	if tape == nil {
		tape = extism2.NewTape(nil)
	}
	return &ReplayDispatcher[K]{
		tape: tape,
		next: next,
	}, nil
}

// Tape owns replay cursor state and decides how newly observed outcomes are stored.
type Tape interface {
	Next(call Call) (Outcome, bool, error)
	Record(call Call, outcome Outcome) error
	Remaining() int
}

// ReplayDispatcher serves recorded outcomes before delegating new calls.
type ReplayDispatcher[K any] struct {
	tape Tape
	next dispatcher.Dispatcher[K]
}

func (d *ReplayDispatcher[K]) Dispatch(ctx context.Context, key K, call extism2.Call) (extism2.Outcome, error) {
	outcome, replayed, err := d.tape.Next(call)
	if err != nil || replayed {
		return outcome, err
	}
	outcome, err = d.next.Dispatch(ctx, key, call)
	if err != nil {
		return extism2.Outcome{}, err
	}
	if err := d.tape.Record(ctx, call, outcome); err != nil {
		return extism2.Outcome{}, err
	}
	return outcome, nil
}

func (d *ReplayDispatcher[K]) Remaining() int {
	return d.tape.Remaining()
}

func (d *ReplayDispatcher[K]) CheckCompleted() error {
	if remaining := d.Remaining(); remaining > 0 {
		return ReplayIncompleteError{Remaining: remaining}
	}
	return nil
}

// ReplayDivergedError means the guest requested a different call than history recorded.
type ReplayDivergedError struct {
	Index int
	Want  Call
	Got   Call
}

func (e ReplayDivergedError) Error() string {
	return fmt.Sprintf("replay diverged at call %d: want %q got %q", e.Index, e.Want.Name, e.Got.Name)
}

// ReplayIncompleteError means the guest completed before replaying all recorded calls.
type ReplayIncompleteError struct {
	Remaining int
}

func (e ReplayIncompleteError) Error() string {
	return fmt.Sprintf("replay incomplete: %d recorded calls were not consumed", e.Remaining)
}

func copyRecord(record Record) Record {
	record.Call = copyCall(record.Call)
	record.Outcome = copyOutcome(record.Outcome)
	return record
}
