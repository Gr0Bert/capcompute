package journaled_test

import (
	"capcompute/dispatcher"
	"capcompute/dispatcher/replay/tape/journaled"
	"capcompute/dispatcher/replay/tape/journaled/journal/memory"
	"testing"
)

func TestRecordConsumesNewRecord(t *testing.T) {
	tape := journaled.NewTape(memory.NewJournal())

	if err := tape.Record(dispatcher.Call{Name: "step.one"}, dispatcher.Result(nil)); err != nil {
		t.Fatalf("record: %v", err)
	}
	if remaining := tape.Remaining(); remaining != 0 {
		t.Fatalf("remaining = %d, want 0", remaining)
	}
}

func TestResetReplaysRecordedResults(t *testing.T) {
	tape := journaled.NewTape(memory.NewJournal())
	call := dispatcher.Call{Name: "step.one"}

	if err := tape.Record(call, dispatcher.Result([]byte(`{"ok":true}`))); err != nil {
		t.Fatalf("record: %v", err)
	}

	tape.Reset()
	outcome, ok, err := tape.Next(call)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if !ok {
		t.Fatal("record was not replayed")
	}
	if string(outcome.Result()) != `{"ok":true}` {
		t.Fatalf("result = %s", outcome.Result())
	}
	if remaining := tape.Remaining(); remaining != 0 {
		t.Fatalf("remaining = %d, want 0", remaining)
	}
}
