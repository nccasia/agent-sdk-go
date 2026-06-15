---
id: 00-bootstrap
title: Go module skeleton, deps policy, ladder, and parity ledger
group: foundations
outputs:
  - go.mod
  - PARITY.md
  - cmd/parity/main.go
  - cmd/bench/main.go
  - .claude/workflows/port-sdk.js
  - .claude/settings.json
checks:
  - id: build
    cmd: go build ./...
    description: the empty module builds
  - id: parity-tool
    cmd: go run ./cmd/parity PARITY.md >/dev/null; test $? -le 1
    description: the parity checker runs (exit 0 or 1, not a crash)
mode: task
---
# Rung 00 — bootstrap

Scaffold the Go module `github.com/mezon/agent-sdk-go` (Go 1.19+; the heavy deps
— `modernc.org/sqlite`, `github.com/redis/go-redis/v9`, `go.opentelemetry.io/otel`
— are added lazily at rungs 11/12, not here, so the core ladder stays offline).

Deliverables:
- `go.mod` with the module path.
- The package directory skeleton mirroring `../agent-sdk/agent_sdk/` (see the plan's package map).
- `PARITY.md` — the 82-export `__all__` ledger (`- [ ]` per export).
- `cmd/parity` — parses PARITY.md, exits non-zero while any box is unchecked.
- `cmd/bench` — placeholder runner (rung 14 fills it).
- `.claude/workflows/port-sdk.js` — the ladder driver.
- `.claude/settings.json` — allowlist for go/gofmt/git.

Dep policy (pragmatic): real Go libs where Python uses deps; each deviation
documented in the package doc-comment. numpy→stdlib math; pydantic→struct tags +
encoding/json; cachetools→small TTL/LRU.

This rung is already scaffolded by hand; the workflow only verifies it builds.
