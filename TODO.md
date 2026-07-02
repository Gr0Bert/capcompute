# Scope

The process-reconstruction primitives the library set out to provide are in
place: `ProcessTable` is interface-only (`LoadProcess` / `SaveProcess`),
`CreateProcess` rebuilds a process and its dispatcher chain from a
`ProcessSpec`, and the syscall host function reloads the process from the table
on each guest syscall (see `host.go` and the package doc in `doc.go`).
Reconstructing a yielded process after a restart is therefore `CreateProcess` +
`SaveProcess` under the application's control — the library deliberately
exposes no `Replay(pid)` entry point, because *when* a process resumes and
*what* is injected back are the wrapping system's decisions.

This library deliberately does not own, and will not grow:

- concrete durable store implementations;
- replay scheduling, queues, or async completion;
- dispatching syscalls to other guests;
- schedulers, engines, or product-specific workflow code.

Those belong to the systems built on top of `capcompute`.

For scored next steps that flow from the OS model (journal versioning,
kernel-law tests, spawn), see `docs/ROADMAP.md`.
