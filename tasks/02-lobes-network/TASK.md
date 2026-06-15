---
id: 02-lobes-network
title: Lobe machinery, paths, production network, per-lobe signals
group: core
inputs:
  - agent_sdk/core/spec/
  - agent_sdk/core/activate/
outputs:
  - agent_sdk/lobes/
  - agent_sdk/core/feature/
  - agent_sdk/network/
  - agent_sdk/preact/
checks:
  - id: network-tests
    cmd: go test ./agent_sdk/lobes/... ./agent_sdk/core/feature/... ./agent_sdk/network/... ./agent_sdk/preact/...
    description: lobe/network tests pass
mode: task
---
# Rung 02 — lobes + paths + production network

Python → Go:
- `agent_sdk/lobes/{runtime,registry,weights,network,rows,patterns}.py` → `lobes` (Lobe, LobeRegistry, B-layer concat, `add_row`).
- `agent_sdk/paths/{qna,research,clarify,relational,onboarding,common}.py` → `core/feature` recognizers (PATHS, `RecognizePaths`).
- `agent_sdk/preact/{production,defaults,lobes}.py` → `network` (the 18-lobe / 8-path production net, embedded as JSON via go:embed) + `preact` (`Lobes`/`Stages`/`Flows` `.Default()`/`.Minimal()`).
- **Per-lobe signal extraction**: the perception layer (`core/feature`) must emit the `f::lobe::signal` context flags so `memory_recall`, `classify`, `cite`, etc. cross their thresholds — the OY-axis parity that a raw-query `Inspect` depends on. Regenerate the network from Python's `conformance/export_production.py` (source of truth) rather than hand-editing.

Translate: `tests/test_phase0.py`, `tests/test_default_efficiency.py`, plus golden routing/activation fixtures exported from Python (`fixtures_production`, `fixtures_routing`) — assert `Propagate` reproduces Python's activated-lobe lists.

PARITY.md: Lobe, LobeRegistry, Lobes, Stages, Flows.
