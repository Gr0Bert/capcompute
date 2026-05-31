package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func ArgsHash(raw json.RawMessage) (string, error) {
	canonical, err := CanonicalJSON(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func CanonicalJSON(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode command args: %w", err)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode canonical command args: %w", err)
	}
	return canonical, nil
}

func IdempotencyKey(runID, moduleDigest string, command Command) string {
	joined := runID + "\x00" + moduleDigest + "\x00" + command.ID + "\x00" + command.Name + "\x00" + command.ArgsHash
	sum := sha256.Sum256([]byte(joined))
	return "sha256:" + hex.EncodeToString(sum[:])
}
