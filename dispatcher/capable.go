package dispatcher

import "context"

// Capable decorates a dispatcher with guest-callable capability metadata.
type Capable[K any] struct {
	next         Dispatcher[K]
	capabilities []Capability
}

func WithCapabilities[K any](next Dispatcher[K], capabilities []Capability) *Capable[K] {
	return &Capable[K]{
		next:         next,
		capabilities: cloneCapabilities(capabilities),
	}
}

func (d *Capable[K]) Dispatch(ctx context.Context, guestData K, call Call) (Outcome, error) {
	return d.next.Dispatch(ctx, guestData, call)
}

func (d *Capable[K]) Capabilities() []Capability {
	return cloneCapabilities(d.capabilities)
}

func cloneCapabilities(capabilities []Capability) []Capability {
	out := make([]Capability, len(capabilities))
	for i, capability := range capabilities {
		out[i] = capability
		out[i].InputSchema = append([]byte(nil), capability.InputSchema...)
	}
	return out
}
