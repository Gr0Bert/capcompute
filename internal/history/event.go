package history

import "encoding/json"

type EventType string

const (
	WorkflowStarted        EventType = "WorkflowStarted"
	WorkflowCompleted      EventType = "WorkflowCompleted"
	WorkflowFailed         EventType = "WorkflowFailed"
	CommandScheduled       EventType = "CommandScheduled"
	CommandStarted         EventType = "CommandStarted"
	CommandCompleted       EventType = "CommandCompleted"
	CommandFailed          EventType = "CommandFailed"
	CommandPending         EventType = "CommandPending"
	CommandUnknown         EventType = "CommandUnknown"
	NondeterminismDetected EventType = "NondeterminismDetected"
)

type Event struct {
	Seq            int64
	Type           EventType
	RunID          string
	ModuleDigest   string
	CommandID      string
	CommandName    string
	CommandMode    string
	CommandArgsSHA string
	Result         json.RawMessage
	Message        string
}
