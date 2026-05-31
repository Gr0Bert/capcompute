package file

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"capcompute/history"
	"capcompute/internal/history/storetest"
)

func TestStoreContract(t *testing.T) {
	storetest.Contract(t, func(t *testing.T) history.Store {
		t.Helper()
		return NewStore(filepath.Join(t.TempDir(), "history.json"))
	})
}

func TestStorePersistsRunAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	first := NewStore(path)

	err := first.CreateRun(context.Background(), history.Run{
		ID:     "run-1",
		Status: "running",
		Input:  json.RawMessage(`{"input":true}`),
	}, history.Event{Type: history.WorkflowStarted})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	second := NewStore(path)
	run, events, err := second.LoadRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.ID != "run-1" {
		t.Fatalf("run id = %q", run.ID)
	}
	if run.SchemaVersion != history.SchemaVersion {
		t.Fatalf("run schema version = %d", run.SchemaVersion)
	}
	if len(events) != 1 || events[0].Type != history.WorkflowStarted {
		t.Fatalf("events = %#v", events)
	}
}

func TestStoreWritesSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path)

	err := store.CreateRun(context.Background(), history.Run{ID: "run-1", Status: "running"}, history.Event{Type: history.WorkflowStarted})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("history file is invalid json: %s", data)
	}
	if !contains(data, `"schema_version": 1`) {
		t.Fatalf("history file does not include schema version: %s", data)
	}
}

func TestStoreRejectsUnsupportedFileSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":999,"runs":{}}`), 0o600); err != nil {
		t.Fatalf("write history file: %v", err)
	}

	store := NewStore(path)
	_, _, err := store.LoadRun(context.Background(), "run-1")
	if err == nil {
		t.Fatal("expected unsupported schema error")
	}
	var unsupported history.UnsupportedSchemaError
	if !errors.As(err, &unsupported) {
		t.Fatalf("error = %T %v, want UnsupportedSchemaError", err, err)
	}
}

func contains(data []byte, value string) bool {
	return strings.Contains(string(data), value)
}
