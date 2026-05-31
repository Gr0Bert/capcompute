package static

import (
	"testing"

	"capcompute/capability"
	"capcompute/command"
	"capcompute/module"
	"capcompute/run"
)

func TestStaticBrokerMatchesOptionalGrantFields(t *testing.T) {
	broker := NewStaticBroker([]capability.Grant{{
		PrincipalType:  "user",
		PrincipalID:    "rob",
		SourceType:     "test",
		SourceID:       "unit",
		ModuleDigest:   "sha256:test",
		CommandID:      "echo-step",
		CommandName:    "test.echo",
		CommandMode:    command.ModeCommand,
		CommandArgsSHA: "sha256:args",
	}})

	decision := broker.Authorize(capability.Request{
		RunID:     "run-1",
		Principal: run.Principal{Type: "user", ID: "rob"},
		Source:    run.Source{Type: "test", ID: "unit"},
		Module:    module.Ref{Digest: "sha256:test"},
		Command: command.Command{
			ID:   "echo-step",
			Name: "test.echo",
			Mode: command.ModeCommand,
		},
		CommandArgsSHA: "sha256:args",
	})
	if !decision.Allowed {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestStaticBrokerDeniesOptionalGrantMismatch(t *testing.T) {
	broker := NewStaticBroker([]capability.Grant{{
		PrincipalType: "user",
		PrincipalID:   "rob",
		SourceType:    "production",
		ModuleDigest:  "sha256:test",
		CommandName:   "test.echo",
	}})

	decision := broker.Authorize(capability.Request{
		Principal: run.Principal{Type: "user", ID: "rob"},
		Source:    run.Source{Type: "test", ID: "unit"},
		Module:    module.Ref{Digest: "sha256:test"},
		Command:   command.Command{Name: "test.echo"},
	})
	if decision.Allowed {
		t.Fatalf("decision = %#v", decision)
	}
}
