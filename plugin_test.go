package capcompute

import (
	"capcompute/dispatcher"
	"context"
	"testing"
)

type testSessionKey struct {
	id    string
	value string
}

func (k testSessionKey) SessionKey() string {
	return k.id
}

type testDispatcher struct{}

func (testDispatcher) Dispatch(context.Context, testSessionKey, dispatcher.Call) (dispatcher.Outcome, error) {
	return dispatcher.Result(nil), nil
}

type testDispatcherFactory struct {
	err error
}

func (f testDispatcherFactory) NewDispatcher(context.Context, testSessionKey) (dispatcher.Dispatcher[testSessionKey], error) {
	if f.err != nil {
		return nil, f.err
	}
	return testDispatcher{}, nil
}

type testSessionStore struct {
	sessions map[string]*Session[testSessionKey]
	saveErr  error
}

func newTestSessionStore(sessions map[string]*Session[testSessionKey]) *testSessionStore {
	if sessions == nil {
		sessions = make(map[string]*Session[testSessionKey])
	}
	return &testSessionStore{sessions: sessions}
}

func (s *testSessionStore) LoadSession(_ context.Context, sessionID string) (*Session[testSessionKey], error) {
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionRequired
	}
	return session, nil
}

func (s *testSessionStore) SaveSession(_ context.Context, sessionID string, session *Session[testSessionKey]) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.sessions[sessionID] = session
	return nil
}

func TestNewComputeCompiledPluginRequiresDispatcherFactory(t *testing.T) {
	_, err := NewComputeCompiledPlugin[string, testSessionKey](context.Background(), Config[string, testSessionKey]{
		SessionStore: newTestSessionStore(nil),
	})
	if err != ErrDispatcherRequired {
		t.Fatalf("error = %v, want ErrDispatcherRequired", err)
	}
}

func TestNewComputeCompiledPluginRequiresSessionStore(t *testing.T) {
	_, err := NewComputeCompiledPlugin[string, testSessionKey](context.Background(), Config[string, testSessionKey]{
		Dispatchers: testDispatcherFactory{},
	})
	if err != ErrSessionStoreRequired {
		t.Fatalf("error = %v, want ErrSessionStoreRequired", err)
	}
}

func TestComputeCompiledPluginExposesCompiledCleanup(t *testing.T) {
	var _ func(*ComputeCompiledPlugin[string, testSessionKey], context.Context) error = (*ComputeCompiledPlugin[string, testSessionKey]).CloseCompiled
}

func TestPlayStatusReadsYieldedOutput(t *testing.T) {
	if got := playStatus([]byte(`{"status":"yielded"}`)); got != PlayYielded {
		t.Fatalf("status = %s, want %s", got, PlayYielded)
	}
}

func TestPlayStatusDefaultsToCompleted(t *testing.T) {
	for _, output := range [][]byte{
		[]byte(`{"status":"completed"}`),
		[]byte(`{"answer":"done"}`),
		[]byte(`not json`),
	} {
		if got := playStatus(output); got != PlayCompleted {
			t.Fatalf("status = %s for %s, want %s", got, output, PlayCompleted)
		}
	}
}
