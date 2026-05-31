package extism_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	capruntime "capcompute"
	"capcompute/capability"
	"capcompute/command"
	"capcompute/module"
	"capcompute/run"
)

func TestExtismRuntimeRunsExampleWorkflow(t *testing.T) {
	dir := t.TempDir()
	moduleRef := buildEchoWorkflow(t, dir)
	engine := capruntime.New(
		capruntime.WithExtismRuntime(capruntime.WithExtismPluginCache(true)),
		capruntime.WithFileStore(filepath.Join(dir, "history.json")),
		capruntime.WithCommandHandler("test.echo", echoHandler{}),
		capruntime.WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  moduleRef.Digest,
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), run.Invocation{
		RunID:     "example",
		Module:    moduleRef,
		Principal: run.Principal{Type: "user", ID: "rob"},
		Source:    run.Source{Type: "test", ID: "integration"},
		Input:     json.RawMessage(`{"message":"hello"}`),
	})
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	first, err := engine.Tick(context.Background(), "example")
	if err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if first.Status != run.TickRunning || first.CommandID != "echo-step" {
		t.Fatalf("first tick = %#v", first)
	}

	second, err := engine.Tick(context.Background(), "example")
	if err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if second.Status != run.TickCompleted {
		t.Fatalf("second tick = %#v", second)
	}
}

func TestExtismRuntimeCanDisableWASI(t *testing.T) {
	dir := t.TempDir()
	moduleRef := buildEchoWorkflow(t, dir)
	engine := capruntime.New(
		capruntime.WithExtismRuntime(capruntime.WithExtismWASI(false)),
		capruntime.WithCommandHandler("test.echo", echoHandler{}),
		capruntime.WithGrant(capability.Grant{
			PrincipalType: "user",
			PrincipalID:   "rob",
			ModuleDigest:  moduleRef.Digest,
			CommandName:   "test.echo",
		}),
	)

	_, err := engine.Start(context.Background(), run.Invocation{
		RunID:     "wasi-disabled",
		Module:    moduleRef,
		Principal: run.Principal{Type: "user", ID: "rob"},
		Source:    run.Source{Type: "test", ID: "integration"},
		Input:     json.RawMessage(`{"message":"hello"}`),
	})
	if err != nil {
		t.Fatalf("start workflow: %v", err)
	}

	_, err = engine.Tick(context.Background(), "wasi-disabled")
	if err == nil {
		t.Fatal("expected WASI import error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "wasi") {
		t.Fatalf("error = %v", err)
	}
}

func buildEchoWorkflow(t *testing.T, dir string) module.Ref {
	t.Helper()

	tinygo, err := exec.LookPath("tinygo")
	if err != nil {
		t.Skip("tinygo is required to build the example workflow")
	}

	wasmPath := filepath.Join(dir, "echo_workflow.wasm")
	build := exec.Command(tinygo, "build", "-target", "wasip1", "-buildmode=c-shared", "-o", wasmPath, "./examples/echo_workflow")
	build.Dir = "../../.."
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("find home directory: %v", err)
		}
		gopath = filepath.Join(home, "go")
	}
	build.Env = append(os.Environ(),
		"HOME="+dir,
		"GOPATH="+gopath,
		"GOCACHE="+filepath.Join(dir, "tinygo-cache"),
	)
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build example workflow: %v\n%s", err, output)
	}

	moduleRef, err := module.FileRef("echo-workflow", wasmPath, "run")
	if err != nil {
		t.Fatalf("create module ref: %v", err)
	}
	return moduleRef
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
