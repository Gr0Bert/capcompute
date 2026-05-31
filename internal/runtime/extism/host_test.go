package extism

import (
	"encoding/json"
	"strings"
	"testing"

	"capcompute/command"
	internalcommand "capcompute/internal/command"
)

func TestCommandHostReturnsRecordedResultForSameCommand(t *testing.T) {
	cmd := command.Command{
		ID:   "step-1",
		Name: "test.echo",
		Mode: command.ModeCommand,
		Args: json.RawMessage(`{"message":"hello"}`),
	}
	normalized, err := internalcommand.New(cmd.ID, cmd.Name, string(cmd.Mode), cmd.Args)
	if err != nil {
		t.Fatalf("normalize command: %v", err)
	}

	host := newCommandHost([]command.Result{{
		ID:       cmd.ID,
		Name:     cmd.Name,
		Mode:     cmd.Mode,
		ArgsHash: normalized.ArgsHash,
		Result:   json.RawMessage(`{"ok":true}`),
	}})
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}

	response, err := host.handle(data)
	if err != nil {
		t.Fatalf("handle command: %v", err)
	}
	if response.Status != "completed" {
		t.Fatalf("status = %q", response.Status)
	}
	if string(response.Result) != `{"ok":true}` {
		t.Fatalf("result = %s", response.Result)
	}
}

func TestCommandHostRejectsReplayArgsMismatch(t *testing.T) {
	cmd := command.Command{
		ID:   "step-1",
		Name: "test.echo",
		Mode: command.ModeCommand,
		Args: json.RawMessage(`{"message":"changed"}`),
	}
	host := newCommandHost([]command.Result{{
		ID:       cmd.ID,
		Name:     cmd.Name,
		Mode:     cmd.Mode,
		ArgsHash: "old-hash",
		Result:   json.RawMessage(`{"ok":true}`),
	}})
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}

	_, err = host.handle(data)
	if err == nil {
		t.Fatal("expected nondeterminism error")
	}
	if !strings.Contains(err.Error(), "nondeterministic command") {
		t.Fatalf("error = %v", err)
	}
}
