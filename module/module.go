package module

// Ref identifies the exact workflow module a run should execute.
// Digest is part of replay safety: active runs must keep using the same module bytes.
type Ref struct {
	Name       string `json:"name"`
	Digest     string `json:"digest"`
	Entrypoint string `json:"entrypoint"`
	Source     string `json:"source"`
}
