package extism2

import (
	"context"
	"encoding/json"
	"sync"

	extism "github.com/extism/go-sdk"
)

const defaultEntrypoint = "run"

// SessionKey lets user-owned run data expose the stable identity used for session maps.
type SessionKey[ID comparable] interface {
	SessionKey() ID
}

// Config contains everything needed to compile a module and create per-run instances.
type Config[ID comparable, K SessionKey[ID]] struct {
	Manifest       extism.Manifest
	PluginConfig   extism.PluginConfig
	InstanceConfig extism.PluginInstanceConfig
	Entrypoint     string
	Dispatchers    DispatcherFactory[K]
}

// ComputeCompiledPlugin owns one compiled module, dispatcher factory, and per-key sessions.
type ComputeCompiledPlugin[ID comparable, K SessionKey[ID]] struct {
	compiled       *extism.CompiledPlugin
	dispatchers    DispatcherFactory[K]
	instanceConfig extism.PluginInstanceConfig
	entrypoint     string

	sessionsMu sync.Mutex
	sessions   map[ID]*Session[K]
	active     map[ID]struct{}
}

// Session owns the reusable Extism plugin instance for one key.
// Session state is not thread-safe; ComputeCompiledPlugin serializes Play per key.
type Session[K any] struct {
	Key     K
	plugin  *extism.Plugin
	ready   bool
	yielded *Call
}

// NewComputeCompiledPlugin compiles a module and registers the dispatcher host function.
func NewComputeCompiledPlugin[ID comparable, K SessionKey[ID]](ctx context.Context, config Config[ID, K]) (*ComputeCompiledPlugin[ID, K], error) {
	if config.Dispatchers == nil {
		return nil, ErrDispatcherRequired
	}

	entrypoint := config.Entrypoint
	if entrypoint == "" {
		entrypoint = defaultEntrypoint
	}

	compute := &ComputeCompiledPlugin[ID, K]{
		dispatchers:    config.Dispatchers,
		instanceConfig: config.InstanceConfig,
		entrypoint:     entrypoint,
		sessions:       make(map[ID]*Session[K]),
		active:         make(map[ID]struct{}),
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

// PlayRequest is one guest invocation attempt.
type PlayRequest struct {
	Input      json.RawMessage
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
type PlayResult[K any] struct {
	Key     K
	Status  PlayStatus
	Output  json.RawMessage
	Yielded *Call
	Exit    uint32
	Err     error
}

// Play starts one exclusive guest invocation for key in its own goroutine.
func (c *ComputeCompiledPlugin[ID, K]) Play(ctx context.Context, key K, req PlayRequest) (<-chan PlayResult[K], error) {
	dispatcher, err := c.dispatchers.NewDispatcher(ctx, key)
	if err != nil {
		return nil, err
	}
	session, err := c.beginPlay(ctx, key)
	if err != nil {
		return nil, err
	}

	results := make(chan PlayResult[K], 1)
	go func() {
		defer close(results)
		defer c.endPlay(key)
		results <- c.play(ctx, key, session, dispatcher, req)
	}()
	return results, nil
}

// Ready reports whether an async result has marked the session ready for another play.
func (c *ComputeCompiledPlugin[ID, K]) Ready(key K) bool {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	session, ok := c.sessions[key.SessionKey()]
	return ok && session.ready
}

// MarkReady flips the per-session ready flag after an async result has been recorded.
func (c *ComputeCompiledPlugin[ID, K]) MarkReady(key K) bool {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	session, ok := c.sessions[key.SessionKey()]
	if !ok {
		return false
	}
	session.ready = true
	return true
}

// Close releases all session instances and the compiled plugin.
func (c *ComputeCompiledPlugin[ID, K]) Close(ctx context.Context) error {
	c.sessionsMu.Lock()
	if len(c.active) > 0 {
		c.sessionsMu.Unlock()
		return ErrSessionActive
	}
	sessions := c.sessions
	compiled := c.compiled
	c.sessions = make(map[ID]*Session[K])
	c.active = make(map[ID]struct{})
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

func (c *ComputeCompiledPlugin[ID, K]) beginPlay(ctx context.Context, key K) (*Session[K], error) {
	sessionKey := key.SessionKey()

	c.sessionsMu.Lock()
	if _, ok := c.active[sessionKey]; ok {
		c.sessionsMu.Unlock()
		return nil, ErrSessionActive
	}
	if session, ok := c.sessions[sessionKey]; ok {
		c.active[sessionKey] = struct{}{}
		session.ready = false
		session.yielded = nil
		c.sessionsMu.Unlock()
		return session, nil
	}
	if c.compiled == nil {
		c.sessionsMu.Unlock()
		return nil, ErrCompiledPluginRequired
	}
	c.active[sessionKey] = struct{}{}
	c.sessionsMu.Unlock()

	plugin, err := c.compiled.Instance(ctx, c.instanceConfig)
	if err != nil {
		c.endPlay(key)
		return nil, err
	}
	session := &Session[K]{Key: key, plugin: plugin}

	c.sessionsMu.Lock()
	c.sessions[sessionKey] = session
	c.sessionsMu.Unlock()
	return session, nil
}

func (c *ComputeCompiledPlugin[ID, K]) endPlay(key K) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	delete(c.active, key.SessionKey())
}

func (c *ComputeCompiledPlugin[ID, K]) markYielded(key K, call Call) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	session, ok := c.sessions[key.SessionKey()]
	if !ok {
		return
	}
	copied := copyCall(call)
	session.yielded = &copied
}

func (c *ComputeCompiledPlugin[ID, K]) play(ctx context.Context, key K, session *Session[K], dispatcher Dispatcher[K], req PlayRequest) PlayResult[K] {
	entrypoint := req.Entrypoint
	if entrypoint == "" {
		entrypoint = c.entrypoint
	}
	if entrypoint == "" {
		entrypoint = defaultEntrypoint
	}

	state := &playState[K]{
		key:        session.Key,
		dispatcher: dispatcher,
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
	if checker, ok := dispatcher.(CompletionChecker); ok {
		if err := checker.CheckCompleted(); err != nil {
			return PlayResult[K]{Key: key, Status: PlayFailed, Exit: exit, Err: err}
		}
	}
	return PlayResult[K]{
		Key:    key,
		Status: PlayCompleted,
		Output: append(json.RawMessage(nil), output...),
		Exit:   exit,
	}
}
