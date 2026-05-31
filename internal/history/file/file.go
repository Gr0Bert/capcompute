package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"capcompute/history"
)

// Store persists runs and history events into one JSON file for local durability.
// It is intended for bootstrapping and tests, not high-concurrency production use.
type Store struct {
	mu   sync.Mutex
	path string
}

var _ history.Store = (*Store)(nil)

type diskState struct {
	SchemaVersion int                `json:"schema_version"`
	Runs          map[string]diskRun `json:"runs"`
}

type diskRun struct {
	Run    history.Run     `json:"run"`
	Events []history.Event `json:"events"`
}

// NewStore creates a file-backed history store at path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) CreateRun(_ context.Context, run history.Run, events ...history.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}
	if _, ok := state.Runs[run.ID]; ok {
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
	state.Runs[run.ID] = diskRun{
		Run:    copyRun(run),
		Events: copiedEvents,
	}
	return s.saveLocked(state)
}

func (s *Store) LoadRun(_ context.Context, runID string) (history.Run, []history.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return history.Run{}, nil, err
	}
	stored, ok := state.Runs[runID]
	if !ok {
		return history.Run{}, nil, history.NotFoundError{RunID: runID}
	}
	return copyRun(stored.Run), copyEvents(stored.Events), nil
}

func (s *Store) ListRuns(_ context.Context, filter history.RunFilter) ([]history.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return nil, err
	}

	runs := make([]history.Run, 0, len(state.Runs))
	for _, stored := range state.Runs {
		if filter.Status != "" && stored.Run.Status != filter.Status {
			continue
		}
		runs = append(runs, copyRun(stored.Run))
	}
	return runs, nil
}

func (s *Store) Append(_ context.Context, runID string, expectedVersion int64, events ...history.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}
	if err := appendEvents(state, runID, expectedVersion, events...); err != nil {
		return err
	}
	return s.saveLocked(state)
}

func (s *Store) Complete(_ context.Context, runID string, expectedVersion int64, result json.RawMessage, events ...history.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}
	if err := appendEvents(state, runID, expectedVersion, events...); err != nil {
		return err
	}
	stored := state.Runs[runID]
	stored.Run.Status = "completed"
	stored.Run.Output = append(json.RawMessage(nil), result...)
	state.Runs[runID] = stored
	return s.saveLocked(state)
}

func (s *Store) Fail(_ context.Context, runID string, expectedVersion int64, message string, events ...history.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadLocked()
	if err != nil {
		return err
	}
	if err := appendEvents(state, runID, expectedVersion, events...); err != nil {
		return err
	}
	stored := state.Runs[runID]
	stored.Run.Status = "failed"
	stored.Run.FailureMessage = message
	state.Runs[runID] = stored
	return s.saveLocked(state)
}

func appendEvents(state diskState, runID string, expectedVersion int64, events ...history.Event) error {
	stored, ok := state.Runs[runID]
	if !ok {
		return history.NotFoundError{RunID: runID}
	}
	if stored.Run.Version != expectedVersion {
		return history.VersionConflictError{RunID: runID, Expected: expectedVersion, Got: stored.Run.Version}
	}

	for _, event := range events {
		if err := checkSchema("event", event.SchemaVersion); err != nil {
			return err
		}
		event.SchemaVersion = schemaVersion(event.SchemaVersion)
		event.Seq = stored.Run.Version + 1
		event.RunID = runID
		stored.Events = append(stored.Events, copyEvent(event))
		stored.Run.Version++
	}
	state.Runs[runID] = stored
	return nil
}

func (s *Store) loadLocked() (diskState, error) {
	if s.path == "" {
		return diskState{}, fmt.Errorf("history file path is required")
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return diskState{SchemaVersion: history.SchemaVersion, Runs: make(map[string]diskRun)}, nil
		}
		return diskState{}, fmt.Errorf("read history file: %w", err)
	}
	if len(data) == 0 {
		return diskState{SchemaVersion: history.SchemaVersion, Runs: make(map[string]diskRun)}, nil
	}

	var state diskState
	if err := json.Unmarshal(data, &state); err != nil {
		return diskState{}, fmt.Errorf("decode history file: %w", err)
	}
	if err := checkSchema("history file", state.SchemaVersion); err != nil {
		return diskState{}, err
	}
	if state.Runs == nil {
		state.Runs = make(map[string]diskRun)
	}
	state.SchemaVersion = schemaVersion(state.SchemaVersion)
	for _, stored := range state.Runs {
		if err := checkSchema("run", stored.Run.SchemaVersion); err != nil {
			return diskState{}, err
		}
		for _, event := range stored.Events {
			if err := checkSchema("event", event.SchemaVersion); err != nil {
				return diskState{}, err
			}
		}
	}
	return state, nil
}

func (s *Store) saveLocked(state diskState) error {
	state.SchemaVersion = schemaVersion(state.SchemaVersion)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create history directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode history file: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write history file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace history file: %w", err)
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
