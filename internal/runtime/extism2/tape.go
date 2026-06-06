package extism2

import (
	"bytes"
	"context"
	"fmt"
)

// Record is one completed host call outcome that can be replayed later.
type Record struct {
	Call    Call
	Outcome Outcome
}

// Tape owns replay cursor state and decides how newly observed outcomes are stored.
type Tape interface {
	Next(call Call) (Outcome, bool, error)
	Record(ctx context.Context, call Call, outcome Outcome) error
	Remaining() int
}

// MemoryTape replays an immutable in-memory record snapshot.
type MemoryTape struct {
	records []Record
	cursor  int
}

// NewTape creates an in-memory replay tape whose cursor starts at the beginning.
func NewTape(records []Record) *MemoryTape {
	copied := make([]Record, len(records))
	for i, record := range records {
		copied[i] = copyRecord(record)
	}
	return &MemoryTape{records: copied}
}

// Next returns a recorded outcome for call, or ok=false when call is new.
func (t *MemoryTape) Next(call Call) (Outcome, bool, error) {
	if t == nil || t.cursor >= len(t.records) {
		return Outcome{}, false, nil
	}

	record := t.records[t.cursor]
	if !sameCall(record.Call, call) {
		return Outcome{}, false, ReplayDivergedError{
			Index: t.cursor,
			Want:  record.Call,
			Got:   call,
		}
	}
	t.cursor++
	if !validOutcome(record.Outcome) {
		return Outcome{}, false, fmt.Errorf("invalid recorded outcome %q", record.Outcome.Kind())
	}
	return record.Outcome, true, nil
}

// Record ignores new outcomes because MemoryTape is not durable.
func (t *MemoryTape) Record(_ context.Context, call Call, outcome Outcome) error {
	return nil
}

// Remaining returns the number of replay records not consumed by this play.
func (t *MemoryTape) Remaining() int {
	if t == nil {
		return 0
	}
	return len(t.records) - t.cursor
}

// Journal stores durable records for a tape.
type Journal[K any] interface {
	Load(ctx context.Context, key K) ([]Record, error)
	Record(ctx context.Context, key K, call Call, outcome Outcome) error
}

// JournalFunc adapts a function into a Journal.
type JournalFunc[K any] struct {
	LoadFunc   func(context.Context, K) ([]Record, error)
	RecordFunc func(context.Context, K, Call, Outcome) error
}

func (f JournalFunc[K]) Load(ctx context.Context, key K) ([]Record, error) {
	if f.LoadFunc == nil {
		return nil, nil
	}
	return f.LoadFunc(ctx, key)
}

func (f JournalFunc[K]) Record(ctx context.Context, key K, call Call, outcome Outcome) error {
	if f.RecordFunc == nil {
		return nil
	}
	return f.RecordFunc(ctx, key, call, outcome)
}

// JournalTape is a replay tape backed by a durable journal.
type JournalTape[K any] struct {
	replay  *MemoryTape
	journal Journal[K]
	key     K
}

func NewJournalTape[K any](ctx context.Context, key K, journal Journal[K]) (*JournalTape[K], error) {
	var records []Record
	var err error
	if journal != nil {
		records, err = journal.Load(ctx, key)
		if err != nil {
			return nil, err
		}
	}
	return &JournalTape[K]{
		replay:  NewTape(records),
		journal: journal,
		key:     key,
	}, nil
}

func (t *JournalTape[K]) Next(call Call) (Outcome, bool, error) {
	if t == nil {
		return Outcome{}, false, nil
	}
	return t.replay.Next(call)
}

func (t *JournalTape[K]) Record(ctx context.Context, call Call, outcome Outcome) error {
	if t == nil || t.journal == nil || !replayableOutcome(outcome) {
		return nil
	}
	return t.journal.Record(ctx, t.key, call, outcome)
}

func (t *JournalTape[K]) Remaining() int {
	if t == nil {
		return 0
	}
	return t.replay.Remaining()
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

func sameCall(left Call, right Call) bool {
	return left.Name == right.Name && bytes.Equal(left.Args, right.Args)
}

func replayableOutcome(outcome Outcome) bool {
	return outcome.Kind() == OutcomeResult
}
