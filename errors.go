package capruntime

import "errors"

var (
	ErrRuntimeRequired = errors.New("runtime is required")
	ErrRunIDRequired   = errors.New("run id is required")
	ErrModuleRequired  = errors.New("module digest is required")
)
