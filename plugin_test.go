package capcompute

import (
	"capcompute/dispatcher"
	"context"
	"encoding/json"
	"errors"
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
	active   map[string]struct{}
	saveErr  error
	endErr   error
}

func newTestSessionStore(sessions map[string]*Session[testSessionKey]) *testSessionStore {
	if sessions == nil {
		sessions = make(map[string]*Session[testSessionKey])
	}
	return &testSessionStore{sessions: sessions, active: make(map[string]struct{})}
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

func (s *testSessionStore) DeleteSession(_ context.Context, sessionID string) error {
	delete(s.sessions, sessionID)
	delete(s.active, sessionID)
	return nil
}

func (s *testSessionStore) ListSessions(context.Context) (map[string]*Session[testSessionKey], error) {
	sessions := make(map[string]*Session[testSessionKey], len(s.sessions))
	for sessionID, session := range s.sessions {
		sessions[sessionID] = session
	}
	return sessions, nil
}

func (s *testSessionStore) BeginSession(_ context.Context, sessionID string) error {
	if _, ok := s.sessions[sessionID]; !ok {
		return ErrSessionRequired
	}
	if _, ok := s.active[sessionID]; ok {
		return ErrSessionActive
	}
	s.active[sessionID] = struct{}{}
	return nil
}

func (s *testSessionStore) EndSession(_ context.Context, sessionID string) error {
	if s.endErr != nil {
		return s.endErr
	}
	delete(s.active, sessionID)
	return nil
}

func (s *testSessionStore) IsSessionActive(_ context.Context, sessionID string) (bool, error) {
	if _, ok := s.sessions[sessionID]; !ok {
		return false, ErrSessionRequired
	}
	_, ok := s.active[sessionID]
	return ok, nil
}

func (s *testSessionStore) markActive(sessionID string) {
	s.active[sessionID] = struct{}{}
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

func TestBeginPlayRejectsActiveSession(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key},
	})
	store.markActive("run-1")
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	_, err := compute.beginPlay(context.Background(), "run-1", key, PlayRequest{})
	if err != ErrSessionActive {
		t.Fatalf("error = %v, want ErrSessionActive", err)
	}
}

func TestBeginPlayReusesSessionAndResetsDispatcher(t *testing.T) {
	oldKey := testSessionKey{id: "run-1", value: "old"}
	newKey := testSessionKey{id: "run-1", value: "new"}
	request := PlayRequest{Input: json.RawMessage(`{"x":1}`), Entrypoint: "custom"}
	existing := &Session[testSessionKey]{
		guestData:  oldKey,
		dispatcher: testDispatcher{},
	}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{"run-1": existing})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	session, err := compute.beginPlay(context.Background(), "run-1", newKey, request)
	if err != nil {
		t.Fatalf("begin play: %v", err)
	}
	if session != existing {
		t.Fatal("begin play did not reuse existing session")
	}
	if session.guestData != newKey {
		t.Fatalf("guest data = %#v, want %#v", session.guestData, newKey)
	}
	if string(session.request.Input) != `{"x":1}` || session.request.Entrypoint != "custom" {
		t.Fatalf("request = %#v", session.request)
	}
	if session.dispatcher != nil {
		t.Fatal("dispatcher was not reset")
	}
	if _, ok := store.active["run-1"]; !ok {
		t.Fatal("session was not marked active")
	}
}

func TestPlayReleasesActiveSessionWhenDispatcherFactoryFails(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	dispatcherErr := errors.New("dispatcher failed")
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{
		dispatchers:  testDispatcherFactory{err: dispatcherErr},
		sessionStore: store,
	}

	results, err := compute.Play(context.Background(), key, PlayRequest{})
	if !errors.Is(err, dispatcherErr) {
		t.Fatalf("error = %v, want dispatcher error", err)
	}
	if results != nil {
		t.Fatal("results should be nil when Play fails before starting")
	}
	if _, ok := store.active["run-1"]; ok {
		t.Fatal("active session was not released")
	}
}

func TestSessionKeepsDispatcherAfterYield(t *testing.T) {
	session := &Session[testSessionKey]{}
	session.startPlay(testDispatcher{})
	session.finishPlay(true)

	if session.dispatcher == nil {
		t.Fatal("dispatcher should be kept after yield")
	}

	session.finishPlay(false)
	if session.dispatcher != nil {
		t.Fatal("dispatcher should be cleared after completion")
	}
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
