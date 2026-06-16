# How agent-sdk-go maps to the Python reference

**agent-sdk-go is a port, not a rewrite.** The Python
[`agent-sdk`](https://github.com/nccasia/agent-sdk) is the **source of truth** for behavior; where a
Go test and the Python reference disagree, the reference wins — a divergence is a port bug.

PreAct is portable because it cleaves cleanly into two halves:

1. **A deterministic core** — intent recognition, lobe/stage/flow activation, attention/budget
   selection, and flow resolution. Pure functions of `(spec, context)`: no I/O, no clock, no LLM,
   no randomness. Same `spec` + same `context` ⇒ identical routing in Go and Python.
2. **A handful of I/O seams** — `LlmCall`, `ToolRuntime`, `Embed`, `SessionStore`, `MemoryStore`,
   `EventSink` — the only things that touch the outside world, implemented with Go's native
   `net/http` / `database/sql` / channels.

## Package map

The Go layout is package-for-package with the Python `agent_sdk/`:

| Python | Go package | Notes |
|---|---|---|
| `agent_sdk` (façade) | `agent_sdk/agent` | `PreactAgent`, `Config`, `Query`/`Act` |
| `agent_sdk.engine` | `agent_sdk/engine` | the generic kernel + `ToolLoop` |
| `agent_sdk.clients` | `agent_sdk/clients` | Anthropic / OpenAI / MiniMax / Mixed / Fake |
| `agent_sdk.session` / `agent_sdk.stores` | `agent_sdk/session`, `agent_sdk/stores/*` | in-memory · Redis · SQLite (`modernc.org/sqlite`) |
| `agent_sdk.memory` | `agent_sdk/memory` | universal memory, scratchpad, semantic cache |
| `agent_sdk.plugins` | `agent_sdk/plugins` | registry + built-in plugins |
| `agent_sdk.skills` / `tools` / `mcp` | `agent_sdk/skills`, `agent_sdk/tools`, `agent_sdk/mcp` | |
| `agent_sdk.metacognition` / `cognition` | `agent_sdk/metacognition`, `agent_sdk/cognition` | |
| `agent_sdk.probe` / `bench` / `report` / `viewer` | `agent_sdk/probe`, `bench`, `report`, `viewer` | inspection surface |
| `agent_sdk.serve` | `agent_sdk/serve` | stateless `AgentWorker` pool |
| `agent_sdk.spec` | `agent_sdk/core/spec` + `agent.FromSpec` | serializable spec + rebuilder |

## Naming conventions

- `snake_case` → `CamelCase` (`agent_from_spec` → `agent.FromSpec`; `make_client` → `clients.MakeClient`).
- Python test `test_tight_adjust` → Go `TestTightAdjust`.
- Optional keyword args become either a second method (`Inspect(q)` / `InspectWithState(q, state)`)
  or functional options (`tools.Tool(name, fn, tools.Desc(…), tools.Param(…))`).
- Async (`async def` / `await`) becomes synchronous Go with `context.Context`; streaming uses a
  channel (`agent.Act(ctx, q).Iter()`).

## Dependency policy

Pragmatic: vetted Go libraries where Python uses deps — `modernc.org/sqlite` (pure-Go SQLite, no
CGO) for the SQLite store; numpy → stdlib `math`; pydantic → struct tags + `encoding/json`;
cachetools → TTL/LRU. The only direct third-party module is `modernc.org/sqlite`.

## Conformance

The port is conformant when:

- **API parity** — every `agent_sdk.__all__` export is present in Go with a matching signature
  (`PARITY.md`; `go run ./cmd/parity` → 116/116).
- **Behavior parity** — the benches reproduce their Python source-of-truth verdicts
  (`go run ./cmd/bench`; cross-checked against the Python `run.py` `ok`/`Verdict.Status`).
- **Suite green** — `go test ./... -race`, `gofmt -l .` empty, `go vet ./...` clean.

## How the port is driven

A deterministic keep/revert ratchet walks the dependency-ordered ladder in `tasks/` one rung at a
time (drivers in `.claude/workflows/`): for each `tasks/NN-*/TASK.md` it translates the named Python
tests to Go (red), implements that rung's package(s) to green, checks off `PARITY.md`, and commits —
only when every `checks` command exits 0. A rung that can't go green is recorded in a `BLOCKED.md`
and reverted rather than committed red.
