package journaled_test

import (
	"errors"
	"testing"

	"github.com/aurora-capcompute/capcompute/sys"
	"github.com/aurora-capcompute/capcompute/sys/replay/tape/journaled"
)

// fakeJournal is an in-memory journaled.Journal. The tape needs only the Journal
// contract, so the package's own tests supply it directly rather than depending
// on a concrete store from a consumer module.
type fakeJournal struct {
	header    journaled.Header
	hasHeader bool
	records   []journaled.Record
}

func (j *fakeJournal) Header() (journaled.Header, bool, error) {
	return j.header, j.hasHeader, nil
}

func (j *fakeJournal) SetHeader(header journaled.Header) error {
	j.header = header
	j.hasHeader = true
	return nil
}

func (j *fakeJournal) Load(idx int) (journaled.Record, error) {
	return j.records[idx], nil
}

func (j *fakeJournal) Store(_ int, syscall sys.Syscall, result sys.SyscallResult) error {
	j.records = append(j.records, journaled.Record{Syscall: syscall, Result: result})
	return nil
}

func (j *fakeJournal) Length() int { return len(j.records) }

var testHeader = journaled.Header{ABI: sys.ABIVersion, Program: "sha256:test"}

func newTestTape(t *testing.T, journal journaled.Journal) *journaled.Tape {
	t.Helper()
	tape, err := journaled.NewTape(journal, testHeader)
	if err != nil {
		t.Fatalf("new tape: %v", err)
	}
	return tape
}

func TestNewTapeStampsFreshJournal(t *testing.T) {
	journal := &fakeJournal{}
	newTestTape(t, journal)

	header, ok, err := journal.Header()
	if err != nil || !ok {
		t.Fatalf("header = %v, ok = %v, err = %v", header, ok, err)
	}
	if header != testHeader {
		t.Fatalf("header = %+v, want %+v", header, testHeader)
	}
}

func TestNewTapeAcceptsMatchingHeader(t *testing.T) {
	journal := &fakeJournal{header: testHeader, hasHeader: true}
	newTestTape(t, journal)
}

func TestNewTapeRefusesIncompatibleJournal(t *testing.T) {
	recorded := journaled.Header{ABI: sys.ABIVersion, Program: "sha256:other"}
	journal := &fakeJournal{header: recorded, hasHeader: true}

	_, err := journaled.NewTape(journal, testHeader)
	var incompatible journaled.ReplayIncompatibleError
	if !errors.As(err, &incompatible) {
		t.Fatalf("error = %v, want ReplayIncompatibleError", err)
	}
	if incompatible.Recorded != recorded || incompatible.Current != testHeader {
		t.Fatalf("error = %+v", incompatible)
	}
}

func TestRecordConsumesNewRecord(t *testing.T) {
	tape := newTestTape(t, &fakeJournal{})

	if err := tape.Record(sys.Syscall{Name: "step.one"}, sys.Result(nil)); err != nil {
		t.Fatalf("record: %v", err)
	}
	if remaining := tape.Remaining(); remaining != 0 {
		t.Fatalf("remaining = %d, want 0", remaining)
	}
}

func TestResetReplaysRecordedResults(t *testing.T) {
	tape := newTestTape(t, &fakeJournal{})
	syscall := sys.Syscall{Name: "step.one"}

	if err := tape.Record(syscall, sys.Result([]byte(`{"ok":true}`))); err != nil {
		t.Fatalf("record: %v", err)
	}

	tape.Reset()
	result, ok, err := tape.Next(syscall)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if !ok {
		t.Fatal("record was not replayed")
	}
	if string(result.Result()) != `{"ok":true}` {
		t.Fatalf("result = %s", result.Result())
	}
	if remaining := tape.Remaining(); remaining != 0 {
		t.Fatalf("remaining = %d, want 0", remaining)
	}
}

func TestResetReplaysRecordedFailures(t *testing.T) {
	tape := newTestTape(t, &fakeJournal{})
	syscall := sys.Syscall{Name: "step.one"}

	if err := tape.Record(syscall, sys.FailCode(sys.ErrnoDenied, "permission denied")); err != nil {
		t.Fatalf("record: %v", err)
	}

	tape.Reset()
	result, ok, err := tape.Next(syscall)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if !ok {
		t.Fatal("failure was not replayed")
	}
	if result.Status() != sys.StatusFailed || result.Message() != "permission denied" {
		t.Fatalf("result = %#v", result)
	}
	if result.Errno() != sys.ErrnoDenied {
		t.Fatalf("errno = %q, want denied", result.Errno())
	}
}

func TestYieldIsNotRecorded(t *testing.T) {
	journal := &fakeJournal{}
	tape := newTestTape(t, journal)
	if err := tape.Record(sys.Syscall{Name: "step.one"}, sys.Yield("waiting")); err != nil {
		t.Fatalf("record yield: %v", err)
	}
	if journal.Length() != 0 {
		t.Fatalf("journal length = %d, want 0", journal.Length())
	}
}
