package history

import (
	"context"
	"encoding/json"
)

// SchemaVersion is the current durable history schema version.
const SchemaVersion = 1

// EventType names a durable fact recorded about a workflow run.
type EventType string

const (
	WorkflowStarted        EventType = "WorkflowStarted"
	WorkflowCompleted      EventType = "WorkflowCompleted"
	WorkflowFailed         EventType = "WorkflowFailed"
	CommandScheduled       EventType = "CommandScheduled"
	CommandStarted         EventType = "CommandStarted"
	CommandCompleted       EventType = "CommandCompleted"
	CommandDenied          EventType = "CommandDenied"
	CommandFailed          EventType = "CommandFailed"
	CommandPending         EventType = "CommandPending"
	CommandUnknown         EventType = "CommandUnknown"
	NondeterminismDetected EventType = "NondeterminismDetected"
)

// Event is one durable history entry used for replay and recovery.
type Event struct {
	SchemaVersion  int
	Seq            int64
	Type           EventType
	RunID          string
	ModuleDigest   string
	CommandID      string
	CommandName    string
	CommandMode    string
	CommandArgsSHA string
	Result         json.RawMessage
	Message        string
}

// Run is the persisted form of a workflow run.
type Run struct {
	SchemaVersion      int
	ID                 string
	ModuleName         string
	ModuleDigest       string
	ModuleEntrypoint   string
	ModuleSource       string
	PrincipalType      string
	PrincipalID        string
	SourceType         string
	SourceID           string
	Status             string
	FailureMessage     string
	Input              json.RawMessage
	Output             json.RawMessage
	TimeoutMillis      int
	MemoryMaxPages     int
	MaxInputBytes      int
	MaxOutputBytes     int
	MaxCommandsPerTick int
	MaxReplaySteps     int
	Version            int64
}

// RunFilter limits run listing in stores. A zero-value filter returns all runs.
type RunFilter struct {
	Status string
}

// Store persists runs and ordered history events.
// Append uses expectedVersion so competing workers cannot write conflicting histories.
type Store interface {
	CreateRun(ctx context.Context, run Run, events ...Event) error
	LoadRun(ctx context.Context, runID string) (Run, []Event, error)
	ListRuns(ctx context.Context, filter RunFilter) ([]Run, error)
	Append(ctx context.Context, runID string, expectedVersion int64, events ...Event) error
	Complete(ctx context.Context, runID string, expectedVersion int64, result json.RawMessage, events ...Event) error
	Fail(ctx context.Context, runID string, expectedVersion int64, message string, events ...Event) error
}
