package capruntime

import (
	"context"
	"encoding/json"
	"sort"

	publicapproval "capcompute/approval"
	publiccommand "capcompute/command"
	historystore "capcompute/history"
	publicrun "capcompute/run"
)

// ListRuns returns stored runs that match the filter.
func (e *Engine) ListRuns(ctx context.Context, filter publicrun.Filter) ([]publicrun.Run, error) {
	runs, err := e.store.ListRuns(ctx, historystore.RunFilter{Status: string(filter.Status)})
	if err != nil {
		return nil, err
	}

	publicRuns := make([]publicrun.Run, 0, len(runs))
	for _, run := range runs {
		publicRuns = append(publicRuns, publicRun(run))
	}
	sort.Slice(publicRuns, func(i, j int) bool {
		return publicRuns[i].ID < publicRuns[j].ID
	})
	return publicRuns, nil
}

// ListPendingCommands returns commands waiting for an external result.
func (e *Engine) ListPendingCommands(ctx context.Context, runID string) ([]publiccommand.PendingCommand, error) {
	_, events, err := e.store.LoadRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	return publicPendingCommands(events), nil
}

// ListUnknownCommands returns commands that require manual outcome recovery.
func (e *Engine) ListUnknownCommands(ctx context.Context, runID string) ([]publiccommand.UnknownCommand, error) {
	_, events, err := e.store.LoadRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	return publicUnknownCommands(events), nil
}

// ListApprovals returns pending commands as approval requests.
func (e *Engine) ListApprovals(ctx context.Context, runID string) ([]publicapproval.Request, error) {
	pending, err := e.ListPendingCommands(ctx, runID)
	if err != nil {
		return nil, err
	}

	requests := make([]publicapproval.Request, 0, len(pending))
	for _, cmd := range pending {
		requests = append(requests, publicapproval.Request{
			RunID:   runID,
			Command: cmd,
		})
	}
	return requests, nil
}

func publicCommandResults(events []historystore.Event) []publiccommand.Result {
	scheduled := make(map[string]historystore.Event)
	for _, event := range events {
		if event.Type == historystore.CommandScheduled {
			scheduled[event.CommandID] = event
		}
	}

	results := make([]publiccommand.Result, 0)
	for _, event := range events {
		if event.Type != historystore.CommandCompleted {
			continue
		}
		commandName := event.CommandName
		commandMode := event.CommandMode
		argsHash := event.CommandArgsSHA
		if original, ok := scheduled[event.CommandID]; ok {
			if commandName == "" {
				commandName = original.CommandName
			}
			if commandMode == "" {
				commandMode = original.CommandMode
			}
			if argsHash == "" {
				argsHash = original.CommandArgsSHA
			}
		}
		results = append(results, publiccommand.Result{
			ID:       event.CommandID,
			Name:     commandName,
			Mode:     publiccommand.Mode(commandMode),
			ArgsHash: argsHash,
			Result:   append(json.RawMessage(nil), event.Result...),
		})
	}
	return results
}

func publicPendingCommands(events []historystore.Event) []publiccommand.PendingCommand {
	status := commandStatuses(events)
	pending := make([]publiccommand.PendingCommand, 0)
	for _, state := range status {
		if state.Type != historystore.CommandPending {
			continue
		}
		pending = append(pending, publiccommand.PendingCommand{
			RunID:    state.RunID,
			ID:       state.CommandID,
			Name:     state.CommandName,
			Mode:     publiccommand.Mode(state.CommandMode),
			ArgsHash: state.CommandArgsSHA,
			Reason:   state.Message,
		})
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].ID < pending[j].ID
	})
	return pending
}

func publicUnknownCommands(events []historystore.Event) []publiccommand.UnknownCommand {
	status := commandStatuses(events)
	unknown := make([]publiccommand.UnknownCommand, 0)
	for _, state := range status {
		if state.Type != historystore.CommandUnknown {
			continue
		}
		unknown = append(unknown, publiccommand.UnknownCommand{
			RunID:    state.RunID,
			ID:       state.CommandID,
			Name:     state.CommandName,
			Mode:     publiccommand.Mode(state.CommandMode),
			ArgsHash: state.CommandArgsSHA,
			Reason:   state.Message,
		})
	}
	sort.Slice(unknown, func(i, j int) bool {
		return unknown[i].ID < unknown[j].ID
	})
	return unknown
}

func commandStatuses(events []historystore.Event) map[string]historystore.Event {
	status := make(map[string]historystore.Event)
	for _, event := range events {
		if event.CommandID == "" {
			continue
		}
		switch event.Type {
		case historystore.CommandPending,
			historystore.CommandUnknown,
			historystore.CommandCompleted,
			historystore.CommandFailed,
			historystore.CommandDenied:
			status[event.CommandID] = event
		}
	}
	return status
}
