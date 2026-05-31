# capcompute

`capcompute` is a durable WASM capability runtime.

Workflow code runs inside WASM. The host owns persistence, replay, capability
checks, approvals, and side effects. Workflow modules can complete, fail, or emit
one host command. The engine records command results and feeds them back during
replay.

## Core Flow

```go
moduleRef, err := module.FileRef("echo-workflow", "echo_workflow.wasm", "run")
if err != nil {
    return err
}

engine := capruntime.New(
    capruntime.WithExtismRuntime(),
    capruntime.WithFileStore("history.json"),
    capruntime.WithCommandHandler("test.echo", echoHandler{}),
    capruntime.WithGrant(capability.Grant{
        PrincipalType: "user",
        PrincipalID:   "example",
        ModuleDigest:  moduleRef.Digest,
        CommandName:   "test.echo",
    }),
)

_, err = engine.Start(ctx, run.Invocation{
    RunID:     "run-1",
    Module:    moduleRef,
    Principal: run.Principal{Type: "user", ID: "example"},
    Source:    run.Source{Type: "example", ID: "local"},
    Input:     json.RawMessage(`{"message":"hello"}`),
})
if err != nil {
    return err
}

for {
    tick, err := engine.Tick(ctx, "run-1")
    if err != nil {
        return err
    }
    if tick.Status == run.TickCompleted {
        fmt.Printf("completed: %s\n", tick.Output)
        return nil
    }
    if tick.Status == run.TickFailed {
        return tick.Error
    }
}
```

Extism runtime options are explicit. WASI is enabled by default because TinyGo
reactor modules need it, but filesystem and HTTP access remain unavailable unless
allowed:

```go
engine := capruntime.New(
    capruntime.WithExtismRuntime(
        capruntime.WithExtismWASI(true),
        capruntime.WithExtismPluginCache(true),
        capruntime.WithExtismAllowedPath("/host/config", "/config"),
        capruntime.WithExtismAllowedHost("api.example.com"),
    ),
)
```

## Commands

Handlers own side effects. The engine authorizes commands before handlers run and
records successful receipts for replay.

```go
type echoHandler struct{}

func (echoHandler) Safety() command.Safety {
    return command.Safety{
        SideEffecting:          true,
        RequiresIdempotencyKey: true,
        UnknownPolicy:          command.UnknownQuarantine,
    }
}

func (echoHandler) Execute(_ context.Context, req command.Request) (command.Receipt, error) {
    return command.Receipt{Result: req.Command.Args}, nil
}
```

Use `command.Pending(reason)` when a command needs external approval or waiting.
Use `command.Unknown(err)` when the handler cannot prove whether an external side
effect happened.

## Approvals And Recovery

Pending commands can be inspected and resolved without re-running handlers.

```go
approvals, err := engine.ListApprovals(ctx, "run-1")
if err != nil {
    return err
}

err = engine.Approve(ctx, "run-1", approvals[0].Command.ID, command.Receipt{
    Result: json.RawMessage(`{"approved":true}`),
})
```

To reject a pending or unknown command:

```go
err := engine.Deny(ctx, "run-1", "command-id", "approval denied")
```

To recover an unknown command after verifying the external side effect:

```go
err := engine.RecoverCommand(ctx, "run-1", "command-id", command.Receipt{
    Result: json.RawMessage(`{"verified":true}`),
})
```

## Inspection

```go
runs, err := engine.ListRuns(ctx, run.Filter{Status: run.Running})
pending, err := engine.ListPendingCommands(ctx, "run-1")
unknown, err := engine.ListUnknownCommands(ctx, "run-1")
history, err := engine.LoadHistory(ctx, "run-1")
```

## WASM Guest

Workflow modules can use `capcompute/runtime/guest`.

```go
//go:wasmexport run
func run() int32 {
    invocation, err := guest.Input()
    if err != nil {
        return guest.Fail(err.Error())
    }

    result, ok, err := guest.Execute("echo-step", "test.echo", invocation.Input)
    if err != nil {
        return guest.Fail(err.Error())
    }
    if !ok {
        return 0
    }

    return guest.Complete(json.RawMessage(result))
}
```

Build TinyGo reactor modules with:

```sh
tinygo build -target wasip1 -buildmode=c-shared -o echo_workflow.wasm ./examples/echo_workflow
```

Then run the host example:

```sh
go run ./examples/echo_host ./echo_workflow.wasm
```

For a WASM module that exercises the pending approval path:

```sh
tinygo build -target wasip1 -buildmode=c-shared -o approval_workflow.wasm ./examples/approval_workflow
```

## Recovery Example

The recovery example uses an in-process runtime to show the command lifecycle
without building WASM:

```sh
go run ./examples/recovery_host
```

It demonstrates pending approval, denial, unknown outcome recovery, failed
commands, and idempotency key checks inside a handler.
