package runtime

import (
	"context"
	"encoding/json"

	"capcompute/command"
	"capcompute/module"
	"capcompute/run"
)

// Runtime invokes workflow code and reports either completion, failure, or one emitted command.
type Runtime interface {
	Invoke(ctx context.Context, req Request) (Result, error)
}

// Request is the host-to-runtime invocation envelope for one replay/tick.
type Request struct {
	RunID          string
	Module         module.Ref
	Input          json.RawMessage
	CommandResults []command.Result
	Limits         run.RuntimeLimits
}

// Result is the runtime-to-engine response for one invocation.
type Result struct {
	Status  Status
	Output  json.RawMessage
	Command command.Command
	Message string
}

// Status tells the engine how to handle the runtime result.
type Status string

const (
	Completed Status = "completed"
	Command   Status = "command"
	Failed    Status = "failed"
)
