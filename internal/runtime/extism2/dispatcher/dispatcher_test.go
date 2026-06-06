package dispatcher

import (
	"capcompute/internal/runtime/extism2"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestDefaultDispatcherExecutesHandler(t *testing.T) {
	dispatcher := &DefaultDispatcher[string]{
		Policy: PolicyFunc[string](func(context.Context, string, extism2.Call) Decision {
			return Decision{Allowed: true}
		}),
		Handlers: HandlerFunc[string](func(context.Context, string, extism2.Call) (extism2.Outcome, error) {
			return extism2.Result(json.RawMessage(`{"ok":true}`)), nil
		}),
	}

	outcome, err := dispatcher.Dispatch(context.Background(), "run-1", extism2.Call{Name: "step.one"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if outcome.Kind() != extism2.OutcomeResult {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestDefaultDispatcherRequiresPolicyForNewCall(t *testing.T) {
	dispatcher := &DefaultDispatcher[string]{
		Handlers: HandlerFunc[string](func(context.Context, string, extism2.Call) (extism2.Outcome, error) {
			return extism2.Result(json.RawMessage(`{}`)), nil
		}),
	}

	_, err := dispatcher.Dispatch(context.Background(), "run-1", extism2.Call{Name: "step.one"})
	if !errors.Is(err, extism2.ErrPolicyRequired) {
		t.Fatalf("error = %v, want ErrPolicyRequired", err)
	}
}

func TestDefaultDispatcherRejectsEmptyHandlerOutcome(t *testing.T) {
	dispatcher := &DefaultDispatcher[string]{
		Policy: PolicyFunc[string](func(context.Context, string, extism2.Call) Decision {
			return Decision{Allowed: true}
		}),
		Handlers: HandlerFunc[string](func(context.Context, string, extism2.Call) (extism2.Outcome, error) {
			return extism2.Outcome{}, nil
		}),
	}

	_, err := dispatcher.Dispatch(context.Background(), "run-1", extism2.Call{Name: "step.one"})
	if !errors.Is(err, extism2.ErrOutcomeRequired) {
		t.Fatalf("error = %v, want ErrOutcomeRequired", err)
	}
}

func TestDefaultDispatcherReturnsDeniedCall(t *testing.T) {
	dispatcher := &DefaultDispatcher[string]{
		Policy: PolicyFunc[string](func(context.Context, string, extism2.Call) Decision {
			return Decision{Allowed: false, Reason: "not allowed"}
		}),
		Handlers: HandlerFunc[string](func(context.Context, string, extism2.Call) (extism2.Outcome, error) {
			t.Fatal("handler should not run")
			return extism2.Result(nil), nil
		}),
	}

	outcome, err := dispatcher.Dispatch(context.Background(), "run-1", extism2.Call{Name: "step.one"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if outcome.Kind() != extism2.OutcomeFailed || outcome.Message() != "not allowed" {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestTerminalOutcomeError(t *testing.T) {
	if err := extism2.terminalOutcomeError(extism2.Result(json.RawMessage(`{}`))); err != nil {
		t.Fatalf("result terminal error = %v", err)
	}
	if err := extism2.terminalOutcomeError(extism2.Yield("waiting")); err != nil {
		t.Fatalf("yield terminal error = %v", err)
	}
	if err := extism2.terminalOutcomeError(extism2.Unknown("maybe wrote")); err == nil {
		t.Fatal("expected unknown terminal error")
	}
	if err := extism2.terminalOutcomeError(extism2.Failed("denied")); err == nil {
		t.Fatal("expected failed terminal error")
	}
}
