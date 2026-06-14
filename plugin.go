package capcompute

import (
	"capcompute/dispatcher"
	"capcompute/dispatcher/replay"
	"context"
	"encoding/json"
	"errors"

	extism "github.com/extism/go-sdk"
)

var (
	ErrCompiledPluginRequired = errors.New("compiled plugin is required")
	ErrDispatcherRequired     = errors.New("dispatcher is required")
	ErrSessionRequired        = errors.New("session is required")
	ErrSessionActive          = errors.New("session is already playing")
)

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
	Dispatchers    dispatcher.DispatcherFactory[K]
	SessionStore   SessionStore[ID, K]
}

// ComputeCompiledPlugin owns one compiled module, dispatcher factory, and per-key sessions.
type ComputeCompiledPlugin[ID comparable, K SessionKey[ID]] struct {
	compiled       *extism.CompiledPlugin
	dispatchers    dispatcher.DispatcherFactory[K]
	sessionStore   SessionStore[ID, K]
	instanceConfig extism.PluginInstanceConfig
	entrypoint     string
}

// Session owns the reusable Extism plugin instance for one key.
// Session state is not thread-safe; ComputeCompiledPlugin serializes Play per key.
type Session[K any] struct {
	guestData  K
	request    PlayRequest
	plugin     *extism.Plugin
	dispatcher dispatcher.Dispatcher[K]
}

// SessionStore owns per-key sessions and their active/idle lifecycle.
type SessionStore[ID comparable, K SessionKey[ID]] interface {
	LoadSession(ctx context.Context, sessionID ID) (*Session[K], error)
	SaveSession(ctx context.Context, sessionID ID, session *Session[K]) error
	DeleteSession(ctx context.Context, sessionID ID) error
	ListSessions(ctx context.Context) (map[ID]*Session[K], error)
}

// NewComputeCompiledPlugin compiles a module and registers the dispatcher host function.
func NewComputeCompiledPlugin[ID comparable, K SessionKey[ID]](ctx context.Context, config Config[ID, K]) (*ComputeCompiledPlugin[ID, K], error) {
	compute := &ComputeCompiledPlugin[ID, K]{
		dispatchers:    config.Dispatchers,
		sessionStore:   config.SessionStore,
		instanceConfig: config.InstanceConfig,
		entrypoint:     config.Entrypoint,
	}

	compiled, err := extism.NewCompiledPlugin(ctx, config.Manifest, config.PluginConfig, []extism.HostFunction{
		hostFunction(compute.sessionStore),
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
	Yielded *dispatcher.Call
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
		c.endPlay(ctx, sessionKey)
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
	session, dispatcher, err := c.beginReplay(ctx, sessionKey)
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
	sessions, err := c.sessionStore.ListSessions(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session.active {
			return ErrSessionActive
		}
	}
	compiled := c.compiled
	c.compiled = nil

	var closeErr error
	for _, session := range sessions {
		if session.plugin == nil {
			continue
		}
		if err := session.plugin.Close(ctx); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	for sessionID := range sessions {
		if err := c.sessionStore.DeleteSession(ctx, sessionID); err != nil && closeErr == nil {
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
	session, err := c.sessionStore.LoadSession(ctx, sessionKey)
	if err == nil {
		if session.active {
			return nil, ErrSessionActive
		}
		session.active = true
		session.guestData = guestData
		session.request = req
		session.resetPlay()
		return session, nil
	}
	if !errors.Is(err, ErrSessionRequired) {
		return nil, err
	}
	if c.compiled == nil {
		return nil, ErrCompiledPluginRequired
	}
	session = &Session[K]{guestData: guestData, request: req, active: true}

	plugin, err := c.compiled.Instance(ctx, c.instanceConfig)
	if err != nil {
		return nil, err
	}
	session.plugin = plugin
	if err := c.sessionStore.SaveSession(ctx, sessionKey, session); err != nil {
		if closeErr := plugin.Close(ctx); closeErr != nil && err == nil {
			err = closeErr
		}
		return nil, err
	}
	return session, nil
}

func (c *ComputeCompiledPlugin[ID, K]) beginReplay(ctx context.Context, sessionKey ID) (*Session[K], dispatcher.Dispatcher[K], error) {
	session, err := c.sessionStore.LoadSession(ctx, sessionKey)
	if err != nil {
		return nil, nil, ErrSessionRequired
	}
	if session.active {
		return nil, nil, ErrSessionActive
	}
	if session.dispatcher == nil {
		return nil, nil, ErrDispatcherRequired
	}
	session.active = true
	return session, session.dispatcher, nil
}

func (c *ComputeCompiledPlugin[ID, K]) endPlay(ctx context.Context, sessionKey ID) {
	session, err := c.sessionStore.LoadSession(ctx, sessionKey)
	if err == nil {
		session.active = false
	}
}

func (c *ComputeCompiledPlugin[ID, K]) finishPlayResult(ctx context.Context, sessionKey ID, result PlayResult[K]) error {
	if result.Status == PlayYielded {
		if err := c.saveSession(ctx, sessionKey); err != nil {
			c.endPlay(ctx, sessionKey)
			return err
		}
		c.endPlay(ctx, sessionKey)
		return nil
	}
	return c.finishSession(ctx, sessionKey)
}

func (c *ComputeCompiledPlugin[ID, K]) finishSession(ctx context.Context, sessionKey ID) error {
	session, err := c.sessionStore.LoadSession(ctx, sessionKey)
	if err != nil && !errors.Is(err, ErrSessionRequired) {
		return err
	}
	if err := c.sessionStore.DeleteSession(ctx, sessionKey); err != nil {
		return err
	}

	var closeErr error
	if session != nil && session.plugin != nil {
		closeErr = session.plugin.Close(ctx)
	}
	return closeErr
}

func (c *ComputeCompiledPlugin[ID, K]) play(ctx context.Context, sessionKey ID, guestData K, session *Session[K], dispatch dispatcher.Dispatcher[K], req PlayRequest) PlayResult[K] {
	session.startPlay(dispatch)

	callCtx := context.WithValue(ctx, sessionKeyContextKey{}, sessionKey)

	exit, output, err := session.plugin.CallWithContext(callCtx, c.playEntrypoint(req), req.Input)
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
	if checker, ok := dispatch.(replay.CompletionChecker); ok {
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

func (session *Session[K]) resetPlay() {
	session.dispatcher = nil
	session.yielded = nil
	session.err = nil
}

func (c *ComputeCompiledPlugin[ID, K]) playEntrypoint(req PlayRequest) string {
	if req.Entrypoint != "" {
		return req.Entrypoint
	}
	if c.entrypoint != "" {
		return c.entrypoint
	}
	return defaultEntrypoint
}

func (session *Session[K]) startPlay(dispatch dispatcher.Dispatcher[K]) {
	session.dispatcher = dispatch
	session.yielded = nil
	session.err = nil
}

func (session *Session[K]) finishPlay(keepDispatcher bool) {
	if !keepDispatcher {
		session.dispatcher = nil
		session.yielded = nil
	}
	session.err = nil
}

func (c *ComputeCompiledPlugin[ID, K]) saveSession(ctx context.Context, sessionKey ID) error {
	session, err := c.sessionStore.LoadSession(ctx, sessionKey)
	if err != nil {
		return err
	}
	return c.sessionStore.SaveSession(ctx, sessionKey, session)
}
