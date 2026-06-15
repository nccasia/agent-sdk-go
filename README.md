# agent-sdk-go

A production-ready Go port of the Python **PreAct** SDK (`agent_sdk`), at 100% API
and behavior parity. Built test-first: every Python test/benchmark/example check
becomes a Go test written red, then implemented to green.

## Layout

- `agent_sdk/` — the SDK, package-for-package with the Python `agent_sdk/` (core,
  engine, memory, metacognition, tools, mcp, skills, clients, serve, plugins,
  agent facade, probe/bench/report/viewer).
- `benchmarks/` — the 16 benchmark suites + the verdict/ratchet ecosystem.
- `examples/` — coding-agent and subagents-analytics.
- `cmd/parity` — the 82-export `__all__` parity gate (reads `PARITY.md`).
- `cmd/bench` — the free-benchmark runner (READY gate).
- `tasks/` — the **porting ladder**: one `TASK.md` rung per subsystem,
  dependency-ordered, each gated by exit-0 `go test` checks (Converge format).
- `.claude/workflows/port-sdk.js` — the Workflow-tool driver that walks the
  ladder rung-by-rung with a deterministic keep/revert ratchet, ending in the
  production-ready parity gateway.

## Driving the port

Run the ladder via the Workflow tool: `Workflow({ name: "port-sdk" })`. It reads
each `tasks/NN-*/TASK.md`, TDD-ports the listed Python tests, implements to green,
commits only when every check exits 0, and finishes at the parity gateway. The
ladder is idempotent/resumable — re-running skips rungs whose checks already pass.

## Verification

```
gofmt -l .            # empty
go vet ./...          # clean
go test ./... -race   # green
go run ./cmd/parity   # 82/82 exports
go run ./cmd/bench    # free benches READY
go test ./examples/...
```

## Dependency policy

Pragmatic: vetted Go libs where Python uses deps — `modernc.org/sqlite` (pure-Go
SQLite store), `github.com/redis/go-redis/v9`, `go.opentelemetry.io/otel`; numpy →
stdlib `math`; pydantic → struct tags + `encoding/json`; cachetools → TTL/LRU.
Heavy deps are added at the rung that first needs them and documented in the
owning package's doc-comment. The Python reference is the sibling `../agent-sdk`.
