package storetest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"capcompute/history"
)

type NewStore func(t *testing.T) history.Store

func Contract(t *testing.T, newStore NewStore) {
	t.Helper()

	t.Run("create and load run", func(t *testing.T) {
		store := newStore(t)
		err := store.CreateRun(context.Background(), history.Run{
			ID:     "run-1",
			Status: "running",
			Input:  json.RawMessage(`{"input":true}`),
		}, history.Event{Type: history.WorkflowStarted})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}

		run, events, err := store.LoadRun(context.Background(), "run-1")
		if err != nil {
			t.Fatalf("load run: %v", err)
		}
		if run.SchemaVersion != history.SchemaVersion {
			t.Fatalf("run schema version = %d", run.SchemaVersion)
		}
		if run.Version != 1 {
			t.Fatalf("version = %d, want 1", run.Version)
		}
		if compactJSON(t, run.Input) != `{"input":true}` {
			t.Fatalf("input = %s", run.Input)
		}
		if len(events) != 1 || events[0].Type != history.WorkflowStarted {
			t.Fatalf("events = %#v", events)
		}
		if events[0].SchemaVersion != history.SchemaVersion {
			t.Fatalf("event schema version = %d", events[0].SchemaVersion)
		}
	})

	t.Run("append uses expected version", func(t *testing.T) {
		store := newStore(t)
		err := store.CreateRun(context.Background(), history.Run{ID: "run-1", Status: "running"}, history.Event{Type: history.WorkflowStarted})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}

		run, _, err := store.LoadRun(context.Background(), "run-1")
		if err != nil {
			t.Fatalf("load run: %v", err)
		}
		if err := store.Append(context.Background(), "run-1", run.Version, history.Event{Type: history.CommandScheduled, CommandID: "cmd"}); err != nil {
			t.Fatalf("append with current version: %v", err)
		}
		if err := store.Append(context.Background(), "run-1", run.Version, history.Event{Type: history.CommandCompleted, CommandID: "cmd"}); err == nil {
			t.Fatal("expected stale version error")
		} else {
			var conflict history.VersionConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("error = %T %v, want VersionConflictError", err, err)
			}
		}
	})

	t.Run("load missing run returns typed error", func(t *testing.T) {
		store := newStore(t)
		_, _, err := store.LoadRun(context.Background(), "missing")
		if err == nil {
			t.Fatal("expected missing run error")
		}
		var notFound history.NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("error = %T %v, want NotFoundError", err, err)
		}
	})

	t.Run("rejects unsupported schema versions", func(t *testing.T) {
		store := newStore(t)
		err := store.CreateRun(context.Background(), history.Run{
			SchemaVersion: history.SchemaVersion + 1,
			ID:            "run-1",
			Status:        "running",
		})
		if err == nil {
			t.Fatal("expected unsupported schema error")
		}
		var unsupported history.UnsupportedSchemaError
		if !errors.As(err, &unsupported) {
			t.Fatalf("error = %T %v, want UnsupportedSchemaError", err, err)
		}

		if err := store.CreateRun(context.Background(), history.Run{ID: "run-2", Status: "running"}); err != nil {
			t.Fatalf("create run: %v", err)
		}
		run, _, err := store.LoadRun(context.Background(), "run-2")
		if err != nil {
			t.Fatalf("load run: %v", err)
		}
		err = store.Append(context.Background(), "run-2", run.Version, history.Event{
			SchemaVersion: history.SchemaVersion + 1,
			Type:          history.CommandScheduled,
			CommandID:     "cmd",
		})
		if err == nil {
			t.Fatal("expected unsupported event schema error")
		}
		if !errors.As(err, &unsupported) {
			t.Fatalf("error = %T %v, want UnsupportedSchemaError", err, err)
		}
	})

	t.Run("complete appends and marks run", func(t *testing.T) {
		store := newStore(t)
		err := store.CreateRun(context.Background(), history.Run{ID: "run-1", Status: "running"}, history.Event{Type: history.WorkflowStarted})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		run, _, err := store.LoadRun(context.Background(), "run-1")
		if err != nil {
			t.Fatalf("load run: %v", err)
		}

		err = store.Complete(context.Background(), "run-1", run.Version, json.RawMessage(`{"ok":true}`), history.Event{Type: history.WorkflowCompleted})
		if err != nil {
			t.Fatalf("complete run: %v", err)
		}

		run, events, err := store.LoadRun(context.Background(), "run-1")
		if err != nil {
			t.Fatalf("load completed run: %v", err)
		}
		if run.Status != "completed" {
			t.Fatalf("status = %q", run.Status)
		}
		if compactJSON(t, run.Output) != `{"ok":true}` {
			t.Fatalf("output = %s", run.Output)
		}
		if len(events) != 2 || events[1].Type != history.WorkflowCompleted {
			t.Fatalf("events = %#v", events)
		}
	})

	t.Run("fail appends and marks run", func(t *testing.T) {
		store := newStore(t)
		err := store.CreateRun(context.Background(), history.Run{ID: "run-1", Status: "running"}, history.Event{Type: history.WorkflowStarted})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		run, _, err := store.LoadRun(context.Background(), "run-1")
		if err != nil {
			t.Fatalf("load run: %v", err)
		}

		err = store.Fail(context.Background(), "run-1", run.Version, "boom", history.Event{Type: history.WorkflowFailed, Message: "boom"})
		if err != nil {
			t.Fatalf("fail run: %v", err)
		}

		run, events, err := store.LoadRun(context.Background(), "run-1")
		if err != nil {
			t.Fatalf("load failed run: %v", err)
		}
		if run.Status != "failed" {
			t.Fatalf("status = %q", run.Status)
		}
		if run.FailureMessage != "boom" {
			t.Fatalf("failure message = %q", run.FailureMessage)
		}
		if len(events) != 2 || events[1].Type != history.WorkflowFailed {
			t.Fatalf("events = %#v", events)
		}
	})

	t.Run("list runs filters status", func(t *testing.T) {
		store := newStore(t)
		if err := store.CreateRun(context.Background(), history.Run{ID: "run-1", Status: "running"}); err != nil {
			t.Fatalf("create running run: %v", err)
		}
		if err := store.CreateRun(context.Background(), history.Run{ID: "run-2", Status: "failed"}); err != nil {
			t.Fatalf("create failed run: %v", err)
		}

		all, err := store.ListRuns(context.Background(), history.RunFilter{})
		if err != nil {
			t.Fatalf("list all runs: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("all runs = %#v", all)
		}

		running, err := store.ListRuns(context.Background(), history.RunFilter{Status: "running"})
		if err != nil {
			t.Fatalf("list running runs: %v", err)
		}
		if len(running) != 1 || running[0].ID != "run-1" {
			t.Fatalf("running runs = %#v", running)
		}
	})

	t.Run("only one concurrent append wins", func(t *testing.T) {
		store := newStore(t)
		err := store.CreateRun(context.Background(), history.Run{ID: "run-1", Status: "running"}, history.Event{Type: history.WorkflowStarted})
		if err != nil {
			t.Fatalf("create run: %v", err)
		}
		run, _, err := store.LoadRun(context.Background(), "run-1")
		if err != nil {
			t.Fatalf("load run: %v", err)
		}

		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- store.Append(context.Background(), "run-1", run.Version, history.Event{Type: history.CommandScheduled, CommandID: "cmd"})
			}()
		}
		wg.Wait()
		close(errs)

		var successes int
		var stale int
		for err := range errs {
			if err == nil {
				successes++
				continue
			}
			var conflict history.VersionConflictError
			if errors.As(err, &conflict) {
				stale++
				continue
			}
			t.Fatalf("unexpected append error: %v", err)
		}
		if successes != 1 || stale != 1 {
			t.Fatalf("successes=%d stale=%d", successes, stale)
		}
	})
}

func compactJSON(t *testing.T, data json.RawMessage) string {
	t.Helper()

	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		t.Fatalf("compact json: %v", err)
	}
	return buf.String()
}
