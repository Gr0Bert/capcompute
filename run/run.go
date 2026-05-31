package run

import (
	"encoding/json"

	"capcompute/module"
)

// Status is the durable lifecycle state of a workflow run.
type Status string

const (
	Running   Status = "running"
	Completed Status = "completed"
	Failed    Status = "failed"
)

// TickStatus describes what happened during one engine tick.
type TickStatus string

const (
	TickRunning   TickStatus = "running"
	TickCompleted TickStatus = "completed"
	TickFailed    TickStatus = "failed"
)

// Invocation is the public request used to start a workflow run.
// It carries caller identity, source identity, module identity, and deterministic input.
type Invocation struct {
	RunID     string
	Module    module.Ref
	Principal Principal
	Source    Source
	Input     json.RawMessage
	Limits    RuntimeLimits
}

// Run is the public view of a stored workflow run.
type Run struct {
	ID             string
	Module         module.Ref
	Principal      Principal
	Source         Source
	Status         Status
	FailureMessage string
	Output         json.RawMessage
	Version        int64
}

// Filter limits run listing. A zero-value filter returns all runs.
type Filter struct {
	Status Status
}

// Principal identifies who or what is asking the runtime to execute work.
type Principal struct {
	Type string
	ID   string
}

// Source identifies the adapter or system that produced the invocation.
type Source struct {
	Type string
	ID   string
}

// RuntimeLimits carries per-run execution limits that runtime backends can enforce.
type RuntimeLimits struct {
	TimeoutMillis      int
	MemoryMaxPages     int
	MaxInputBytes      int
	MaxOutputBytes     int
	MaxCommandsPerTick int
	MaxReplaySteps     int
}

// TickResult is the public result of advancing a workflow run once.
type TickResult struct {
	RunID     string
	Status    TickStatus
	Output    json.RawMessage
	CommandID string
	Error     error
}
