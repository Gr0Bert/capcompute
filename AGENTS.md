# AGENTS.md

This project values simple Go code with clear ownership.

The goal is not to minimize packages. The goal is to make it obvious:

- what a package owns;
- whether a type is public API or implementation detail;
- which interface an implementation satisfies;
- why a dependency exists.

Decide in this order:

```text
ownership -> visibility -> package -> file
```

Do not start with a file tree and justify it later.

---

## Core Shape

The root package is the library entrypoint. It should stay small.

For this project, root owns:

```text
Engine
New(...)
Options
top-level errors
```

Root may wire defaults, but it should not become a dumping ground for every
runtime concept.

Domain concepts that users need should live in their own public packages:

```text
run          // Invocation, Run, Principal, Source, TickResult
module       // module.Ref
runtime      // Runtime interface and invoke request/result
command      // command protocol and command handlers
capability   // public capability grant configuration
history      // durable run/event history and Store interface
```

Implementation-only concepts should live under `internal/`.

---

## Parent Package Owns Vocabulary

If a concept has an interface, the package that owns the interface should also
own the types spoken by that interface.

Good:

```text
history
  Store
  Run
  Event
  EventType
```

Then users of that boundary read clearly:

```go
history.Store
history.Run
history.Event
```

Avoid splitting an interface from its vocabulary:

```go
internal.Store // bad if it speaks in history.Run and history.Event
```

That makes ownership harder to understand.

---

## Child Packages Are Implementations

When a parent package owns an interface, concrete implementations should live in
child packages named by implementation strategy, not generic names like `impl`,
`service`, or `manager`.

Good:

```text
history
  Store
  Run
  Event

internal/history/memory
  MemoryStore
```

Good:

```text
capability
  Broker
  Request
  Decision

internal/capability/static
  StaticBroker
```

Good:

```text
internal/replay
  Matcher
  Match

internal/replay/strict
  StrictMatcher
```

Call sites should explain the relationship:

```go
var s history.Store = memory.NewMemoryStore()
var b capability.Broker = static.NewStaticBroker(grants)
var m replay.Matcher = strict.NewStrictMatcher()
```

Use compile-time checks in implementation packages when useful:

```go
var _ history.Store = (*MemoryStore)(nil)
```

---

## Package Names Should Read Well

Package prefixes matter because they appear at call sites.

Prefer:

```go
run.Invocation
module.Ref
runtime.Request
command.Handler
capability.Grant
history.Event
memory.NewMemoryStore()
```

Avoid vague prefixes:

```go
internal.Event
common.Type
utils.Hash(...)
impl.New(...)
```

If a package name forces aliases everywhere to be readable, reconsider the
package name.

---

## Interfaces

Define interfaces where they are consumed or where the boundary is owned.

Create an interface only when at least one is true:

- there are multiple implementations;
- tests need substitution;
- the dependency performs I/O;
- the implementation is volatile;
- the boundary is a real domain or component boundary.

Keep interfaces small. One to three methods is usually enough.

Avoid:

```go
type EngineInterface interface {}
type EngineImpl struct {}
```

Prefer:

```go
type Engine struct {
    runtime runtime.Runtime
    store   history.Store
}
```

---

## Public vs Internal

Public packages are for API users.

Internal packages are for implementation details the library owns.

Do not export implementation details from root just because they are convenient.
Do not hide public vocabulary under `internal/` if callers must use it to build
against the library.

Examples:

```text
runtime.Runtime      // public: users can provide a runtime backend
command.Handler      // public: users can register command handlers
history.Store        // public: users can provide durable history backends
capability.Broker    // public: users can provide authorization policy
```

---

## Import Direction

Keep dependencies one-way.

Prefer:

```text
root -> public concept packages
root -> internal boundaries
internal implementation -> parent boundary
```

Avoid:

```text
internal package -> root package
```

If an import cycle appears, fix ownership. Do not work around it with aliases or
extra glue packages.

---

## Tests

Test public behavior through the root API when possible:

```text
engine_test.go
```

Test implementation details near the implementation:

```text
internal/history/memory/memory_test.go
internal/command/hash_test.go
```

Do not put unrelated subsystem tests at root.

---

## Before Writing Code

Before creating a file, package, or interface, answer:

1. Who owns this concept?
2. Is it public API or internal implementation?
3. Which package should own the vocabulary?
4. Is this new package a real boundary or just a folder?
5. Is this interface needed, or would a concrete type be clearer?
6. Does the package prefix read well at call sites?
7. Would this placement still make sense after the next implementation?

If ownership is unclear, stop and clarify ownership first.

---

## Final Rule

Keep the code local, readable, and boring.

Use small packages when they express real ownership.
Use child packages for concrete implementations.
Keep root small.
Do not create `common`, `utils`, `models`, `helpers`, `impl`, or flat root files
because ownership was not decided.
