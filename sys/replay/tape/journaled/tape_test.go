package journaled_test

import (
	"testing"

	"github.com/aurora-capcompute/capcompute/sys"
	"github.com/aurora-capcompute/capcompute/sys/replay/tape/journaled"
)

// fakeJournal is an in-memory journaled.Journal. The tape needs only the Journal
// contract, so the package's own tests supply it directly rather than depending
// on a concrete store from a consumer module.
type fakeJournal struct {
	records []journaled.Record
}

func (j *fakeJournal) Load(idx int) (journaled.Record, error) {
	return j.records[idx], nil
}

func (j *fakeJournal) Store(_ int, syscall sys.Syscall, result sys.SyscallResult) error {
	j.records = append(j.records, journaled.Record{Syscall: syscall, Result: result})
	return nil
}

func (j *fakeJournal) Length() int { return len(j.records) }

func TestRecordConsumesNewRecord(t *testing.T) {
	tape := journaled.NewTape(&fakeJournal{})

	if err := tape.Record(sys.Syscall{Name: "step.one"}, sys.Result(nil)); err != nil {
		t.Fatalf("record: %v", err)
	}
	if remaining := tape.Remaining(); remaining != 0 {
		t.Fatalf("remaining = %d, want 0", remaining)
	}
}

func TestResetReplaysRecordedResults(t *testing.T) {
	tape := journaled.NewTape(&fakeJournal{})
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
	tape := journaled.NewTape(&fakeJournal{})
	syscall := sys.Syscall{Name: "step.one"}

	if err := tape.Record(syscall, sys.Fail("permission denied")); err != nil {
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
}

func TestYieldIsNotRecorded(t *testing.T) {
	journal := &fakeJournal{}
	tape := journaled.NewTape(journal)
	if err := tape.Record(sys.Syscall{Name: "step.one"}, sys.Yield("waiting")); err != nil {
		t.Fatalf("record yield: %v", err)
	}
	if journal.Length() != 0 {
		t.Fatalf("journal length = %d, want 0", journal.Length())
	}
}
