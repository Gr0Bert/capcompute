//go:build wasm

package main

import (
	"encoding/json"

	"capcompute/runtime/guest"
)

func main() {
	// Required by package main / WASI, but not used as the workflow entrypoint.
}

type approvalRequest struct {
	Reason string          `json:"reason"`
	Input  json.RawMessage `json:"input"`
}

//go:wasmexport run
func run() int32 {
	invocation, err := guest.Input()
	if err != nil {
		return guest.Fail(err.Error())
	}

	args := approvalRequest{
		Reason: "workflow requested approval",
		Input:  invocation.Input,
	}
	result, ok, err := guest.Execute("approval-step", "approval.request", args)
	if err != nil {
		return guest.Fail(err.Error())
	}
	if !ok {
		return 0
	}

	var approved struct {
		Approved bool            `json:"approved"`
		Metadata json.RawMessage `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(result, &approved); err != nil {
		return guest.Fail(err.Error())
	}
	if !approved.Approved {
		return guest.Fail("approval rejected")
	}

	return guest.Complete(map[string]any{
		"approved": true,
		"metadata": approved.Metadata,
	})
}
