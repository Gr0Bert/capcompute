package session_store_memory

import (
	"capcompute"
	"context"
	"testing"
)

type testSessionKey struct {
	id string
}

func (k testSessionKey) SessionKey() string {
	return k.id
}

func TestStoreSaveLoadDeleteAndList(t *testing.T) {
	ctx := context.Background()
	store := New[string, testSessionKey]()
	session := &capcompute.Session[testSessionKey]{}

	if err := store.SaveSession(ctx, "run-1", session); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := store.LoadSession(ctx, "run-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded != session {
		t.Fatal("loaded session differs from saved session")
	}
	sessions, err := store.ListSessions(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 1 || sessions["run-1"] != session {
		t.Fatalf("sessions = %#v", sessions)
	}
	if err := store.DeleteSession(ctx, "run-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.LoadSession(ctx, "run-1"); err != capcompute.ErrSessionRequired {
		t.Fatalf("load after delete = %v, want ErrSessionRequired", err)
	}
}

func TestStoreActiveLifecycle(t *testing.T) {
	ctx := context.Background()
	store := New[string, testSessionKey]()
	if err := store.SaveSession(ctx, "run-1", &capcompute.Session[testSessionKey]{}); err != nil {
		t.Fatalf("save: %v", err)
	}

	active, err := store.IsSessionActive(ctx, "run-1")
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if active {
		t.Fatal("session should start inactive")
	}
	if err := store.BeginSession(ctx, "run-1"); err != nil {
		t.Fatalf("begin: %v", err)
	}
	active, err = store.IsSessionActive(ctx, "run-1")
	if err != nil {
		t.Fatalf("active after begin: %v", err)
	}
	if !active {
		t.Fatal("session should be active")
	}
	if err := store.BeginSession(ctx, "run-1"); err != capcompute.ErrSessionActive {
		t.Fatalf("second begin = %v, want ErrSessionActive", err)
	}
	if err := store.EndSession(ctx, "run-1"); err != nil {
		t.Fatalf("end: %v", err)
	}
	active, err = store.IsSessionActive(ctx, "run-1")
	if err != nil {
		t.Fatalf("active after end: %v", err)
	}
	if active {
		t.Fatal("session should be inactive")
	}
}

func TestStoreDeleteClearsActive(t *testing.T) {
	ctx := context.Background()
	store := New[string, testSessionKey]()
	if err := store.SaveSession(ctx, "run-1", &capcompute.Session[testSessionKey]{}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := store.BeginSession(ctx, "run-1"); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := store.DeleteSession(ctx, "run-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.IsSessionActive(ctx, "run-1"); err != capcompute.ErrSessionRequired {
		t.Fatalf("active after delete = %v, want ErrSessionRequired", err)
	}
}
