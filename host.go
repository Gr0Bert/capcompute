package capcompute

import (
	"context"
	"encoding/json"
	"fmt"

	extism "github.com/extism/go-sdk"

	"github.com/aurora-capcompute/capcompute/sys"
)

type pidContextKey struct{}

type hostResponse struct {
	Status  sys.SyscallStatus `json:"status"`
	Result  json.RawMessage   `json:"result,omitempty"`
	Message string            `json:"message,omitempty"`
}

// hostFunction registers the single syscall entry point. Guests import
// `extism:host/compute syscall`; every host capability flows through it.
func hostFunction[ID comparable, K PID[ID]](table ProcessTable[ID, K]) extism.HostFunction {
	host := extism.NewHostFunctionWithStack(
		"syscall",
		func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
			stack[0] = dispatchSyscall(ctx, table, plugin, stack[0])
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
	host.SetNamespace("extism:host/compute")
	return host
}

func dispatchSyscall[ID comparable, K PID[ID]](
	ctx context.Context,
	table ProcessTable[ID, K],
	plugin *extism.CurrentPlugin,
	offset uint64,
) uint64 {
	pid, ok := ctx.Value(pidContextKey{}).(ID)
	if !ok {
		return returnToGuest(plugin, hostResponse{Status: sys.StatusFailed, Message: "pid missing from context"})
	}
	process, err := table.LoadProcess(ctx, pid)
	if err != nil {
		return returnToGuest(plugin, hostResponse{Status: sys.StatusFailed, Message: "process not found"})
	}
	rawSyscall, err := plugin.ReadBytes(offset)
	if err != nil {
		return returnToGuest(plugin, hostResponse{Status: sys.StatusFailed, Message: fmt.Errorf("read raw syscall: %w", err).Error()})
	}

	var syscall sys.Syscall
	if err := json.Unmarshal(rawSyscall, &syscall); err != nil {
		return returnToGuest(plugin, hostResponse{Status: sys.StatusFailed, Message: fmt.Errorf("decode syscall: %w", err).Error()})
	}

	result, err := process.dispatcher.Dispatch(ctx, process.GuestData, syscall, sys.Authorization{})
	if err != nil {
		return returnToGuest(plugin, hostResponse{
			Status:  sys.StatusFailed,
			Message: err.Error(),
		})
	}

	return returnToGuest(plugin, hostResponse{
		Status:  result.Status(),
		Result:  result.Result(),
		Message: result.Message(),
	})
}

func returnToGuest(plugin *extism.CurrentPlugin, response hostResponse) uint64 {
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
