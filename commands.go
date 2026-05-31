package capruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	publiccommand "capcompute/command"
	historystore "capcompute/history"
	"capcompute/internal/command"
	"capcompute/module"
	publicrun "capcompute/run"
)

// ResolveCommand records the result for a pending command so the next tick can replay it.
func (e *Engine) ResolveCommand(ctx context.Context, runID string, commandID string, receipt publiccommand.Receipt) error {
	storedRun, events, err := e.store.LoadRun(ctx, runID)
	if err != nil {
		return err
	}

	pending, err := pendingCommand(events, commandID)
	if err != nil {
		return err
	}

	return e.store.Append(ctx, runID, storedRun.Version, historystore.Event{
		Type:           historystore.CommandCompleted,
		RunID:          runID,
		CommandID:      pending.CommandID,
		CommandName:    pending.CommandName,
		CommandMode:    pending.CommandMode,
		CommandArgsSHA: pending.CommandArgsSHA,
		Result:         append(json.RawMessage(nil), receipt.Result...),
	})
}

// Approve records the result for an approved pending command.
func (e *Engine) Approve(ctx context.Context, runID string, commandID string, receipt publiccommand.Receipt) error {
	return e.ResolveCommand(ctx, runID, commandID, receipt)
}

// Deny marks a pending or unknown command as denied and fails the run.
func (e *Engine) Deny(ctx context.Context, runID string, commandID string, reason string) error {
	if reason == "" {
		reason = "approval denied"
	}
	return e.FailCommand(ctx, runID, commandID, reason)
}

// RecoverCommand records the externally verified result for an unknown command.
// It lets replay continue without automatically retrying an ambiguous side effect.
func (e *Engine) RecoverCommand(ctx context.Context, runID string, commandID string, receipt publiccommand.Receipt) error {
	storedRun, events, err := e.store.LoadRun(ctx, runID)
	if err != nil {
		return err
	}

	unknown, err := unknownCommand(events, commandID)
	if err != nil {
		return err
	}

	return e.store.Append(ctx, runID, storedRun.Version, historystore.Event{
		Type:           historystore.CommandCompleted,
		RunID:          runID,
		CommandID:      unknown.CommandID,
		CommandName:    unknown.CommandName,
		CommandMode:    unknown.CommandMode,
		CommandArgsSHA: unknown.CommandArgsSHA,
		Result:         append(json.RawMessage(nil), receipt.Result...),
	})
}

// FailCommand marks a pending or unknown command as failed and fails the run.
func (e *Engine) FailCommand(ctx context.Context, runID string, commandID string, message string) error {
	storedRun, events, err := e.store.LoadRun(ctx, runID)
	if err != nil {
		return err
	}

	unresolved, err := unresolvedCommand(events, commandID)
	if err != nil {
		return err
	}
	if message == "" {
		message = fmt.Sprintf("command %q failed", commandID)
	}

	return e.store.Fail(ctx, runID, storedRun.Version, message, historystore.Event{
		Type:           historystore.CommandFailed,
		RunID:          runID,
		CommandID:      unresolved.CommandID,
		CommandName:    unresolved.CommandName,
		CommandMode:    unresolved.CommandMode,
		CommandArgsSHA: unresolved.CommandArgsSHA,
		Message:        message,
	})
}

func (e *Engine) handleCommand(ctx context.Context, run historystore.Run, events []historystore.Event, moduleRef module.Ref, public publiccommand.Command) (publicrun.TickResult, error) {
	internalCommand, err := command.New(public.ID, public.Name, string(public.Mode), public.Args)
	if err != nil {
		return publicrun.TickResult{}, err
	}
	match, err := e.matcher.Match(events, internalCommand)
	if err != nil {
		err = NondeterminismError{CommandID: public.ID, Err: err}
		event := historystore.Event{
			Type:           historystore.NondeterminismDetected,
			RunID:          run.ID,
			CommandID:      public.ID,
			CommandName:    public.Name,
			CommandArgsSHA: internalCommand.ArgsHash,
			Message:        err.Error(),
		}
		_ = e.store.Fail(ctx, run.ID, run.Version, err.Error(), event)
		return publicrun.TickResult{}, err
	}
	if match.Completed {
		return publicrun.TickResult{RunID: run.ID, Status: publicrun.TickRunning, CommandID: public.ID}, nil
	}
	if match.Pending {
		return publicrun.TickResult{RunID: run.ID, Status: publicrun.TickRunning, CommandID: public.ID}, nil
	}
	if match.Unknown {
		return publicrun.TickResult{}, CommandStateError{
			CommandID: public.ID,
			State:     "unknown",
			Message:   fmt.Sprintf("command %q outcome is unknown; manual recovery required", public.ID),
		}
	}
	if match.Failed {
		return publicrun.TickResult{}, CommandStateError{
			CommandID: public.ID,
			State:     "failed",
			Message:   fmt.Sprintf("command %q previously failed; retry policy is not configured", public.ID),
		}
	}

	if err := e.authorize(run, moduleRef, public, internalCommand.ArgsHash); err != nil {
		if appendErr := e.store.Fail(ctx, run.ID, run.Version, err.Error(),
			historystore.Event{
				Type:           historystore.CommandScheduled,
				RunID:          run.ID,
				CommandID:      public.ID,
				CommandName:    public.Name,
				CommandMode:    string(public.Mode),
				CommandArgsSHA: internalCommand.ArgsHash,
			},
			historystore.Event{
				Type:           historystore.CommandDenied,
				RunID:          run.ID,
				CommandID:      public.ID,
				CommandName:    public.Name,
				CommandMode:    string(public.Mode),
				CommandArgsSHA: internalCommand.ArgsHash,
				Message:        err.Error(),
			},
		); appendErr != nil {
			return publicrun.TickResult{}, appendErr
		}
		return publicrun.TickResult{}, err
	}

	handler, ok := e.handlers[public.Name]
	if !ok {
		err := fmt.Errorf("command handler %q is not registered", public.Name)
		return e.failEmittedCommand(ctx, run, public, internalCommand, err)
	}
	safety := handler.Safety()
	if public.Mode == publiccommand.ModeQuery && safety.SideEffecting {
		err := fmt.Errorf("query command %q is registered with a side-effecting handler", public.Name)
		return e.failEmittedCommand(ctx, run, public, internalCommand, err)
	}

	idempotencyKey := command.IdempotencyKey(run.ID, moduleRef.Digest, internalCommand)
	req := publiccommand.Request{
		RunID:          run.ID,
		Module:         moduleRef,
		Principal:      publicrun.Principal{Type: run.PrincipalType, ID: run.PrincipalID},
		Source:         publicrun.Source{Type: run.SourceType, ID: run.SourceID},
		Command:        public,
		ArgsHash:       internalCommand.ArgsHash,
		IdempotencyKey: idempotencyKey,
	}
	if safety.RequiresIdempotencyKey && req.IdempotencyKey == "" {
		err := fmt.Errorf("command handler %q requires idempotency key", public.Name)
		return e.failEmittedCommand(ctx, run, public, internalCommand, err)
	}

	receipt, err := handler.Execute(ctx, req)
	if err != nil {
		var pending publiccommand.PendingError
		if errors.As(err, &pending) {
			if appendErr := e.store.Append(ctx, run.ID, run.Version,
				historystore.Event{
					Type:           historystore.CommandScheduled,
					RunID:          run.ID,
					CommandID:      public.ID,
					CommandName:    public.Name,
					CommandMode:    string(public.Mode),
					CommandArgsSHA: internalCommand.ArgsHash,
				},
				historystore.Event{
					Type:           historystore.CommandPending,
					RunID:          run.ID,
					CommandID:      public.ID,
					CommandName:    public.Name,
					CommandMode:    string(public.Mode),
					CommandArgsSHA: internalCommand.ArgsHash,
					Message:        pending.Error(),
				},
			); appendErr != nil {
				return publicrun.TickResult{}, appendErr
			}
			return publicrun.TickResult{RunID: run.ID, Status: publicrun.TickRunning, CommandID: public.ID}, nil
		}

		var unknown publiccommand.UnknownError
		if errors.As(err, &unknown) {
			if appendErr := e.store.Append(ctx, run.ID, run.Version,
				historystore.Event{
					Type:           historystore.CommandScheduled,
					RunID:          run.ID,
					CommandID:      public.ID,
					CommandName:    public.Name,
					CommandMode:    string(public.Mode),
					CommandArgsSHA: internalCommand.ArgsHash,
				},
				historystore.Event{
					Type:           historystore.CommandStarted,
					RunID:          run.ID,
					CommandID:      public.ID,
					CommandName:    public.Name,
					CommandMode:    string(public.Mode),
					CommandArgsSHA: internalCommand.ArgsHash,
				},
				historystore.Event{
					Type:           historystore.CommandUnknown,
					RunID:          run.ID,
					CommandID:      public.ID,
					CommandName:    public.Name,
					CommandMode:    string(public.Mode),
					CommandArgsSHA: internalCommand.ArgsHash,
					Message:        unknown.Error(),
				},
			); appendErr != nil {
				return publicrun.TickResult{}, appendErr
			}
			return publicrun.TickResult{}, err
		}

		if appendErr := e.store.Fail(ctx, run.ID, run.Version, err.Error(),
			historystore.Event{
				Type:           historystore.CommandScheduled,
				RunID:          run.ID,
				CommandID:      public.ID,
				CommandName:    public.Name,
				CommandMode:    string(public.Mode),
				CommandArgsSHA: internalCommand.ArgsHash,
			},
			historystore.Event{
				Type:           historystore.CommandStarted,
				RunID:          run.ID,
				CommandID:      public.ID,
				CommandName:    public.Name,
				CommandMode:    string(public.Mode),
				CommandArgsSHA: internalCommand.ArgsHash,
			},
			historystore.Event{
				Type:           historystore.CommandFailed,
				RunID:          run.ID,
				CommandID:      public.ID,
				CommandName:    public.Name,
				CommandMode:    string(public.Mode),
				CommandArgsSHA: internalCommand.ArgsHash,
				Message:        err.Error(),
			},
		); appendErr != nil {
			return publicrun.TickResult{}, appendErr
		}
		return publicrun.TickResult{}, err
	}

	err = e.store.Append(ctx, run.ID, run.Version,
		historystore.Event{
			Type:           historystore.CommandScheduled,
			RunID:          run.ID,
			CommandID:      public.ID,
			CommandName:    public.Name,
			CommandMode:    string(public.Mode),
			CommandArgsSHA: internalCommand.ArgsHash,
		},
		historystore.Event{
			Type:           historystore.CommandStarted,
			RunID:          run.ID,
			CommandID:      public.ID,
			CommandName:    public.Name,
			CommandMode:    string(public.Mode),
			CommandArgsSHA: internalCommand.ArgsHash,
		},
		historystore.Event{
			Type:           historystore.CommandCompleted,
			RunID:          run.ID,
			CommandID:      public.ID,
			CommandName:    public.Name,
			CommandMode:    string(public.Mode),
			CommandArgsSHA: internalCommand.ArgsHash,
			Result:         append(json.RawMessage(nil), receipt.Result...),
		},
	)
	if err != nil {
		return publicrun.TickResult{}, err
	}

	return publicrun.TickResult{RunID: run.ID, Status: publicrun.TickRunning, CommandID: public.ID}, nil
}

func (e *Engine) failEmittedCommand(ctx context.Context, run historystore.Run, public publiccommand.Command, internalCommand command.Command, err error) (publicrun.TickResult, error) {
	if appendErr := e.store.Fail(ctx, run.ID, run.Version, err.Error(),
		historystore.Event{
			Type:           historystore.CommandScheduled,
			RunID:          run.ID,
			CommandID:      public.ID,
			CommandName:    public.Name,
			CommandMode:    string(public.Mode),
			CommandArgsSHA: internalCommand.ArgsHash,
		},
		historystore.Event{
			Type:           historystore.CommandFailed,
			RunID:          run.ID,
			CommandID:      public.ID,
			CommandName:    public.Name,
			CommandMode:    string(public.Mode),
			CommandArgsSHA: internalCommand.ArgsHash,
			Message:        err.Error(),
		},
	); appendErr != nil {
		return publicrun.TickResult{}, appendErr
	}
	return publicrun.TickResult{}, err
}

func pendingCommand(events []historystore.Event, commandID string) (historystore.Event, error) {
	var pending *historystore.Event
	var completed bool
	var unknown bool

	for i := range events {
		event := events[i]
		if event.CommandID != commandID {
			continue
		}
		switch event.Type {
		case historystore.CommandPending:
			copied := event
			pending = &copied
		case historystore.CommandCompleted:
			completed = true
		case historystore.CommandUnknown:
			unknown = true
		}
	}

	if pending == nil {
		return historystore.Event{}, CommandStateError{CommandID: commandID, State: "not_found", Message: fmt.Sprintf("pending command %q not found", commandID)}
	}
	if completed {
		return historystore.Event{}, CommandStateError{CommandID: commandID, State: "completed", Message: fmt.Sprintf("pending command %q is already completed", commandID)}
	}
	if unknown {
		return historystore.Event{}, CommandStateError{CommandID: commandID, State: "unknown", Message: fmt.Sprintf("pending command %q is unknown; manual recovery required", commandID)}
	}
	return *pending, nil
}

func unknownCommand(events []historystore.Event, commandID string) (historystore.Event, error) {
	var unknown *historystore.Event
	var completed bool

	for i := range events {
		event := events[i]
		if event.CommandID != commandID {
			continue
		}
		switch event.Type {
		case historystore.CommandUnknown:
			copied := event
			unknown = &copied
		case historystore.CommandCompleted:
			completed = true
		}
	}

	if unknown == nil {
		return historystore.Event{}, CommandStateError{CommandID: commandID, State: "not_found", Message: fmt.Sprintf("unknown command %q not found", commandID)}
	}
	if completed {
		return historystore.Event{}, CommandStateError{CommandID: commandID, State: "completed", Message: fmt.Sprintf("unknown command %q is already completed", commandID)}
	}
	return *unknown, nil
}

func unresolvedCommand(events []historystore.Event, commandID string) (historystore.Event, error) {
	var unresolved *historystore.Event
	var completed bool
	var failed bool

	for i := range events {
		event := events[i]
		if event.CommandID != commandID {
			continue
		}
		switch event.Type {
		case historystore.CommandPending, historystore.CommandUnknown:
			copied := event
			unresolved = &copied
		case historystore.CommandCompleted:
			completed = true
		case historystore.CommandFailed:
			failed = true
		}
	}

	if unresolved == nil {
		return historystore.Event{}, CommandStateError{CommandID: commandID, State: "not_found", Message: fmt.Sprintf("unresolved command %q not found", commandID)}
	}
	if completed {
		return historystore.Event{}, CommandStateError{CommandID: commandID, State: "completed", Message: fmt.Sprintf("unresolved command %q is already completed", commandID)}
	}
	if failed {
		return historystore.Event{}, CommandStateError{CommandID: commandID, State: "failed", Message: fmt.Sprintf("unresolved command %q is already failed", commandID)}
	}
	return *unresolved, nil
}
