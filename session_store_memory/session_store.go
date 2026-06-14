package session_store_memory

import (
	"capcompute"
	"context"
	"sync"
)

type Store[ID comparable, K capcompute.SessionKey[ID]] struct {
	mu       sync.Mutex
	sessions map[ID]*capcompute.Session[K]
	active   map[ID]struct{}
}

func New[ID comparable, K capcompute.SessionKey[ID]]() *Store[ID, K] {
	return &Store[ID, K]{
		sessions: make(map[ID]*capcompute.Session[K]),
		active:   make(map[ID]struct{}),
	}
}

func (s *Store[ID, K]) LoadSession(_ context.Context, sessionID ID) (*capcompute.Session[K], error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, capcompute.ErrSessionRequired
	}
	return session, nil
}

func (s *Store[ID, K]) SaveSession(_ context.Context, sessionID ID, session *capcompute.Session[K]) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sessions == nil {
		s.sessions = make(map[ID]*capcompute.Session[K])
	}
	s.sessions[sessionID] = session
	return nil
}

func (s *Store[ID, K]) DeleteSession(_ context.Context, sessionID ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionID)
	delete(s.active, sessionID)
	return nil
}

func (s *Store[ID, K]) ListSessions(context.Context) (map[ID]*capcompute.Session[K], error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessions := make(map[ID]*capcompute.Session[K], len(s.sessions))
	for sessionID, session := range s.sessions {
		sessions[sessionID] = session
	}
	return sessions, nil
}

func (s *Store[ID, K]) BeginSession(_ context.Context, sessionID ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return capcompute.ErrSessionRequired
	}
	if _, ok := s.active[sessionID]; ok {
		return capcompute.ErrSessionActive
	}
	if s.active == nil {
		s.active = make(map[ID]struct{})
	}
	s.active[sessionID] = struct{}{}
	return nil
}

func (s *Store[ID, K]) EndSession(_ context.Context, sessionID ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.active, sessionID)
	return nil
}

func (s *Store[ID, K]) IsSessionActive(_ context.Context, sessionID ID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return false, capcompute.ErrSessionRequired
	}
	_, ok := s.active[sessionID]
	return ok, nil
}
