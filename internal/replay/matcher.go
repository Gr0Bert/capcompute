package replay

// Match describes whether an emitted command already exists in history.
type Match struct {
	Completed bool
	Pending   bool
	Unknown   bool
	Failed    bool
}

// Matcher compares emitted commands with history to detect replay divergence.
type Matcher interface {
	Match(events []Event, cmd Command) (Match, error)
}
