package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	capruntime "capcompute"
	"capcompute/capability"
	"capcompute/command"
	"capcompute/module"
	"capcompute/run"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: echo_host <echo_workflow.wasm>\n")
		os.Exit(2)
	}
	if err := runExample(context.Background(), os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func runExample(ctx context.Context, wasmPath string) error {
	moduleRef, err := module.FileRef("echo-workflow", wasmPath, "run")
	if err != nil {
		return err
	}

	dir, err := os.MkdirTemp("", "capcompute-echo-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	engine := capruntime.New(
		capruntime.WithExtismRuntime(),
		capruntime.WithFileStore(filepath.Join(dir, "history.json")),
		capruntime.WithCommandHandler("test.echo", echoHandler{}),
		capruntime.WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "example",
			ModuleDigest:  moduleRef.Digest,
			CommandName:   "test.echo",
		}),
	)

	_, err = engine.Start(ctx, run.Invocation{
		RunID:     "example",
		Module:    moduleRef,
		Principal: run.Principal{Type: "user", ID: "example"},
		Source:    run.Source{Type: "example", ID: "echo-host"},
		Input:     json.RawMessage(`{"message":"hello"}`),
	})
	if err != nil {
		return fmt.Errorf("start workflow: %w", err)
	}

	for {
		result, err := engine.Tick(ctx, "example")
		if err != nil {
			return fmt.Errorf("tick workflow: %w", err)
		}
		switch result.Status {
		case run.TickRunning:
			fmt.Printf("command recorded: %s\n", result.CommandID)
		case run.TickCompleted:
			fmt.Printf("completed: %s\n", result.Output)
			return nil
		case run.TickFailed:
			return result.Error
		default:
			return fmt.Errorf("unknown tick status %q", result.Status)
		}
	}
}

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
