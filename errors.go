package capruntime

import (
	"errors"
	"fmt"
)

var (
	// ErrRuntimeRequired means Tick was called before configuring a runtime backend.
	ErrRuntimeRequired = errors.New("runtime is required")
	// ErrRunIDRequired means an invocation did not provide a durable run ID.
	ErrRunIDRequired = errors.New("run id is required")
	// ErrModuleRequired means an invocation did not pin a module digest.
	ErrModuleRequired = errors.New("module digest is required")
)

// LimitError means a run exceeded a configured runtime limit.
type LimitError struct {
	Name  string
	Limit int
	Got   int
}

func (e LimitError) Error() string {
	return fmt.Sprintf("%s limit exceeded: got %d, limit %d", e.Name, e.Got, e.Limit)
}

// DeniedCommandError means capability policy rejected an emitted command.
type DeniedCommandError struct {
	CommandName string
	Reason      string
}

func (e DeniedCommandError) Error() string {
	return fmt.Sprintf("command %q denied: %s", e.CommandName, e.Reason)
}

// CommandStateError means a manual command operation is invalid for its state.
type CommandStateError struct {
	CommandID string
	State     string
	Message   string
}

func (e CommandStateError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("command %q is %s", e.CommandID, e.State)
}

// NondeterminismError means replay detected a different command sequence or identity.
type NondeterminismError struct {
	CommandID string
	Err       error
}

func (e NondeterminismError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("nondeterministic command %q", e.CommandID)
	}
	return e.Err.Error()
}

func (e NondeterminismError) Unwrap() error {
	return e.Err
}
