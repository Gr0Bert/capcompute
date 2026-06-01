package extism2

import (
	"bytes"
	"fmt"
)

// Record is one replay tape entry loaded from the journal.
type Record struct {
	Call    Call
	Outcome Outcome
}

// Tape is a per-play replay cursor over immutable recorded calls.
type Tape struct {
	records []Record
	cursor  int
}

// NewTape creates a replay tape whose cursor starts at the beginning.
func NewTape(records []Record) *Tape {
	copied := make([]Record, len(records))
	for i, record := range records {
		copied[i] = copyRecord(record)
	}
	return &Tape{records: copied}
}

// Next returns a recorded outcome for call, or ok=false when call is new.
func (t *Tape) Next(call Call) (Outcome, bool, error) {
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

// Remaining returns the number of recorded calls not consumed by this play.
func (t *Tape) Remaining() int {
	if t == nil {
		return 0
	}
	return len(t.records) - t.cursor
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
