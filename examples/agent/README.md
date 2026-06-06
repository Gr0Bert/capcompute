# Agent Example

This is a small agent-shaped demo for `capcompute`.

The guest is a Wasm module that re-enters from the top on every replay.
It runs a bounded autonomous loop:

1. receive a user question;
2. call `llm.complete` to choose an action;
3. yield while the model result is pending;
4. replay and receive the recorded model result;
5. either call a tool or finish;
6. append tool observations to the transcript;
7. repeat until the model returns a final answer or the step limit is reached.

The host has:

- an OpenAI-compatible chat completions client;
- deterministic `tool.search`, `tool.read`, and `tool.remember` handlers;
- a replay dispatcher;
- a journal-backed tape using the in-memory journal.

There are no schedulers, queues, databases, or product-specific agent
abstractions in this example. The point is to show how those things can sit
outside the library while the guest execution remains replayable.

Build the guest:

```sh
sh examples/agent/guest/build.sh
```

Run the demo:

```sh
export OPENAI_API_KEY=...
go run ./examples/agent
```

Run without a model API:

```sh
export AGENT_LLM=fake
go run ./examples/agent
```

OpenAI-compatible configuration:

```sh
export AGENT_LLM=openai                           # optional
export OPENAI_BASE_URL=https://api.openai.com/v1   # optional
export OPENAI_MODEL=gpt-4o-mini                    # optional
export OPENAI_ORG_ID=...                           # optional
export OPENAI_PROJECT_ID=...                       # optional
export OPENAI_TEMPERATURE=0.2                      # optional
export OPENAI_MAX_TOKENS=400                       # optional
export OPENAI_SEED=42                              # optional
export OPENAI_TIMEOUT_SECONDS=60                   # optional
export OPENAI_MAX_RETRIES=2                        # optional
export OPENAI_RESPONSE_FORMAT=true                 # optional
export AGENT_RUN_ID=agent-1                        # optional
export AGENT_WASM_PATH=examples/agent/agent.wasm   # optional
export AGENT_TRACE=text                            # optional: text or json
export AGENT_MAX_STEPS=4                           # optional
export AGENT_MAX_TICKS=8                           # optional
export AGENT_QUESTION="What does capcompute make possible?"
```

`OPENAI_RESPONSE_FORMAT=true` asks OpenAI-compatible providers for JSON output
on the planning call. Set it to `false` if your compatible provider does not
support `response_format`.

Expected shape:

```text
agent: run=agent-1
question: What does capcompute make possible?
tick 1: yielded on llm.complete
async 1: stored llm.complete result
model 1: {"tool":"tool.search","query":"..."}
tick 2: yielded on llm.complete
async 2: stored llm.complete result
model 2: {"tool":"tool.read","url":"..."}
tick 3: yielded on llm.complete
async 3: stored llm.complete result
model 3: {"tool":"tool.remember","note":"..."}
tick 4: yielded on llm.complete
async 4: stored llm.complete result
model 4: {"action":"final","answer":"..."}
agent: completed
output: {"answer":"...","steps":[...],"sources":[...]}
```

Replay is the important part: after each fake async model completion is written
to the journal, the guest starts again from `run`, and the replay tape feeds
back already completed calls instead of invoking those side effects again.

The local tools use a tiny ranked in-memory corpus. `tool.search` returns ranked
candidates, `tool.read` loads full source content, and `tool.remember` stores
short per-run notes. Model actions include `reason`, so the output trace records
why each step happened. The control flow is still deterministic in tests, but it
now behaves like a real tool-using agent loop rather than a fixed script.

`AGENT_MAX_STEPS` limits decisions inside the guest. `AGENT_MAX_TICKS` limits
host replay ticks, which protects the outer scheduler loop.
