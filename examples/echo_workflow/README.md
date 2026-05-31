# Echo Workflow Example

This is a tiny Extism workflow module that emits one `test.echo` command and
then completes with the replayed command result.

Build it with TinyGo:

```sh
tinygo build -target wasip1 -buildmode=c-shared -o echo_workflow.wasm ./examples/echo_workflow
```

Use the resulting `.wasm` file as `module.Ref.Source`, with `Entrypoint` set to
`run`, and construct the engine with `capruntime.WithExtismRuntime()`.

You can run it through the host example:

```sh
go run ./examples/echo_host ./echo_workflow.wasm
```
