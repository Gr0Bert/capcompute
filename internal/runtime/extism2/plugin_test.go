package extism2

import (
	"capcompute/internal/runtime/extism2/dispatcher"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type testSessionKey struct {
	id string
}

func (k testSessionKey) SessionKey() string {
	return k.id
}

type testDispatcher struct{}

func (testDispatcher) Dispatch(context.Context, testSessionKey, dispatcher.Call) (dispatcher.Outcome, error) {
	return dispatcher.Result(nil), nil
}

func TestBeginReplayRequiresReadySession(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	compute := &ComputeCompiledPlugin[string, testSessionKey]{
		sessions: map[string]*Session[testSessionKey]{
			"run-1": {
				guestData:  key,
				dispatcher: testDispatcher{},
			},
		},
		active: map[string]struct{}{},
	}

	_, _, err := compute.beginReplay("run-1")
	if !errors.Is(err, ErrSessionNotReady) {
		t.Fatalf("error = %v, want ErrSessionNotReady", err)
	}
}

func TestBeginReplayUsesExistingDispatcher(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	existing := testDispatcher{}
	request := PlayRequest{Input: json.RawMessage(`{"x":1}`), Entrypoint: "run"}
	compute := &ComputeCompiledPlugin[string, testSessionKey]{
		sessions: map[string]*Session[testSessionKey]{
			"run-1": {
				guestData:  key,
				request:    request,
				ready:      true,
				dispatcher: existing,
				yielded:    &dispatcher.Call{Name: "step.one"},
			},
		},
		active: map[string]struct{}{},
	}

	session, replayDispatcher, err := compute.beginReplay("run-1")
	if err != nil {
		t.Fatalf("begin replay: %v", err)
	}
	if session.ready {
		t.Fatal("session should no longer be ready while replay is active")
	}
	if _, ok := compute.active["run-1"]; !ok {
		t.Fatal("session was not marked active")
	}
	if replayDispatcher == nil {
		t.Fatal("dispatcher is nil")
	}
	if string(session.request.Input) != `{"x":1}` || session.request.Entrypoint != "run" {
		t.Fatalf("request = %#v", session.request)
	}
}

func TestSessionKeepsDispatcherAfterYield(t *testing.T) {
	session := &Session[testSessionKey]{}
	session.startPlay(testDispatcher{})
	session.recordYield(dispatcher.Call{Name: "step.one"})
	session.finishPlay(true)

	if session.dispatcher == nil {
		t.Fatal("dispatcher should be kept after yield")
	}
	if session.yielded == nil || session.yielded.Name != "step.one" {
		t.Fatalf("yielded = %#v", session.yielded)
	}

	session.finishPlay(false)
	if session.dispatcher != nil {
		t.Fatal("dispatcher should be cleared after completion")
	}
	if session.yielded != nil {
		t.Fatal("yielded should be cleared after completion")
	}
}

func TestFinishPlayResultKeepsYieldedSession(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	compute := &ComputeCompiledPlugin[string, testSessionKey]{
		sessions: map[string]*Session[testSessionKey]{
			"run-1": {guestData: key},
		},
		active: map[string]struct{}{"run-1": {}},
	}

	err := compute.finishPlayResult(context.Background(), "run-1", PlayResult[testSessionKey]{
		Key:    key,
		Status: PlayYielded,
	})
	if err != nil {
		t.Fatalf("finish play: %v", err)
	}
	if _, ok := compute.active["run-1"]; ok {
		t.Fatal("yielded session should not remain active")
	}
	if _, ok := compute.sessions["run-1"]; !ok {
		t.Fatal("yielded session should be retained")
	}
}

func TestFinishPlayResultRemovesCompletedSession(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	compute := &ComputeCompiledPlugin[string, testSessionKey]{
		sessions: map[string]*Session[testSessionKey]{
			"run-1": {guestData: key},
		},
		active: map[string]struct{}{"run-1": {}},
	}

	err := compute.finishPlayResult(context.Background(), "run-1", PlayResult[testSessionKey]{
		Key:    key,
		Status: PlayCompleted,
	})
	if err != nil {
		t.Fatalf("finish play: %v", err)
	}
	if _, ok := compute.active["run-1"]; ok {
		t.Fatal("completed session should not remain active")
	}
	if _, ok := compute.sessions["run-1"]; ok {
		t.Fatal("completed session should be removed")
	}
}
