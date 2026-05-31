package capruntime

import (
	"context"
	"encoding/json"
)

type CommandMode string

const (
	CommandModeQuery   CommandMode = "query"
	CommandModeCommand CommandMode = "command"
)

type Command struct {
	ID   string
	Name string
	Mode CommandMode
	Args json.RawMessage
}

type CommandResult struct {
	ID     string
	Name   string
	Result json.RawMessage
}

type CommandRequest struct {
	RunID          string
	Module         ModuleRef
	Principal      Principal
	Source         Source
	Command        Command
	ArgsHash       string
	IdempotencyKey string
}

type CommandReceipt struct {
	Result json.RawMessage
}

type CommandHandler interface {
	Execute(ctx context.Context, req CommandRequest) (CommandReceipt, error)
}
