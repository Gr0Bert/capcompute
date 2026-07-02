// Package journaled is a replay Tape backed by an append-only Journal: it serves
// recorded syscall/result pairs in order and appends new ones, raising
// ReplayDivergedError when a re-run guest issues a syscall that does not match
// the recorded sequence. It owns the on-tape record format and the divergence
// check; the Journal itself — where records durably live — is supplied by the
// caller.
package journaled

import (
	"bytes"
	"fmt"

	"github.com/aurora-capcompute/capcompute/sys"
)

type Tape struct {
	records Journal
	cursor  int
}

// Journal stores durable records for a tape.
type Journal interface {
	Load(idx int) (Record, error)
	Store(idx int, syscall sys.Syscall, result sys.SyscallResult) error
	Length() int
}

type Record struct {
	Syscall sys.Syscall
	Result  sys.SyscallResult
}

// ReplayDivergedError means the guest requested a different syscall than history recorded.
type ReplayDivergedError struct {
	Index int
	Want  sys.Syscall
	Got   sys.Syscall
}

func (e ReplayDivergedError) Error() string {
	return fmt.Sprintf("replay diverged at syscall %d: want %q got %q", e.Index, e.Want.Name, e.Got.Name)
}

// NewTape creates a journal-backed replay tape whose cursor starts at the beginning.
func NewTape(journal Journal) *Tape {
	return &Tape{journal, 0}
}

// Next returns a recorded result for syscall, or ok=false when syscall is new.
func (t *Tape) Next(syscall sys.Syscall) (sys.SyscallResult, bool, error) {
	if t == nil || t.cursor >= t.records.Length() {
		return sys.SyscallResult{}, false, nil
	}

	record, err := t.records.Load(t.cursor)
	if err != nil {
		return sys.SyscallResult{}, false, err
	}
	if !sameSyscall(record.Syscall, syscall) {
		return sys.SyscallResult{}, false, ReplayDivergedError{
			Index: t.cursor,
			Want:  record.Syscall,
			Got:   syscall,
		}
	}
	t.cursor++
	return record.Result, true, nil
}

func (t *Tape) Record(syscall sys.Syscall, result sys.SyscallResult) error {
	if t == nil || result.Status() == sys.StatusYield {
		return nil
	}
	if result.Status() != sys.StatusResult && result.Status() != sys.StatusFailed {
		return fmt.Errorf("cannot record invalid result %q", result.Status())
	}
	if err := t.records.Store(t.records.Length(), syscall, result); err != nil {
		return err
	}
	t.cursor++
	return nil
}

func (t *Tape) Reset() {
	if t == nil {
		return
	}
	t.cursor = 0
}

func (t *Tape) Remaining() int {
	if t == nil {
		return 0
	}
	return t.records.Length() - t.cursor
}

func sameSyscall(left sys.Syscall, right sys.Syscall) bool {
	return left.Name == right.Name && bytes.Equal(left.Args, right.Args)
}
