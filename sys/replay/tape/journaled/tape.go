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

// Header identifies what wrote a journal: the syscall ABI version and the
// program (e.g. an artifact digest). Replay against a different program is
// refused up front — the versioned-replay law (see docs/ARCHITECTURE.md,
// "Coherence under growth") — instead of failing later as a confusing
// divergence.
type Header struct {
	ABI     int    `json:"abi"`
	Program string `json:"program"`
}

// Journal stores durable records for a tape, plus the header identifying
// their writer.
type Journal interface {
	Header() (header Header, ok bool, err error)
	SetHeader(Header) error
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

// ReplayIncompatibleError means the journal was written by a different program
// or ABI than the one attempting to replay it.
type ReplayIncompatibleError struct {
	Recorded Header
	Current  Header
}

func (e ReplayIncompatibleError) Error() string {
	return fmt.Sprintf("journal written by program %q (abi %d); cannot replay as program %q (abi %d)",
		e.Recorded.Program, e.Recorded.ABI, e.Current.Program, e.Current.ABI)
}

// NewTape creates a journal-backed replay tape whose cursor starts at the
// beginning. The header identifies the program about to run: a fresh journal
// is stamped with it; a journal written by a different program or ABI is
// refused with ReplayIncompatibleError.
func NewTape(journal Journal, header Header) (*Tape, error) {
	recorded, ok, err := journal.Header()
	if err != nil {
		return nil, err
	}
	if !ok {
		if err := journal.SetHeader(header); err != nil {
			return nil, err
		}
		return &Tape{journal, 0}, nil
	}
	if recorded != header {
		return nil, ReplayIncompatibleError{Recorded: recorded, Current: header}
	}
	return &Tape{journal, 0}, nil
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
