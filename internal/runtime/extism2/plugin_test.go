package extism2

import (
	"capcompute/internal/runtime/extism2/dispatcher"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type testRunKey struct {
	id   string
	tags []string
}

func (k testRunKey) SessionKey() string {
	return k.id
}

func TestComputeCompiledPluginRejectsConcurrentPlay(t *testing.T) {
	key := testRunKey{id: "run-1", tags: []string{"non-comparable"}}
	compute := &ComputeCompiledPlugin[string, testRunKey]{
		sessions: map[string]*Session[testRunKey]{"run-1": {Key: key}},
		active:   map[string]struct{}{"run-1": {}},
		dispatchers: dispatcher.DispatcherFactoryFunc[testRunKey](func(context.Context, testRunKey) (dispatcher.Dispatcher[testRunKey], error) {
			return &dispatcher.DefaultDispatcher[testRunKey]{}, nil
		}),
	}

	_, err := compute.Play(context.Background(), key, PlayRequest{})
	if !errors.Is(err, ErrSessionActive) {
		t.Fatalf("error = %v, want ErrSessionActive", err)
	}
}

func TestComputeCompiledPluginRejectsCloseWhileActive(t *testing.T) {
	key := testRunKey{id: "run-1", tags: []string{"non-comparable"}}
	compute := &ComputeCompiledPlugin[string, testRunKey]{
		sessions: map[string]*Session[testRunKey]{"run-1": {Key: key}},
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
	key := testRunKey{id: "run-1", tags: []string{"non-comparable"}}
	compute := &ComputeCompiledPlugin[string, testRunKey]{
		sessions: map[string]*Session[testRunKey]{"run-1": {Key: key}},
		active:   map[string]struct{}{},
	}

	if !compute.MarkReady(key) {
		t.Fatal("mark ready returned false")
	}
	if !compute.Ready(key) {
		t.Fatal("session should be ready")
	}

	compute.markYielded(key, Call{Name: "step.one", Args: json.RawMessage(`{"x":1}`)})
	if !compute.Ready(key) {
		t.Fatal("yield bookkeeping should not clear a concurrently ready session")
	}
	if compute.sessions["run-1"].yielded == nil || compute.sessions["run-1"].yielded.Name != "step.one" {
		t.Fatalf("yielded = %#v", compute.sessions["run-1"].yielded)
	}
}
