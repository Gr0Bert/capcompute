Durable WASM Capability Runtime — architecture report

This report is meant to serve two purposes:

1. Implementation brief for a coding agent: enough structure, interfaces, semantics, and corner cases to start building the shared library and products.
2. Product pitch for developers: a clear explanation of why this exists, what it replaces, and why the architecture is safer than “custom agent with kubeconfig and shell.”

The core idea is:

Build a reusable durable WASM capability runtime.
WASM modules contain deterministic decision/workflow logic.
All side effects go through host-mediated, policy-checked, replay-aware commands.
The same runtime powers both an AI assistant and a Kubernetes operations automation product.

⸻

1. One-paragraph product pitch

Custom agents are powerful but usually unsafe: they run arbitrary code, hold secrets, call terminals, mutate clusters directly, and are hard to audit. This project provides a safer middle ground: developers write operational logic as sandboxed WASM modules, while the host owns credentials, capabilities, approvals, runtime limits, audit, replay, and side effects. It is “Kubewarden-like” for operational actions, and “Temporal-like” for replayable workflows, but optimized for small, embeddable, policy-brokered modules that can be invoked by Kubernetes events, webhooks, cron, or AI assistant tool calls.

⸻

2. High-level conclusion

Build three layers:

1. capruntime-core
   Shared Go library:
   - WASM module loading
   - workflow command protocol
   - event history
   - replay
   - capability broker
   - runtime limits
   - command handlers
   - approvals
   - audit
   - pluggable durability
2. ops-controller
   Kubernetes product:
   - CRDs
   - reconcilers
   - Kubernetes event/watch triggers
   - ActionPolicy / ActionRun lifecycle
   - Kubernetes capabilities and command handlers
3. assistantd
   AI assistant product:
   - Telegram adapter
   - LLM tool registry
   - maps LLM tool calls to workflow invocations
   - approval UX
   - optional MCP/export layer later

The key abstraction is not “agent.” The key abstraction is:

deterministic WASM workflow
+ replay-aware host commands
+ explicit capabilities
+ pluggable event history

⸻

3. Technology background and decisions

3.1 WebAssembly

WASM is useful here because it gives a portable, sandboxed execution target. The important property is not raw speed; it is the ability to run plugin code with no ambient access to files, network, Kubernetes, secrets, or shell unless the host explicitly exposes imports. WebAssembly itself is a portable binary instruction format and compilation target for multiple languages.  ￼

In this project, the WASM module should be treated as a deterministic calculator/orchestrator, not as a process with OS access.

WASM module:
  - receives input
  - executes deterministic logic
  - emits workflow commands
  - receives cached command results during replay
  - returns completion output
Host:
  - owns credentials
  - owns side effects
  - owns persistence
  - owns approval
  - owns policy

⸻

3.2 WASI

WASI is a set of standard system interfaces for running WASM outside the browser. Kubewarden’s docs describe WASI as allowing WASM modules to interact with system primitives such as stdin, stdout, stderr, environment variables, and more. They also explicitly advise that regular Kubewarden policy authors should not use plain WASI system interfaces under normal circumstances.  ￼

For this project:

Default stance:
  Disable or minimize WASI.
Allowed:
  only if a language runtime requires it,
  and even then with no meaningful filesystem/network privileges.
Preferred:
  expose a single controlled host import:
    workflow.command(...)

WASI should not be the capability model. Your capability model should be owned by the Go host.

⸻

3.3 WIT and Component Model

WIT is an interface definition language for the WebAssembly Component Model. It defines contracts between components; it does not define behavior.  ￼ WASI Preview 2 is built on WIT and the Component Model type system, making WASI more modular and virtualizable than Preview 1.  ￼

Long term, WIT and the Component Model are attractive because they can make plugin imports/exports typed and composable. But for this project’s V1, they are not the best starting dependency.

Decision:

V1:
  Extism + JSON envelopes + explicit manifest.
Later:
  Add optional WIT/Component Model runtime adapter.
Reason:
  WIT simplifies ABI typing, but it does not replace:
    - capability policy
    - replay semantics
    - approval
    - idempotency
    - audit
    - plugin manifest
    - LLM tool schemas

⸻

3.4 waPC

waPC is a communication convention for host ↔ WASM guest calls. Kubewarden used waPC for bidirectional communication between policies and the policy runtime, while plain WASI policies are a special case for situations where waPC cannot be used.  ￼

In this project, waPC is useful as prior art, but not necessary for V1 if Extism is used. Extism already gives a higher-level plugin call and host-function model.

⸻

3.5 Extism

Extism is a strong V1 choice. It is a WASM plugin framework that handles much of the annoying ABI/memory layer. Extism host functions let a plugin invoke host application code, appearing as WASM imports.  ￼ Extism’s memory model is deliberately bytes-in / bytes-out, with SDKs and PDKs helping serialize data between host and plugin.  ￼

Extism is also aligned with Kubernetes tooling direction. Helm 4’s new WASM plugin system is based on Extism, and Helm’s HIP says the new plugin system is WASM-based for improved security over subprocess plugins, with additional work around signing and provenance.  ￼ Helm 4 docs describe an optional WebAssembly runtime for enhanced plugin security and expanded capabilities.  ￼

Decision:

Use Extism as the first runtime backend.
Expose only one custom host function:
  workflow.command(input_json) -> output_json
Keep runtime pluggable:
  runtime/extism
  runtime/wazero-json later if needed
  runtime/component later if WIT becomes attractive

Extism also advertises host-controlled HTTP without WASI, runtime limiters/timers, persistent module-scope variables, and simpler host function linking.  ￼ For this project, avoid relying on Extism HTTP directly for workflow operations; put all external I/O behind replay-aware commands instead.

⸻

3.6 Kubewarden as precedent

Kubewarden proves the “Kubernetes + WASM policy modules” shape. It is a Kubernetes Dynamic Admission Controller that uses policies written in WebAssembly.  ￼ Kubewarden recommends distributing policies through OCI-compliant registries.  ￼

This project is inspired by Kubewarden but not the same:

Kubewarden:
  admission request -> policy -> allow / deny / mutate
This project:
  event/tool call -> WASM workflow -> replay-aware commands -> controlled side effects

Kubewarden is about enforcement. This project is about safe operational automation.

⸻

3.7 Temporal-style replay

Temporal’s core model is relevant: workflow code emits commands, events are recorded, and event history is used to recover workflow state after failures. Temporal docs describe event history as the durable record used to recreate workflow state after a worker crash.  ￼ Temporal also states that workflow commands map to events, and replay compares emitted commands with existing event history.  ￼

This project should borrow the concept, not the entire Temporal platform:

WASM workflow code emits commands.
Host records command events.
On replay, the same command returns the recorded result.
If a new command appears, host executes/schedules it.
If command identity or args mismatch history, runtime reports nondeterminism.

Temporal’s docs are also clear about the hard edge: activities may execute more than once, especially if a worker crashes after a side effect but before recording completion, so activity implementations should be idempotent.  ￼ This same edge exists here.

⸻

3.8 Kubernetes operator model

The Kubernetes product should follow the operator/controller pattern. Kubernetes describes operators as extensions that use custom resources and follow the control-loop principle.  ￼ Operator SDK best practices recommend one controller per CRD, OpenAPI structural schemas, meaningful status, metrics, avoiding hard-coded namespaces, and not relying on the default ServiceAccount.  ￼

The Kubernetes product should be a normal controller-runtime/operator project, not a special admission-path system at first.

⸻

4. Core design philosophy

4.1 The module is not trusted

The WASM sandbox is only one layer. The module can be malicious, buggy, nondeterministic, or poorly written.

Never give a plugin:

- shell
- kubeconfig
- raw Kubernetes client
- raw filesystem
- raw network
- Telegram token
- GitHub token
- arbitrary HTTP
- environment variables with secrets

Plugins get one host capability:

workflow.command(...)

Everything meaningful flows through the command broker.

⸻

4.2 Host calls are workflow commands

The final design should not expose arbitrary host functions like:

github.create_issue(...)
k8s.patch(...)
http.fetch(...)

Instead expose one workflow-aware host function:

workflow.command(command_json) -> response_json

The command handler decides whether this is:

- deterministic helper
- recorded query
- recorded side-effecting command
- approval request
- timer
- signal wait

This is the key distinction:

Plain host call:
  plugin calls host
  host performs work
  host returns result
Workflow command:
  plugin emits command with stable ID
  host checks history
  host validates capabilities
  host executes / waits / returns cached result
  host records event
  replay returns recorded result

⸻

4.3 Deterministic workflow, side-effecting host

The WASM module should be replayable from the beginning. It should not directly read time, randomness, network, filesystem, environment, Kubernetes state, or external APIs.

Instead:

ctx.now()              -> workflow command
ctx.randomUUID()       -> workflow command
ctx.k8s.getPod()       -> workflow command
ctx.k8s.logs()         -> workflow command
ctx.github.issue()     -> workflow command
ctx.sleep()            -> workflow command
ctx.approval.request() -> workflow command

Even reads should be recorded if workflow branching depends on them. Otherwise replay may observe different logs or cluster state and produce different commands.

⸻

4.4 Durability is pluggable, but semantics must be explicit

Durability should be a backend interface, not mandatory Postgres.

Supported history stores:

memory:
  good for local dev and short assistant workflows
  no crash recovery
sqlite:
  good for single-node deployment
postgres:
  recommended production backend
kubernetes:
  possible but avoid storing large histories in CR status
object storage:
  useful for large logs/artifacts

The runtime must clearly expose selected guarantees:

history:
  backend: memory
  guarantees:
    crashRecovery: false
    duplicateSideEffectProtection: external-only

or:

history:
  backend: postgres
  guarantees:
    crashRecovery: true
    replayAfterRestart: true

⸻

5. Proposed product architecture

5.1 Shared library: capruntime-core

Working name:

capruntime

Purpose:

A Go library for running deterministic WASM workflows with explicit,
policy-checked, replay-aware host commands.

Responsibilities:

- load and verify WASM modules
- instantiate Extism runtime
- expose workflow.command host function
- maintain event history
- replay modules
- detect nondeterminism
- route commands to handlers
- enforce capabilities
- enforce runtime limits
- manage approvals
- manage timers/signals
- audit every command
- support pluggable persistence

Non-responsibilities:

- Telegram protocol
- LLM conversation state
- Kubernetes watch loops
- CRD reconciliation
- direct product UX

⸻

5.2 Product 1: assistantd

Purpose:

AI assistant that can invoke safe operational workflows from Telegram
or another chat/UI adapter.

Responsibilities:

- Telegram webhook or polling adapter
- user/chat identity mapping
- LLM tool schema generation
- tool call -> workflow invocation
- approval UX via chat
- conversation memory if needed
- final response formatting

The model should only see tool schemas. OpenAI’s function/tool calling docs describe tools as functionality made available to the model, with function tools defined by JSON Schema and executed by the application side.  ￼

The LLM should never see:

- Kubernetes token
- plugin runtime internals
- raw command host API
- Telegram bot token

The LLM asks for a tool call. The assistant maps that to a workflow start or signal.

⸻

5.3 Product 2: ops-controller

Purpose:

Kubernetes-native event-driven automation controller.

Responsibilities:

- CRDs:
  - WasmModule
  - CapabilityGrant
  - ActionPolicy
  - ActionRun
- controllers:
  - module resolver/cache
  - policy matcher
  - workflow run reconciler
  - approval/timer/signal reconciler
- event sources:
  - Kubernetes watches
  - Kubernetes Events
  - cron/schedule
  - webhooks later
  - Alertmanager later

This product should behave like a normal operator: CRDs are the API, controllers reconcile state, and status is written meaningfully. Kubernetes operator best practices emphasize structural CRD schemas, metrics, status, cleanup, configurable namespaces, and dedicated ServiceAccounts.  ￼

⸻

6. End-to-end execution model

6.1 Initial invocation

Adapter receives event
  ↓
Adapter builds InvocationRequest
  ↓
Engine creates WorkflowRun
  ↓
Engine loads module by digest
  ↓
Engine runs module with input + history
  ↓
Module emits command or completes

Example invocation envelope:

{
  "apiVersion": "capruntime.dev/v1alpha1",
  "kind": "Invocation",
  "metadata": {
    "runId": "wr_01HX...",
    "module": "crashloop-remediator",
    "moduleDigest": "sha256:abc123",
    "source": "kubernetes.event"
  },
  "principal": {
    "type": "serviceaccount",
    "id": "ops-system/ops-controller"
  },
  "subject": {
    "apiVersion": "v1",
    "kind": "Pod",
    "namespace": "dev",
    "name": "api-84d9c6c7f9-px2mn"
  },
  "input": {
    "reason": "BackOff"
  }
}

⸻

6.2 Workflow command

Plugin emits:

{
  "apiVersion": "capruntime.dev/v1alpha1",
  "kind": "Command",
  "id": "read-pod-logs",
  "name": "k8s.logs.read",
  "mode": "query",
  "args": {
    "namespace": "dev",
    "pod": "api-84d9c6c7f9-px2mn",
    "lines": 300
  }
}

Host calculates:

argsHash = sha256(canonical_json(args))
commandKey = runId + moduleDigest + command.id + command.name + argsHash

Then:

if matching completed event exists:
  return cached result
if matching scheduled/pending event exists:
  return waiting/yield response
if no event exists:
  authorize command
  append CommandScheduled
  execute/schedule/wait
  append CommandCompleted or CommandPending

⸻

6.3 Replay

Replay is simple conceptually:

Input + EventHistory + Same Module Digest = Same Commands

During replay:

module emits read-pod-logs
host finds CommandCompleted(read-pod-logs)
host returns cached logs
module continues
module emits create-github-issue
host finds CommandCompleted(create-github-issue)
host returns cached issue URL
module continues
module returns complete

If the module emits a command with the same ID but different args hash:

NonDeterministicWorkflowError:
  command ID reused with different command name or args

If the module emits commands in a different sequence than history expects, decide between two policies:

strict-order:
  fail if command order differs
id-based:
  allow lookup by command ID, but detect conflicts

Recommendation for V1:

Use strict-order by default.
Allow id-based mode later for advanced SDKs.

⸻

7. Command categories

The runtime should classify commands.

7.1 Deterministic helper

No recording required.

Examples:

json.validate
template.render
string.normalize

Most of these should simply be plugin code, not host commands.

⸻

7.2 Recorded query

Read-only but nondeterministic. Record the result.

Examples:

k8s.object.get
k8s.logs.read
http.fetch
web.search
time.now
random.uuid
secret.resolve

Even though these are reads, record them because workflow decisions may depend on them.

⸻

7.3 Side-effecting command

External mutation. Always record lifecycle and receipt.

Examples:

k8s.object.apply
k8s.object.patch
github.issue.create
github.issue.comment
notify.user
slack.message.send
pagerduty.incident.create

⸻

7.4 Approval command

A special command that pauses until a human or policy grants approval.

{
  "id": "approve-prod-restart",
  "name": "approval.request",
  "mode": "approval",
  "args": {
    "summary": "Restart Deployment prod/api",
    "risk": "high",
    "details": "Patch pod template annotation to trigger rollout."
  }
}

⸻

7.5 Timer command

A special command that pauses until a time has passed.

{
  "id": "wait-rollout",
  "name": "timer.sleep",
  "mode": "timer",
  "args": {
    "duration": "30s"
  }
}

⸻

7.6 Signal wait

A command that waits for an external event.

{
  "id": "wait-human-input",
  "name": "signal.wait",
  "mode": "signal",
  "args": {
    "signal": "operator-response"
  }
}

⸻

8. Event history model

8.1 Minimal event types

WorkflowStarted
WorkflowCompleted
WorkflowFailed
WorkflowCanceled
CommandScheduled
CommandStarted
CommandCompleted
CommandFailed
CommandPending
CommandUnknown
ApprovalRequested
ApprovalGranted
ApprovalDenied
TimerScheduled
TimerFired
SignalReceived
NondeterminismDetected

Example event history:

[
  {
    "seq": 1,
    "type": "WorkflowStarted",
    "runId": "wr_01HX",
    "moduleDigest": "sha256:abc123"
  },
  {
    "seq": 2,
    "type": "CommandScheduled",
    "commandId": "read-pod-logs",
    "name": "k8s.logs.read",
    "argsHash": "sha256:aaa"
  },
  {
    "seq": 3,
    "type": "CommandCompleted",
    "commandId": "read-pod-logs",
    "resultRef": "artifact://logs-aaa"
  },
  {
    "seq": 4,
    "type": "CommandScheduled",
    "commandId": "create-issue",
    "name": "github.issue.create",
    "argsHash": "sha256:bbb"
  },
  {
    "seq": 5,
    "type": "CommandCompleted",
    "commandId": "create-issue",
    "receipt": {
      "issueNumber": 123,
      "url": "https://github.com/org/repo/issues/123"
    }
  },
  {
    "seq": 6,
    "type": "WorkflowCompleted",
    "result": {
      "message": "Created issue #123."
    }
  }
]

8.2 History size limits

Do not put large logs or HTTP bodies inline. Store large payloads as artifacts and reference them.

{
  "resultRef": "artifact://wr_01HX/read-pod-logs/result.json"
}

Temporal has explicit history limits; this is a useful reminder that event histories should not grow without bounds.  ￼

⸻

9. The crash window and how to handle it

The hard failure case:

1. Host appends CommandStarted.
2. Host calls external system.
3. External system succeeds.
4. Host crashes before appending CommandCompleted.

After restart, the runtime cannot know whether the external effect happened unless one of these is available:

- same transactional store
- external idempotency key
- natural key / create-or-get
- recovery probe
- manual reconciliation

Temporal documents the same issue: if a worker executes an activity successfully but crashes before notifying Temporal, the activity is retried; without idempotency this can create duplicate charges or duplicate infrastructure resources.  ￼

9.1 Recovery strategies

Strategy A: Same transaction

Strongest option.

BEGIN
  write business state
  write CommandCompleted
COMMIT

Only works when the side effect and workflow history share a transaction boundary.

⸻

Strategy B: External idempotency key

Best general option.

Stripe’s API supports idempotency keys for safely retrying mutating requests, and subsequent requests with the same key return the original saved result.  ￼

Runtime should derive:

idempotencyKey =
  sha256(runId + moduleDigest + commandId + commandName + argsHash)

Handler passes this key to external APIs that support it.

⸻

Strategy C: Natural key / create-or-get

For systems without explicit idempotency keys, create stable identity.

Examples:

Kubernetes:
  apiVersion/kind/namespace/name + fieldManager + annotations
GitHub issue:
  hidden marker in issue body:
  <!-- capruntime: run=wr_01HX command=create-issue -->
DNS:
  stable record name
Cloud resource:
  stable name/client token/tag

⸻

Strategy D: Transactional outbox

Useful for internal async command execution. AWS describes transactional outbox as a pattern to keep database updates and event notifications atomic, while also warning that duplicate messages can still happen and consumers should be idempotent.  ￼

⸻

Strategy E: Unknown/quarantine

For non-idempotent, non-probeable effects:

Mark CommandUnknown.
Stop automatic replay.
Require human reconciliation.

Never silently retry unknown unsafe commands.

⸻

10. Capability model

10.1 Capability is a host-side grant

A module can request capabilities in its manifest, but authority comes from grants.

requested capability:
  module says “I may need k8s.logs.read.”
granted capability:
  operator/admin says “This module digest may read logs in namespace dev,
  max 300 lines, when invoked by these principals.”

10.2 Capability check inputs

Every command authorization should consider:

- principal
- source adapter
- module name
- module digest
- workflow run ID
- command name
- command mode
- args schema
- target resource
- capability grant
- approval policy
- rate limit
- runtime policy

10.3 Capability grant example

apiVersion: capruntime.dev/v1alpha1
kind: CapabilityGrant
metadata:
  name: crashloop-remediator-dev
spec:
  moduleRef:
    name: crashloop-remediator
    digest: sha256:abc123
  principals:
    - type: serviceaccount
      id: ops-system/ops-controller
    - type: telegram-user
      id: "123456789"
  grants:
    - capability: k8s.logs.read
      namespaces: ["dev"]
      maxLines: 300
    - capability: github.issue.create
      repositories:
        - platform/incidents
      idempotency:
        strategy: natural-key
        marker: issue-body
    - capability: k8s.object.patch
      namespaces: ["dev"]
      allowedKinds:
        - apiVersion: apps/v1
          kind: Deployment
      approval:
        required: true

⸻

11. Runtime limits and sandboxing

11.1 WASM/runtime limits

Each module execution should have:

- max wall-clock runtime
- max memory pages
- max input bytes
- max output bytes
- max command count per tick
- max replay count per tick
- max artifact bytes
- max log bytes

Extism manifests support runtime constraints such as memory, timeouts, allowed hosts, allowed paths, and config.  ￼ Extism’s host-side configuration also controls whether a plugin gets WASI functions and which network hosts or file paths are available through manifest configuration.  ￼

Recommended defaults:

runtimeLimits:
  timeoutMs: 3000
  memoryMaxPages: 32
  maxInputBytes: 1048576
  maxOutputBytes: 1048576
  maxCommandsPerTick: 20
  maxReplaySteps: 100
  allowWasi: false
  allowedHosts: []
  allowedPaths: {}

11.2 Kubernetes defense in depth

For Kubernetes deployment:

- dedicated ServiceAccount
- namespace-scoped Roles where possible
- no cluster-admin
- NetworkPolicy default-deny egress
- non-root container
- read-only root filesystem
- resource requests/limits
- audit logs

Kubernetes service account docs explicitly recommend granting only the minimum permissions required so ServiceAccount permissions follow least privilege.  ￼ Kubernetes NetworkPolicies are enforced by a compatible network plugin; by default pods are non-isolated for egress, and egress isolation only applies once a policy selects the pod and includes egress policy types.  ￼

⸻

12. Kubernetes command handlers

12.1 Read commands

k8s.object.get
k8s.object.list
k8s.logs.read
k8s.events.list
k8s.rollout.status

12.2 Write commands

k8s.object.apply
k8s.object.patch
k8s.object.delete
k8s.event.create
k8s.deployment.restart

12.3 Write safety

For Kubernetes writes:

- prefer server-side apply for declarative object management
- prefer server-side dry-run before apply
- require fieldManager
- require namespace/kind allowlists
- require approval for risky operations
- store command ID annotations on managed objects

Kubernetes Server-Side Apply tracks field ownership for objects and helps controllers manage resources declaratively.  ￼ Kubernetes dry-run is designed to send requests to modifying endpoints and see whether they would succeed without actually persisting changes.  ￼

Example restart command implemented as a patch:

{
  "id": "restart-api",
  "name": "k8s.deployment.restart",
  "args": {
    "namespace": "dev",
    "name": "api",
    "reason": "requested-by-workflow"
  }
}

Handler translates to:

PATCH apps/v1 Deployment dev/api
spec.template.metadata.annotations["capruntime.dev/restartedAt"] = now-recorded

But now must be a recorded workflow command or supplied by the host handler deterministically into the receipt.

⸻

13. Module manifest

Use a project-specific manifest, separate from Extism’s manifest.

apiVersion: capruntime.dev/v1alpha1
kind: WasmModule
metadata:
  name: crashloop-remediator
  version: 0.1.0
spec:
  runtime:
    type: extism/v1
    entrypoint: run
    timeoutMs: 3000
    memoryMaxPages: 32
    allowWasi: false
    allowedHosts: []
    allowedPaths: {}
  module:
    oci: oci://registry.example.com/capruntime/crashloop-remediator:v0.1.0
    digest: sha256:abc123
  sdk:
    protocol: workflow-json-v1
  requestedCapabilities:
    - name: k8s.object.get
    - name: k8s.logs.read
    - name: github.issue.create
    - name: k8s.object.patch
  tools:
    - name: ops.inspect_crashloop
      description: Inspect a crashlooping pod and create or propose remediation.
      inputSchema:
        type: object
        properties:
          namespace:
            type: string
          pod:
            type: string
        required: ["namespace", "pod"]
        additionalProperties: false

OCI distribution is a good default: ORAS documents that OCI artifacts can be referenced by tag or digest, with digests treated as immutable while tags may be mutable.  ￼ Use digest pinning by default. Signing should be added early; Cosign supports signing and verifying OCI containers and other artifacts.  ￼

⸻

14. Kubernetes CRDs

14.1 WasmModule

Defines module source and requested permissions.

apiVersion: ops.capruntime.dev/v1alpha1
kind: WasmModule
metadata:
  name: crashloop-remediator
spec:
  source:
    oci: oci://registry.example.com/capruntime/crashloop-remediator:v0.1.0
    digest: sha256:abc123
    verify:
      cosign:
        required: true
  runtime:
    type: extism/v1
    entrypoint: run
    timeoutMs: 3000
    memoryMaxPages: 32
  requestedCapabilities:
    - k8s.object.get
    - k8s.logs.read
    - github.issue.create

⸻

14.2 CapabilityGrant

Grants real authority.

apiVersion: ops.capruntime.dev/v1alpha1
kind: CapabilityGrant
metadata:
  name: crashloop-remediator-dev
spec:
  moduleRef:
    name: crashloop-remediator
    digest: sha256:abc123
  grants:
    - capability: k8s.logs.read
      namespaces: ["dev"]
      maxLines: 300
    - capability: github.issue.create
      repositories: ["platform/incidents"]
    - capability: k8s.object.patch
      namespaces: ["dev"]
      allowedKinds:
        - apiVersion: apps/v1
          kind: Deployment
      approval:
        required: true

⸻

14.3 ActionPolicy

Binds triggers to modules.

apiVersion: ops.capruntime.dev/v1alpha1
kind: ActionPolicy
metadata:
  name: crashloop-remediation
spec:
  trigger:
    type: kubernetes.event
    selector:
      namespace: dev
      reason: BackOff
      involvedObjectKind: Pod
  moduleRef:
    name: crashloop-remediator
    digest: sha256:abc123
  execution:
    mode: apply
    concurrencyPolicy: Forbid
    history:
      backend: postgres
    retry:
      maxAttempts: 3
      backoffSeconds: 30

⸻

14.4 ActionRun

Records execution summary and status.

apiVersion: ops.capruntime.dev/v1alpha1
kind: ActionRun
metadata:
  name: crashloop-remediation-01hx
status:
  phase: WaitingForApproval
  runId: wr_01HX
  moduleDigest: sha256:abc123
  startedAt: "2026-05-30T12:00:00Z"
  pending:
    commandId: restart-api
    approvalId: appr_01HX
  historyRef: postgres://capruntime/workflow_runs/wr_01HX
  summary: "Detected missing DATABASE_URL; proposed restart after config patch."

Do not store full logs or large histories in CR status. Store references.

⸻

15. Go package layout

Recommended repository layout:

/cmd
  /assistantd
  /ops-controller
  /capctl
/pkg
  /capruntime
    engine.go
    invocation.go
    result.go
  /capruntime/runtime
    runtime.go
    /extism
      runtime.go
  /capruntime/workflow
    command.go
    event.go
    replay.go
    nondeterminism.go
  /capruntime/history
    store.go
    /memory
      memory.go
    /sqlite
      sqlite.go
    /postgres
      postgres.go
  /capruntime/capability
    broker.go
    grant.go
    scope.go
    registry.go
  /capruntime/commands
    handler.go
    receipt.go
    recovery.go
  /capruntime/approval
    store.go
    policy.go
  /capruntime/audit
    sink.go
    event.go
  /capruntime/artifact
    store.go
    /filesystem
    /s3
    /postgres
  /capruntime/llm
    tools.go
    schemas.go
  /capruntime-k8s
    /commands
      object_get.go
      logs_read.go
      object_apply.go
      object_patch.go
      deployment_restart.go
    /authz
    /dryrun
    /diff
  /capruntime-github
    /commands
      issue_create.go
      issue_comment.go
  /capruntime-http
    /commands
      fetch.go
  /assistant
    /telegram
    /toolrouter
    /approvals
  /ops
    /api
      /v1alpha1
    /controllers
    /triggers

⸻

16. Core Go interfaces

16.1 Engine

type Engine struct {
    Runtime       Runtime
    Modules       ModuleResolver
    History       HistoryStore
    Capabilities  CapabilityBroker
    Commands      CommandRegistry
    Approvals     ApprovalStore
    Artifacts     ArtifactStore
    Audit         AuditSink
    Clock         Clock
}
func (e *Engine) Start(ctx context.Context, req InvocationRequest) (*WorkflowRun, error)
func (e *Engine) Tick(ctx context.Context, runID string) (*TickResult, error)
func (e *Engine) Signal(ctx context.Context, runID string, signal Signal) error
func (e *Engine) Approve(ctx context.Context, approvalID string, decision ApprovalDecision) error

⸻

16.2 Runtime

type Runtime interface {
    Invoke(ctx context.Context, req RuntimeInvokeRequest) (*RuntimeInvokeResult, error)
}
type RuntimeInvokeRequest struct {
    Module     ModuleRef
    Entrypoint string
    Input      []byte
    History    []Event
    Limits     RuntimeLimits
}
type RuntimeInvokeResult struct {
    Status  RuntimeStatus // completed, command, failed
    Output  []byte
    Command *Command
    Error   *WorkflowError
}

⸻

16.3 History store

type HistoryStore interface {
    CreateRun(ctx context.Context, run WorkflowRun, events ...Event) error
    LoadRun(ctx context.Context, runID string) (*WorkflowRun, []Event, error)
    Append(ctx context.Context, runID string, expectedVersion int64, events ...Event) error
    MarkComplete(ctx context.Context, runID string, result json.RawMessage) error
}

Use optimistic concurrency:

expectedVersion prevents two workers from appending conflicting events.

⸻

16.4 Command handler

type CommandHandler interface {
    Name() string
    Schema() CommandSchema
    Safety() CommandSafety
    Plan(ctx context.Context, req CommandRequest) (*CommandPlan, error)
    Execute(ctx context.Context, req CommandRequest) (*CommandReceipt, error)
    Recover(ctx context.Context, req CommandRecoveryRequest) (*RecoveryResult, error)
}

⸻

16.5 Command safety

type CommandSafety struct {
    SideEffecting              bool
    RequiresDurableHistory     bool
    RequiresIdempotencyKey     bool
    SupportsExternalIdempotency bool
    SupportsNaturalKeyRecovery bool
    SupportsDryRun             bool
    ApprovalDefault            ApprovalDefault
    OnUnknown                  UnknownPolicy
}
type UnknownPolicy string
const (
    UnknownQuarantine UnknownPolicy = "quarantine"
)

⸻

16.6 Capability broker

type CapabilityBroker interface {
    Authorize(ctx context.Context, req AuthorizationRequest) (*AuthorizationDecision, error)
}
type AuthorizationRequest struct {
    Principal    Principal
    Source       Source
    Module       ModuleRef
    RunID        string
    Command      Command
    Capability   string
    Args         json.RawMessage
}

⸻

17. Workflow command protocol

17.1 Plugin → host command

{
  "apiVersion": "capruntime.dev/v1alpha1",
  "kind": "Command",
  "id": "create-incident-issue",
  "name": "github.issue.create",
  "mode": "command",
  "args": {
    "repo": "platform/incidents",
    "title": "dev/api crashlooping",
    "body": "Observed CrashLoopBackOff for pod api-123..."
  }
}

17.2 Host → plugin response

Resolved:

{
  "status": "resolved",
  "result": {
    "issueNumber": 123,
    "url": "https://github.com/platform/incidents/issues/123"
  }
}

Yield / waiting:

{
  "status": "yield",
  "reason": "pending_approval",
  "commandId": "restart-api",
  "approvalId": "appr_01HX"
}

Denied:

{
  "status": "denied",
  "reason": "capability_not_granted",
  "message": "Module is not allowed to patch deployments in namespace prod."
}

Nondeterministic:

{
  "status": "error",
  "code": "NONDETERMINISTIC_COMMAND",
  "message": "Command create-incident-issue was replayed with a different args hash."
}

⸻

18. Plugin SDK shape

The SDK should hide replay mechanics.

18.1 TypeScript-like ideal API

export async function run(ctx: Context, input: Input): Promise<Output> {
  const pod = await ctx.k8s.getPod("read-pod", input.namespace, input.pod);
  const logs = await ctx.k8s.logs("read-logs", {
    namespace: input.namespace,
    pod: input.pod,
    lines: 300,
  });
  if (logs.text.includes("DATABASE_URL")) {
    const issue = await ctx.github.createIssue("create-issue", {
      repo: "platform/incidents",
      title: `${input.namespace}/${input.pod} crashlooping`,
      body: logs.summary,
    });
    await ctx.approval.request("approve-annotation", {
      summary: `Annotate deployment with ${issue.url}`,
      risk: "low",
    });
    await ctx.k8s.patch("annotate-deployment", {
      apiVersion: "apps/v1",
      kind: "Deployment",
      namespace: input.namespace,
      name: "api",
      patchType: "merge",
      patch: {
        metadata: {
          annotations: {
            "capruntime.dev/incident": issue.url,
          },
        },
      },
    });
    return { message: `Created ${issue.url} and annotated deployment.` };
  }
  return { message: "No known remediation found." };
}

18.2 Reality of V1 SDK

V1 does not need real async machinery. It can use a cooperative protocol:

run(input, history) -> completed | command

The SDK can throw/return an internal yield when a command is not resolved yet. Next tick, the host replays the module with updated history.

⸻

19. Assistant product design

19.1 Adapter vs tool

Telegram should be an adapter, not a plugin.

Telegram adapter owns:
  - bot token
  - webhook validation
  - user/chat identity
  - rate limits
  - /confirm and /cancel
  - final message delivery
Workflow modules own:
  - decision logic
  - command emission

The model sees tools generated from WasmModule.spec.tools.

Example LLM tool schema:

{
  "type": "function",
  "name": "ops.inspect_crashloop",
  "description": "Inspect a crashlooping pod and create or propose remediation.",
  "parameters": {
    "type": "object",
    "properties": {
      "namespace": {
        "type": "string",
        "enum": ["dev", "staging"]
      },
      "pod": {
        "type": "string"
      }
    },
    "required": ["namespace", "pod"],
    "additionalProperties": false
  }
}

Assistant flow:

Telegram message
  ↓
assistant maps user -> principal
  ↓
LLM chooses tool
  ↓
assistant starts workflow
  ↓
workflow may complete or wait for approval
  ↓
assistant sends result or approval prompt

⸻

20. Ops-controller product design

20.1 Trigger matching

ActionPolicy maps events to workflows.

Supported V1 triggers:

- kubernetes.event
- kubernetes.object
- cron
- manual start

Later:

- Alertmanager webhook
- generic webhook
- GitHub webhook
- Kafka/NATS/CloudEvents

20.2 Reconciliation loop

watch ActionPolicy, WasmModule, CapabilityGrant
watch relevant Kubernetes events/objects
create ActionRun when trigger matches
call engine.Start
reconcile ActionRun:
  - Tick until Complete or Waiting
  - update status
  - handle timers
  - handle approvals
  - retry failures according to policy

20.3 Concurrency

ActionPolicy.spec.execution.concurrencyPolicy:

Allow:
  multiple runs allowed
Forbid:
  do not start new run if matching run active
Replace:
  cancel previous matching run and start new

20.4 Idempotent run IDs

For event-driven automation, run IDs should be stable:

runId = hash(policyUID + policyGeneration + eventUID + subjectUID + triggerReason)

This enables stable idempotency keys even if memory history is lost.

For assistant:

runId = hash(adapter + chatID + messageID + toolCallID)

⸻

21. Persistence modes

21.1 Memory mode

Use for:

- local dev
- tests
- short assistant workflows
- demos

Semantics:

- no crash recovery
- active workflow state lost on process crash
- side-effect duplication protection depends only on external idempotency/natural keys

21.2 SQLite mode

Use for:

- single-node personal deployment
- lightweight assistant

Semantics:

- local crash recovery
- no multi-replica writes unless carefully configured

21.3 Postgres mode

Use for:

- production assistant
- ops-controller
- multi-replica controllers

Semantics:

- durable event history
- optimistic concurrency
- pending command recovery
- retention policies

21.4 Kubernetes mode

Use carefully.

ActionRun.status:
  summary, phase, refs
External store:
  full event history, artifacts, logs

Avoid large histories in CRD status.

⸻

22. Supply chain

Module distribution should use OCI artifacts by digest.

module.wasm
manifest.yaml
sbom.json optional
signature/referrer

Kubewarden’s OCI distribution model is a good precedent.  ￼ ORAS docs clarify that digest references should be considered immutable, while tags may be mutable.  ￼

Minimum V1:

- digest pinning required
- local unsigned modules allowed only in dev mode
- OCI resolver
- SHA-256 verification

V1.1:

- cosign verification
- provenance metadata
- module allowlist

V2:

- SBOM
- SLSA provenance
- registry policy

⸻

23. Security model

23.1 Threats

Assume:

- malicious module
- buggy module
- prompt-injected assistant request
- compromised Telegram user
- compromised controller pod
- malicious OCI artifact/tag swap
- nondeterministic workflow code
- external API timeout after success
- replay after module upgrade

23.2 Mitigations

malicious module:
  WASM sandbox, no raw WASI/network/fs, capability broker
buggy module:
  timeouts, memory limits, command limits, replay limits
prompt injection:
  LLM only sees schemas; capabilities enforced server-side
compromised user:
  principal-scoped grants and approvals
compromised controller:
  Kubernetes RBAC, NetworkPolicy, non-root, no cluster-admin
tag swap:
  digest pinning and signature verification
nondeterminism:
  command IDs, args hashes, strict replay comparison
external side-effect ambiguity:
  idempotency keys, natural-key recovery, quarantine
module upgrade:
  WorkflowRun pins module digest

⸻

24. Versioning rules

24.1 Pin module digest per workflow run

Never replay an active run with a different module digest.

WorkflowRun.moduleDigest is immutable.

24.2 New module versions affect only new runs

WasmModule tag may update,
but ActionRun stores digest.

24.3 Command schema versioning

Each command handler should version schemas:

k8s.object.patch@v1
github.issue.create@v1

Later, aliases can preserve compatibility.

⸻

25. Observability

Emit metrics:

workflow_runs_started_total
workflow_runs_completed_total
workflow_runs_failed_total
workflow_ticks_total
workflow_replays_total
workflow_nondeterminism_total
commands_scheduled_total
commands_completed_total
commands_failed_total
commands_unknown_total
approvals_requested_total
runtime_timeouts_total
runtime_memory_limit_total

Logs should include:

runId
moduleName
moduleDigest
principal
source
commandId
commandName
decision
duration
receiptRef

Audit events should be append-only and structured.

⸻

26. Testing strategy

26.1 Unit tests

- command args canonicalization
- args hashing
- history append concurrency
- capability matching
- manifest validation
- replay matching
- nondeterminism detection
- idempotency key derivation

26.2 Runtime tests

- plugin completes without commands
- plugin emits one query
- plugin emits command then completes on replay
- plugin emits different args on replay -> error
- plugin exceeds command count -> error
- plugin exceeds timeout -> error
- plugin exceeds memory -> error

26.3 Failure injection tests

- crash after CommandScheduled
- crash after CommandStarted before external result
- crash after external result before CommandCompleted
- duplicate Tick workers racing
- history store unavailable
- artifact store unavailable
- approval granted after timeout

26.4 Kubernetes integration tests

- server-side dry-run passes/fails
- forbidden namespace rejected
- forbidden kind rejected
- RBAC denied surfaced correctly
- patch approval required
- ActionRun status updates

⸻

27. MVP implementation plan

Phase 0: skeleton

Deliver:

- Go module
- Engine interface
- memory HistoryStore
- command/event structs
- basic replay loop
- local test module support

Acceptance:

A fake runtime returns CommandScheduled, then Completed on replay.

⸻

Phase 1: Extism runtime

Deliver:

- runtime/extism
- workflow.command host function
- JSON protocol
- timeout and memory options
- one sample plugin

Acceptance:

A WASM plugin emits a command; host resolves it; replay completes.

⸻

Phase 2: command registry and capabilities

Deliver:

- CommandHandler interface
- CapabilityBroker
- static YAML grants
- command schemas
- authorization decisions

Acceptance:

Denied commands fail safely.
Allowed commands execute and record receipts.

⸻

Phase 3: durability backends

Deliver:

- memory store
- sqlite or postgres store
- optimistic concurrency
- pending command recovery

Acceptance:

Workflow can resume after process restart with durable backend.

⸻

Phase 4: Kubernetes command package

Deliver:

- k8s.object.get
- k8s.logs.read
- k8s.object.patch
- server-side dry-run
- namespace/kind scopes

Acceptance:

Plugin can inspect logs and propose/perform a scoped patch through host command.

⸻

Phase 5: ops-controller

Deliver:

- CRDs
- ActionPolicy controller
- ActionRun controller
- WasmModule resolver
- CapabilityGrant handling

Acceptance:

Kubernetes event triggers a WASM workflow and records ActionRun status.

⸻

Phase 6: assistantd

Deliver:

- Telegram adapter
- LLM tool schema generation
- tool call -> workflow start
- approval via /confirm

Acceptance:

User asks Telegram assistant to inspect pod; assistant invokes workflow and returns result.

⸻

28. Non-goals for V1

Do not build these initially:

- arbitrary shell access
- raw terminal tool
- arbitrary plugin HTTP
- raw plugin filesystem
- full WIT/Component Model runtime
- full Temporal replacement
- multi-language polished SDKs
- admission webhook mode
- distributed worker queues
- visual workflow editor

V1 should be boring and safe.

⸻

29. Open design choices

29.1 Runtime name

Candidates:

capruntime
wasmflow
safeops
wasmops
actionflow

29.2 Strict vs flexible command ordering

Recommendation:

V1: strict command order.
V2: id-based matching with careful nondeterminism detection.

29.3 SDK language

Recommendation:

V1 plugin examples:
  Rust or TinyGo
V1 SDK:
  minimal JSON protocol helper
V2:
  TypeScript/AssemblyScript or Rust async-like SDK

29.4 Extism vs raw wazero

Recommendation:

Use Extism now.
Hide it behind Runtime interface.

Extism directly addresses the host/plugin memory and host-function ergonomics, while Helm’s adoption gives confidence for a Kubernetes-adjacent plugin story.  ￼

⸻

30. Coding-agent implementation prompt

Use the following as the direct build prompt for Codex or another coding agent.

You are implementing a Go project called capruntime.
Goal:
Build a reusable durable WASM capability runtime. WASM modules contain deterministic workflow logic. Modules cannot access external systems directly. They can only call a single host function, workflow.command(json), which emits replay-aware commands. The host records command events, enforces capabilities, executes side effects, returns cached results during replay, and detects nondeterminism.
Architecture:
- capruntime-core is a reusable Go library.
- runtime/extism is the first WASM backend.
- history store is pluggable; implement memory first.
- commands are pluggable handlers.
- capabilities are host-side grants.
- products assistantd and ops-controller will use the library later.
Implement first:
1. Core data types:
   - WorkflowRun
   - InvocationRequest
   - Event
   - Command
   - CommandReceipt
   - RuntimeLimits
   - Principal
   - ModuleRef
2. HistoryStore interface:
   - CreateRun
   - LoadRun
   - Append with expectedVersion
   - MarkComplete
   Implement memory store with optimistic concurrency.
3. CommandHandler and CommandRegistry:
   - Name
   - Schema
   - Safety
   - Plan
   - Execute
   - Recover
   Add fake echo command for tests.
4. CapabilityBroker:
   - Static grant broker
   - Deny by default
   - Authorize using principal, module digest, command name, args.
5. Engine:
   - Start(ctx, InvocationRequest)
   - Tick(ctx, runID)
   - Signal(ctx, runID, Signal)
   - Approve(ctx, approvalID, ApprovalDecision)
   Tick must:
     a. load run + history
     b. invoke runtime
     c. if runtime completed, append WorkflowCompleted
     d. if runtime emitted command:
        - canonicalize args
        - compute argsHash
        - check history for matching completed command
        - if complete, return cached result to runtime on next replay
        - if new, authorize and execute/schedule
        - append CommandScheduled/Started/Completed/Pending
     e. detect command ID/name/args mismatch as nondeterminism.
6. Runtime interface:
   - Invoke(ctx, RuntimeInvokeRequest) -> RuntimeInvokeResult
   Implement a fake runtime first for unit tests.
   Then implement Extism runtime.
7. Extism runtime:
   - Load module from file path initially.
   - Expose host function workflow.command.
   - Use JSON envelopes.
   - Enforce timeout/memory limits where available.
   - Disable or minimize WASI by default.
   - No allowed hosts/paths by default.
8. Tests:
   - workflow completes without commands
   - workflow emits command, command executes, workflow completes on replay
   - command result is cached
   - same command ID different args causes nondeterminism error
   - denied capability prevents execution
   - memory store optimistic concurrency works
   - command safety with unknown recovery status is represented
Design constraints:
- No plugin gets raw shell, network, filesystem, Kubernetes client, or secrets.
- All side effects must go through workflow.command.
- Event history is source of replay state.
- WorkflowRun pins module digest.
- Command idempotency key must be derived from runID + moduleDigest + commandID + commandName + argsHash.
- Memory history backend is allowed but must document that crash recovery is not guaranteed.
- Non-idempotent commands without recovery support must be able to enter CommandUnknown/quarantine state after ambiguous failure.
Do not implement Kubernetes controller or Telegram yet. Only build capruntime-core with fake commands and Extism runtime skeleton.

⸻

31. Final architectural summary

The final mental model:

WASM module:
  deterministic workflow brain
workflow.command:
  the only way to ask for outside-world work
host command handler:
  actual side-effect executor
capability broker:
  authority and policy
event history:
  memory and replay source of truth
history store:
  pluggable durability guarantee
approval system:
  human gate for risky commands
assistantd:
  LLM/Telegram UX over workflows
ops-controller:
  Kubernetes event-to-workflow automation

The most important invariant:

No external side effect may happen unless it is:
  - represented as a stable command,
  - authorized by capability policy,
  - recorded in event history,
  - tied to an idempotency/recovery strategy,
  - audited.

That invariant is what makes this more than “WASM plugins.” It makes it a reusable, safer operational automation platform.
