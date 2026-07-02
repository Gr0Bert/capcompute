package capcompute

import "testing"

func TestProcessStateLifecycle(t *testing.T) {
	var state processState

	if err := state.start(); err != nil {
		t.Fatalf("start idle process: %v", err)
	}
	if err := state.start(); err != ErrProcessActive {
		t.Fatalf("start active process error = %v, want ErrProcessActive", err)
	}

	state.finish(false)
	if err := state.start(); err != nil {
		t.Fatalf("restart yielded process: %v", err)
	}

	state.finish(true)
	if err := state.start(); err != ErrProcessTerminated {
		t.Fatalf("start terminated process error = %v, want ErrProcessTerminated", err)
	}
}

func TestProcessStateTerminate(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		var state processState
		if err := state.terminate(); err != nil {
			t.Fatalf("terminate idle process: %v", err)
		}
		if err := state.terminate(); err != nil {
			t.Fatalf("terminate terminated process: %v", err)
		}
		if err := state.start(); err != ErrProcessTerminated {
			t.Fatalf("start terminated process error = %v, want ErrProcessTerminated", err)
		}
	})

	t.Run("active", func(t *testing.T) {
		var state processState
		if err := state.start(); err != nil {
			t.Fatalf("start process: %v", err)
		}
		if err := state.terminate(); err != ErrProcessActive {
			t.Fatalf("terminate active process error = %v, want ErrProcessActive", err)
		}
		state.finish(false)
	})
}
