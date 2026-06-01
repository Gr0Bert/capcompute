package extism2

import (
	"context"
	"encoding/json"
	"sync"

	extism "github.com/extism/go-sdk"
)

const defaultEntrypoint = "run"

// Config contains everything needed to compile a module and create per-run instances.
type Config[K comparable] struct {
	Manifest       extism.Manifest
	PluginConfig   extism.PluginConfig
	InstanceConfig extism.PluginInstanceConfig
	Entrypoint     string
	Dispatcher     Dispatcher[K]
	Journal        Journal[K]
}

// ComputeCompiledPlugin owns one compiled module, its dispatcher, and per-key sessions.
type ComputeCompiledPlugin[K comparable] struct {
	compiled       *extism.CompiledPlugin
	dispatcher     Dispatcher[K]
	journal        Journal[K]
	instanceConfig extism.PluginInstanceConfig
	entrypoint     string

	sessionsMu sync.Mutex
	sessions   map[K]*Session[K]
	active     map[K]struct{}
}

// Session owns the reusable Extism plugin instance for one key.
// Session state is not thread-safe; ComputeCompiledPlugin serializes Play per key.
type Session[K comparable] struct {
	plugin  *extism.Plugin
	ready   bool
	yielded *Call
}

// NewComputeCompiledPlugin compiles a module and registers the dispatcher host function.
func NewComputeCompiledPlugin[K comparable](ctx context.Context, config Config[K]) (*ComputeCompiledPlugin[K], error) {
	if config.Dispatcher == nil {
		return nil, ErrDispatcherRequired
	}

	entrypoint := config.Entrypoint
	if entrypoint == "" {
		entrypoint = defaultEntrypoint
	}

	compute := &ComputeCompiledPlugin[K]{
		dispatcher:     config.Dispatcher,
		journal:        config.Journal,
		instanceConfig: config.InstanceConfig,
		entrypoint:     entrypoint,
		sessions:       make(map[K]*Session[K]),
		active:         make(map[K]struct{}),
	}

	compiled, err := extism.NewCompiledPlugin(ctx, config.Manifest, config.PluginConfig, []extism.HostFunction{
		compute.hostFunction(),
	})
	if err != nil {
		return nil, err
	}
	compute.compiled = compiled
	return compute, nil
}

// PlayRequest is one guest invocation attempt. Records are replayed from the start each time.
type PlayRequest struct {
	Input      json.RawMessage
	Records    []Record
	UseRecords bool
	Entrypoint string
}

// PlayStatus is the result of one guest invocation attempt.
type PlayStatus string

const (
	PlayCompleted PlayStatus = "completed"
	PlayYielded   PlayStatus = "yielded"
	PlayFailed    PlayStatus = "failed"
)

// PlayResult is delivered when the play goroutine exits.
type PlayResult[K comparable] struct {
	Key     K
	Status  PlayStatus
	Output  json.RawMessage
	Yielded *Call
	Exit    uint32
	Err     error
}

// Play starts one exclusive guest invocation for key in its own goroutine.
func (c *ComputeCompiledPlugin[K]) Play(ctx context.Context, key K, req PlayRequest) (<-chan PlayResult[K], error) {
	records := req.Records
	if !req.UseRecords {
		loaded, err := c.loadRecords(ctx, key)
		if err != nil {
			return nil, err
		}
		records = loaded
	}

	session, err := c.beginPlay(ctx, key)
	if err != nil {
		return nil, err
	}
	req.Records = records

	results := make(chan PlayResult[K], 1)
	go func() {
		defer close(results)
		defer c.endPlay(key)
		results <- c.play(ctx, key, session, req)
	}()
	return results, nil
}

// Ready reports whether an async result has marked the session ready for another play.
func (c *ComputeCompiledPlugin[K]) Ready(key K) bool {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	session, ok := c.sessions[key]
	return ok && session.ready
}

// MarkReady flips the per-session ready flag after an async result has been recorded.
func (c *ComputeCompiledPlugin[K]) MarkReady(key K) bool {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	session, ok := c.sessions[key]
	if !ok {
		return false
	}
	session.ready = true
	return true
}

// Close releases all session instances and the compiled plugin.
func (c *ComputeCompiledPlugin[K]) Close(ctx context.Context) error {
	c.sessionsMu.Lock()
	if len(c.active) > 0 {
		c.sessionsMu.Unlock()
		return ErrSessionActive
	}
	sessions := c.sessions
	compiled := c.compiled
	c.sessions = make(map[K]*Session[K])
	c.active = make(map[K]struct{})
	c.compiled = nil
	c.sessionsMu.Unlock()

	var closeErr error
	for _, session := range sessions {
		if session.plugin == nil {
			continue
		}
		if err := session.plugin.Close(ctx); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if compiled != nil {
		if err := compiled.Close(ctx); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (c *ComputeCompiledPlugin[K]) beginPlay(ctx context.Context, key K) (*Session[K], error) {
	c.sessionsMu.Lock()
	if _, ok := c.active[key]; ok {
		c.sessionsMu.Unlock()
		return nil, ErrSessionActive
	}
	if session, ok := c.sessions[key]; ok {
		c.active[key] = struct{}{}
		session.ready = false
		session.yielded = nil
		c.sessionsMu.Unlock()
		return session, nil
	}
	if c.compiled == nil {
		c.sessionsMu.Unlock()
		return nil, ErrCompiledPluginRequired
	}
	c.active[key] = struct{}{}
	c.sessionsMu.Unlock()

	plugin, err := c.compiled.Instance(ctx, c.instanceConfig)
	if err != nil {
		c.endPlay(key)
		return nil, err
	}
	session := &Session[K]{plugin: plugin}

	c.sessionsMu.Lock()
	c.sessions[key] = session
	c.sessionsMu.Unlock()
	return session, nil
}

func (c *ComputeCompiledPlugin[K]) endPlay(key K) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	delete(c.active, key)
}

func (c *ComputeCompiledPlugin[K]) markYielded(key K, call Call) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	session, ok := c.sessions[key]
	if !ok {
		return
	}
	copied := copyCall(call)
	session.yielded = &copied
}

func (c *ComputeCompiledPlugin[K]) loadRecords(ctx context.Context, key K) ([]Record, error) {
	if c.journal == nil {
		return nil, nil
	}
	return c.journal.Load(ctx, key)
}

func (c *ComputeCompiledPlugin[K]) record(ctx context.Context, key K, call Call, outcome Outcome) error {
	if c.journal == nil {
		return nil
	}
	return c.journal.Record(ctx, key, call, outcome)
}

func (c *ComputeCompiledPlugin[K]) play(ctx context.Context, key K, session *Session[K], req PlayRequest) PlayResult[K] {
	entrypoint := req.Entrypoint
	if entrypoint == "" {
		entrypoint = c.entrypoint
	}
	if entrypoint == "" {
		entrypoint = defaultEntrypoint
	}

	state := &playState[K]{
		key:  key,
		tape: NewTape(req.Records),
	}
	callCtx := context.WithValue(ctx, playStateContextKey{}, state)

	exit, output, err := session.plugin.CallWithContext(callCtx, entrypoint, req.Input)
	if state.err != nil {
		return PlayResult[K]{Key: key, Status: PlayFailed, Exit: exit, Err: state.err}
	}
	if err != nil {
		return PlayResult[K]{Key: key, Status: PlayFailed, Exit: exit, Err: err}
	}
	if state.yielded != nil {
		return PlayResult[K]{
			Key:     key,
			Status:  PlayYielded,
			Yielded: state.yielded,
			Exit:    exit,
		}
	}
	if remaining := state.tape.Remaining(); remaining > 0 {
		return PlayResult[K]{
			Key:    key,
			Status: PlayFailed,
			Exit:   exit,
			Err:    ReplayIncompleteError{Remaining: remaining},
		}
	}
	return PlayResult[K]{
		Key:    key,
		Status: PlayCompleted,
		Output: append(json.RawMessage(nil), output...),
		Exit:   exit,
	}
}
