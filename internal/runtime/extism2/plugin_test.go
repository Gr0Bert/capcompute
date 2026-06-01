package extism2

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestComputeCompiledPluginRejectsConcurrentPlay(t *testing.T) {
	compute := &ComputeCompiledPlugin[string]{
		sessions: map[string]*Session[string]{"run-1": {}},
		active:   map[string]struct{}{"run-1": {}},
	}

	_, err := compute.Play(context.Background(), "run-1", PlayRequest{})
	if !errors.Is(err, ErrSessionActive) {
		t.Fatalf("error = %v, want ErrSessionActive", err)
	}
}

func TestComputeCompiledPluginRejectsCloseWhileActive(t *testing.T) {
	compute := &ComputeCompiledPlugin[string]{
		sessions: map[string]*Session[string]{"run-1": {}},
		active:   map[string]struct{}{"run-1": {}},
	}

	err := compute.Close(context.Background())
	if !errors.Is(err, ErrSessionActive) {
		t.Fatalf("error = %v, want ErrSessionActive", err)
	}
	if len(compute.sessions) != 1 {
		t.Fatalf("sessions = %#v", compute.sessions)
	}
}

func TestComputeCompiledPluginTracksReadyAndYielded(t *testing.T) {
	compute := &ComputeCompiledPlugin[string]{
		sessions: map[string]*Session[string]{"run-1": {}},
		active:   map[string]struct{}{},
	}

	if !compute.MarkReady("run-1") {
		t.Fatal("mark ready returned false")
	}
	if !compute.Ready("run-1") {
		t.Fatal("session should be ready")
	}

	compute.markYielded("run-1", Call{Name: "step.one", Args: json.RawMessage(`{"x":1}`)})
	if !compute.Ready("run-1") {
		t.Fatal("yield bookkeeping should not clear a concurrently ready session")
	}
	if compute.sessions["run-1"].yielded == nil || compute.sessions["run-1"].yielded.Name != "step.one" {
		t.Fatalf("yielded = %#v", compute.sessions["run-1"].yielded)
	}
}

func TestComputeCompiledPluginRecordsThroughJournal(t *testing.T) {
	var recordedKey string
	var recordedCall Call
	var recordedOutcome Outcome
	compute := &ComputeCompiledPlugin[string]{
		journal: JournalFunc[string]{
			RecordFunc: func(_ context.Context, key string, call Call, outcome Outcome) error {
				recordedKey = key
				recordedCall = call
				recordedOutcome = outcome
				return nil
			},
		},
	}

	err := compute.record(context.Background(), "run-1", Call{Name: "step.one"}, Result(json.RawMessage(`{"ok":true}`)))
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if recordedKey != "run-1" || recordedCall.Name != "step.one" || recordedOutcome.Kind() != OutcomeResult {
		t.Fatalf("recorded key=%q call=%#v outcome=%#v", recordedKey, recordedCall, recordedOutcome)
	}
}

func TestComputeCompiledPluginLoadsRecordsThroughJournal(t *testing.T) {
	compute := &ComputeCompiledPlugin[string]{
		journal: JournalFunc[string]{
			LoadFunc: func(_ context.Context, key string) ([]Record, error) {
				if key != "run-1" {
					t.Fatalf("key = %q", key)
				}
				return []Record{{Call: Call{Name: "step.one"}, Outcome: Result(json.RawMessage(`{"ok":true}`))}}, nil
			},
		},
	}

	records, err := compute.loadRecords(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("load records: %v", err)
	}
	if len(records) != 1 || records[0].Call.Name != "step.one" {
		t.Fatalf("records = %#v", records)
	}
}
