package memory

import (
	"testing"

	"capcompute/history"
	"capcompute/internal/history/storetest"
)

func TestMemoryStoreContract(t *testing.T) {
	storetest.Contract(t, func(t *testing.T) history.Store {
		t.Helper()
		return NewMemoryStore()
	})
}
