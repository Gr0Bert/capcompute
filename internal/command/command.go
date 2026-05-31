package command

import (
	"encoding/json"
	"errors"
)

// Command is the internal command identity used for replay and idempotency.
type Command struct {
	ID       string
	Name     string
	Mode     string
	Args     json.RawMessage
	ArgsHash string
}

// New validates the command identity and computes the canonical args hash.
func New(id, name, mode string, args json.RawMessage) (Command, error) {
	if id == "" {
		return Command{}, errors.New("command id is required")
	}
	if name == "" {
		return Command{}, errors.New("command name is required")
	}
	hash, err := ArgsHash(args)
	if err != nil {
		return Command{}, err
	}
	return Command{
		ID:       id,
		Name:     name,
		Mode:     mode,
		Args:     append(json.RawMessage(nil), args...),
		ArgsHash: hash,
	}, nil
}
