package strict

import (
	"fmt"

	"capcompute/history"
	"capcompute/internal/replay"
)

// StrictMatcher requires a repeated command ID to keep the same command name and args hash.
type StrictMatcher struct{}

var _ replay.Matcher = (*StrictMatcher)(nil)

// NewStrictMatcher creates the default V1 replay matcher.
func NewStrictMatcher() *StrictMatcher {
	return &StrictMatcher{}
}

func (StrictMatcher) Match(events []replay.Event, cmd replay.Command) (replay.Match, error) {
	var found bool
	var completed bool
	var pending bool
	var unknown bool
	var failed bool

	if err := requireSameUnresolvedCommand(events, cmd); err != nil {
		return replay.Match{}, err
	}

	for _, event := range events {
		if event.Type != history.CommandScheduled {
			continue
		}
		if event.CommandID != cmd.ID {
			continue
		}
		found = true
		if event.CommandName != cmd.Name || event.CommandArgsSHA != cmd.ArgsHash {
			return replay.Match{}, fmt.Errorf("nondeterministic command %q: expected %s %s got %s %s",
				cmd.ID,
				event.CommandName,
				event.CommandArgsSHA,
				cmd.Name,
				cmd.ArgsHash,
			)
		}
	}

	if !found {
		return replay.Match{}, nil
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
		case history.CommandUnknown:
			unknown = true
		case history.CommandFailed:
			failed = true
		}
	}

	return replay.Match{Completed: completed, Pending: pending, Unknown: unknown, Failed: failed}, nil
}

func requireSameUnresolvedCommand(events []replay.Event, cmd replay.Command) error {
	status := make(map[string]replay.Match)
	order := make([]string, 0)

	for _, event := range events {
		if event.CommandID == "" {
			continue
		}
		if _, ok := status[event.CommandID]; !ok {
			order = append(order, event.CommandID)
		}
		match := status[event.CommandID]
		switch event.Type {
		case history.CommandCompleted:
			match.Completed = true
		case history.CommandPending:
			match.Pending = true
		case history.CommandUnknown:
			match.Unknown = true
		case history.CommandFailed:
			match.Failed = true
		}
		status[event.CommandID] = match
	}

	for _, commandID := range order {
		match := status[commandID]
		if match.Completed {
			continue
		}
		if !match.Pending && !match.Unknown && !match.Failed {
			continue
		}
		if commandID != cmd.ID {
			return fmt.Errorf("nondeterministic command order: unresolved command %q before emitted command %q", commandID, cmd.ID)
		}
	}
	return nil
}
