package command

import (
	"encoding/json"
	"testing"
)

func TestArgsHashCanonicalizesObjectKeys(t *testing.T) {
	first, err := ArgsHash(json.RawMessage(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatalf("hash first args: %v", err)
	}
	second, err := ArgsHash(json.RawMessage(`{"a":1,"b":2}`))
	if err != nil {
		t.Fatalf("hash second args: %v", err)
	}
	if first != second {
		t.Fatalf("hashes differ: %s != %s", first, second)
	}
}
