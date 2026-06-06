package capruntime

import (
	"capcompute/capability"
	"capcompute/command"
	"capcompute/history"
	filehistory "capcompute/internal/history/file"
	extismruntime "capcompute/internal/runtime/extism"
	"capcompute/runtime"
)

// Option customizes Engine construction.
type Option func(*Engine)

// ExtismRuntimeOption customizes the built-in Extism runtime backend.
type ExtismRuntimeOption func(*extismRuntimeOptions)

type extismRuntimeOptions struct {
	enableWASI   bool
	allowedHosts []string
	allowedPaths map[string]string
	cachePlugins bool
}

// WithRuntime sets the workflow runtime backend.
func WithRuntime(runtime runtime.Runtime) Option {
	return func(engine *Engine) {
		engine.runtime = runtime
	}
}

// WithExtismRuntime uses the built-in Extism WASM runtime backend.
func WithExtismRuntime(options ...ExtismRuntimeOption) Option {
	settings := extismRuntimeOptions{enableWASI: true}
	for _, option := range options {
		option(&settings)
	}
	return WithRuntime(extismruntime.NewRuntime(extismruntime.Config{
		EnableWASI:   settings.enableWASI,
		AllowedHosts: append([]string(nil), settings.allowedHosts...),
		AllowedPaths: copyExtismPaths(settings.allowedPaths),
		CachePlugins: settings.cachePlugins,
	}))
}

// WithExtismWASI controls whether the Extism runtime instantiates WASI imports.
func WithExtismWASI(enabled bool) ExtismRuntimeOption {
	return func(options *extismRuntimeOptions) {
		options.enableWASI = enabled
	}
}

// WithExtismAllowedHost allows Extism host-controlled HTTP access for matching hosts.
func WithExtismAllowedHost(host string) ExtismRuntimeOption {
	return func(options *extismRuntimeOptions) {
		options.allowedHosts = append(options.allowedHosts, host)
	}
}

// WithExtismAllowedPath maps a host path into the guest filesystem.
func WithExtismAllowedPath(hostPath string, guestPath string) ExtismRuntimeOption {
	return func(options *extismRuntimeOptions) {
		if options.allowedPaths == nil {
			options.allowedPaths = make(map[string]string)
		}
		options.allowedPaths[hostPath] = guestPath
	}
}

// WithExtismPluginCache reuses compiled modules across runtime invocations.
func WithExtismPluginCache(enabled bool) ExtismRuntimeOption {
	return func(options *extismRuntimeOptions) {
		options.cachePlugins = enabled
	}
}

func copyExtismPaths(paths map[string]string) map[string]string {
	if len(paths) == 0 {
		return nil
	}
	copied := make(map[string]string, len(paths))
	for hostPath, guestPath := range paths {
		copied[hostPath] = guestPath
	}
	return copied
}

// WithStore replaces the host in-memory run/event store.
func WithStore(store history.Store) Option {
	return func(engine *Engine) {
		engine.store = store
	}
}

// WithFileStore uses a local JSON file as the run/event store.
func WithFileStore(path string) Option {
	return WithStore(filehistory.NewStore(path))
}

// WithCapabilityBroker replaces the host static grant broker.
func WithCapabilityBroker(broker capability.Broker) Option {
	return func(engine *Engine) {
		engine.capabilities = broker
	}
}

// WithCommandHandler registers the host handler for one command name.
func WithCommandHandler(name string, handler command.Handler) Option {
	return func(engine *Engine) {
		engine.handlers[name] = handler
	}
}

// WithGrant adds a static capability grant used by the host broker.
func WithGrant(grant capability.Grant) Option {
	return func(engine *Engine) {
		engine.grants = append(engine.grants, grant)
	}
}
