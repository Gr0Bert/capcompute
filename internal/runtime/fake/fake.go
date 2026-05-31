package fake

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"capcompute/command"
	"capcompute/runtime"
)

// Runtime is a test/runtime backend that returns scripted or function-driven results.
type Runtime struct {
	mu     sync.Mutex
	steps  []Step
	invoke func(context.Context, runtime.Request) (runtime.Result, error)
}

var _ runtime.Runtime = (*Runtime)(nil)

// Step produces one runtime result for a scripted fake runtime.
type Step func(runtime.Request) (runtime.Result, error)

// NewRuntime creates a runtime that consumes scripted steps in order.
// If the script is exhausted, the last step is repeated.
func NewRuntime(steps ...Step) *Runtime {
	return &Runtime{steps: append([]Step(nil), steps...)}
}

// NewFuncRuntime creates a runtime whose result can depend on each request.
func NewFuncRuntime(invoke func(context.Context, runtime.Request) (runtime.Result, error)) *Runtime {
	return &Runtime{invoke: invoke}
}

// Invoke implements runtime.Runtime.
func (r *Runtime) Invoke(ctx context.Context, req runtime.Request) (runtime.Result, error) {
	if r.invoke != nil {
		return r.invoke(ctx, req)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.steps) == 0 {
		return runtime.Result{}, errors.New("fake runtime has no steps")
	}

	step := r.steps[0]
	if len(r.steps) > 1 {
		r.steps = r.steps[1:]
	}
	return step(req)
}

// Complete returns a step that completes the workflow with output.
func Complete(output json.RawMessage) Step {
	return func(runtime.Request) (runtime.Result, error) {
		return runtime.Result{
			Status: runtime.Completed,
			Output: append(json.RawMessage(nil), output...),
		}, nil
	}
}

// Emit returns a step that emits one workflow command.
func Emit(cmd command.Command) Step {
	return func(runtime.Request) (runtime.Result, error) {
		cmd.Args = append(json.RawMessage(nil), cmd.Args...)
		return runtime.Result{
			Status:  runtime.Command,
			Command: cmd,
		}, nil
	}
}

// CompleteWithFirstCommandResult returns the first replayed command result as workflow output.
func CompleteWithFirstCommandResult() Step {
	return func(req runtime.Request) (runtime.Result, error) {
		if len(req.CommandResults) == 0 {
			return runtime.Result{}, errors.New("fake runtime expected at least one command result")
		}
		return runtime.Result{
			Status: runtime.Completed,
			Output: append(json.RawMessage(nil), req.CommandResults[0].Result...),
		}, nil
	}
}
