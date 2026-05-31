// Package guest contains helpers for workflow modules running inside Extism.
//
// Host applications use the normal capruntime Engine. WASM workflow modules can
// import this package to read the invocation envelope, complete or fail a run,
// and emit replay-aware host commands through workflow.command.
package guest
