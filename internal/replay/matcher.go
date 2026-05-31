package replay

import (
	"fmt"

	"capcompute/internal/command"
	"capcompute/internal/history"
)

type Matcher struct{}

type Match struct {
	Completed bool
	Pending   bool
}

func NewMatcher() Matcher {
	return Matcher{}
}

func (Matcher) Match(events []history.Event, cmd command.Command) (Match, error) {
	var found bool
	var completed bool
	var pending bool

	for _, event := range events {
		if event.Type != history.CommandScheduled {
			continue
		}
		if event.CommandID != cmd.ID {
			continue
		}
		found = true
		if event.CommandName != cmd.Name || event.CommandArgsSHA != cmd.ArgsHash {
			return Match{}, fmt.Errorf("nondeterministic command %q: expected %s %s got %s %s",
				cmd.ID,
				event.CommandName,
				event.CommandArgsSHA,
				cmd.Name,
				cmd.ArgsHash,
			)
		}
	}

	if !found {
		return Match{}, nil
	}

	for _, event := range events {
		if event.CommandID != cmd.ID {
			continue
		}
		switch event.Type {
		case history.CommandCompleted:
			completed = true
		case history.CommandPending:
			pending = true
		}
	}

	return Match{Completed: completed, Pending: pending}, nil
}
