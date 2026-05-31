//go:build wasm

package guest

import (
	"encoding/json"
	"fmt"

	"github.com/extism/go-pdk"
)

//go:wasmimport extism:host/workflow command
func hostCommand(uint64) uint64

type workflowResponse struct {
	Status  string  `json:"status"`
	Output  any     `json:"output,omitempty"`
	Command Command `json:"command,omitempty"`
	Message string  `json:"message,omitempty"`
}

type commandResponse struct {
	Status  string          `json:"status"`
	Result  json.RawMessage `json:"result,omitempty"`
	Message string          `json:"message,omitempty"`
}

// Input reads the host invocation envelope.
func Input() (Invocation, error) {
	var invocation Invocation
	if err := pdk.InputJSON(&invocation); err != nil {
		return Invocation{}, fmt.Errorf("read invocation: %w", err)
	}
	return invocation, nil
}

// Complete writes a successful workflow response and returns an Extism exit code.
func Complete(output any) int32 {
	if err := pdk.OutputJSON(workflowResponse{Status: "completed", Output: output}); err != nil {
		pdk.SetError(err)
		return 1
	}
	return 0
}

// Fail writes a failed workflow response and returns an Extism exit code.
func Fail(message string) int32 {
	if err := pdk.OutputJSON(workflowResponse{Status: "failed", Message: message}); err != nil {
		pdk.SetError(err)
		return 1
	}
	return 0
}

// Execute emits a side-effecting command or returns its replayed result.
// The ok return is false when the workflow should stop for this tick.
func Execute(id string, name string, args any) (json.RawMessage, bool, error) {
	return emit(ModeCommand, id, name, args)
}

// Query emits a read-only command or returns its replayed result.
// The ok return is false when the workflow should stop for this tick.
func Query(id string, name string, args any) (json.RawMessage, bool, error) {
	return emit(ModeQuery, id, name, args)
}

func emit(mode Mode, id string, name string, args any) (json.RawMessage, bool, error) {
	data, err := json.Marshal(args)
	if err != nil {
		return nil, false, fmt.Errorf("encode command args: %w", err)
	}
	cmd := Command{
		ID:   id,
		Name: name,
		Mode: mode,
		Args: data,
	}

	mem, err := pdk.AllocateJSON(cmd)
	if err != nil {
		return nil, false, fmt.Errorf("allocate command: %w", err)
	}
	defer mem.Free()

	responseOffset := hostCommand(mem.Offset())
	responseMem := pdk.FindMemory(responseOffset)
	defer responseMem.Free()

	var response commandResponse
	if err := json.Unmarshal(responseMem.ReadBytes(), &response); err != nil {
		return nil, false, fmt.Errorf("decode command response: %w", err)
	}

	switch response.Status {
	case "completed":
		return append(json.RawMessage(nil), response.Result...), true, nil
	case "command":
		if err := pdk.OutputJSON(workflowResponse{Status: "command", Command: cmd}); err != nil {
			return nil, false, fmt.Errorf("emit command response: %w", err)
		}
		return nil, false, nil
	case "failed":
		return nil, false, fmt.Errorf("command failed: %s", response.Message)
	default:
		return nil, false, fmt.Errorf("unknown command response status %q", response.Status)
	}
}
