package dispatcher

import (
	"context"
	"encoding/json"
	"testing"
)

type capableDispatcher struct {
	capabilities []Capability
}

func (capableDispatcher) Dispatch(context.Context, string, Call) (Outcome, error) {
	return Result(nil), nil
}

func (d capableDispatcher) Capabilities() []Capability {
	return d.capabilities
}

func TestCapabilitiesReturnsDefensiveCopy(t *testing.T) {
	source := capableDispatcher{capabilities: []Capability{{
		Name:        "test.call",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}}

	got := Capabilities[string](source)
	got[0].Name = "changed"
	got[0].InputSchema[0] = 'x'

	if source.capabilities[0].Name != "test.call" {
		t.Fatal("capability name was not copied")
	}
	if string(source.capabilities[0].InputSchema) != `{"type":"object"}` {
		t.Fatal("capability schema was not copied")
	}
}

func TestWithCapabilitiesDelegatesAndExports(t *testing.T) {
	wrapped := WithCapabilities[string](capableDispatcher{}, []Capability{{Name: "test.call"}})
	if _, err := wrapped.Dispatch(context.Background(), "key", Call{Name: "test.call"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := wrapped.Capabilities(); len(got) != 1 || got[0].Name != "test.call" {
		t.Fatalf("capabilities = %+v", got)
	}
}
