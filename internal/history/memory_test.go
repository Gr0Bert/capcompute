package history

import (
	"context"
	"testing"
)

func TestMemoryStoreAppendUsesExpectedVersion(t *testing.T) {
	store := NewMemoryStore()
	err := store.CreateRun(context.Background(), Run{ID: "run-1", Status: "running"}, Event{Type: WorkflowStarted})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	run, _, err := store.LoadRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.Version != 1 {
		t.Fatalf("version = %d, want 1", run.Version)
	}

	if err := store.Append(context.Background(), "run-1", run.Version, Event{Type: CommandScheduled, CommandID: "cmd"}); err != nil {
		t.Fatalf("append with current version: %v", err)
	}

	if err := store.Append(context.Background(), "run-1", run.Version, Event{Type: CommandCompleted, CommandID: "cmd"}); err == nil {
		t.Fatal("expected stale version error")
	}
}
