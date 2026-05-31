package command

// PendingCommand is a command waiting for an external result.
type PendingCommand struct {
	RunID    string
	ID       string
	Name     string
	Mode     Mode
	ArgsHash string
	Reason   string
}

// UnknownCommand is a command whose external outcome could not be proven.
type UnknownCommand struct {
	RunID    string
	ID       string
	Name     string
	Mode     Mode
	ArgsHash string
	Reason   string
}
