package journaled

import (
	"bytes"
	"context"
	"fmt"
)

type Tape struct {
	records Journal
	cursor  int
}

// Journal stores durable records for a tape.
type Journal interface {
	Load(idx int) (Record, error)
	Store(idx int, call Call, outcome Outcome) error
	Length() int
}

type Record struct {
	Call    Call
	Outcome Outcome
}

// NewTape creates an in-memory replay tape whose cursor starts at the beginning.
func NewTape(records Journal) *Tape {
	return &Tape{records, 0}
}

// Next returns a recorded outcome for call, or ok=false when call is new.
func (t *Tape) Next(call Call) (Outcome, bool, error) {
	if t == nil || t.cursor >= t.records.Length() {
		return Outcome{}, false, nil
	}

	record := t.records.Load(t.cursor)
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

// Record ignores new outcomes because Tape is not durable.
func (t *Tape) Record(_ context.Context, call Call, outcome Outcome) error {
	return nil
}

// Remaining returns the number of replay records not consumed by this play.
func (t *Tape) Remaining() int {
	if t == nil {
		return 0
	}
	return len(t.records) - t.cursor
}

func sameCall(left Call, right Call) bool {
	return left.Name == right.Name && bytes.Equal(left.Args, right.Args)
}

func replayableOutcome(outcome Outcome) bool {
	return outcome.Kind() == OutcomeResult
}
