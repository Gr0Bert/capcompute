package extism

import (
	"context"
	"encoding/json"
	"fmt"

	extismsdk "github.com/extism/go-sdk"

	"capcompute/command"
	internalcommand "capcompute/internal/command"
)

type commandHost struct {
	results []command.Result
	emitted *command.Command
}

type commandHostKey struct{}

type commandResponse struct {
	Status  string          `json:"status"`
	Result  json.RawMessage `json:"result,omitempty"`
	Message string          `json:"message,omitempty"`
}

func newCommandHost(results []command.Result) *commandHost {
	return &commandHost{results: append([]command.Result(nil), results...)}
}

func function() extismsdk.HostFunction {
	fn := extismsdk.NewHostFunctionWithStack(
		"command",
		func(ctx context.Context, plugin *extismsdk.CurrentPlugin, stack []uint64) {
			host, ok := ctx.Value(commandHostKey{}).(*commandHost)
			if !ok {
				outputError(plugin, stack, "command host is missing")
				return
			}

			output, err := host.execute(plugin, stack[0])
			if err != nil {
				output = commandResponse{Status: "failed", Message: err.Error()}
			}

			data, err := json.Marshal(output)
			if err != nil {
				data = []byte(`{"status":"failed","message":"encode command response"}`)
			}
			offset, err := plugin.WriteBytes(data)
			if err != nil {
				panic(err)
			}
			stack[0] = offset
		},
		[]extismsdk.ValueType{extismsdk.ValueTypePTR},
		[]extismsdk.ValueType{extismsdk.ValueTypePTR},
	)
	fn.SetNamespace("extism:host/workflow")
	return fn
}

func outputError(plugin *extismsdk.CurrentPlugin, stack []uint64, message string) {
	data, err := json.Marshal(commandResponse{Status: "failed", Message: message})
	if err != nil {
		data = []byte(`{"status":"failed","message":"encode command response"}`)
	}
	offset, err := plugin.WriteBytes(data)
	if err != nil {
		panic(err)
	}
	stack[0] = offset
}

func (h *commandHost) execute(plugin *extismsdk.CurrentPlugin, offset uint64) (commandResponse, error) {
	data, err := plugin.ReadBytes(offset)
	if err != nil {
		return commandResponse{}, fmt.Errorf("read command: %w", err)
	}
	return h.handle(data)
}

func (h *commandHost) handle(data []byte) (commandResponse, error) {
	var cmd command.Command
	if err := json.Unmarshal(data, &cmd); err != nil {
		return commandResponse{}, fmt.Errorf("decode command: %w", err)
	}
	normalized, err := internalcommand.New(cmd.ID, cmd.Name, string(cmd.Mode), cmd.Args)
	if err != nil {
		return commandResponse{}, err
	}

	for _, result := range h.results {
		if result.ID == cmd.ID {
			if result.Name != cmd.Name || result.Mode != cmd.Mode || result.ArgsHash != normalized.ArgsHash {
				return commandResponse{}, fmt.Errorf("nondeterministic command %q: expected %s %s got %s %s",
					cmd.ID,
					result.Name,
					result.ArgsHash,
					cmd.Name,
					normalized.ArgsHash,
				)
			}
			return commandResponse{
				Status: "completed",
				Result: append(json.RawMessage(nil), result.Result...),
			}, nil
		}
	}

	if h.emitted != nil {
		return commandResponse{}, fmt.Errorf("workflow emitted multiple unresolved commands in one invocation")
	}

	cmd.Args = append(json.RawMessage(nil), cmd.Args...)
	h.emitted = &cmd
	return commandResponse{Status: "command"}, nil
}
