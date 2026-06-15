---
id: 05-memory
title: Memory subsystem + shared context + funnel
group: domains
inputs:
  - agent_sdk/engine/
  - agent_sdk/session/
outputs:
  - agent_sdk/memory/
  - agent_sdk/context/
  - agent_sdk/react/funnel.go
checks:
  - id: memory-tests
    cmd: go test ./agent_sdk/memory/... ./agent_sdk/context/...
    description: memory + context tests pass
mode: task
---
# Rung 05 — memory

Python → Go (`agent_sdk/memory/` → `memory`):
- `durable.py` (scoped store + auto-wired `memory` tool; scopes turn/conversation/channel/user/bot), `universal.py` (MemoryStore/MemoryEntry two-tier digest+offload, FLASH_SCOPE drop on snapshot, ToJSON/FromJSON), `scratchpad.py` (turn RAM), `recall_tool.py`, `establish.py` (deterministic bullet-fact offload), `prefetch.py` (`## Memory` index), `golden_head.py` (cosine Q→A), `summarize.py` (`deterministic_digest`), `semantic_cache.py`, `lobes/` (memory_recall, session_recall, ctxvar_resolve).
- `agent_sdk/context/context.py` → `context` (AgentContext, Scope, Evidence, current_context, bind_context).
- `agent_sdk/react/funnel.py` → `react.funnel` (`tier_observations` — re-tier the working set every hop).

Translate: `tests/test_memory_index.py`, `tests/test_semantic_recall.py`, `tests/test_universal_memory.py`, `tests/test_universal_memory_engine.py`, `tests/test_universal_memory_snapshot.py`, `tests/test_telemetry.py`, `tests/test_funnel_workingset.py`.

Embeddings seam: cosine over float vectors via stdlib `math` (no numpy dep). When the embed seam is nil, L2 attention is skipped (parity).

PARITY.md: Memory, MemoryItem, Scratchpad, SemanticCache, AgentContext, Scope, Evidence, current_context, bind_context.
