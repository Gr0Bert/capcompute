package extism2

import (
	dispatcher2 "capcompute/internal/runtime/extism2/dispatcher"
	"context"
	"encoding/json"
	"fmt"

	extism "github.com/extism/go-sdk"
)

type playStateContextKey struct{}

type playState[K any] struct {
	key        K
	dispatcher dispatcher2.Dispatcher[K]
	yielded    *Call
	err        error
}

type hostResponse struct {
	Status  OutcomeKind     `json:"status"`
	Result  json.RawMessage `json:"result,omitempty"`
	Message string          `json:"message,omitempty"`
}

func (c *ComputeCompiledPlugin[ID, K]) hostFunction() extism.HostFunction {
	host := extism.NewHostFunctionWithStack(
		"play",
		func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
			stack[0] = c.dispatchHostCall(ctx, plugin, stack[0])
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
	host.SetNamespace("extism:host/compute")
	return host
}

func (c *ComputeCompiledPlugin[ID, K]) dispatchHostCall(ctx context.Context, plugin *extism.CurrentPlugin, offset uint64) uint64 {
	state, ok := ctx.Value(playStateContextKey{}).(*playState[K])
	if !ok {
		return writeHostResponse(plugin, hostResponse{
			Status:  OutcomeFailed,
			Message: "play state missing from context",
		})
	}

	data, err := plugin.ReadBytes(offset)
	if err != nil {
		state.err = fmt.Errorf("read call: %w", err)
		return writeHostResponse(plugin, hostResponse{
			Status:  OutcomeFailed,
			Message: state.err.Error(),
		})
	}

	var call Call
	if err := json.Unmarshal(data, &call); err != nil {
		state.err = fmt.Errorf("decode call: %w", err)
		return writeHostResponse(plugin, hostResponse{
			Status:  OutcomeFailed,
			Message: state.err.Error(),
		})
	}

	outcome, err := state.dispatcher.Dispatch(ctx, state.key, call)
	if err != nil {
		state.err = err
		return writeHostResponse(plugin, hostResponse{
			Status:  OutcomeFailed,
			Message: err.Error(),
		})
	}
	if outcome.Kind() == OutcomeYield {
		copied := copyCall(call)
		c.markYielded(state.key, copied)
		state.yielded = &copied
	}
	if err := terminalOutcomeError(outcome); err != nil {
		state.err = err
	}

	return writeHostResponse(plugin, hostResponse{
		Status:  outcome.Kind(),
		Result:  outcome.Result(),
		Message: outcome.Message(),
	})
}

func writeHostResponse(plugin *extism.CurrentPlugin, response hostResponse) uint64 {
	data, err := json.Marshal(response)
	if err != nil {
		panic(fmt.Errorf("encode host response: %w", err))
	}

	offset, err := plugin.WriteBytes(data)
	if err != nil {
		panic(fmt.Errorf("write host response: %w", err))
	}
	return offset
}
