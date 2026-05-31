package extism

import (
	"encoding/json"
	"strings"
	"testing"

	extismsdk "github.com/extism/go-sdk"

	"capcompute/command"
	"capcompute/module"
	"capcompute/run"
	publicruntime "capcompute/runtime"
)

func TestEncodeInvocation(t *testing.T) {
	data, err := encodeInvocation(publicruntime.Request{
		RunID: "run-1",
		Module: module.Ref{
			Name:       "workflow",
			Digest:     "sha256:test",
			Entrypoint: "run",
			Source:     "workflow.wasm",
		},
		Input: json.RawMessage(`{"input":true}`),
		CommandResults: []command.Result{{
			ID:       "step-1",
			Name:     "test.echo",
			Mode:     command.ModeCommand,
			ArgsHash: "hash",
			Result:   json.RawMessage(`{"ok":true}`),
		}},
	})
	if err != nil {
		t.Fatalf("encode invocation: %v", err)
	}

	var got invocation
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode invocation: %v", err)
	}
	if got.RunID != "run-1" {
		t.Fatalf("run id = %q", got.RunID)
	}
	if !json.Valid(data) {
		t.Fatalf("invocation is not json: %s", data)
	}
	if !containsJSONField(data, `"source":"workflow.wasm"`) {
		t.Fatalf("invocation uses unstable module field names: %s", data)
	}
	if got.Module.Source != "workflow.wasm" {
		t.Fatalf("module source = %q", got.Module.Source)
	}
	if len(got.CommandResults) != 1 || got.CommandResults[0].ID != "step-1" {
		t.Fatalf("command results = %#v", got.CommandResults)
	}
}

func containsJSONField(data []byte, field string) bool {
	return strings.Contains(string(data), field)
}

func TestDecodeCommandResponse(t *testing.T) {
	result, err := decodeResponse([]byte(`{
		"status":"command",
		"command":{
			"id":"step-1",
			"name":"test.echo",
			"mode":"command",
			"args":{"message":"hello"}
		}
	}`))
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Status != publicruntime.Command {
		t.Fatalf("status = %s", result.Status)
	}
	if result.Command.ID != "step-1" {
		t.Fatalf("command id = %q", result.Command.ID)
	}
	if string(result.Command.Args) != `{"message":"hello"}` {
		t.Fatalf("command args = %s", result.Command.Args)
	}
}

func TestDecodePluginOutputUsesEmittedCommandWhenGuestReturnsEmpty(t *testing.T) {
	emitted := command.Command{
		ID:   "step-1",
		Name: "test.echo",
		Mode: command.ModeCommand,
		Args: json.RawMessage(`{"message":"hello"}`),
	}

	result, err := decodePluginOutput(nil, &emitted)
	if err != nil {
		t.Fatalf("decode plugin output: %v", err)
	}
	if result.Status != publicruntime.Command {
		t.Fatalf("status = %s", result.Status)
	}
	if result.Command.ID != "step-1" {
		t.Fatalf("command id = %q", result.Command.ID)
	}
}

func TestDecodePluginOutputRejectsDroppedEmittedCommand(t *testing.T) {
	emitted := command.Command{ID: "step-1", Name: "test.echo", Mode: command.ModeCommand}

	_, err := decodePluginOutput([]byte(`{"status":"completed","output":{"ok":true}}`), &emitted)
	if err == nil {
		t.Fatal("expected protocol error")
	}
	if !strings.Contains(err.Error(), "emitted command") {
		t.Fatalf("error = %v", err)
	}
}

func TestManifestUsesModuleSourceAndLimits(t *testing.T) {
	runtime := NewRuntime(Config{
		EnableWASI:   true,
		AllowedHosts: []string{"example.com"},
		AllowedPaths: map[string]string{"/host": "/guest"},
	})
	manifest, err := runtime.manifest(publicruntime.Request{
		Module: module.Ref{
			Name:   "workflow",
			Digest: "sha256:test",
			Source: "workflow.wasm",
		},
		Limits: run.RuntimeLimits{
			TimeoutMillis:  5000,
			MemoryMaxPages: 64,
		},
	})
	if err != nil {
		t.Fatalf("create manifest: %v", err)
	}

	wasm, ok := manifest.Wasm[0].(extismsdk.WasmFile)
	if !ok {
		t.Fatalf("wasm = %T, want WasmFile", manifest.Wasm[0])
	}
	if !strings.Contains(wasm.Path, "workflow.wasm") {
		t.Fatalf("wasm path = %q", wasm.Path)
	}
	if manifest.Timeout != 5000 {
		t.Fatalf("timeout = %d", manifest.Timeout)
	}
	if manifest.Memory == nil || manifest.Memory.MaxPages != 64 {
		t.Fatalf("memory = %#v", manifest.Memory)
	}
	if len(manifest.AllowedHosts) != 1 || manifest.AllowedHosts[0] != "example.com" {
		t.Fatalf("allowed hosts = %#v", manifest.AllowedHosts)
	}
	if manifest.AllowedPaths["/host"] != "/guest" {
		t.Fatalf("allowed paths = %#v", manifest.AllowedPaths)
	}
}
