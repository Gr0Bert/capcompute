package guest

import "encoding/json"

// Mode classifies command safety for the host engine.
type Mode string

const (
	ModeQuery   Mode = "query"
	ModeCommand Mode = "command"
)

// ModuleRef identifies the workflow module the host is invoking.
type ModuleRef struct {
	Name       string `json:"name"`
	Digest     string `json:"digest"`
	Entrypoint string `json:"entrypoint"`
	Source     string `json:"source"`
}

// Invocation is the input envelope the host passes to the workflow export.
type Invocation struct {
	RunID          string          `json:"run_id"`
	Module         ModuleRef       `json:"module"`
	Input          json.RawMessage `json:"input"`
	CommandResults []CommandResult `json:"command_results,omitempty"`
}

// Command is the guest-to-host command envelope.
type Command struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Mode Mode            `json:"mode"`
	Args json.RawMessage `json:"args,omitempty"`
}

// CommandResult is a replayed result for a previously completed command.
type CommandResult struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Mode     Mode            `json:"mode"`
	ArgsHash string          `json:"args_hash"`
	Result   json.RawMessage `json:"result,omitempty"`
}
