package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDigestBytes(t *testing.T) {
	got := DigestBytes([]byte("hello"))
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Fatalf("digest = %q, want %q", got, want)
	}
}

func TestFileRef(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.wasm")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write module file: %v", err)
	}

	ref, err := FileRef("workflow", path, "run")
	if err != nil {
		t.Fatalf("create file ref: %v", err)
	}
	if ref.Name != "workflow" {
		t.Fatalf("name = %q", ref.Name)
	}
	if ref.Source != path {
		t.Fatalf("source = %q", ref.Source)
	}
	if ref.Entrypoint != "run" {
		t.Fatalf("entrypoint = %q", ref.Entrypoint)
	}
	if ref.Digest != DigestBytes([]byte("hello")) {
		t.Fatalf("digest = %q", ref.Digest)
	}
}
