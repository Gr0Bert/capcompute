package memory

import (
	"context"
	"sync"

	"github.com/aurora-capcompute/capcompute"
)

type ProcessTable[ID comparable, K capcompute.PID[ID]] struct {
	mu        sync.Mutex
	processes map[ID]*capcompute.Process[K]
}

func NewProcessTable[ID comparable, K capcompute.PID[ID]]() *ProcessTable[ID, K] {
	return &ProcessTable[ID, K]{
		processes: make(map[ID]*capcompute.Process[K]),
	}
}

func (t *ProcessTable[ID, K]) LoadProcess(_ context.Context, pid ID) (*capcompute.Process[K], error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	process, ok := t.processes[pid]
	if !ok {
		return nil, capcompute.ErrProcessRequired
	}
	return process, nil
}

func (t *ProcessTable[ID, K]) SaveProcess(_ context.Context, pid ID, process *capcompute.Process[K]) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.processes == nil {
		t.processes = make(map[ID]*capcompute.Process[K])
	}
	t.processes[pid] = process
	return nil
}

func (t *ProcessTable[ID, K]) DeleteProcess(_ context.Context, pid ID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.processes, pid)
	return nil
}

func (t *ProcessTable[ID, K]) ListProcesses(context.Context) (map[ID]*capcompute.Process[K], error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	processes := make(map[ID]*capcompute.Process[K], len(t.processes))
	for pid, process := range t.processes {
		processes[pid] = process
	}
	return processes, nil
}
