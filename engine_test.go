package capruntime

import (
	"capcompute/capability"
	"capcompute/command"
	"capcompute/history"
	"capcompute/internal/history/memory"
	"capcompute/internal/runtime/fake"
	"capcompute/module"
	"capcompute/run"
	"capcompute/runtime"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEngineCompletesWorkflowWithoutCommands(t *testing.T) {
	engine := New(WithRuntime(fake.NewRuntime(
		fake.Complete(json.RawMessage(`{"done":true}`)),
	)))

	_, err := engine.Start(context.Background(), testInvocation("complete"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	result, err := engine.Tick(context.Background(), "complete")
	if err != nil {
		t.Fatalf("tick workflow: %v", err)
	}
	if result.Status != run.TickCompleted {
		t.Fatalf("status = %s, want %s", result.Status, run.TickCompleted)
	}
	if string(result.Output) != `{"done":true}` {
		t.Fatalf("output = %s", result.Output)
	}

	loaded, err := engine.LoadRun(context.Background(), "complete")
	if err != nil {
		t.Fatalf("load completed run: %v", err)
	}
	if string(loaded.Output) != `{"done":true}` {
		t.Fatalf("loaded output = %s", loaded.Output)
	}

	again, err := engine.Tick(context.Background(), "complete")
	if err != nil {
		t.Fatalf("tick completed workflow: %v", err)
	}
	if again.Status != run.TickCompleted {
		t.Fatalf("completed tick status = %s", again.Status)
	}
	if string(again.Output) != `{"done":true}` {
		t.Fatalf("completed tick output = %s", again.Output)
	}
}

func TestEnginePassesRuntimeLimitsToRuntime(t *testing.T) {
	var got run.RuntimeLimits
	engine := New(WithRuntime(fake.NewFuncRuntime(func(_ context.Context, req runtime.Request) (runtime.Result, error) {
		got = req.Limits
		return runtime.Result{
			Status: runtime.Completed,
			Output: json.RawMessage(`{"done":true}`),
		}, nil
	})))

	invocation := testInvocation("limits")
	invocation.Limits = run.RuntimeLimits{
		TimeoutMillis:      3000,
		MemoryMaxPages:     32,
		MaxInputBytes:      1024,
		MaxOutputBytes:     2048,
		MaxCommandsPerTick: 20,
		MaxReplaySteps:     100,
	}

	_, err := engine.Start(context.Background(), invocation)
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	if _, err := engine.Tick(context.Background(), "limits"); err != nil {
		t.Fatalf("tick workflow: %v", err)
	}

	if got != invocation.Limits {
		t.Fatalf("limits = %#v, want %#v", got, invocation.Limits)
	}
}

func TestEngineTimeoutCancelsRuntimeInvocation(t *testing.T) {
	engine := New(WithRuntime(fake.NewFuncRuntime(func(ctx context.Context, _ runtime.Request) (runtime.Result, error) {
		<-ctx.Done()
		return runtime.Result{}, ctx.Err()
	})))

	invocation := testInvocation("timeout")
	invocation.Limits = run.RuntimeLimits{TimeoutMillis: 1}

	_, err := engine.Start(context.Background(), invocation)
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	start := time.Now()
	_, err = engine.Tick(context.Background(), "timeout")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout took too long: %s", time.Since(start))
	}
}

func TestEngineListsRuns(t *testing.T) {
	engine := New(WithRuntime(fake.NewRuntime(
		fake.Complete(json.RawMessage(`{"done":true}`)),
	)))

	if _, err := engine.Start(context.Background(), testInvocation("list-a")); err != nil {
		t.Fatalf("start list-a: %v", err)
	}
	if _, err := engine.Start(context.Background(), testInvocation("list-b")); err != nil {
		t.Fatalf("start list-b: %v", err)
	}
	if _, err := engine.Tick(context.Background(), "list-a"); err != nil {
		t.Fatalf("complete list-a: %v", err)
	}

	all, err := engine.ListRuns(context.Background(), run.Filter{})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all runs = %#v", all)
	}

	running, err := engine.ListRuns(context.Background(), run.Filter{Status: run.Running})
	if err != nil {
		t.Fatalf("list running runs: %v", err)
	}
	if len(running) != 1 || running[0].ID != "list-b" {
		t.Fatalf("running runs = %#v", running)
	}
}

func TestEngineUsesConfiguredStore(t *testing.T) {
	store := memory.NewMemoryStore()
	engine := New(
		WithStore(store),
		WithRuntime(fake.NewRuntime(
			fake.Complete(json.RawMessage(`{"done":true}`)),
		)),
	)

	_, err := engine.Start(context.Background(), testInvocation("custom-store"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	storedRun, events, err := store.LoadRun(context.Background(), "custom-store")
	if err != nil {
		t.Fatalf("load from configured store: %v", err)
	}
	if storedRun.ID != "custom-store" {
		t.Fatalf("stored run id = %q", storedRun.ID)
	}
	if len(events) != 1 || events[0].Type != history.WorkflowStarted {
		t.Fatalf("events = %#v", events)
	}
}

func TestEnginePersistsModuleSource(t *testing.T) {
	engine := New(WithRuntime(fake.NewRuntime(
		fake.Complete(json.RawMessage(`{"done":true}`)),
	)))

	invocation := testInvocation("module-source")
	invocation.Module.Source = "workflow.wasm"

	if _, err := engine.Start(context.Background(), invocation); err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	stored, err := engine.LoadRun(context.Background(), "module-source")
	if err != nil {
		t.Fatalf("load workflow: %v", err)
	}
	if stored.Module.Source != "workflow.wasm" {
		t.Fatalf("module source = %q", stored.Module.Source)
	}
}

func TestEngineUsesFileStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	first := New(
		WithFileStore(path),
		WithRuntime(fake.NewRuntime(
			fake.Complete(json.RawMessage(`{"done":true}`)),
		)),
	)

	if _, err := first.Start(context.Background(), testInvocation("file-store")); err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	second := New(
		WithFileStore(path),
		WithRuntime(fake.NewRuntime(
			fake.Complete(json.RawMessage(`{"done":true}`)),
		)),
	)
	stored, err := second.LoadRun(context.Background(), "file-store")
	if err != nil {
		t.Fatalf("load workflow: %v", err)
	}
	if stored.ID != "file-store" {
		t.Fatalf("run id = %q", stored.ID)
	}
}

func TestEngineRejectsInputOverLimit(t *testing.T) {
	engine := New(WithRuntime(fake.NewRuntime(
		fake.Complete(json.RawMessage(`{"done":true}`)),
	)))

	invocation := testInvocation("input-limit")
	invocation.Input = json.RawMessage(`{"too":"large"}`)
	invocation.Limits = run.RuntimeLimits{MaxInputBytes: 2}

	_, err := engine.Start(context.Background(), invocation)
	if err == nil {
		t.Fatal("expected input limit error")
	}
	if !strings.Contains(err.Error(), "input bytes limit exceeded") {
		t.Fatalf("error = %v", err)
	}
}

func TestEngineRejectsOutputOverLimit(t *testing.T) {
	engine := New(WithRuntime(fake.NewRuntime(
		fake.Complete(json.RawMessage(`{"too":"large"}`)),
	)))

	invocation := testInvocation("output-limit")
	invocation.Limits = run.RuntimeLimits{MaxOutputBytes: 2}

	if _, err := engine.Start(context.Background(), invocation); err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	_, err := engine.Tick(context.Background(), "output-limit")
	if err == nil {
		t.Fatal("expected output limit error")
	}
	if !strings.Contains(err.Error(), "output bytes limit exceeded") {
		t.Fatalf("error = %v", err)
	}
}

func TestEngineRejectsReplayStepsOverLimit(t *testing.T) {
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommandWithID("echo-one", json.RawMessage(`{"message":"one"}`))),
			fake.Emit(echoCommandWithID("echo-two", json.RawMessage(`{"message":"two"}`))),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			return command.Receipt{Result: json.RawMessage(`{}`)}, nil
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	invocation := testInvocation("replay-limit")
	invocation.Limits = run.RuntimeLimits{MaxReplaySteps: 1}

	if _, err := engine.Start(context.Background(), invocation); err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	if _, err := engine.Tick(context.Background(), "replay-limit"); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if _, err := engine.Tick(context.Background(), "replay-limit"); err != nil {
		t.Fatalf("second tick: %v", err)
	}

	_, err := engine.Tick(context.Background(), "replay-limit")
	if err == nil {
		t.Fatal("expected replay steps limit error")
	}
	if !strings.Contains(err.Error(), "replay steps limit exceeded") {
		t.Fatalf("error = %v", err)
	}
}

func TestEngineExecutesCommandThenCompletesOnReplay(t *testing.T) {
	var calls int
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
			fake.CompleteWithFirstCommandResult(),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(_ context.Context, req command.Request) (command.Receipt, error) {
			calls++
			if req.IdempotencyKey == "" {
				t.Fatal("idempotency key is empty")
			}
			return command.Receipt{Result: json.RawMessage(`{"message":"hello"}`)}, nil
		})),
		WithGrant(capability.Grant{
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
	if first.Status != run.TickRunning || first.CommandID != "echo-step" {
		t.Fatalf("first tick = %#v", first)
	}

	second, err := engine.Tick(context.Background(), "command-flow")
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if second.Status != run.TickCompleted {
		t.Fatalf("status = %s, want %s", second.Status, run.TickCompleted)
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
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			calls++
			return command.Receipt{Result: json.RawMessage(`{}`)}, nil
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
	var denied DeniedCommandError
	if !errors.As(err, &denied) {
		t.Fatalf("error = %T %v, want DeniedCommandError", err, err)
	}
	if denied.CommandName != "test.echo" {
		t.Fatalf("denied command = %q", denied.CommandName)
	}
	if calls != 0 {
		t.Fatalf("handler calls = %d, want 0", calls)
	}

	stored, err := engine.LoadRun(context.Background(), "denied")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if stored.Status != run.Failed {
		t.Fatalf("run status = %s, want %s", stored.Status, run.Failed)
	}

	events, err := engine.LoadHistory(context.Background(), "denied")
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if events[len(events)-1].Type != history.CommandDenied {
		t.Fatalf("last event = %s, want %s", events[len(events)-1].Type, history.CommandDenied)
	}

	again, err := engine.Tick(context.Background(), "denied")
	if err != nil {
		t.Fatalf("tick failed run: %v", err)
	}
	if again.Status != run.TickFailed {
		t.Fatalf("tick status = %s, want %s", again.Status, run.TickFailed)
	}
}

func TestEngineUsesConfiguredCapabilityBroker(t *testing.T) {
	var calls int
	broker := &recordingBroker{}
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
		)),
		WithCapabilityBroker(broker),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			calls++
			return command.Receipt{Result: json.RawMessage(`{"ok":true}`)}, nil
		})),
	)

	_, err := engine.Start(context.Background(), testInvocation("custom-broker"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	_, err = engine.Tick(context.Background(), "custom-broker")
	if err != nil {
		t.Fatalf("tick workflow: %v", err)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if broker.request.RunID != "custom-broker" {
		t.Fatalf("authorize run id = %q", broker.request.RunID)
	}
	if broker.request.Principal.Type != "user" || broker.request.Principal.ID != "rob" {
		t.Fatalf("authorize principal = %#v", broker.request.Principal)
	}
	if broker.request.Source.Type != "test" || broker.request.Source.ID != "unit" {
		t.Fatalf("authorize source = %#v", broker.request.Source)
	}
	if broker.request.Module.Digest != "sha256:test" {
		t.Fatalf("authorize module = %#v", broker.request.Module)
	}
	if broker.request.Command.ID != "echo-step" || broker.request.Command.Name != "test.echo" {
		t.Fatalf("authorize command = %#v", broker.request.Command)
	}
	if broker.request.CommandArgsSHA == "" {
		t.Fatal("authorize command args hash is empty")
	}
}

func TestEngineRejectsSideEffectingHandlerForQueryCommand(t *testing.T) {
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(command.Command{
				ID:   "query-step",
				Name: "test.echo",
				Mode: command.ModeQuery,
				Args: json.RawMessage(`{"message":"hello"}`),
			}),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			return command.Receipt{Result: json.RawMessage(`{}`)}, nil
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("query-side-effect"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	_, err = engine.Tick(context.Background(), "query-side-effect")
	if err == nil {
		t.Fatal("expected query safety error")
	}
	if !strings.Contains(err.Error(), "side-effecting handler") {
		t.Fatalf("error = %v", err)
	}

	stored, err := engine.LoadRun(context.Background(), "query-side-effect")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if stored.Status != run.Failed {
		t.Fatalf("run status = %s, want %s", stored.Status, run.Failed)
	}
}

func TestEngineFailsRunWhenCommandHandlerIsMissing(t *testing.T) {
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
		)),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("missing-handler"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	_, err = engine.Tick(context.Background(), "missing-handler")
	if err == nil {
		t.Fatal("expected missing handler error")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("error = %v", err)
	}

	stored, err := engine.LoadRun(context.Background(), "missing-handler")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if stored.Status != run.Failed {
		t.Fatalf("run status = %s, want %s", stored.Status, run.Failed)
	}
}

func TestEngineDetectsNondeterministicCommandArgs(t *testing.T) {
	var ticks int
	engine := New(
		WithRuntime(fake.NewFuncRuntime(func(context.Context, runtime.Request) (runtime.Result, error) {
			ticks++
			args := json.RawMessage(`{"message":"first"}`)
			if ticks > 1 {
				args = json.RawMessage(`{"message":"second"}`)
			}
			return runtime.Result{
				Status:  runtime.Command,
				Command: echoCommand(args),
			}, nil
		})),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			return command.Receipt{Result: json.RawMessage(`{"ok":true}`)}, nil
		})),
		WithGrant(capability.Grant{
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
	var nondeterminism NondeterminismError
	if !errors.As(err, &nondeterminism) {
		t.Fatalf("error = %T %v, want NondeterminismError", err, err)
	}
	if nondeterminism.CommandID != "echo-step" {
		t.Fatalf("nondeterministic command = %q", nondeterminism.CommandID)
	}
}

func TestEngineQuarantinesUnknownCommandOutcome(t *testing.T) {
	var calls int
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			calls++
			return command.Receipt{}, command.Unknown(errors.New("external write may have succeeded"))
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("unknown"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	_, err = engine.Tick(context.Background(), "unknown")
	if err == nil {
		t.Fatal("expected unknown command error")
	}
	if !strings.Contains(err.Error(), "command outcome unknown") {
		t.Fatalf("error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}

	unknown, err := engine.ListUnknownCommands(context.Background(), "unknown")
	if err != nil {
		t.Fatalf("list unknown commands: %v", err)
	}
	if len(unknown) != 1 || unknown[0].ID != "echo-step" {
		t.Fatalf("unknown commands = %#v", unknown)
	}
	if !strings.Contains(unknown[0].Reason, "external write may have succeeded") {
		t.Fatalf("unknown reason = %q", unknown[0].Reason)
	}

	_, err = engine.Tick(context.Background(), "unknown")
	if err == nil {
		t.Fatal("expected unknown command to require manual recovery")
	}
	var stateErr CommandStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("error = %T %v, want CommandStateError", err, err)
	}
	if stateErr.State != "unknown" {
		t.Fatalf("command state = %q", stateErr.State)
	}
	if calls != 1 {
		t.Fatalf("handler calls after retry = %d, want 1", calls)
	}
}

func TestEngineRecoversUnknownCommandOutcome(t *testing.T) {
	var calls int
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
			fake.CompleteWithFirstCommandResult(),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			calls++
			return command.Receipt{}, command.Unknown(errors.New("external write may have succeeded"))
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("recover-unknown"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	_, err = engine.Tick(context.Background(), "recover-unknown")
	if err == nil {
		t.Fatal("expected unknown command error")
	}

	err = engine.RecoverCommand(context.Background(), "recover-unknown", "echo-step", command.Receipt{
		Result: json.RawMessage(`{"message":"verified"}`),
	})
	if err != nil {
		t.Fatalf("recover command: %v", err)
	}

	result, err := engine.Tick(context.Background(), "recover-unknown")
	if err != nil {
		t.Fatalf("tick after recovery: %v", err)
	}
	if result.Status != run.TickCompleted {
		t.Fatalf("status = %s, want %s", result.Status, run.TickCompleted)
	}
	if string(result.Output) != `{"message":"verified"}` {
		t.Fatalf("output = %s", result.Output)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestEngineRejectsRecoveringCompletedUnknownCommand(t *testing.T) {
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			return command.Receipt{}, command.Unknown(errors.New("external write may have succeeded"))
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("recover-once"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	if _, err := engine.Tick(context.Background(), "recover-once"); err == nil {
		t.Fatal("expected unknown command error")
	}
	if err := engine.RecoverCommand(context.Background(), "recover-once", "echo-step", command.Receipt{Result: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("recover command: %v", err)
	}

	err = engine.RecoverCommand(context.Background(), "recover-once", "echo-step", command.Receipt{Result: json.RawMessage(`{}`)})
	if err == nil {
		t.Fatal("expected duplicate recovery error")
	}
	var stateErr CommandStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("error = %T %v, want CommandStateError", err, err)
	}
	if stateErr.State != "completed" {
		t.Fatalf("command state = %q", stateErr.State)
	}
}

func TestEngineFailsPendingCommand(t *testing.T) {
	var calls int
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			calls++
			return command.Receipt{}, command.Pending("waiting for approval")
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("fail-pending"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	first, err := engine.Tick(context.Background(), "fail-pending")
	if err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if first.Status != run.TickRunning || first.CommandID != "echo-step" {
		t.Fatalf("first tick = %#v", first)
	}

	err = engine.Deny(context.Background(), "fail-pending", "echo-step", "approval denied")
	if err != nil {
		t.Fatalf("fail command: %v", err)
	}

	stored, err := engine.LoadRun(context.Background(), "fail-pending")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if stored.Status != run.Failed {
		t.Fatalf("run status = %s, want %s", stored.Status, run.Failed)
	}
	if stored.FailureMessage != "approval denied" {
		t.Fatalf("failure message = %q", stored.FailureMessage)
	}

	result, err := engine.Tick(context.Background(), "fail-pending")
	if err != nil {
		t.Fatalf("tick failed run: %v", err)
	}
	if result.Status != run.TickFailed {
		t.Fatalf("tick status = %s, want %s", result.Status, run.TickFailed)
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "approval denied") {
		t.Fatalf("tick error = %v", result.Error)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestEngineDoesNotRetryFailedCommandWithoutRetryPolicy(t *testing.T) {
	var calls int
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			calls++
			return command.Receipt{}, errors.New("handler failed")
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("failed-command"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	_, err = engine.Tick(context.Background(), "failed-command")
	if err == nil {
		t.Fatal("expected handler failure")
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}

	result, err := engine.Tick(context.Background(), "failed-command")
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if result.Status != run.TickFailed {
		t.Fatalf("status = %s, want %s", result.Status, run.TickFailed)
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "handler failed") {
		t.Fatalf("result error = %v", result.Error)
	}
	if calls != 1 {
		t.Fatalf("handler calls after failed replay = %d, want 1", calls)
	}

	loaded, err := engine.LoadRun(context.Background(), "failed-command")
	if err != nil {
		t.Fatalf("load run: %v", err)
	}
	if loaded.Status != run.Failed {
		t.Fatalf("loaded status = %s, want %s", loaded.Status, run.Failed)
	}
	if !strings.Contains(loaded.FailureMessage, "handler failed") {
		t.Fatalf("failure message = %q", loaded.FailureMessage)
	}
}

func TestEngineDoesNotRetryPendingCommand(t *testing.T) {
	var calls int
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			calls++
			return command.Receipt{}, command.Pending("waiting for approval")
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("pending"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	first, err := engine.Tick(context.Background(), "pending")
	if err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if first.Status != run.TickRunning || first.CommandID != "echo-step" {
		t.Fatalf("first tick = %#v", first)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}

	second, err := engine.Tick(context.Background(), "pending")
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if second.Status != run.TickRunning || second.CommandID != "echo-step" {
		t.Fatalf("second tick = %#v", second)
	}
	if calls != 1 {
		t.Fatalf("handler calls after pending replay = %d, want 1", calls)
	}

	pending, err := engine.ListPendingCommands(context.Background(), "pending")
	if err != nil {
		t.Fatalf("list pending commands: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "echo-step" {
		t.Fatalf("pending commands = %#v", pending)
	}
	if !strings.Contains(pending[0].Reason, "waiting for approval") {
		t.Fatalf("pending reason = %q", pending[0].Reason)
	}

	approvals, err := engine.ListApprovals(context.Background(), "pending")
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(approvals) != 1 || approvals[0].Command.ID != "echo-step" {
		t.Fatalf("approvals = %#v", approvals)
	}

	events, err := engine.LoadHistory(context.Background(), "pending")
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if len(events) != 3 || events[2].Type != history.CommandPending {
		t.Fatalf("events = %#v", events)
	}
}

func TestEngineRejectsCommandAfterUnresolvedPendingCommand(t *testing.T) {
	var calls int
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommandWithID("echo-one", json.RawMessage(`{"message":"one"}`))),
			fake.Emit(echoCommandWithID("echo-two", json.RawMessage(`{"message":"two"}`))),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			calls++
			return command.Receipt{}, command.Pending("waiting for approval")
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("pending-order"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}
	if _, err := engine.Tick(context.Background(), "pending-order"); err != nil {
		t.Fatalf("first tick: %v", err)
	}

	_, err = engine.Tick(context.Background(), "pending-order")
	if err == nil {
		t.Fatal("expected strict replay order error")
	}
	if !strings.Contains(err.Error(), "unresolved command") {
		t.Fatalf("error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestEngineResolvesPendingCommand(t *testing.T) {
	var calls int
	engine := New(
		WithRuntime(fake.NewRuntime(
			fake.Emit(echoCommand(json.RawMessage(`{"message":"hello"}`))),
			fake.CompleteWithFirstCommandResult(),
		)),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			calls++
			return command.Receipt{}, command.Pending("waiting for approval")
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), testInvocation("resolve-pending"))
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	first, err := engine.Tick(context.Background(), "resolve-pending")
	if err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if first.Status != run.TickRunning || first.CommandID != "echo-step" {
		t.Fatalf("first tick = %#v", first)
	}

	err = engine.Approve(context.Background(), "resolve-pending", "echo-step", command.Receipt{
		Result: json.RawMessage(`{"approved":true}`),
	})
	if err != nil {
		t.Fatalf("resolve command: %v", err)
	}

	second, err := engine.Tick(context.Background(), "resolve-pending")
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if second.Status != run.TickCompleted {
		t.Fatalf("status = %s, want %s", second.Status, run.TickCompleted)
	}
	if string(second.Output) != `{"approved":true}` {
		t.Fatalf("output = %s", second.Output)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
}

func TestConcurrentTicksConflictOnSameRunVersion(t *testing.T) {
	store := memory.NewMemoryStore()
	blockingRuntime := newBlockingCommandRuntime(echoCommand(json.RawMessage(`{"message":"hello"}`)))
	first := New(
		WithStore(store),
		WithRuntime(blockingRuntime),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			return command.Receipt{Result: json.RawMessage(`{"ok":true}`)}, nil
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)
	second := New(
		WithStore(store),
		WithRuntime(blockingRuntime),
		WithCommandHandler("test.echo", commandHandlerFunc(func(context.Context, command.Request) (command.Receipt, error) {
			return command.Receipt{Result: json.RawMessage(`{"ok":true}`)}, nil
		})),
		WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  "sha256:test",
			CommandName:   "test.echo",
		}),
	)

	if _, err := first.Start(context.Background(), testInvocation("concurrent-tick")); err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	errs := make(chan error, 2)
	go func() {
		_, err := first.Tick(context.Background(), "concurrent-tick")
		errs <- err
	}()
	go func() {
		_, err := second.Tick(context.Background(), "concurrent-tick")
		errs <- err
	}()

	blockingRuntime.releaseWhenBothReady(t)

	var successes int
	var conflicts int
	for i := 0; i < 2; i++ {
		err := <-errs
		if err == nil {
			successes++
			continue
		}
		var conflict history.VersionConflictError
		if errors.As(err, &conflict) {
			conflicts++
			continue
		}
		t.Fatalf("unexpected tick error: %T %v", err, err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

type commandHandlerFunc func(context.Context, command.Request) (command.Receipt, error)

type recordingBroker struct {
	request capability.Request
}

type blockingCommandRuntime struct {
	command command.Command
	ready   chan struct{}
	release chan struct{}

	mu    sync.Mutex
	calls int
}

func newBlockingCommandRuntime(command command.Command) *blockingCommandRuntime {
	return &blockingCommandRuntime{
		command: command,
		ready:   make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (r *blockingCommandRuntime) Invoke(_ context.Context, _ runtime.Request) (runtime.Result, error) {
	r.mu.Lock()
	r.calls++
	if r.calls == 2 {
		close(r.ready)
	}
	r.mu.Unlock()

	<-r.release
	return runtime.Result{
		Status:  runtime.Command,
		Command: r.command,
	}, nil
}

func (r *blockingCommandRuntime) releaseWhenBothReady(t *testing.T) {
	t.Helper()

	select {
	case <-r.ready:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for concurrent runtime invocations")
	}
	close(r.release)
}

func (b *recordingBroker) Authorize(req capability.Request) capability.Decision {
	b.request = req
	return capability.Decision{Allowed: true}
}

func (f commandHandlerFunc) Safety() command.Safety {
	return command.Safety{
		SideEffecting:          true,
		RequiresIdempotencyKey: true,
		UnknownPolicy:          command.UnknownQuarantine,
	}
}

func (f commandHandlerFunc) Execute(ctx context.Context, req command.Request) (command.Receipt, error) {
	return f(ctx, req)
}

func echoCommand(args json.RawMessage) command.Command {
	return echoCommandWithID("echo-step", args)
}

func echoCommandWithID(id string, args json.RawMessage) command.Command {
	return command.Command{
		ID:   id,
		Name: "test.echo",
		Mode: command.ModeCommand,
		Args: args,
	}
}

func testInvocation(runID string) run.Invocation {
	return run.Invocation{
		RunID: runID,
		Module: module.Ref{
			Name:       "test-module",
			Digest:     "sha256:test",
			Entrypoint: "run",
		},
		Principal: run.Principal{Type: "user", ID: "rob"},
		Source:    run.Source{Type: "test", ID: "unit"},
		Input:     json.RawMessage(`{"input":true}`),
	}
}
