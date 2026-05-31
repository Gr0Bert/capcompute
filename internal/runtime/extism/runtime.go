package extism

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	extismsdk "github.com/extism/go-sdk"

	"capcompute/command"
	publicruntime "capcompute/runtime"
)

const defaultEntrypoint = "run"

// Config controls the private Extism runtime adapter.
type Config struct {
	EnableWASI   bool
	AllowedHosts []string
	AllowedPaths map[string]string
	CachePlugins bool
}

// Runtime invokes Extism WASM plugins through the public runtime.Runtime boundary.
type Runtime struct {
	config Config
	mu     sync.Mutex
	cache  map[string]*extismsdk.CompiledPlugin
}

var _ publicruntime.Runtime = (*Runtime)(nil)

// NewRuntime creates an Extism-backed runtime.
func NewRuntime(config Config) *Runtime {
	config.AllowedHosts = append([]string(nil), config.AllowedHosts...)
	config.AllowedPaths = copyStringMap(config.AllowedPaths)
	runtime := &Runtime{config: config}
	if config.CachePlugins {
		runtime.cache = make(map[string]*extismsdk.CompiledPlugin)
	}
	return runtime
}

// Invoke loads the module for one deterministic tick and calls its workflow export.
func (r *Runtime) Invoke(ctx context.Context, req publicruntime.Request) (publicruntime.Result, error) {
	manifest, err := r.manifest(req)
	if err != nil {
		return publicruntime.Result{}, err
	}

	host := newCommandHost(req.CommandResults)
	plugin, err := r.plugin(ctx, manifest)
	if err != nil {
		return publicruntime.Result{}, fmt.Errorf("create extism plugin: %w", err)
	}
	defer plugin.Close(ctx)

	input, err := encodeInvocation(req)
	if err != nil {
		return publicruntime.Result{}, fmt.Errorf("encode workflow invocation: %w", err)
	}

	entrypoint := req.Module.Entrypoint
	if entrypoint == "" {
		entrypoint = defaultEntrypoint
	}

	callCtx := context.WithValue(ctx, commandHostKey{}, host)
	exit, output, err := plugin.CallWithContext(callCtx, entrypoint, input)
	if err != nil {
		return publicruntime.Result{}, fmt.Errorf("call workflow %q exit %d: %w", entrypoint, exit, err)
	}

	return decodePluginOutput(output, host.emitted)
}

// Close releases cached compiled plugins.
func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var closeErr error
	for key, plugin := range r.cache {
		if err := plugin.Close(ctx); err != nil && closeErr == nil {
			closeErr = err
		}
		delete(r.cache, key)
	}
	return closeErr
}

func (r *Runtime) plugin(ctx context.Context, manifest extismsdk.Manifest) (*extismsdk.Plugin, error) {
	config := extismsdk.PluginConfig{EnableWasi: r.config.EnableWASI}
	if !r.config.CachePlugins {
		return extismsdk.NewPlugin(ctx, manifest, config, []extismsdk.HostFunction{function()})
	}

	compiled, err := r.compiled(ctx, manifest, config)
	if err != nil {
		return nil, err
	}
	return compiled.Instance(ctx, extismsdk.PluginInstanceConfig{})
}

func (r *Runtime) compiled(ctx context.Context, manifest extismsdk.Manifest, config extismsdk.PluginConfig) (*extismsdk.CompiledPlugin, error) {
	key, err := cacheKey(manifest, r.config)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.cache[key]; ok {
		return cached, nil
	}
	compiled, err := extismsdk.NewCompiledPlugin(ctx, manifest, config, []extismsdk.HostFunction{function()})
	if err != nil {
		return nil, err
	}
	r.cache[key] = compiled
	return compiled, nil
}

func (r *Runtime) manifest(req publicruntime.Request) (extismsdk.Manifest, error) {
	if req.Module.Source == "" {
		return extismsdk.Manifest{}, fmt.Errorf("module source is required for extism runtime")
	}

	manifest := extismsdk.Manifest{
		Wasm: []extismsdk.Wasm{
			extismsdk.WasmFile{
				Path: req.Module.Source,
				Hash: req.Module.Digest,
				Name: req.Module.Name,
			},
		},
		AllowedHosts: append([]string(nil), r.config.AllowedHosts...),
		AllowedPaths: copyStringMap(r.config.AllowedPaths),
	}
	if req.Limits.TimeoutMillis > 0 {
		manifest.Timeout = uint64(req.Limits.TimeoutMillis)
	}
	if req.Limits.MemoryMaxPages > 0 {
		manifest.Memory = &extismsdk.ManifestMemory{MaxPages: uint32(req.Limits.MemoryMaxPages)}
	}
	return manifest, nil
}

func copyStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func cacheKey(manifest extismsdk.Manifest, config Config) (string, error) {
	data, err := json.Marshal(struct {
		Manifest   extismsdk.Manifest `json:"manifest"`
		EnableWASI bool               `json:"enable_wasi"`
	}{
		Manifest:   manifest,
		EnableWASI: config.EnableWASI,
	})
	if err != nil {
		return "", fmt.Errorf("encode extism cache key: %w", err)
	}
	return string(data), nil
}

func decodePluginOutput(output []byte, emitted *command.Command) (publicruntime.Result, error) {
	if len(output) == 0 {
		if emitted == nil {
			return publicruntime.Result{}, fmt.Errorf("workflow returned empty response")
		}
		return publicruntime.Result{
			Status:  publicruntime.Command,
			Command: *emitted,
		}, nil
	}

	result, err := decodeResponse(output)
	if err != nil {
		return publicruntime.Result{}, err
	}
	if emitted == nil {
		return result, nil
	}
	if result.Status == publicruntime.Command {
		return result, nil
	}
	return publicruntime.Result{}, fmt.Errorf("workflow emitted command %q but returned status %q", emitted.ID, result.Status)
}
