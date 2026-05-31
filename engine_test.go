package capruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestEngineCompletesWorkflowWithoutCommands(t *testing.T) {
	engine := New(WithRuntime(runtimeFunc(func(context.Context, RuntimeRequest) (RuntimeResult, error) {
		return RuntimeResult{
			Status: RuntimeCompleted,
			Output: json.RawMessage(`{"done":true}`),
		}, nil
	})))

	_, err := engine.Start(context.Background(), testInvocation("complete"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	result, err := engine.Tick(context.Background(), "complete")
	if err != nil {
		t.Fatalf("tick workflow: %v", err)
	}
	if result.Status != TickCompleted {
		t.Fatalf("status = %s, want %s", result.Status, TickCompleted)
	}
	if string(result.Output) != `{"done":true}` {
		t.Fatalf("output = %s", result.Output)
	}
}

func TestEngineExecutesCommandThenCompletesOnReplay(t *testing.T) {
	var calls int
	engine := New(
		WithRuntime(runtimeFunc(func(_ context.Context, req RuntimeRequest) (RuntimeResult, error) {
			if len(req.CommandResults) == 0 {
				return RuntimeResult{
					Status: RuntimeCommand,
					Command: Command{
						ID:   "echo-step",
						Name: "test.echo",
						Mode: CommandModeCommand,
						Args: json.RawMessage(`{"message":"hello"}`),
					},
				}, nil
			}
			return RuntimeResult{
				Status: RuntimeCompleted,
				Output: req.CommandResults[0].Result,
			}, nil
		})),
		WithCommandHandler("test.echo", commandHandlerFunc(func(_ context.Context, req CommandRequest) (CommandReceipt, error) {
			calls++
			if req.IdempotencyKey == "" {
				t.Fatal("idempotency key is empty")
			}
			return CommandReceipt{Result: json.RawMessage(`{"message":"hello"}`)}, nil
		})),
		WithGrant(Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("command-flow"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	first, err := engine.Tick(context.Background(), "command-flow")
	if err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if first.Status != TickRunning || first.CommandID != "echo-step" {
		t.Fatalf("first tick = %#v", first)
	}

	second, err := engine.Tick(context.Background(), "command-flow")
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if second.Status != TickCompleted {
		t.Fatalf("status = %s, want %s", second.Status, TickCompleted)
	}
	if string(second.Output) != `{"message":"hello"}` {
		t.Fatalf("output = %s", second.Output)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestEngineDeniesCommandWithoutGrant(t *testing.T) {
	var calls int
	engine := New(
		WithRuntime(runtimeFunc(func(context.Context, RuntimeRequest) (RuntimeResult, error) {
			return RuntimeResult{
				Status: RuntimeCommand,
				Command: Command{
					ID:   "echo-step",
					Name: "test.echo",
					Mode: CommandModeCommand,
					Args: json.RawMessage(`{"message":"hello"}`),
				},
			}, nil
		})),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, CommandRequest) (CommandReceipt, error) {
			calls++
			return CommandReceipt{Result: json.RawMessage(`{}`)}, nil
		})),
	)

	_, err := engine.Start(context.Background(), testInvocation("denied"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	_, err = engine.Tick(context.Background(), "denied")
	if err == nil {
		t.Fatal("expected denied command error")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Fatalf("error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("handler calls = %d, want 0", calls)
	}
}

func TestEngineDetectsNondeterministicCommandArgs(t *testing.T) {
	var ticks int
	engine := New(
		WithRuntime(runtimeFunc(func(context.Context, RuntimeRequest) (RuntimeResult, error) {
			ticks++
			args := json.RawMessage(`{"message":"first"}`)
			if ticks > 1 {
				args = json.RawMessage(`{"message":"second"}`)
			}
			return RuntimeResult{
				Status: RuntimeCommand,
				Command: Command{
					ID:   "echo-step",
					Name: "test.echo",
					Mode: CommandModeCommand,
					Args: args,
				},
			}, nil
		})),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, CommandRequest) (CommandReceipt, error) {
			return CommandReceipt{Result: json.RawMessage(`{"ok":true}`)}, nil
		})),
		WithGrant(Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("nondeterministic"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	if _, err := engine.Tick(context.Background(), "nondeterministic"); err != nil {
		t.Fatalf("first tick: %v", err)
	}

	_, err = engine.Tick(context.Background(), "nondeterministic")
	if err == nil {
		t.Fatal("expected nondeterminism error")
	}
	if !strings.Contains(err.Error(), "nondeterministic") {
		t.Fatalf("error = %v", err)
	}
}

type runtimeFunc func(context.Context, RuntimeRequest) (RuntimeResult, error)

func (f runtimeFunc) Invoke(ctx context.Context, req RuntimeRequest) (RuntimeResult, error) {
	return f(ctx, req)
}

type commandHandlerFunc func(context.Context, CommandRequest) (CommandReceipt, error)

func (f commandHandlerFunc) Execute(ctx context.Context, req CommandRequest) (CommandReceipt, error) {
	return f(ctx, req)
}

func testInvocation(runID string) InvocationRequest {
	return InvocationRequest{
		RunID: runID,
		Module: ModuleRef{
			Name:       "test-module",
			Digest:     "sha256:test",
			Entrypoint: "run",
		},
		Principal: Principal{Type: "user", ID: "rob"},
		Source:    Source{Type: "test", ID: "unit"},
		Input:     json.RawMessage(`{"input":true}`),
	}
}
