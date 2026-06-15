---
id: 16-parity-gateway
title: Production-ready parity gateway
group: gateway
inputs:
  - benchmarks/
  - examples/codingagent/
  - PARITY.md
outputs:
  - PARITY.md
checks:
  - id: gofmt
    cmd: test -z "$(gofmt -l .)"
    description: gofmt clean
  - id: vet
    cmd: go vet ./...
    description: vet clean
  - id: tests
    cmd: go test ./... -race
    description: full suite green under race
  - id: parity
    cmd: go run ./cmd/parity
    description: 100% of the 82 __all__ exports present
  - id: benches
    cmd: go run ./cmd/bench
    description: all free benches READY
  - id: examples
    cmd: go test ./examples/...
    description: examples green
mode: converger
converge:
  max_waves: 5
  halt_when:
    - parity
    - tests
    - benches
---
# Rung 16 — parity gateway (production-ready)

The terminal gate. Loops (converger) until every check passes:
- `gofmt -l .` empty, `go vet ./...` clean, `go test ./... -race` green.
- `go run ./cmd/parity` → 100% (all 82 `__all__` exports present, signatures match `docs/api.md`).
- `go run ./cmd/bench` → all free benches READY (cross-language parity vs Python).
- `go test ./examples/...` green.
- Confirm `docs/api.md` section map reproduced for the Go public surface.

Each wave: pick the highest-value failing check, diagnose, apply the smallest fix
in the owning package, re-run. Commit per fix. When all `halt_when` checks pass,
the Go SDK is at 100% API + behavior parity and production-ready.
