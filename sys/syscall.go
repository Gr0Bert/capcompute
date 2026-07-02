package sys

import (
	"encoding/json"
)

// Syscall is the guest-to-host request crossing the syscall boundary.
type Syscall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

func (sc Syscall) Copy() Syscall {
	sc.Args = append(json.RawMessage(nil), sc.Args...)
	return sc
}

// SyscallStatus identifies a handler or replay result.
type SyscallStatus string

const (
	StatusResult SyscallStatus = "result"
	StatusYield  SyscallStatus = "yield"
	StatusFailed SyscallStatus = "failed"
)

// SyscallResult is the ADT returned to guest syscalls.
type SyscallResult struct {
	status  SyscallStatus
	result  json.RawMessage
	message string
}

func Result(result json.RawMessage) SyscallResult {
	return SyscallResult{status: StatusResult, result: append(json.RawMessage(nil), result...)}
}

func Yield(message string) SyscallResult {
	return SyscallResult{status: StatusYield, message: message}
}

func Fail(message string) SyscallResult {
	if message == "" {
		message = "command failed"
	}
	return SyscallResult{status: StatusFailed, message: message}
}

func (r SyscallResult) Status() SyscallStatus {
	return r.status
}

func (r SyscallResult) Result() json.RawMessage {
	return append(json.RawMessage(nil), r.result...)
}

func (r SyscallResult) Message() string {
	return r.message
}

func (r SyscallResult) Copy() SyscallResult {
	r.result = append(json.RawMessage(nil), r.result...)
	return r
}
