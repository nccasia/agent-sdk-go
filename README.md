# agent-sdk-go

A Go port of the Python **PreAct** SDK (`agent_sdk`) — the sibling project at
[`../agent-sdk`](../agent-sdk). The goal is package-for-package API and behavior
parity, built **test-first**: every Python test/benchmark/example check is
translated into a Go test, written red, then implemented to green. No rung is
committed until its checks exit 0.

> **This is a port, not a rewrite.** The Python `agent_sdk` is the source of
> truth for behavior. Where a Go test and the Python reference disagree, the
> Python reference wins — divergences are treated as port bugs, not redesigns.

## Status

Ported rung-by-rung along a dependency-ordered ladder (`tasks/`):

| Rungs | Subsystem | State |
|-------|-----------|-------|
| 00–12 | contracts, lobes/network, events/result/session, engine, memory, metacognition/cognition, tools/mcp, skills, clients, agent facade, serve, plugins | ✅ ported |
| 13 | inspection: probe, bench, report, viewer, blocks | ✅ ported |
| 14 | benchmark spine (verdict/ratchet/provider, free-gate) + benches | ✅ ported¹ |
| 15 | runnable examples (coding-agent, subagents-analytics) | ✅ ported |

**Parity: 116/116 exports** (`go run ./cmd/parity`) — 100% of the
`agent_sdk.__all__` API contract, including `tool_loop → engine.ToolLoop`.
`gofmt`/`go vet` clean and `go test ./... -race` green across all ported
packages; both examples run offline-deterministic (`go test ./examples/...`).

¹ The benchmark **spine** (verdict/ratchet/provider/registry/free-gate) plus
every bench is ported (tracked as progress rows in `PARITY.md`, excluded from the
export gate — they are not `__all__` exports). The free-gate (`go run ./cmd/bench`)
is green and exits 0: each bench reproduces its Python source-of-truth verdict.
- **Free benches** (deterministic, gated): `statelessbench`, `flowbench`,
  `promptbench`, `toolbench`, `corgictionbech` → `READY`; `attentionbench` →
  `NOT_READY` (the qna/research grounding scenarios reference a `cite` lobe that
  does not fire without RAG — `run.py` exits 1 in Python too). Each ships a
  check-id parity test against its `run.py` check ids.
- **Live benches** (agent/task/extension/skill/coding-agent/delegation) — require
  a real provider, so they report `UNMEASURED` without one and are not gated.

## Layout

- `agent_sdk/` — the SDK, package-for-package with the Python `agent_sdk/` (core,
  engine, memory, metacognition, tools, mcp, skills, clients, serve, plugins,
  agent facade, probe/bench/report/viewer).
- `cmd/parity` — the export parity gate (reads `PARITY.md`, the `__all__` ledger).
- `cmd/bench` — the free-benchmark runner (READY gate).
- `benchmarks/` — the 12 ported bench suites (rung 14) behind the free-gate;
  `examples/` — the runnable coding-agent + subagents-analytics demos (rung 15).
- `tasks/` — the **porting ladder**: one `TASK.md` rung per subsystem,
  dependency-ordered, each gated by exit-0 `go test` checks (Converge format).
- `.claude/workflows/` — the Workflow-tool drivers that walk the ladder.

## How the port is driven

The port is executed by a deterministic **keep/revert ratchet** running over the
ladder, one rung at a time, via the Workflow tool:

- `.claude/workflows/port-sdk.js` — the full driver: for each `tasks/NN-*/TASK.md`
  it reads the listed Python modules + tests, writes the equivalent Go `*_test.go`
  red, implements only that rung's package(s) until every `checks` command exits 0,
  checks off the exports in `PARITY.md`, and commits. A rung that can't go green in
  `MAX_WAVES` attempts is recorded in a `BLOCKED.md` and reverted (never committed
  red) so it can't poison later rungs. It ends at a production-ready parity gateway
  (`gofmt`/`vet`/`go test -race`/parity/benches/examples).
- `.claude/workflows/port-sdk-resume.js` — the same ratchet, narrowed to the
  remaining rungs so a resumed run doesn't re-walk already-committed ones.

Run the ladder: `Workflow({ name: "port-sdk" })`. The ladder is
idempotent/resumable — re-running skips rungs whose checks already pass.

## Verification

```
gofmt -l .            # empty
go vet ./...          # clean
go test ./...         # green
go run ./cmd/parity   # 116/116 exports (100%)
go run ./cmd/bench    # free-gate green / exit 0 (rung 14)
go test ./examples/...# (rung 15)
```

## Dependency policy

Pragmatic: vetted Go libs where Python uses deps — `modernc.org/sqlite` (pure-Go
SQLite store), `github.com/redis/go-redis/v9`, `go.opentelemetry.io/otel`; numpy →
stdlib `math`; pydantic → struct tags + `encoding/json`; cachetools → TTL/LRU.
Heavy deps are added at the rung that first needs them and documented in the
owning package's doc-comment. The Python reference is the sibling `../agent-sdk`.
