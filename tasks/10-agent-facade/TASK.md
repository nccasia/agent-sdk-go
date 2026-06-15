---
id: 10-agent-facade
title: PreactAgent facade + AgentStream + react discipline
group: facade
inputs:
  - agent_sdk/engine/
  - agent_sdk/clients/
  - agent_sdk/memory/
outputs:
  - agent_sdk/agent/
  - agent_sdk/react/
checks:
  - id: agent-tests
    cmd: go test ./agent_sdk/agent/... ./agent_sdk/react/...
    description: agent facade + react tests pass
mode: task
---
# Rung 10 — agent facade

Port the public façade — the surface every example and live bench drives.

Python → Go:
- `agent_sdk/agent.py` → `agent` (`PreactAgent` with `Query` (→AgentResult), `Act` (→AgentStream), `Inspect` (→ActivationSnapshot, no-LLM), `RunSnapshot` (stateless turn), `Spec`, `With`, `Submit`/`Events` (serving), `Connect` (MCP), `LastTrace`, `SuggestOptimizations`; `MEMORY_DIRECTIVE`; `FromSpec`).
- `agent_sdk/react/{conversation,grounding,hedge}.py` → `react` (enriched living conversation, DocGroundingGuard, anti-hedge retry).

Translate: `tests/test_agent_e2e.py`, `tests/test_hedge_retry.py`, `tests/test_finalize_hook.py`.

PARITY.md: PreactAgent, AgentStream, DocGroundingGuard.
