package extism

import (
	"encoding/json"
	"fmt"

	"capcompute/command"
	"capcompute/module"
	"capcompute/runtime"
)

// invocation is the JSON payload sent to the workflow export.
type invocation struct {
	RunID          string           `json:"run_id"`
	Module         module.Ref       `json:"module"`
	Input          json.RawMessage  `json:"input"`
	CommandResults []command.Result `json:"command_results,omitempty"`
}

// response is the JSON payload returned by the workflow export.
type response struct {
	Status  runtime.Status  `json:"status"`
	Output  json.RawMessage `json:"output,omitempty"`
	Command command.Command `json:"command,omitempty"`
	Message string          `json:"message,omitempty"`
}

func encodeInvocation(req runtime.Request) ([]byte, error) {
	return json.Marshal(invocation{
		RunID:          req.RunID,
		Module:         req.Module,
		Input:          req.Input,
		CommandResults: req.CommandResults,
	})
}

func decodeResponse(data []byte) (runtime.Result, error) {
	var res response
	if err := json.Unmarshal(data, &res); err != nil {
		return runtime.Result{}, fmt.Errorf("decode workflow response: %w", err)
	}

	switch res.Status {
	case runtime.Completed:
		return runtime.Result{
			Status: res.Status,
			Output: append(json.RawMessage(nil), res.Output...),
		}, nil
	case runtime.Command:
		res.Command.Args = append(json.RawMessage(nil), res.Command.Args...)
		return runtime.Result{
			Status:  res.Status,
			Command: res.Command,
		}, nil
	case runtime.Failed:
		return runtime.Result{
			Status:  res.Status,
			Message: res.Message,
		}, nil
	default:
		return runtime.Result{}, fmt.Errorf("unknown workflow response status %q", res.Status)
	}
}
