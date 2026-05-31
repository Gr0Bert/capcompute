package replay

import (
	"capcompute/history"
	"capcompute/internal/command"
)

// Event is the store event type replay matching consumes.
type Event = history.Event

// Command is the internal command identity replay matching consumes.
type Command = command.Command
