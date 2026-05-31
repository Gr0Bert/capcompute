package capruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	publiccapability "capcompute/capability"
	publiccommand "capcompute/command"
	historystore "capcompute/history"
	staticcapability "capcompute/internal/capability/static"
	"capcompute/internal/history/memory"
	"capcompute/internal/replay"
	strictreplay "capcompute/internal/replay/strict"
	"capcompute/module"
	publicrun "capcompute/run"
	publicruntime "capcompute/runtime"
)

// Engine coordinates workflow execution, replay, authorization, command handling, and history.
type Engine struct {
	runtime      publicruntime.Runtime
	store        historystore.Store
	matcher      replay.Matcher
	capabilities publiccapability.Broker
	handlers     map[string]publiccommand.Handler
	grants       []publiccapability.Grant
}

// New creates an Engine with local in-memory defaults unless options replace them.
func New(options ...Option) *Engine {
	engine := &Engine{
		store:    memory.NewMemoryStore(),
		matcher:  strictreplay.NewStrictMatcher(),
		handlers: make(map[string]publiccommand.Handler),
	}
	for _, option := range options {
		option(engine)
	}
	if engine.capabilities == nil {
		engine.capabilities = staticcapability.NewStaticBroker(engine.grants)
	}
	return engine
}

// Start creates a run record and writes the initial WorkflowStarted event.
func (e *Engine) Start(ctx context.Context, req publicrun.Invocation) (publicrun.Run, error) {
	if req.RunID == "" {
		return publicrun.Run{}, ErrRunIDRequired
	}
	if req.Module.Digest == "" {
		return publicrun.Run{}, ErrModuleRequired
	}
	if err := checkLimit("input bytes", req.Limits.MaxInputBytes, len(req.Input)); err != nil {
		return publicrun.Run{}, err
	}

	run := historystore.Run{
		ID:                 req.RunID,
		ModuleName:         req.Module.Name,
		ModuleDigest:       req.Module.Digest,
		ModuleEntrypoint:   req.Module.Entrypoint,
		ModuleSource:       req.Module.Source,
		PrincipalType:      req.Principal.Type,
		PrincipalID:        req.Principal.ID,
		SourceType:         req.Source.Type,
		SourceID:           req.Source.ID,
		Status:             string(publicrun.Running),
		Input:              append(json.RawMessage(nil), req.Input...),
		TimeoutMillis:      req.Limits.TimeoutMillis,
		MemoryMaxPages:     req.Limits.MemoryMaxPages,
		MaxInputBytes:      req.Limits.MaxInputBytes,
		MaxOutputBytes:     req.Limits.MaxOutputBytes,
		MaxCommandsPerTick: req.Limits.MaxCommandsPerTick,
		MaxReplaySteps:     req.Limits.MaxReplaySteps,
	}
	if err := e.store.CreateRun(ctx, run, historystore.Event{
		Type:         historystore.WorkflowStarted,
		RunID:        req.RunID,
		ModuleDigest: req.Module.Digest,
	}); err != nil {
		return publicrun.Run{}, err
	}

	loaded, _, err := e.store.LoadRun(ctx, req.RunID)
	if err != nil {
		return publicrun.Run{}, err
	}
	return publicRun(loaded), nil
}

// LoadRun returns the current public view of a stored workflow run.
func (e *Engine) LoadRun(ctx context.Context, runID string) (publicrun.Run, error) {
	storedRun, _, err := e.store.LoadRun(ctx, runID)
	if err != nil {
		return publicrun.Run{}, err
	}
	return publicRun(storedRun), nil
}

// LoadHistory returns the durable event history for a run.
func (e *Engine) LoadHistory(ctx context.Context, runID string) ([]historystore.Event, error) {
	_, events, err := e.store.LoadRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	return events, nil
}

// Tick advances a run by invoking the runtime and applying the emitted result or command.
func (e *Engine) Tick(ctx context.Context, runID string) (publicrun.TickResult, error) {
	if e.runtime == nil {
		return publicrun.TickResult{}, ErrRuntimeRequired
	}

	storedRun, events, err := e.store.LoadRun(ctx, runID)
	if err != nil {
		return publicrun.TickResult{}, err
	}
	if storedRun.Status == string(publicrun.Completed) {
		return publicrun.TickResult{
			RunID:  runID,
			Status: publicrun.TickCompleted,
			Output: append(json.RawMessage(nil), storedRun.Output...),
		}, nil
	}
	if storedRun.Status == string(publicrun.Failed) {
		return publicrun.TickResult{
			RunID:  runID,
			Status: publicrun.TickFailed,
			Error:  fmt.Errorf("run failed: %s", storedRun.FailureMessage),
		}, nil
	}
	limits := publicLimits(storedRun)
	commandResults := publicCommandResults(events)
	if err := checkLimit("replay steps", limits.MaxReplaySteps, len(commandResults)); err != nil {
		return publicrun.TickResult{}, err
	}

	moduleRef := module.Ref{
		Name:       storedRun.ModuleName,
		Digest:     storedRun.ModuleDigest,
		Entrypoint: storedRun.ModuleEntrypoint,
		Source:     storedRun.ModuleSource,
	}
	invokeCtx := ctx
	var cancel context.CancelFunc
	if limits.TimeoutMillis > 0 {
		invokeCtx, cancel = context.WithTimeout(ctx, time.Duration(limits.TimeoutMillis)*time.Millisecond)
		defer cancel()
	}

	result, err := e.runtime.Invoke(invokeCtx, publicruntime.Request{
		RunID:          runID,
		Module:         moduleRef,
		Input:          append(json.RawMessage(nil), storedRun.Input...),
		CommandResults: commandResults,
		Limits:         limits,
	})
	if err != nil {
		return publicrun.TickResult{}, err
	}

	switch result.Status {
	case publicruntime.Completed:
		if err := checkLimit("output bytes", limits.MaxOutputBytes, len(result.Output)); err != nil {
			return publicrun.TickResult{}, err
		}
		event := historystore.Event{
			Type:   historystore.WorkflowCompleted,
			RunID:  runID,
			Result: append(json.RawMessage(nil), result.Output...),
		}
		if err := e.store.Complete(ctx, runID, storedRun.Version, result.Output, event); err != nil {
			return publicrun.TickResult{}, err
		}
		return publicrun.TickResult{RunID: runID, Status: publicrun.TickCompleted, Output: result.Output}, nil

	case publicruntime.Command:
		if err := checkLimit("commands per tick", limits.MaxCommandsPerTick, 1); err != nil {
			return publicrun.TickResult{}, err
		}
		return e.handleCommand(ctx, storedRun, events, moduleRef, result.Command)

	case publicruntime.Failed:
		message := fmt.Sprintf("runtime failed: %s", result.Message)
		event := historystore.Event{
			Type:    historystore.WorkflowFailed,
			RunID:   runID,
			Message: result.Message,
		}
		if err := e.store.Fail(ctx, runID, storedRun.Version, message, event); err != nil {
			return publicrun.TickResult{}, err
		}
		return publicrun.TickResult{RunID: runID, Status: publicrun.TickFailed, Error: fmt.Errorf("%s", message)}, nil

	default:
		return publicrun.TickResult{}, fmt.Errorf("unknown runtime status %q", result.Status)
	}
}

func (e *Engine) authorize(run historystore.Run, moduleRef module.Ref, cmd publiccommand.Command, argsHash string) error {
	decision := e.capabilities.Authorize(publiccapability.Request{
		RunID:          run.ID,
		Principal:      publicrun.Principal{Type: run.PrincipalType, ID: run.PrincipalID},
		Source:         publicrun.Source{Type: run.SourceType, ID: run.SourceID},
		Module:         moduleRef,
		Command:        cmd,
		CommandArgsSHA: argsHash,
	})
	if !decision.Allowed {
		return DeniedCommandError{CommandName: cmd.Name, Reason: decision.Reason}
	}
	return nil
}

func publicRun(run historystore.Run) publicrun.Run {
	return publicrun.Run{
		ID: run.ID,
		Module: module.Ref{
			Name:       run.ModuleName,
			Digest:     run.ModuleDigest,
			Entrypoint: run.ModuleEntrypoint,
			Source:     run.ModuleSource,
		},
		Principal:      publicrun.Principal{Type: run.PrincipalType, ID: run.PrincipalID},
		Source:         publicrun.Source{Type: run.SourceType, ID: run.SourceID},
		Status:         publicrun.Status(run.Status),
		FailureMessage: run.FailureMessage,
		Output:         append(json.RawMessage(nil), run.Output...),
		Version:        run.Version,
	}
}

func publicLimits(run historystore.Run) publicrun.RuntimeLimits {
	return publicrun.RuntimeLimits{
		TimeoutMillis:      run.TimeoutMillis,
		MemoryMaxPages:     run.MemoryMaxPages,
		MaxInputBytes:      run.MaxInputBytes,
		MaxOutputBytes:     run.MaxOutputBytes,
		MaxCommandsPerTick: run.MaxCommandsPerTick,
		MaxReplaySteps:     run.MaxReplaySteps,
	}
}

func checkLimit(name string, limit int, got int) error {
	if limit <= 0 || got <= limit {
		return nil
	}
	return LimitError{Name: name, Limit: limit, Got: got}
}
