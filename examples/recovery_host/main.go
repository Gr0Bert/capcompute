package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	capruntime "capcompute"
	"capcompute/capability"
	"capcompute/command"
	"capcompute/module"
	"capcompute/run"
	"capcompute/runtime"
)

func main() {
	ctx := context.Background()
	if err := approvalFlow(ctx); err != nil {
		panic(err)
	}
	if err := denialFlow(ctx); err != nil {
		panic(err)
	}
	if err := unknownRecoveryFlow(ctx); err != nil {
		panic(err)
	}
	if err := failedCommandFlow(ctx); err != nil {
		panic(err)
	}
}

func approvalFlow(ctx context.Context) error {
	engine := newEngine(pendingHandler{})
	if _, err := engine.Start(ctx, invocation("approval")); err != nil {
		return err
	}
	if _, err := engine.Tick(ctx, "approval"); err != nil {
		return err
	}

	approvals, err := engine.ListApprovals(ctx, "approval")
	if err != nil {
		return err
	}
	if len(approvals) != 1 {
		return fmt.Errorf("approvals = %d, want 1", len(approvals))
	}

	if err := engine.Approve(ctx, "approval", approvals[0].Command.ID, command.Receipt{Result: json.RawMessage(`{"approved":true}`)}); err != nil {
		return err
	}
	tick, err := engine.Tick(ctx, "approval")
	if err != nil {
		return err
	}
	fmt.Printf("approval completed: %s\n", tick.Output)
	return nil
}

func denialFlow(ctx context.Context) error {
	engine := newEngine(pendingHandler{})
	if _, err := engine.Start(ctx, invocation("denial")); err != nil {
		return err
	}
	if _, err := engine.Tick(ctx, "denial"); err != nil {
		return err
	}
	if err := engine.Deny(ctx, "denial", "step-1", "operator denied"); err != nil {
		return err
	}
	loaded, err := engine.LoadRun(ctx, "denial")
	if err != nil {
		return err
	}
	fmt.Printf("denial status: %s %s\n", loaded.Status, loaded.FailureMessage)
	return nil
}

func unknownRecoveryFlow(ctx context.Context) error {
	engine := newEngine(unknownHandler{})
	if _, err := engine.Start(ctx, invocation("unknown")); err != nil {
		return err
	}
	if _, err := engine.Tick(ctx, "unknown"); err == nil {
		return errors.New("expected unknown command error")
	}

	unknown, err := engine.ListUnknownCommands(ctx, "unknown")
	if err != nil {
		return err
	}
	if len(unknown) != 1 {
		return fmt.Errorf("unknown commands = %d, want 1", len(unknown))
	}

	if err := engine.RecoverCommand(ctx, "unknown", unknown[0].ID, command.Receipt{Result: json.RawMessage(`{"verified":true}`)}); err != nil {
		return err
	}
	tick, err := engine.Tick(ctx, "unknown")
	if err != nil {
		return err
	}
	fmt.Printf("unknown recovered: %s\n", tick.Output)
	return nil
}

func failedCommandFlow(ctx context.Context) error {
	engine := newEngine(failingHandler{})
	if _, err := engine.Start(ctx, invocation("failed")); err != nil {
		return err
	}
	if _, err := engine.Tick(ctx, "failed"); err == nil {
		return errors.New("expected command failure")
	}
	loaded, err := engine.LoadRun(ctx, "failed")
	if err != nil {
		return err
	}
	fmt.Printf("failure status: %s %s\n", loaded.Status, loaded.FailureMessage)
	return nil
}

func newEngine(handler command.Handler) *capruntime.Engine {
	return capruntime.New(
		capruntime.WithRuntime(scriptedRuntime{}),
		capruntime.WithCommandHandler("example.work", handler),
		capruntime.WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "example",
			ModuleDigest:  "sha256:example",
			CommandName:   "example.work",
		}),
	)
}

func invocation(runID string) run.Invocation {
	return run.Invocation{
		RunID: runID,
		Module: module.Ref{
			Name:       "example",
			Digest:     "sha256:example",
			Entrypoint: "run",
		},
		Principal: run.Principal{Type: "user", ID: "example"},
		Source:    run.Source{Type: "example", ID: "recovery-host"},
		Input:     json.RawMessage(`{"input":true}`),
	}
}

type scriptedRuntime struct{}

func (scriptedRuntime) Invoke(_ context.Context, req runtime.Request) (runtime.Result, error) {
	if len(req.CommandResults) > 0 {
		return runtime.Result{Status: runtime.Completed, Output: req.CommandResults[0].Result}, nil
	}
	return runtime.Result{
		Status: runtime.Command,
		Command: command.Command{
			ID:   "step-1",
			Name: "example.work",
			Mode: command.ModeCommand,
			Args: json.RawMessage(`{"value":42}`),
		},
	}, nil
}

type pendingHandler struct{}

func (pendingHandler) Safety() command.Safety {
	return command.Safety{SideEffecting: true, RequiresIdempotencyKey: true, UnknownPolicy: command.UnknownQuarantine}
}

func (pendingHandler) Execute(_ context.Context, req command.Request) (command.Receipt, error) {
	if req.IdempotencyKey == "" {
		return command.Receipt{}, errors.New("missing idempotency key")
	}
	return command.Receipt{}, command.Pending("waiting for operator approval")
}

type unknownHandler struct{}

func (unknownHandler) Safety() command.Safety {
	return command.Safety{SideEffecting: true, RequiresIdempotencyKey: true, UnknownPolicy: command.UnknownQuarantine}
}

func (unknownHandler) Execute(context.Context, command.Request) (command.Receipt, error) {
	return command.Receipt{}, command.Unknown(errors.New("external result could not be confirmed"))
}

type failingHandler struct{}

func (failingHandler) Safety() command.Safety {
	return command.Safety{SideEffecting: true, RequiresIdempotencyKey: true, UnknownPolicy: command.UnknownQuarantine}
}

func (failingHandler) Execute(context.Context, command.Request) (command.Receipt, error) {
	return command.Receipt{}, errors.New("external system rejected command")
}
