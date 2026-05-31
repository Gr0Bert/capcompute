package capruntime

type Option func(*Engine)

func WithRuntime(runtime Runtime) Option {
	return func(engine *Engine) {
		engine.runtime = runtime
	}
}

func WithCommandHandler(name string, handler CommandHandler) Option {
	return func(engine *Engine) {
		engine.handlers[name] = handler
	}
}

func WithGrant(grant Grant) Option {
	return func(engine *Engine) {
		engine.grants = append(engine.grants, grant)
	}
}

type Grant struct {
	PrincipalType string
	PrincipalID   string
	ModuleDigest  string
	CommandName   string
}
