package dispatcher

import (
	"encoding/json"
)

// Call is the guest-to-host function request.
type Call struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

func (call Call) Copy() Call {
	call.Args = append(json.RawMessage(nil), call.Args...)
	return call
}

// OutcomeKind identifies a handler or replay result.
type OutcomeKind string

const (
	OutcomeResult OutcomeKind = "result"
	OutcomeYield  OutcomeKind = "yield"
	OutcomeFailed OutcomeKind = "failed"
)

// Outcome is the ADT returned to guest calls.
type Outcome struct {
	kind    OutcomeKind
	result  json.RawMessage
	message string
}

func Result(result json.RawMessage) Outcome {
	return Outcome{kind: OutcomeResult, result: append(json.RawMessage(nil), result...)}
}

func Yield(message string) Outcome {
	return Outcome{kind: OutcomeYield, message: message}
}

func Failed(message string) Outcome {
	if message == "" {
		message = "command failed"
	}
	return Outcome{kind: OutcomeFailed, message: message}
}

func (o Outcome) Kind() OutcomeKind {
	return o.kind
}

func (o Outcome) Result() json.RawMessage {
	return append(json.RawMessage(nil), o.result...)
}

func (o Outcome) Message() string {
	return o.message
}

func (o Outcome) Copy() Outcome {
	o.result = append(json.RawMessage(nil), o.result...)
	return o
}

func (o Outcome) IsValid() bool {
	switch o.Kind() {
	case OutcomeResult, OutcomeYield, OutcomeFailed:
		return true
	default:
		return false
	}
}
