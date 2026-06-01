package extism2

import "errors"

var (
	ErrCompiledPluginRequired = errors.New("compiled plugin is required")
	ErrDispatcherRequired     = errors.New("dispatcher is required")
	ErrHandlersRequired       = errors.New("handlers are required")
	ErrOutcomeRequired        = errors.New("outcome is required")
	ErrPolicyRequired         = errors.New("policy is required")
	ErrSessionActive          = errors.New("session is already playing")
)
