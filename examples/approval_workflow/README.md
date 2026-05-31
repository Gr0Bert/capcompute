# Approval Workflow Example

This TinyGo workflow emits one `approval.request` command and stops until the
host records a result. It is useful for testing the pending-command path with a
real WASM module.

Build it with TinyGo:

```sh
tinygo build -target wasip1 -buildmode=c-shared -o approval_workflow.wasm ./examples/approval_workflow
```

Use the resulting `.wasm` file as `module.Ref.Source`, with `Entrypoint` set to
`run`. Register a handler for `approval.request` that returns
`command.Pending(reason)` when a human or external system must approve it, then
call `engine.Approve` or `engine.Deny` from the host.
