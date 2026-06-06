package journaled

import (
	"capcompute/internal/runtime/extism2"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestTapeResetsForEachPlay(t *testing.T) {
	records := []Record{{
		extism2.Call:    extism2.Call{Name: "step.one", Args: json.RawMessage(`{"x":1}`)},
		extism2.Outcome: extism2.Result(json.RawMessage(`{"ok":true}`)),
	}}

	first := NewTape(records)
	outcome, ok, err := first.Next(extism2.Call{Name: "step.one", Args: json.RawMessage(`{"x":1}`)})
	if err != nil {
		t.Fatalf("first replay: %v", err)
	}
	if !ok || outcome.Kind() != extism2.OutcomeResult {
		t.Fatalf("first outcome = %#v ok=%v", outcome, ok)
	}

	second := NewTape(records)
	outcome, ok, err = second.Next(extism2.Call{Name: "step.one", Args: json.RawMessage(`{"x":1}`)})
	if err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if !ok || outcome.Kind() != extism2.OutcomeResult {
		t.Fatalf("second outcome = %#v ok=%v", outcome, ok)
	}
}

func TestTapeDetectsDivergence(t *testing.T) {
	tape := NewTape([]Record{{
		extism2.Call:    extism2.Call{Name: "step.one", Args: json.RawMessage(`{"x":1}`)},
		extism2.Outcome: extism2.Result(nil),
	}})

	_, _, err := tape.Next(extism2.Call{Name: "step.two", Args: json.RawMessage(`{"x":1}`)})
	if err == nil {
		t.Fatal("expected divergence error")
	}
	var diverged ReplayDivergedError
	if !errors.As(err, &diverged) {
		t.Fatalf("error = %T %v, want ReplayDivergedError", err, err)
	}
}

func TestTapeReportsRemainingRecords(t *testing.T) {
	tape := NewTape([]Record{
		{extism2.Call: extism2.Call{Name: "step.one"}, extism2.Outcome: extism2.Result(nil)},
		{extism2.Call: extism2.Call{Name: "step.two"}, extism2.Outcome: extism2.Result(nil)},
	})

	if remaining := tape.Remaining(); remaining != 2 {
		t.Fatalf("remaining = %d", remaining)
	}
	if _, _, err := tape.Next(extism2.Call{Name: "step.one"}); err != nil {
		t.Fatalf("next: %v", err)
	}
	if remaining := tape.Remaining(); remaining != 1 {
		t.Fatalf("remaining = %d", remaining)
	}
}

func TestMemoryTapeDoesNotRecordNewResults(t *testing.T) {
	tape := NewTape(nil)

	if err := tape.Record(context.Background(), extism2.Call{Name: "step.one"}, extism2.Result(json.RawMessage(`{"ok":true}`))); err != nil {
		t.Fatalf("record: %v", err)
	}
	if outcome, ok, err := tape.Next(extism2.Call{Name: "step.one"}); err != nil || ok {
		t.Fatalf("outcome = %#v ok=%v err=%v", outcome, ok, err)
	}
}

func TestMemoryTapeDoesNotRecordYield(t *testing.T) {
	tape := NewTape(nil)

	if err := tape.Record(context.Background(), extism2.Call{Name: "step.one"}, extism2.Yield("waiting")); err != nil {
		t.Fatalf("record: %v", err)
	}
	if outcome, ok, err := tape.Next(extism2.Call{Name: "step.one"}); err != nil || ok {
		t.Fatalf("outcome = %#v ok=%v err=%v", outcome, ok, err)
	}
}
