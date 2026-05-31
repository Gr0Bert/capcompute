package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"capcompute/history"
)

// MemoryStore is an in-memory Store implementation for tests and local bootstrapping.
// It does not provide crash recovery.
type MemoryStore struct {
	mu   sync.Mutex
	runs map[string]*memoryRun
}

var _ history.Store = (*MemoryStore)(nil)

type memoryRun struct {
	run    history.Run
	events []history.Event
}

// NewMemoryStore creates an empty in-memory workflow store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{runs: make(map[string]*memoryRun)}
}

func (s *MemoryStore) CreateRun(_ context.Context, run history.Run, events ...history.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.runs[run.ID]; ok {
		return fmt.Errorf("run %q already exists", run.ID)
	}
	if err := checkSchema("run", run.SchemaVersion); err != nil {
		return err
	}
	for _, event := range events {
		if err := checkSchema("event", event.SchemaVersion); err != nil {
			return err
		}
	}
	run.SchemaVersion = schemaVersion(run.SchemaVersion)
	run.Version = int64(len(events))
	copiedEvents := copyEvents(events)
	for i := range copiedEvents {
		copiedEvents[i].SchemaVersion = schemaVersion(copiedEvents[i].SchemaVersion)
		copiedEvents[i].Seq = int64(i + 1)
		copiedEvents[i].RunID = run.ID
	}
	s.runs[run.ID] = &memoryRun{
		run:    copyRun(run),
		events: copiedEvents,
	}
	return nil
}

func (s *MemoryStore) LoadRun(_ context.Context, runID string) (history.Run, []history.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.runs[runID]
	if !ok {
		return history.Run{}, nil, history.NotFoundError{RunID: runID}
	}
	return copyRun(stored.run), copyEvents(stored.events), nil
}

func (s *MemoryStore) ListRuns(_ context.Context, filter history.RunFilter) ([]history.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	runs := make([]history.Run, 0, len(s.runs))
	for _, stored := range s.runs {
		if filter.Status != "" && stored.run.Status != filter.Status {
			continue
		}
		runs = append(runs, copyRun(stored.run))
	}
	return runs, nil
}

func (s *MemoryStore) Append(_ context.Context, runID string, expectedVersion int64, events ...history.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.appendLocked(runID, expectedVersion, events...)
}

func (s *MemoryStore) Complete(_ context.Context, runID string, expectedVersion int64, result json.RawMessage, events ...history.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.runs[runID]
	if !ok {
		return history.NotFoundError{RunID: runID}
	}
	if err := s.appendLocked(runID, expectedVersion, events...); err != nil {
		return err
	}
	stored.run.Status = "completed"
	stored.run.Output = append(json.RawMessage(nil), result...)
	return nil
}

func (s *MemoryStore) Fail(_ context.Context, runID string, expectedVersion int64, message string, events ...history.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.runs[runID]
	if !ok {
		return history.NotFoundError{RunID: runID}
	}
	if err := s.appendLocked(runID, expectedVersion, events...); err != nil {
		return err
	}
	stored.run.Status = "failed"
	stored.run.FailureMessage = message
	return nil
}

func (s *MemoryStore) appendLocked(runID string, expectedVersion int64, events ...history.Event) error {
	stored, ok := s.runs[runID]
	if !ok {
		return history.NotFoundError{RunID: runID}
	}
	if stored.run.Version != expectedVersion {
		return history.VersionConflictError{RunID: runID, Expected: expectedVersion, Got: stored.run.Version}
	}

	for _, event := range events {
		if err := checkSchema("event", event.SchemaVersion); err != nil {
			return err
		}
		event.SchemaVersion = schemaVersion(event.SchemaVersion)
		event.Seq = stored.run.Version + 1
		event.RunID = runID
		stored.events = append(stored.events, copyEvent(event))
		stored.run.Version++
	}
	return nil
}

func copyRun(run history.Run) history.Run {
	run.SchemaVersion = schemaVersion(run.SchemaVersion)
	run.Input = append(json.RawMessage(nil), run.Input...)
	run.Output = append(json.RawMessage(nil), run.Output...)
	return run
}

func copyEvents(events []history.Event) []history.Event {
	copied := make([]history.Event, len(events))
	for i, event := range events {
		copied[i] = copyEvent(event)
	}
	return copied
}

func copyEvent(event history.Event) history.Event {
	event.SchemaVersion = schemaVersion(event.SchemaVersion)
	event.Result = append(json.RawMessage(nil), event.Result...)
	return event
}

func schemaVersion(version int) int {
	if version == 0 {
		return history.SchemaVersion
	}
	return version
}

func checkSchema(entity string, version int) error {
	if version > history.SchemaVersion {
		return history.UnsupportedSchemaError{Entity: entity, Version: version, Current: history.SchemaVersion}
	}
	return nil
}
