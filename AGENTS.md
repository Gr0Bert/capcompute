# AGENTS.md

This project values simple Go code with clear ownership.

The goal is not to minimize the number of packages.
The goal is to make it obvious:

- what a piece of code is for;
- who owns it;
- who may use it;
- whether it is public API or internal implementation.

When unsure, choose the simplest local design, but do not dump everything into the root package.

Example application structure could be:
```
root:
service.go // owns interface and all the structures this interface speaks in
service // directory that holds service implementation
    service_impl.go // implementation of service interface; imagine it needs reader;
    reader.go // interface required by service_impl.go
    reader // directory that holds reader's implementation
        reader_impl.go implementation of reader
```
this way user can understand what is implementation of what.
If there is more than one consumer of the interface it could be extracted to top:
```
root:
    service.go // owns interface and all the structures this interface speaks in
    service // directory that holds service implementation
        service_impl.go // implementation of service interface; imagine it needs reader;
    
    reader.go // interface required by service_impl.go
        reader // directory that holds reader's implementation
        reader_impl.go implementation of reader
```

---

## Core Rules

### 1. Start from ownership, not from a file tree

Before proposing or changing structure, first identify the main owned concepts.
Then decide packages and files.
Do not create a tree first and explain it later.

---

### 2. Root package is public API, not a dumping ground

The root package should contain the library API users are expected to depend on.
If a type is not meant for users of the library, it probably should not be exported from root.

---

### 3. A directory is a Go package, and a package is a boundary

Do not create packages mechanically.

Do create a package when it has a real responsibility, such as:

- a public API;
- an internal subsystem;
- a separate domain vocabulary;
- multiple implementations;
- a volatile implementation hidden behind a stable boundary;
- a need to avoid import cycles.
---

### 4. Consumer owns the interface

Define interfaces where they are used, not where they are implemented.

Good:

```go
package search

type IndexReader interface {
    Read(ctx context.Context, query Query) (Result, error)
}

type Service struct {
    index IndexReader
}
```

The implementation only needs to satisfy the interface:

```go
package searchlmdb

type Reader struct {}

func (r *Reader) Read(ctx context.Context, query search.Query) (search.Result, error) {
    // ...
}
```

---

### 5. Keep interfaces small

Prefer one to three methods.

Good:

```go
type ModuleLoader interface {
    Load(ctx context.Context, source ModuleSource) (*Module, error)
}
```

Suspicious:

```go
type Runtime interface {
    Load(...)
    Run(...)
    Replay(...)
    RegisterCommand(...)
    StoreHistory(...)
    ValidateCapability(...)
    ApplyLimits(...)
    Close(...)
}
```

Large interfaces usually mean several responsibilities were mixed together.

---

### 6. Use concrete types by default

Do not create an interface just because it feels clean.

Create an interface only when at least one is true:

- there are multiple implementations;
- tests need substitution;
- the dependency performs I/O;
- the implementation is volatile;
- the boundary represents a real component/domain separation.

Avoid:

```go
type EngineInterface interface {}
type EngineImpl struct {}
```

Prefer:

```go
type Engine struct {
    runtime Runtime
}
```

---

### 7. Promote concepts only when they earn it

Keep code local while it belongs to one owner.

Promote a concept into its own package when most of these are true:

- it has its own vocabulary;
- it can be explained without mentioning only one caller;
- it has meaningful internal logic;
- it has multiple implementations;
- it should be hidden under `internal/`;
- it changes for different reasons than the caller;
- keeping it in root makes the root harder to understand.

Multiple callers are a good reason to promote.
They are not the only reason.

---

### 8. Avoid the flat-root anti-pattern

This is suspicious for a non-trivial library:

```text
engine.go
run.go
invocation.go
module.go
runtime.go
command.go
history.go
replay.go
capability.go
limits.go
errors.go
memory_history.go
static_capability.go
command_registry.go
```

It may compile, but it hides ownership.

Problems:

- public API and internals are mixed;
- every concept looks equally important;
- users can depend on internals accidentally;
- future refactoring becomes harder.

A flat root package is acceptable only when the library is tiny and all files are part of one coherent public API.

---

### 9. Import direction

Prefer:

```text
root package -> internal packages
cmd/examples -> root package
```

Avoid:

```text
internal package -> root package -> same internal package
```

Import cycles are a design smell. Fix ownership instead of fighting the compiler.

---

### 11. Tests

Prefer public behavior tests through the root API:

```text
engine_test.go
```

Use internal tests for complex subsystems:

```text
internal/runtime/runtime_test.go
internal/history/memory_test.go
```

Do not put all unrelated subsystem tests at root.

---

## Before Writing Code

Before creating a file, package, or interface, answer:

1. Who owns this concept?
2. Is it public API or internal implementation?
3. Which package should own it?
4. Is a new package really a boundary, or just a folder?
5. Is an interface really needed, or is a concrete type enough?
6. Would this placement still make sense after the next feature?
7. Am I creating `common`, `utils`, `models`, or a flat root because I did not decide ownership?

If ownership is unclear, stop and clarify the ownership first.

---

## Final Rule

Decide in this order:

```text
ownership -> visibility -> package -> file
```

Not the other way around.

Keep public API small.
Keep internals hidden.
Keep interfaces small.
Keep concrete code where abstraction is not needed.
Do not turn the root package into a junk drawer.
