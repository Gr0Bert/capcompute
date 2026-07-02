//go:build tinygo

package main

import (
	"encoding/json"
	"fmt"

	"github.com/extism/go-pdk"
)

//go:wasmimport extism:host/compute syscall
func hostSyscall(uint64) uint64

type input struct {
	Mode string `json:"mode"`
}

type syscall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type hostResponse struct {
	Status  string          `json:"status"`
	Result  json.RawMessage `json:"result,omitempty"`
	Message string          `json:"message,omitempty"`
}

type output struct {
	Status      string          `json:"status"`
	Observation json.RawMessage `json:"observation,omitempty"`
}

//go:wasmexport run
func run() int32 {
	var in input
	if err := pdk.InputJSON(&in); err != nil {
		pdk.SetError(fmt.Errorf("decode input: %w", err))
		return 1
	}

	switch in.Mode {
	case "completed":
		response, err := dispatch(syscall{Name: "host.echo", Args: json.RawMessage(`{"value":"ok"}`)})
		if err != nil {
			pdk.SetError(err)
			return 1
		}
		if response.Status != "result" {
			pdk.SetErrorString("expected result status")
			return 1
		}
		if err := pdk.OutputJSON(output{Status: "completed", Observation: response.Result}); err != nil {
			pdk.SetError(err)
			return 1
		}
		return 0

	case "yielded":
		response, err := dispatch(syscall{Name: "host.yield"})
		if err != nil {
			pdk.SetError(err)
			return 1
		}
		if response.Status != "yield" {
			pdk.SetErrorString("expected yield status")
			return 1
		}
		if err := pdk.OutputJSON(output{Status: "yielded"}); err != nil {
			pdk.SetError(err)
			return 1
		}
		return 0

	case "failed":
		pdk.SetErrorString("guest requested failure")
		return 1

	case "infinite":
		for {
		}

	default:
		pdk.SetErrorString("unknown mode")
		return 1
	}
}

func dispatch(sc syscall) (hostResponse, error) {
	data, err := json.Marshal(sc)
	if err != nil {
		return hostResponse{}, fmt.Errorf("encode syscall: %w", err)
	}

	request := pdk.AllocateBytes(data)
	defer request.Free()

	responseOffset := hostSyscall(request.Offset())
	var response hostResponse
	if err := pdk.JSONFrom(responseOffset, &response); err != nil {
		return hostResponse{}, fmt.Errorf("decode host response: %w", err)
	}
	if response.Status == "failed" {
		return hostResponse{}, fmt.Errorf("host failed: %s", response.Message)
	}
	return response, nil
}
