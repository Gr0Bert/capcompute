package approval

import "capcompute/command"

// Request is a pending command presented as an approval request.
type Request struct {
	RunID   string
	Command command.PendingCommand
}
