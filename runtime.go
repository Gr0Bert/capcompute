package capruntime

import (
	"context"
	"encoding/json"
)

type Runtime interface {
	Invoke(ctx context.Context, req RuntimeRequest) (RuntimeResult, error)
}

type RuntimeRequest struct {
	RunID          string
	Module         ModuleRef
	Input          json.RawMessage
	CommandResults []CommandResult
	Limits         RuntimeLimits
}

type RuntimeResult struct {
	Status  RuntimeStatus
	Output  json.RawMessage
	Command Command
	Message string
}

type RuntimeStatus string

const (
	RuntimeCompleted RuntimeStatus = "completed"
	RuntimeCommand   RuntimeStatus = "command"
	RuntimeFailed    RuntimeStatus = "failed"
)
