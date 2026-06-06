package capcompute

import (
	"capcompute/dispatcher"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	extism "github.com/extism/go-sdk"
)

type sessionKeyContextKey struct{}

type hostResponse struct {
	Status  dispatcher.OutcomeKind `json:"status"`
	Result  json.RawMessage        `json:"result,omitempty"`
	Message string                 `json:"message,omitempty"`
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
	sessionKey, ok := ctx.Value(sessionKeyContextKey{}).(ID)
	if !ok {
		return writeHostResponse(plugin, hostResponse{
			Status:  dispatcher.OutcomeFailed,
			Message: "session key missing from context",
		})
	}
	session, ok := c.session(sessionKey)
	if !ok {
		return writeHostResponse(plugin, hostResponse{
			Status:  dispatcher.OutcomeFailed,
			Message: "session not found",
		})
	}
	if session.dispatcher == nil {
		session.err = errors.New("session dispatcher missing")
		return writeHostResponse(plugin, hostResponse{
			Status:  dispatcher.OutcomeFailed,
			Message: session.err.Error(),
		})
	}

	data, err := plugin.ReadBytes(offset)
	if err != nil {
		session.err = fmt.Errorf("read call: %w", err)
		return writeHostResponse(plugin, hostResponse{
			Status:  dispatcher.OutcomeFailed,
			Message: session.err.Error(),
		})
	}

	var call dispatcher.Call
	if err := json.Unmarshal(data, &call); err != nil {
		session.err = fmt.Errorf("decode call: %w", err)
		return writeHostResponse(plugin, hostResponse{
			Status:  dispatcher.OutcomeFailed,
			Message: session.err.Error(),
		})
	}

	outcome, err := session.dispatcher.Dispatch(ctx, session.guestData, call)
	if err != nil {
		session.err = err
		return writeHostResponse(plugin, hostResponse{
			Status:  dispatcher.OutcomeFailed,
			Message: err.Error(),
		})
	}
	if outcome.Kind() == dispatcher.OutcomeYield {
		session.recordYield(call)
	}
	if outcome.Kind() == dispatcher.OutcomeFailed {
		session.err = errors.New(outcome.Message())
	}

	return writeHostResponse(plugin, hostResponse{
		Status:  outcome.Kind(),
		Result:  outcome.Result(),
		Message: outcome.Message(),
	})
}

func (c *ComputeCompiledPlugin[ID, K]) session(key ID) (*Session[K], bool) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	session, ok := c.sessions[key]
	return session, ok
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
