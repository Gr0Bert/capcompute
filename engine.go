package capruntime

import (
	"context"
	"encoding/json"
	"fmt"

	"capcompute/internal/capability"
	"capcompute/internal/command"
	"capcompute/internal/history"
	"capcompute/internal/replay"
)

type Engine struct {
	runtime  Runtime
	store    *history.MemoryStore
	matcher  replay.Matcher
	handlers map[string]CommandHandler
	grants   []Grant
}

func New(options ...Option) *Engine {
	engine := &Engine{
		store:    history.NewMemoryStore(),
		matcher:  replay.NewMatcher(),
		handlers: make(map[string]CommandHandler),
	}
	for _, option := range options {
		option(engine)
	}
	return engine
}

func (e *Engine) Start(ctx context.Context, req InvocationRequest) (Run, error) {
	if req.RunID == "" {
		return Run{}, ErrRunIDRequired
	}
	if req.Module.Digest == "" {
		return Run{}, ErrModuleRequired
	}

	run := history.Run{
		ID:               req.RunID,
		ModuleName:       req.Module.Name,
		ModuleDigest:     req.Module.Digest,
		ModuleEntrypoint: req.Module.Entrypoint,
		PrincipalType:    req.Principal.Type,
		PrincipalID:      req.Principal.ID,
		SourceType:       req.Source.Type,
		SourceID:         req.Source.ID,
		Status:           string(RunRunning),
		Input:            append(json.RawMessage(nil), req.Input...),
	}
	if err := e.store.CreateRun(ctx, run, history.Event{
		Type:         history.WorkflowStarted,
		RunID:        req.RunID,
		ModuleDigest: req.Module.Digest,
	}); err != nil {
		return Run{}, err
	}

	loaded, _, err := e.store.LoadRun(ctx, req.RunID)
	if err != nil {
		return Run{}, err
	}
	return publicRun(loaded), nil
}

func (e *Engine) Tick(ctx context.Context, runID string) (TickResult, error) {
	if e.runtime == nil {
		return TickResult{}, ErrRuntimeRequired
	}

	storedRun, events, err := e.store.LoadRun(ctx, runID)
	if err != nil {
		return TickResult{}, err
	}
	if storedRun.Status == string(RunCompleted) {
		return TickResult{RunID: runID, Status: TickCompleted}, nil
	}

	module := ModuleRef{
		Name:       storedRun.ModuleName,
		Digest:     storedRun.ModuleDigest,
		Entrypoint: storedRun.ModuleEntrypoint,
	}
	result, err := e.runtime.Invoke(ctx, RuntimeRequest{
		RunID:          runID,
		Module:         module,
		Input:          append(json.RawMessage(nil), storedRun.Input...),
		CommandResults: publicCommandResults(events),
	})
	if err != nil {
		return TickResult{}, err
	}

	switch result.Status {
	case RuntimeCompleted:
		event := history.Event{
			Type:   history.WorkflowCompleted,
			RunID:  runID,
			Result: append(json.RawMessage(nil), result.Output...),
		}
		if err := e.store.Append(ctx, runID, storedRun.Version, event); err != nil {
			return TickResult{}, err
		}
		if err := e.store.MarkComplete(ctx, runID, result.Output); err != nil {
			return TickResult{}, err
		}
		return TickResult{RunID: runID, Status: TickCompleted, Output: result.Output}, nil

	case RuntimeCommand:
		return e.handleCommand(ctx, storedRun, events, module, result.Command)

	case RuntimeFailed:
		event := history.Event{
			Type:    history.WorkflowFailed,
			RunID:   runID,
			Message: result.Message,
		}
		if err := e.store.Append(ctx, runID, storedRun.Version, event); err != nil {
			return TickResult{}, err
		}
		return TickResult{RunID: runID, Status: TickFailed, Error: fmt.Errorf("runtime failed: %s", result.Message)}, nil

	default:
		return TickResult{}, fmt.Errorf("unknown runtime status %q", result.Status)
	}
}

func (e *Engine) handleCommand(ctx context.Context, run history.Run, events []history.Event, module ModuleRef, public Command) (TickResult, error) {
	internalCommand, err := command.New(public.ID, public.Name, string(public.Mode), public.Args)
	if err != nil {
		return TickResult{}, err
	}
	match, err := e.matcher.Match(events, internalCommand)
	if err != nil {
		event := history.Event{
			Type:           history.NondeterminismDetected,
			RunID:          run.ID,
			CommandID:      public.ID,
			CommandName:    public.Name,
			CommandArgsSHA: internalCommand.ArgsHash,
			Message:        err.Error(),
		}
		_ = e.store.Append(ctx, run.ID, run.Version, event)
		return TickResult{}, err
	}
	if match.Completed {
		return TickResult{RunID: run.ID, Status: TickRunning, CommandID: public.ID}, nil
	}

	if err := e.authorize(run, module, internalCommand); err != nil {
		return TickResult{}, err
	}

	handler, ok := e.handlers[public.Name]
	if !ok {
		return TickResult{}, fmt.Errorf("command handler %q is not registered", public.Name)
	}

	idempotencyKey := command.IdempotencyKey(run.ID, module.Digest, internalCommand)
	req := CommandRequest{
		RunID:          run.ID,
		Module:         module,
		Principal:      Principal{Type: run.PrincipalType, ID: run.PrincipalID},
		Source:         Source{Type: run.SourceType, ID: run.SourceID},
		Command:        public,
		ArgsHash:       internalCommand.ArgsHash,
		IdempotencyKey: idempotencyKey,
	}

	receipt, err := handler.Execute(ctx, req)
	if err != nil {
		if appendErr := e.store.Append(ctx, run.ID, run.Version,
			history.Event{
				Type:           history.CommandScheduled,
				RunID:          run.ID,
				CommandID:      public.ID,
				CommandName:    public.Name,
				CommandMode:    string(public.Mode),
				CommandArgsSHA: internalCommand.ArgsHash,
			},
			history.Event{
				Type:           history.CommandFailed,
				RunID:          run.ID,
				CommandID:      public.ID,
				CommandName:    public.Name,
				CommandArgsSHA: internalCommand.ArgsHash,
				Message:        err.Error(),
			},
		); appendErr != nil {
			return TickResult{}, appendErr
		}
		return TickResult{}, err
	}

	err = e.store.Append(ctx, run.ID, run.Version,
		history.Event{
			Type:           history.CommandScheduled,
			RunID:          run.ID,
			CommandID:      public.ID,
			CommandName:    public.Name,
			CommandMode:    string(public.Mode),
			CommandArgsSHA: internalCommand.ArgsHash,
		},
		history.Event{
			Type:           history.CommandStarted,
			RunID:          run.ID,
			CommandID:      public.ID,
			CommandName:    public.Name,
			CommandArgsSHA: internalCommand.ArgsHash,
		},
		history.Event{
			Type:           history.CommandCompleted,
			RunID:          run.ID,
			CommandID:      public.ID,
			CommandName:    public.Name,
			CommandArgsSHA: internalCommand.ArgsHash,
			Result:         append(json.RawMessage(nil), receipt.Result...),
		},
	)
	if err != nil {
		return TickResult{}, err
	}

	return TickResult{RunID: run.ID, Status: TickRunning, CommandID: public.ID}, nil
}

func (e *Engine) authorize(run history.Run, module ModuleRef, cmd command.Command) error {
	broker := capability.NewStaticBroker(publicGrants(e.grants))
	decision := broker.Authorize(capability.Request{
		PrincipalType: run.PrincipalType,
		PrincipalID:   run.PrincipalID,
		ModuleDigest:  module.Digest,
		CommandName:   cmd.Name,
	})
	if !decision.Allowed {
		return fmt.Errorf("command %q denied: %s", cmd.Name, decision.Reason)
	}
	return nil
}

func publicRun(run history.Run) Run {
	return Run{
		ID: run.ID,
		Module: ModuleRef{
			Name:       run.ModuleName,
			Digest:     run.ModuleDigest,
			Entrypoint: run.ModuleEntrypoint,
		},
		Principal: Principal{Type: run.PrincipalType, ID: run.PrincipalID},
		Source:    Source{Type: run.SourceType, ID: run.SourceID},
		Status:    RunStatus(run.Status),
		Version:   run.Version,
	}
}

func publicCommandResults(events []history.Event) []CommandResult {
	results := make([]CommandResult, 0)
	for _, event := range events {
		if event.Type != history.CommandCompleted {
			continue
		}
		results = append(results, CommandResult{
			ID:     event.CommandID,
			Name:   event.CommandName,
			Result: append(json.RawMessage(nil), event.Result...),
		})
	}
	return results
}

func publicGrants(grants []Grant) []capability.Grant {
	converted := make([]capability.Grant, 0, len(grants))
	for _, grant := range grants {
		converted = append(converted, capability.Grant{
			PrincipalType: grant.PrincipalType,
			PrincipalID:   grant.PrincipalID,
			ModuleDigest:  grant.ModuleDigest,
			CommandName:   grant.CommandName,
		})
	}
	return converted
}
