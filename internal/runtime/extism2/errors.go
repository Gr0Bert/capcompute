package extism2

import "errors"

var (
	ErrCompiledPluginRequired = errors.New("compiled plugin is required")
	ErrDispatcherRequired     = errors.New("dispatcher is required")
	ErrSessionActive          = errors.New("session is already playing")
)
