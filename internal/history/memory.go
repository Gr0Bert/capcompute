package history

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type MemoryStore struct {
	mu   sync.Mutex
	runs map[string]*memoryRun
}

type memoryRun struct {
	run    Run
	events []Event
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{runs: make(map[string]*memoryRun)}
}

func (s *MemoryStore) CreateRun(_ context.Context, run Run, events ...Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.runs[run.ID]; ok {
		return fmt.Errorf("run %q already exists", run.ID)
	}
	run.Version = int64(len(events))
	copiedEvents := copyEvents(events)
	for i := range copiedEvents {
		copiedEvents[i].Seq = int64(i + 1)
		copiedEvents[i].RunID = run.ID
	}
	s.runs[run.ID] = &memoryRun{
		run:    copyRun(run),
		events: copiedEvents,
	}
	return nil
}

func (s *MemoryStore) LoadRun(_ context.Context, runID string) (Run, []Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.runs[runID]
	if !ok {
		return Run{}, nil, fmt.Errorf("run %q not found", runID)
	}
	return copyRun(stored.run), copyEvents(stored.events), nil
}

func (s *MemoryStore) Append(_ context.Context, runID string, expectedVersion int64, events ...Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.runs[runID]
	if !ok {
		return fmt.Errorf("run %q not found", runID)
	}
	if stored.run.Version != expectedVersion {
		return fmt.Errorf("history version mismatch for run %q: expected %d got %d", runID, expectedVersion, stored.run.Version)
	}

	for _, event := range events {
		event.Seq = stored.run.Version + 1
		event.RunID = runID
		stored.events = append(stored.events, copyEvent(event))
		stored.run.Version++
	}
	return nil
}

func (s *MemoryStore) MarkComplete(_ context.Context, runID string, _ json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.runs[runID]
	if !ok {
		return fmt.Errorf("run %q not found", runID)
	}
	stored.run.Status = "completed"
	return nil
}

func copyRun(run Run) Run {
	run.Input = append(json.RawMessage(nil), run.Input...)
	return run
}

func copyEvents(events []Event) []Event {
	copied := make([]Event, len(events))
	for i, event := range events {
		copied[i] = copyEvent(event)
	}
	return copied
}

func copyEvent(event Event) Event {
	event.Result = append(json.RawMessage(nil), event.Result...)
	return event
}
