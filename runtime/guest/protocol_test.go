package guest

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInvocationProtocolUsesStableFieldNames(t *testing.T) {
	data, err := json.Marshal(Invocation{
		RunID: "run-1",
		Module: ModuleRef{
			Name:       "workflow",
			Digest:     "sha256:test",
			Entrypoint: "run",
			Source:     "workflow.wasm",
		},
		Input: json.RawMessage(`{"input":true}`),
		CommandResults: []CommandResult{{
			ID:       "step-1",
			Name:     "test.echo",
			Mode:     ModeCommand,
			ArgsHash: "hash",
			Result:   json.RawMessage(`{"ok":true}`),
		}},
	})
	if err != nil {
		t.Fatalf("marshal invocation: %v", err)
	}

	for _, field := range []string{
		`"run_id":"run-1"`,
		`"entrypoint":"run"`,
		`"command_results"`,
		`"args_hash":"hash"`,
	} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("encoded invocation missing %s: %s", field, data)
		}
	}
}
