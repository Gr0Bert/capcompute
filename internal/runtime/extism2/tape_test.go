package extism2

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestTapeResetsForEachPlay(t *testing.T) {
	records := []Record{{
		Call:    Call{Name: "step.one", Args: json.RawMessage(`{"x":1}`)},
		Outcome: Result(json.RawMessage(`{"ok":true}`)),
	}}

	first := NewTape(records)
	outcome, ok, err := first.Next(Call{Name: "step.one", Args: json.RawMessage(`{"x":1}`)})
	if err != nil {
		t.Fatalf("first replay: %v", err)
	}
	if !ok || outcome.Kind() != OutcomeResult {
		t.Fatalf("first outcome = %#v ok=%v", outcome, ok)
	}

	second := NewTape(records)
	outcome, ok, err = second.Next(Call{Name: "step.one", Args: json.RawMessage(`{"x":1}`)})
	if err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if !ok || outcome.Kind() != OutcomeResult {
		t.Fatalf("second outcome = %#v ok=%v", outcome, ok)
	}
}

func TestTapeDetectsDivergence(t *testing.T) {
	tape := NewTape([]Record{{
		Call:    Call{Name: "step.one", Args: json.RawMessage(`{"x":1}`)},
		Outcome: Result(nil),
	}})

	_, _, err := tape.Next(Call{Name: "step.two", Args: json.RawMessage(`{"x":1}`)})
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
		{Call: Call{Name: "step.one"}, Outcome: Result(nil)},
		{Call: Call{Name: "step.two"}, Outcome: Result(nil)},
	})

	if remaining := tape.Remaining(); remaining != 2 {
		t.Fatalf("remaining = %d", remaining)
	}
	if _, _, err := tape.Next(Call{Name: "step.one"}); err != nil {
		t.Fatalf("next: %v", err)
	}
	if remaining := tape.Remaining(); remaining != 1 {
		t.Fatalf("remaining = %d", remaining)
	}
}
