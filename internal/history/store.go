package history

import (
	"context"
	"encoding/json"
)

type Run struct {
	ID               string
	ModuleName       string
	ModuleDigest     string
	ModuleEntrypoint string
	PrincipalType    string
	PrincipalID      string
	SourceType       string
	SourceID         string
	Status           string
	Input            json.RawMessage
	Version          int64
}

type Store interface {
	CreateRun(ctx context.Context, run Run, events ...Event) error
	LoadRun(ctx context.Context, runID string) (Run, []Event, error)
	Append(ctx context.Context, runID string, expectedVersion int64, events ...Event) error
	MarkComplete(ctx context.Context, runID string, result json.RawMessage) error
}
