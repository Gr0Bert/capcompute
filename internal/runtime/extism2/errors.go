package extism2

import "errors"

var (
	ErrCompiledPluginRequired = errors.New("compiled plugin is required")
	ErrDispatcherRequired     = errors.New("dispatcher is required")
	ErrSessionActive          = errors.New("session is already playing")
	ErrSessionRequired        = errors.New("session is required")
	ErrSessionNotReady        = errors.New("session is not ready")
)
