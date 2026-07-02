// Package capcompute is the kernel of a small library operating system for
// Extism compute guests: wasm programs run as processes whose only access to
// the outside world is host-dispatched syscalls.
//
// The package owns the compiled program image (Kernel), process lifecycle, and
// the guest-to-host syscall wiring. Concrete storage, durable reconstruction,
// and application scheduling stay outside this package.
//
// A typical runtime does the following:
//   - build a Kernel with a wasm Manifest and a ProcessTable;
//   - create a Process from a ProcessSpec, which carries the process's dispatcher;
//   - save that Process in the ProcessTable before invoking Resume if the guest
//     can make syscalls;
//   - call Resume and read the single ResumeResult from the returned handle;
//   - close processes and the Kernel at the application boundary.
//
// The syscall host function receives only the PID through context. It loads the
// Process from the ProcessTable and dispatches the guest Syscall through the
// process dispatcher. This keeps runtime lookup explicit and avoids hidden
// invocation state in context.
//
// Resume has four observable outcomes. A successful guest call whose JSON
// output contains {"status":"yielded"} returns ResumeYielded, while an explicit
// {"status":"completed"} returns ResumeCompleted. Missing or unsupported status
// values return ResumeFailed. A stopped invocation returns ResumeStopped and
// permanently terminates that physical process. Guest and runtime errors also
// return ResumeFailed.
//
// ProcessTable is a runtime lookup boundary. Durable stores should persist the
// data needed by their application to recreate processes, then hydrate a fresh
// Kernel with CreateProcess and SaveProcess when a host process restarts.
// CreateProcess deliberately does not save processes; callers decide when a
// process becomes visible to syscalls and when it is removed.
//
// The library does not own replay scheduling, async completion, durable journal
// policy, or process cleanup timing. Those are conventions of the wrapping
// system using this package.
package capcompute
