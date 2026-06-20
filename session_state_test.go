package capcompute

import "testing"

func TestSessionStateLifecycle(t *testing.T) {
	var state sessionState

	if err := state.start(); err != nil {
		t.Fatalf("start idle session: %v", err)
	}
	if err := state.start(); err != ErrSessionActive {
		t.Fatalf("start active session error = %v, want ErrSessionActive", err)
	}

	state.finish(false)
	if err := state.start(); err != nil {
		t.Fatalf("restart yielded session: %v", err)
	}

	state.finish(true)
	if err := state.start(); err != ErrSessionTerminated {
		t.Fatalf("start terminated session error = %v, want ErrSessionTerminated", err)
	}
}

func TestSessionStateTerminate(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		var state sessionState
		if err := state.terminate(); err != nil {
			t.Fatalf("terminate idle session: %v", err)
		}
		if err := state.terminate(); err != nil {
			t.Fatalf("terminate terminated session: %v", err)
		}
		if err := state.start(); err != ErrSessionTerminated {
			t.Fatalf("start terminated session error = %v, want ErrSessionTerminated", err)
		}
	})

	t.Run("active", func(t *testing.T) {
		var state sessionState
		if err := state.start(); err != nil {
			t.Fatalf("start session: %v", err)
		}
		if err := state.terminate(); err != ErrSessionActive {
			t.Fatalf("terminate active session error = %v, want ErrSessionActive", err)
		}
		state.finish(false)
	})
}
