package replay

import (
	"capcompute/internal/runtime/extism2"
	dispatcher2 "capcompute/internal/runtime/extism2/dispatcher"
	"context"
	"encoding/json"
	"testing"
)

func TestReplayDispatcherReplaysBeforeNext(t *testing.T) {
	var calls int
	dispatcher, err := NewReplayDispatcher[string](
		extism2.NewTape([]extism2.Record{{
			Call:    extism2.Call{Name: "step.one"},
			Outcome: extism2.Result(json.RawMessage(`{"cached":true}`)),
		}}),
		dispatcher2.DispatcherFunc[string](func(context.Context, string, extism2.Call) (extism2.Outcome, error) {
			calls++
			return extism2.Result(json.RawMessage(`{}`)), nil
		}),
	)
	if err != nil {
		t.Fatalf("create replay dispatcher: %v", err)
	}

	outcome, err := dispatcher.Dispatch(context.Background(), "run-1", extism2.Call{Name: "step.one"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if outcome.Kind() != extism2.OutcomeResult || string(outcome.Result()) != `{"cached":true}` {
		t.Fatalf("outcome = %#v", outcome)
	}
	if calls != 0 {
		t.Fatalf("handler calls = %d", calls)
	}
}

func TestReplayDispatcherChecksCompletion(t *testing.T) {
	dispatcher, err := NewReplayDispatcher[string](
		extism2.NewTape([]extism2.Record{{Call: extism2.Call{Name: "step.one"}, Outcome: extism2.Result(nil)}}),
		dispatcher2.DispatcherFunc[string](func(context.Context, string, extism2.Call) (extism2.Outcome, error) {
			return extism2.Result(nil), nil
		}),
	)
	if err != nil {
		t.Fatalf("create replay dispatcher: %v", err)
	}

	if err := dispatcher.CheckCompleted(); err == nil {
		t.Fatal("expected incomplete replay error")
	}
	if _, err := dispatcher.Dispatch(context.Background(), "run-1", extism2.Call{Name: "step.one"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if err := dispatcher.CheckCompleted(); err != nil {
		t.Fatalf("check completed: %v", err)
	}
}

func TestReplayDispatcherRecordsNewOutcome(t *testing.T) {
	var recordedKey string
	var recordedCall extism2.Call
	var recordedOutcome extism2.Outcome
	tape, err := extism2.NewJournalTape[string](
		context.Background(),
		"run-1",
		extism2.JournalFunc[string]{
			RecordFunc: func(_ context.Context, key string, call extism2.Call, outcome extism2.Outcome) error {
				recordedKey = key
				recordedCall = call
				recordedOutcome = outcome
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("create journal tape: %v", err)
	}
	dispatcher, err := NewReplayDispatcher[string](
		tape,
		dispatcher2.DispatcherFunc[string](func(context.Context, string, extism2.Call) (extism2.Outcome, error) {
			return extism2.Result(json.RawMessage(`{"ok":true}`)), nil
		}),
	)
	if err != nil {
		t.Fatalf("create replay dispatcher: %v", err)
	}

	outcome, err := dispatcher.Dispatch(context.Background(), "run-1", extism2.Call{Name: "step.one"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if outcome.Kind() != extism2.OutcomeResult {
		t.Fatalf("outcome = %#v", outcome)
	}
	if recordedKey != "run-1" || recordedCall.Name != "step.one" || recordedOutcome.Kind() != extism2.OutcomeResult {
		t.Fatalf("recorded key=%q call=%#v outcome=%#v", recordedKey, recordedCall, recordedOutcome)
	}
}

func TestReplayDispatcherDoesNotRecordYield(t *testing.T) {
	var records int
	tape, err := extism2.NewJournalTape[string](
		context.Background(),
		"run-1",
		extism2.JournalFunc[string]{
			RecordFunc: func(context.Context, string, extism2.Call, extism2.Outcome) error {
				records++
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("create journal tape: %v", err)
	}
	dispatcher, err := NewReplayDispatcher[string](
		tape,
		dispatcher2.DispatcherFunc[string](func(context.Context, string, extism2.Call) (extism2.Outcome, error) {
			return extism2.Yield("waiting"), nil
		}),
	)
	if err != nil {
		t.Fatalf("create replay dispatcher: %v", err)
	}

	outcome, err := dispatcher.Dispatch(context.Background(), "run-1", extism2.Call{Name: "step.one"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if outcome.Kind() != extism2.OutcomeYield {
		t.Fatalf("outcome = %#v", outcome)
	}
	if records != 0 {
		t.Fatalf("records = %d", records)
	}
}
