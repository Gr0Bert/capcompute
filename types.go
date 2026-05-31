package capruntime

import "encoding/json"

type RunStatus string

const (
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
)

type TickStatus string

const (
	TickRunning   TickStatus = "running"
	TickCompleted TickStatus = "completed"
	TickFailed    TickStatus = "failed"
)

type InvocationRequest struct {
	RunID     string
	Module    ModuleRef
	Principal Principal
	Source    Source
	Input     json.RawMessage
	Limits    RuntimeLimits
}

type Run struct {
	ID        string
	Module    ModuleRef
	Principal Principal
	Source    Source
	Status    RunStatus
	Version   int64
}

type ModuleRef struct {
	Name       string
	Digest     string
	Entrypoint string
	Source     string
}

type Principal struct {
	Type string
	ID   string
}

type Source struct {
	Type string
	ID   string
}

type RuntimeLimits struct {
	TimeoutMillis      int
	MemoryMaxPages     int
	MaxInputBytes      int
	MaxOutputBytes     int
	MaxCommandsPerTick int
	MaxReplaySteps     int
}

type TickResult struct {
	RunID     string
	Status    TickStatus
	Output    json.RawMessage
	CommandID string
	Error     error
}
