package capcompute

import (
	dispatcher2 "capcompute/dispatcher"
	"capcompute/dispatcher/replay"
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
	Dispatchers    dispatcher2.DispatcherFactory[K]
}

// ComputeCompiledPlugin owns one compiled module, dispatcher factory, and per-key sessions.
type ComputeCompiledPlugin[ID comparable, K SessionKey[ID]] struct {
	compiled       *extism.CompiledPlugin
	dispatchers    dispatcher2.DispatcherFactory[K]
	instanceConfig extism.PluginInstanceConfig
	entrypoint     string

	sessionsMu sync.Mutex
	sessions   map[ID]*Session[K]
	active     map[ID]struct{}
}

// Session owns the reusable Extism plugin instance for one key.
// Session state is not thread-safe; ComputeCompiledPlugin serializes Play per key.
type Session[K any] struct {
	guestData  K
	request    PlayRequest
	plugin     *extism.Plugin
	dispatcher dispatcher2.Dispatcher[K]
	yielded    *dispatcher2.Call
	err        error
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
	Yielded *dispatcher2.Call
	Exit    uint32
	Err     error
}

// Play starts one exclusive guest invocation for key in its own goroutine.
func (c *ComputeCompiledPlugin[ID, K]) Play(ctx context.Context, guestData K, req PlayRequest) (<-chan PlayResult[K], error) {
	sessionKey := guestData.SessionKey()
	session, err := c.beginPlay(ctx, sessionKey, guestData, req)
	if err != nil {
		return nil, err
	}
	dispatcher, err := c.dispatchers.NewDispatcher(ctx, guestData)
	if err != nil {
		c.endPlay(sessionKey)
		return nil, err
	}

	results := make(chan PlayResult[K], 1)
	go func() {
		defer close(results)
		result := c.play(ctx, sessionKey, guestData, session, dispatcher, req)
		if err := c.finishPlayResult(ctx, sessionKey, result); err != nil && result.Err == nil {
			result.Status = PlayFailed
			result.Err = err
		}
		results <- result
	}()
	return results, nil
}

// Replay starts another invocation for a yielded session using its existing dispatcher chain.
func (c *ComputeCompiledPlugin[ID, K]) Replay(ctx context.Context, sessionKey ID) (<-chan PlayResult[K], error) {
	session, dispatcher, err := c.beginReplay(sessionKey)
	if err != nil {
		return nil, err
	}

	results := make(chan PlayResult[K], 1)
	go func() {
		defer close(results)
		result := c.play(ctx, sessionKey, session.guestData, session, dispatcher, session.request)
		if err := c.finishPlayResult(ctx, sessionKey, result); err != nil && result.Err == nil {
			result.Status = PlayFailed
			result.Err = err
		}
		results <- result
	}()
	return results, nil
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

func (c *ComputeCompiledPlugin[ID, K]) beginPlay(ctx context.Context, sessionKey ID, guestData K, req PlayRequest) (*Session[K], error) {
	c.sessionsMu.Lock()
	if _, ok := c.active[sessionKey]; ok {
		c.sessionsMu.Unlock()
		return nil, ErrSessionActive
	}
	if session, ok := c.sessions[sessionKey]; ok {
		c.active[sessionKey] = struct{}{}
		session.guestData = guestData
		session.request = req
		session.resetPlay()
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
		c.endPlay(sessionKey)
		return nil, err
	}
	session := &Session[K]{guestData: guestData, request: req, plugin: plugin}

	c.sessionsMu.Lock()
	c.sessions[sessionKey] = session
	c.sessionsMu.Unlock()
	return session, nil
}

func (c *ComputeCompiledPlugin[ID, K]) beginReplay(sessionKey ID) (*Session[K], dispatcher2.Dispatcher[K], error) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	if _, ok := c.active[sessionKey]; ok {
		return nil, nil, ErrSessionActive
	}
	session, ok := c.sessions[sessionKey]
	if !ok {
		return nil, nil, ErrSessionRequired
	}
	if session.dispatcher == nil {
		return nil, nil, ErrDispatcherRequired
	}
	c.active[sessionKey] = struct{}{}
	return session, session.dispatcher, nil
}

func (c *ComputeCompiledPlugin[ID, K]) endPlay(sessionKey ID) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()

	delete(c.active, sessionKey)
}

func (c *ComputeCompiledPlugin[ID, K]) finishPlayResult(ctx context.Context, sessionKey ID, result PlayResult[K]) error {
	if result.Status == PlayYielded {
		c.endPlay(sessionKey)
		return nil
	}
	return c.finishSession(ctx, sessionKey)
}

func (c *ComputeCompiledPlugin[ID, K]) finishSession(ctx context.Context, sessionKey ID) error {
	c.sessionsMu.Lock()
	session := c.sessions[sessionKey]
	delete(c.active, sessionKey)
	delete(c.sessions, sessionKey)
	c.sessionsMu.Unlock()

	if session == nil || session.plugin == nil {
		return nil
	}
	return session.plugin.Close(ctx)
}

func (c *ComputeCompiledPlugin[ID, K]) play(ctx context.Context, sessionKey ID, guestData K, session *Session[K], dispatcher dispatcher2.Dispatcher[K], req PlayRequest) PlayResult[K] {
	entrypoint := req.Entrypoint
	if entrypoint == "" {
		entrypoint = c.entrypoint
	}
	if entrypoint == "" {
		entrypoint = defaultEntrypoint
	}

	session.startPlay(dispatcher)

	callCtx := context.WithValue(ctx, sessionKeyContextKey{}, sessionKey)

	exit, output, err := session.plugin.CallWithContext(callCtx, entrypoint, req.Input)
	if session.err != nil {
		err := session.err
		session.finishPlay(false)
		return PlayResult[K]{Key: guestData, Status: PlayFailed, Exit: exit, Err: err}
	}
	if err != nil {
		session.finishPlay(false)
		return PlayResult[K]{Key: guestData, Status: PlayFailed, Exit: exit, Err: err}
	}
	if session.yielded != nil {
		yielded := session.yielded
		session.finishPlay(true)
		return PlayResult[K]{
			Key:     guestData,
			Status:  PlayYielded,
			Yielded: yielded,
			Exit:    exit,
		}
	}
	if checker, ok := dispatcher.(replay.CompletionChecker); ok {
		if err := checker.CheckCompleted(); err != nil {
			session.finishPlay(false)
			return PlayResult[K]{Key: guestData, Status: PlayFailed, Exit: exit, Err: err}
		}
	}
	session.finishPlay(false)
	return PlayResult[K]{
		Key:    guestData,
		Status: PlayCompleted,
		Output: append(json.RawMessage(nil), output...),
		Exit:   exit,
	}
}

func (s *Session[K]) resetPlay() {
	s.dispatcher = nil
	s.yielded = nil
	s.err = nil
}

func (s *Session[K]) startPlay(dispatcher dispatcher2.Dispatcher[K]) {
	s.dispatcher = dispatcher
	s.yielded = nil
	s.err = nil
}

func (s *Session[K]) finishPlay(keepDispatcher bool) {
	if !keepDispatcher {
		s.dispatcher = nil
		s.yielded = nil
	}
	s.err = nil
}

func (s *Session[K]) recordYield(call dispatcher2.Call) {
	copied := call.Copy()
	s.yielded = &copied
}
