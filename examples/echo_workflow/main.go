//go:build wasm

package main

import (
	"encoding/json"

	"capcompute/runtime/guest"
)

func main() {
	// Required by package main / WASI, but not used as the workflow entrypoint.
}

//go:wasmexport run
func run() int32 {
	invocation, err := guest.Input()
	if err != nil {
		return guest.Fail(err.Error())
	}

	result, ok, err := guest.Execute("echo-step", "test.echo", invocation.Input)
	if err != nil {
		return guest.Fail(err.Error())
	}
	if !ok {
		return 0
	}

	var output json.RawMessage = result
	return guest.Complete(map[string]any{
		"echo": output,
	})
}
