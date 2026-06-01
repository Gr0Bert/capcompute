package extism2

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestDefaultDispatcherExecutesHandler(t *testing.T) {
	dispatcher := &DefaultDispatcher[string]{
		Policy: PolicyFunc[string](func(context.Context, string, Call) Decision {
			return Decision{Allowed: true}
		}),
		Handlers: HandlerFunc[string](func(context.Context, string, Call) (Outcome, error) {
			return Result(json.RawMessage(`{"ok":true}`)), nil
		}),
	}

	outcome, err := dispatcher.Dispatch(context.Background(), "run-1", Call{Name: "step.one"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if outcome.Kind() != OutcomeResult {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestDefaultDispatcherRequiresPolicyForNewCall(t *testing.T) {
	dispatcher := &DefaultDispatcher[string]{
		Handlers: HandlerFunc[string](func(context.Context, string, Call) (Outcome, error) {
			return Result(json.RawMessage(`{}`)), nil
		}),
	}

	_, err := dispatcher.Dispatch(context.Background(), "run-1", Call{Name: "step.one"})
	if !errors.Is(err, ErrPolicyRequired) {
		t.Fatalf("error = %v, want ErrPolicyRequired", err)
	}
}

func TestDefaultDispatcherRejectsEmptyHandlerOutcome(t *testing.T) {
	dispatcher := &DefaultDispatcher[string]{
		Policy: PolicyFunc[string](func(context.Context, string, Call) Decision {
			return Decision{Allowed: true}
		}),
		Handlers: HandlerFunc[string](func(context.Context, string, Call) (Outcome, error) {
			return Outcome{}, nil
		}),
	}

	_, err := dispatcher.Dispatch(context.Background(), "run-1", Call{Name: "step.one"})
	if !errors.Is(err, ErrOutcomeRequired) {
		t.Fatalf("error = %v, want ErrOutcomeRequired", err)
	}
}

func TestDefaultDispatcherReturnsDeniedCall(t *testing.T) {
	dispatcher := &DefaultDispatcher[string]{
		Policy: PolicyFunc[string](func(context.Context, string, Call) Decision {
			return Decision{Allowed: false, Reason: "not allowed"}
		}),
		Handlers: HandlerFunc[string](func(context.Context, string, Call) (Outcome, error) {
			t.Fatal("handler should not run")
			return Result(nil), nil
		}),
	}

	outcome, err := dispatcher.Dispatch(context.Background(), "run-1", Call{Name: "step.one"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if outcome.Kind() != OutcomeFailed || outcome.Message() != "not allowed" {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestTerminalOutcomeError(t *testing.T) {
	if err := terminalOutcomeError(Result(json.RawMessage(`{}`))); err != nil {
		t.Fatalf("result terminal error = %v", err)
	}
	if err := terminalOutcomeError(Yield("waiting")); err != nil {
		t.Fatalf("yield terminal error = %v", err)
	}
	if err := terminalOutcomeError(Unknown("maybe wrote")); err == nil {
		t.Fatal("expected unknown terminal error")
	}
	if err := terminalOutcomeError(Failed("denied")); err == nil {
		t.Fatal("expected failed terminal error")
	}
}
