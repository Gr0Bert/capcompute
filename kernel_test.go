package capcompute

import (
	"context"
	"errors"
	"testing"

	"github.com/aurora-capcompute/capcompute/sys"
)

type testPID struct {
	id    string
	value string
}

func (k testPID) PID() string {
	return k.id
}

type testDispatcher struct{}

func (testDispatcher) Dispatch(_ context.Context, _ testPID, _ sys.Syscall, _ sys.Authorization) (sys.SyscallResult, error) {
	return sys.Result(nil), nil
}

func (testDispatcher) Capabilities() []sys.Capability { return nil }

type testProcessTable struct {
	processes map[string]*Process[testPID]
	saveErr   error
}

func newTestProcessTable(processes map[string]*Process[testPID]) *testProcessTable {
	if processes == nil {
		processes = make(map[string]*Process[testPID])
	}
	return &testProcessTable{processes: processes}
}

func (t *testProcessTable) LoadProcess(_ context.Context, pid string) (*Process[testPID], error) {
	process, ok := t.processes[pid]
	if !ok {
		return nil, ErrProcessRequired
	}
	return process, nil
}

func (t *testProcessTable) SaveProcess(_ context.Context, pid string, process *Process[testPID]) error {
	if t.saveErr != nil {
		return t.saveErr
	}
	t.processes[pid] = process
	return nil
}

func TestNewKernelRequiresProcessTable(t *testing.T) {
	_, err := NewKernel[string, testPID](context.Background(), Config[string, testPID]{})
	if err != ErrProcessTableRequired {
		t.Fatalf("error = %v, want ErrProcessTableRequired", err)
	}
}

func TestKernelExposesShutdown(t *testing.T) {
	var _ func(*Kernel[string, testPID], context.Context) error = (*Kernel[string, testPID]).Shutdown
}

func TestResumeStatusReadsYieldedOutput(t *testing.T) {
	got, err := resumeStatus([]byte(`{"status":"yielded"}`))
	if err != nil {
		t.Fatalf("resume status: %v", err)
	}
	if got != ResumeYielded {
		t.Fatalf("status = %s, want %s", got, ResumeYielded)
	}
}

func TestResumeStatusReadsCompletedOutput(t *testing.T) {
	got, err := resumeStatus([]byte(`{"status":"completed"}`))
	if err != nil {
		t.Fatalf("resume status: %v", err)
	}
	if got != ResumeCompleted {
		t.Fatalf("status = %s, want %s", got, ResumeCompleted)
	}
}

func TestResumeStatusRejectsInvalidOutput(t *testing.T) {
	for _, output := range [][]byte{
		[]byte(`{"answer":"done"}`),
		[]byte(`{"status":"unknown"}`),
		[]byte(`not json`),
	} {
		if _, err := resumeStatus(output); !errors.Is(err, ErrInvalidGuestOutput) {
			t.Fatalf("error = %v for %s, want ErrInvalidGuestOutput", err, output)
		}
	}
}
