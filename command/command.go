package command

import (
	"context"
	"encoding/json"
	"errors"

	"capcompute/module"
	"capcompute/run"
)

// Mode classifies the command so the host can apply the right replay and safety rules.
type Mode string

const (
	ModeQuery   Mode = "query"
	ModeCommand Mode = "command"
)

// Command is the public command envelope emitted by workflow code.
// All outside-world work must pass through this shape.
type Command struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Mode Mode            `json:"mode"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Result is a recorded command result returned to the runtime during replay.
type Result struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Mode     Mode            `json:"mode"`
	ArgsHash string          `json:"args_hash"`
	Result   json.RawMessage `json:"result,omitempty"`
}

// Request is passed to a host command handler after replay and capability checks.
type Request struct {
	RunID          string
	Module         module.Ref
	Principal      run.Principal
	Source         run.Source
	Command        Command
	ArgsHash       string
	IdempotencyKey string
}

// Receipt is the handler output recorded in event history.
type Receipt struct {
	Result json.RawMessage
}

// UnknownPolicy describes how ambiguous command outcomes are handled.
type UnknownPolicy string

const (
	// UnknownQuarantine stops automatic retry and requires manual recovery.
	UnknownQuarantine UnknownPolicy = "quarantine"
)

// Safety describes side-effect and recovery expectations for a command handler.
type Safety struct {
	SideEffecting          bool
	RequiresIdempotencyKey bool
	UnknownPolicy          UnknownPolicy
}

// UnknownError means a handler cannot prove whether the external side effect happened.
// The engine records CommandUnknown and stops automatic retry.
type UnknownError struct {
	Err error
}

func (e UnknownError) Error() string {
	if e.Err == nil {
		return "command outcome unknown"
	}
	return "command outcome unknown: " + e.Err.Error()
}

func (e UnknownError) Unwrap() error {
	return e.Err
}

// Unknown wraps an ambiguous handler error so the engine quarantines the command.
func Unknown(err error) error {
	if err == nil {
		err = errors.New("unknown command outcome")
	}
	return UnknownError{Err: err}
}

// PendingError means a command is waiting on something external, such as approval or a timer.
// The engine records CommandPending and does not re-execute the handler while it remains pending.
type PendingError struct {
	Reason string
}

func (e PendingError) Error() string {
	if e.Reason == "" {
		return "command pending"
	}
	return "command pending: " + e.Reason
}

// Pending creates a handler error that pauses command execution without marking failure.
func Pending(reason string) error {
	return PendingError{Reason: reason}
}

// Handler executes one host command name.
// Implementations own side effects and should use the provided idempotency key where possible.
type Handler interface {
	Safety() Safety
	Execute(ctx context.Context, req Request) (Receipt, error)
}
