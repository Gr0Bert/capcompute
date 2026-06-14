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
	deleted  []string
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

func (s *testSessionStore) DeleteSession(_ context.Context, sessionID string) error {
	delete(s.sessions, sessionID)
	s.deleted = append(s.deleted, sessionID)
	return nil
}

func (s *testSessionStore) ListSessions(context.Context) (map[string]*Session[testSessionKey], error) {
	sessions := make(map[string]*Session[testSessionKey], len(s.sessions))
	for sessionID, session := range s.sessions {
		sessions[sessionID] = session
	}
	return sessions, nil
}

func TestPlayEntrypointUsesRequestThenConfigThenDefault(t *testing.T) {
	compute := &ComputeCompiledPlugin[string, testSessionKey]{entrypoint: "configured"}

	if got := compute.playEntrypoint(PlayRequest{Entrypoint: "request"}); got != "request" {
		t.Fatalf("entrypoint = %q, want request", got)
	}
	if got := compute.playEntrypoint(PlayRequest{}); got != "configured" {
		t.Fatalf("entrypoint = %q, want configured", got)
	}

	compute.entrypoint = ""
	if got := compute.playEntrypoint(PlayRequest{}); got != defaultEntrypoint {
		t.Fatalf("entrypoint = %q, want %q", got, defaultEntrypoint)
	}
}

func TestBeginPlayRejectsActiveSession(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key, active: true},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	_, err := compute.beginPlay(context.Background(), "run-1", key, PlayRequest{})
	if err != ErrSessionActive {
		t.Fatalf("error = %v, want ErrSessionActive", err)
	}
}

func TestBeginPlayReusesSessionAndResetsPlayState(t *testing.T) {
	staleErr := errors.New("stale")
	oldKey := testSessionKey{id: "run-1", value: "old"}
	newKey := testSessionKey{id: "run-1", value: "new"}
	request := PlayRequest{Input: json.RawMessage(`{"x":1}`), Entrypoint: "run"}
	existing := &Session[testSessionKey]{
		guestData:  oldKey,
		dispatcher: testDispatcher{},
		yielded:    &dispatcher.Call{Name: "step.one"},
		err:        staleErr,
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
	if string(session.request.Input) != `{"x":1}` || session.request.Entrypoint != "run" {
		t.Fatalf("request = %#v", session.request)
	}
	if session.dispatcher != nil {
		t.Fatal("dispatcher was not reset")
	}
	if session.yielded != nil {
		t.Fatal("yielded call was not reset")
	}
	if session.err != nil {
		t.Fatalf("err = %v, want nil", session.err)
	}
	if !existing.active {
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
	if store.sessions["run-1"].active {
		t.Fatal("active session was not released")
	}
}

func TestBeginReplayRequiresExistingSession(t *testing.T) {
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: newTestSessionStore(nil)}

	_, _, err := compute.beginReplay(context.Background(), "run-1")
	if err != ErrSessionRequired {
		t.Fatalf("error = %v, want ErrSessionRequired", err)
	}
}

func TestBeginReplayRejectsActiveSession(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {
			guestData:  key,
			dispatcher: testDispatcher{},
			active:     true,
		},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	_, _, err := compute.beginReplay(context.Background(), "run-1")
	if err != ErrSessionActive {
		t.Fatalf("error = %v, want ErrSessionActive", err)
	}
}

func TestBeginReplayRequiresExistingDispatcher(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	_, _, err := compute.beginReplay(context.Background(), "run-1")
	if err != ErrDispatcherRequired {
		t.Fatalf("error = %v, want ErrDispatcherRequired", err)
	}
}

func TestBeginReplayUsesExistingDispatcher(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	existing := testDispatcher{}
	request := PlayRequest{Input: json.RawMessage(`{"x":1}`), Entrypoint: "run"}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {
			guestData:  key,
			request:    request,
			dispatcher: existing,
			yielded:    &dispatcher.Call{Name: "step.one"},
		},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	session, replayDispatcher, err := compute.beginReplay(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("begin replay: %v", err)
	}
	if !session.active {
		t.Fatal("session was not marked active")
	}
	if replayDispatcher == nil {
		t.Fatal("dispatcher is nil")
	}
	if string(session.request.Input) != `{"x":1}` || session.request.Entrypoint != "run" {
		t.Fatalf("request = %#v", session.request)
	}
}

func TestReplayRequiresExistingSession(t *testing.T) {
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: newTestSessionStore(nil)}

	results, err := compute.Replay(context.Background(), "run-1")
	if err != ErrSessionRequired {
		t.Fatalf("error = %v, want ErrSessionRequired", err)
	}
	if results != nil {
		t.Fatal("results should be nil when Replay fails before starting")
	}
}

func TestSessionRecordsYieldedCallCopy(t *testing.T) {
	session := &Session[testSessionKey]{}
	call := dispatcher.Call{Name: "step.one", Args: []byte(`{"x":1}`)}

	session.recordYield(call)
	call.Args[0] = '!'

	if session.yielded == nil {
		t.Fatal("yielded call was not recorded")
	}
	if string(session.yielded.Args) != `{"x":1}` {
		t.Fatalf("yielded args = %s", session.yielded.Args)
	}
}

func TestSessionKeepsDispatcherAndYieldedCallAfterYield(t *testing.T) {
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
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key, active: true},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	err := compute.finishPlayResult(context.Background(), "run-1", PlayResult[testSessionKey]{
		Key:    key,
		Status: PlayYielded,
	})
	if err != nil {
		t.Fatalf("finish play: %v", err)
	}
	if store.sessions["run-1"].active {
		t.Fatal("yielded session should not remain active")
	}
	if _, ok := store.sessions["run-1"]; !ok {
		t.Fatal("yielded session should be retained")
	}
}

func TestFinishPlayResultReleasesActiveWhenSaveFails(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	saveErr := errors.New("save failed")
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key, active: true},
	})
	store.saveErr = saveErr
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	err := compute.finishPlayResult(context.Background(), "run-1", PlayResult[testSessionKey]{
		Key:    key,
		Status: PlayYielded,
	})
	if !errors.Is(err, saveErr) {
		t.Fatalf("error = %v, want save error", err)
	}
	if store.sessions["run-1"].active {
		t.Fatal("active session should be released after save failure")
	}
	if _, ok := store.sessions["run-1"]; !ok {
		t.Fatal("session should be retained after save failure")
	}
}

func TestFinishPlayResultRemovesFailedSession(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key, active: true},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	err := compute.finishPlayResult(context.Background(), "run-1", PlayResult[testSessionKey]{
		Key:    key,
		Status: PlayFailed,
		Err:    errors.New("guest failed"),
	})
	if err != nil {
		t.Fatalf("finish play: %v", err)
	}
	if _, ok := store.sessions["run-1"]; ok {
		t.Fatal("failed session should be removed")
	}
}

func TestFinishPlayResultRemovesCompletedSession(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key, active: true},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	err := compute.finishPlayResult(context.Background(), "run-1", PlayResult[testSessionKey]{
		Key:    key,
		Status: PlayCompleted,
	})
	if err != nil {
		t.Fatalf("finish play: %v", err)
	}
	if _, ok := store.sessions["run-1"]; ok {
		t.Fatal("completed session should be removed")
	}
}

func TestFinishPlayResultDeletesCompletedSession(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key, active: true},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	err := compute.finishPlayResult(context.Background(), "run-1", PlayResult[testSessionKey]{
		Key:    key,
		Status: PlayCompleted,
	})
	if err != nil {
		t.Fatalf("finish play: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "run-1" {
		t.Fatalf("deleted = %#v", store.deleted)
	}
}

func TestCloseRejectsActiveSession(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key, active: true},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	err := compute.Close(context.Background())
	if err != ErrSessionActive {
		t.Fatalf("error = %v, want ErrSessionActive", err)
	}
	if _, ok := store.sessions["run-1"]; !ok {
		t.Fatal("active close should not clear sessions")
	}
	if !store.sessions["run-1"].active {
		t.Fatal("active close should not release session")
	}
}

func TestCloseClearsIdleSessions(t *testing.T) {
	key := testSessionKey{id: "run-1"}
	store := newTestSessionStore(map[string]*Session[testSessionKey]{
		"run-1": {guestData: key},
	})
	compute := &ComputeCompiledPlugin[string, testSessionKey]{sessionStore: store}

	if err := compute.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	if len(store.sessions) != 0 {
		t.Fatalf("sessions = %d, want 0", len(store.sessions))
	}
}
