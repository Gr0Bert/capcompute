package extism2

import (
	"encoding/json"
	"fmt"
)

// Call is the guest-to-host function request.
type Call struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// OutcomeKind identifies a handler or replay result.
type OutcomeKind string

const (
	OutcomeResult  OutcomeKind = "result"
	OutcomeYield   OutcomeKind = "yield"
	OutcomeUnknown OutcomeKind = "unknown"
	OutcomeFailed  OutcomeKind = "failed"
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

func Unknown(message string) Outcome {
	if message == "" {
		message = "command outcome unknown"
	}
	return Outcome{kind: OutcomeUnknown, message: message}
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

func copyCall(call Call) Call {
	call.Args = append(json.RawMessage(nil), call.Args...)
	return call
}

func copyOutcome(outcome Outcome) Outcome {
	outcome.result = append(json.RawMessage(nil), outcome.result...)
	return outcome
}

func validOutcome(outcome Outcome) bool {
	switch outcome.Kind() {
	case OutcomeResult, OutcomeYield, OutcomeUnknown, OutcomeFailed:
		return true
	default:
		return false
	}
}

func terminalOutcomeError(outcome Outcome) error {
	switch outcome.Kind() {
	case OutcomeUnknown, OutcomeFailed:
		return fmt.Errorf("%s: %s", outcome.Kind(), outcome.Message())
	default:
		return nil
	}
}
