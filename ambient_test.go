package capcompute

import (
	"bytes"
	"context"
	"errors"
	"testing"

	extism "github.com/extism/go-sdk"
)

// Kernel law #1 (no ambient authority): images granting ambient network or
// filesystem access are refused at kernel construction.
func TestNewKernelRejectsAmbientHosts(t *testing.T) {
	_, err := NewKernel[string, testPID](context.Background(), Config[string, testPID]{
		Image:        extism.Manifest{AllowedHosts: []string{"example.com"}},
		ProcessTable: newTestProcessTable(nil),
	})
	if !errors.Is(err, ErrAmbientAuthority) {
		t.Fatalf("error = %v, want ErrAmbientAuthority", err)
	}
}

func TestNewKernelRejectsAmbientPaths(t *testing.T) {
	_, err := NewKernel[string, testPID](context.Background(), Config[string, testPID]{
		Image:        extism.Manifest{AllowedPaths: map[string]string{"/tmp": "/tmp"}},
		ProcessTable: newTestProcessTable(nil),
	})
	if !errors.Is(err, ErrAmbientAuthority) {
		t.Fatalf("error = %v, want ErrAmbientAuthority", err)
	}
}

// Kernel law #2 (determinism): the ambient sources the kernel pins must
// produce identical sequences on every fresh instance, so a crash-replay
// observes exactly what the original run observed.
func TestDeterministicRandRestartsIdentically(t *testing.T) {
	first := &deterministicRand{state: 0x9E3779B97F4A7C15}
	second := &deterministicRand{state: 0x9E3779B97F4A7C15}

	a := make([]byte, 64)
	b := make([]byte, 64)
	if _, err := first.Read(a); err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := second.Read(b); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("fresh instances diverged")
	}
	if bytes.Equal(a, make([]byte, 64)) {
		t.Fatal("rand produced all zeros")
	}
}
