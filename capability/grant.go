package capability

import (
	"capcompute/command"
	"capcompute/module"
	"capcompute/run"
)

// Grant is public configuration that allows a principal and module digest to issue a command.
// Empty optional fields match any value.
type Grant struct {
	PrincipalType  string
	PrincipalID    string
	SourceType     string
	SourceID       string
	ModuleDigest   string
	CommandID      string
	CommandName    string
	CommandMode    command.Mode
	CommandArgsSHA string
}

// Request contains the facts needed to decide whether a command may run.
type Request struct {
	RunID          string
	Principal      run.Principal
	Source         run.Source
	Module         module.Ref
	Command        command.Command
	CommandArgsSHA string
}

// Decision is the authorization result returned by a Broker.
type Decision struct {
	Allowed bool
	Reason  string
}

// Broker authorizes commands before their handlers can execute side effects.
type Broker interface {
	Authorize(req Request) Decision
}
