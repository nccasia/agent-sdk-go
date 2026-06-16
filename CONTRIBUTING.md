# Contributing to agent-sdk-go

Thanks for your interest in improving **agent-sdk-go** — the Go port of the Python
[agent-sdk](https://github.com/nccasia/agent-sdk). This guide covers the dev setup, the invariants
every change must keep, and the gates a PR has to pass.

## Dev setup

Requires Go 1.21+ (developed on 1.26). The only third-party dependency is the pure-Go
`modernc.org/sqlite` driver — no CGO, no C toolchain needed.

```bash
git clone https://github.com/nccasia/agent-sdk-go && cd agent-sdk-go
go build ./...
go test ./...
```

## This is a port, not a rewrite

The Python `agent-sdk` is the **source of truth** for behavior. Where a Go test and the Python
reference disagree, the Python reference wins — a divergence is a port bug, not a redesign. Behavior
is added rung-by-rung (`tasks/NN-*/TASK.md`): translate the named Python tests to Go first (red),
implement to green, and only commit when every check exits 0. See [`docs/porting.md`](./docs/porting.md).

## Invariants every change must keep

- **Deterministic core.** Intent recognition, activation, attention/budget, and flow resolution are
  a pure function of `(spec, context)` — no I/O, no clocks, no randomness. Everything that touches
  the outside world (LLM, tools, embeddings, stores, queues) sits behind a narrow interface with an
  in-memory default.
- **Default-network parity.** An agent with no extra plugins must behave identically to the default
  PreAct network.
- **API parity.** The public surface tracks `agent_sdk.__all__` via `PARITY.md`; `go run ./cmd/parity`
  must stay at 100%.
- **Citations-mandatory / safety-pinned.** Pinned output-contract steps (`filter`, and `cite` when
  the RAG plugin is on) are never skipped by metacognition.

## Gates a PR must pass

```bash
gofmt -l .              # must print nothing
go vet ./...            # clean
go test ./... -race     # green
go run ./cmd/parity     # 116/116 exports (100%)
go run ./cmd/bench      # free-gate green / exit 0
go test ./examples/...  # examples green
```

Benchmarks: the free benches are deterministic and gated by `cmd/bench`; the live benches require a
provider (set credentials, then run with a model — they report `UNMEASURED` without one). Every
bench also emits an inspectable HTML report under `benchmarks/results/`.

## Keeping in sync with the Python SDK

The Python [`agent-sdk`](https://github.com/nccasia/agent-sdk) keeps evolving; the Go port tracks it
commit-by-commit. The state lives in [`docs/UPSTREAM.md`](./docs/UPSTREAM.md) (the ledger) and is
machine-checkable via the `Upstream:` commit trailer.

```bash
PYTHON_SDK=../agent-sdk ./scripts/check-upstream.sh   # lists PORTED / PENDING upstream commits
```

For each `PENDING` commit: read its diff in the Python repo, translate it to Go (tests first), keep
the suite green, and commit with an **`Upstream: <full-sha>`** trailer in the message. Then advance
the "Synced to" line and add a row in `docs/UPSTREAM.md`. A hunk with no Go equivalent is recorded as
`n/a` in the ledger rather than skipped silently.

## Commit style

Conventional-commit subjects (`feat(...)`, `fix(...)`, `docs:`, `refactor:`, `chore:`). Keep changes
scoped to one concern; never commit a red tree. Commits that port an upstream change carry an
`Upstream: <sha>` trailer (see above).
