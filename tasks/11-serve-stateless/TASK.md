---
id: 11-serve-stateless
title: Stateless serving worker + sqlite/redis stores
group: serve
inputs:
  - agent_sdk/agent/
  - agent_sdk/session/
outputs:
  - agent_sdk/serve/
  - agent_sdk/stores/sqlite/
  - agent_sdk/stores/redis/
checks:
  - id: serve-tests
    cmd: go test ./agent_sdk/serve/... ./agent_sdk/stores/...
    description: serve + store tests pass
  - id: serve-race
    cmd: go test -race ./agent_sdk/serve/...
    description: worker isolation under race
mode: task
---
# Rung 11 — serve + stateless

Python → Go:
- `agent_sdk/serve.py` → `serve` (Queue, EventSink protocols + InProcess + Redis adapters; AgentWorker pool with one in-flight turn per conversation via a session lock; Job). Every stateful seam takes a TurnKey{Tenant,AgentID,SessionID} to prevent cross-session bleed.
- `agent_sdk/stores/session.py` (SQL) → `stores/sqlite` using **`modernc.org/sqlite`** (pure-Go, no CGO) — add this dep now (`go get modernc.org/sqlite`; needs Go 1.21+, bump toolchain if blocked). Redis store via **`github.com/redis/go-redis/v9`**.

Translate: `tests/test_stateless_snapshot.py` (snapshot/restore, whole-state save, pooled multi-session isolation), `tests/test_bench_plugins_serve.py`.

Deviation note (per package doc-comment): SQLite=modernc.org/sqlite, redis=go-redis. If a Go-1.21+ toolchain is unavailable, fall back to a file-based session store and record it in BLOCKED.md.

PARITY.md: (serve types are not in `__all__` but power stateless serving).
