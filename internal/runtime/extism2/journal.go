package extism2

import "context"

// Journal loads replay records and records newly observed call outcomes.
// ComputeCompiledPlugin owns when to call it; dispatchers do not persist state.
type Journal[K comparable] interface {
	Load(ctx context.Context, key K) ([]Record, error)
	Record(ctx context.Context, key K, call Call, outcome Outcome) error
}

// JournalFunc adapts a function into a Journal.
type JournalFunc[K comparable] struct {
	LoadFunc   func(context.Context, K) ([]Record, error)
	RecordFunc func(context.Context, K, Call, Outcome) error
}

func (f JournalFunc[K]) Load(ctx context.Context, key K) ([]Record, error) {
	if f.LoadFunc == nil {
		return nil, nil
	}
	return f.LoadFunc(ctx, key)
}

func (f JournalFunc[K]) Record(ctx context.Context, key K, call Call, outcome Outcome) error {
	if f.RecordFunc == nil {
		return nil
	}
	return f.RecordFunc(ctx, key, call, outcome)
}
