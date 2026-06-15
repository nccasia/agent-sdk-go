---
id: 03-events-result-session
title: Events, results, session state + in-memory store
group: foundations
inputs:
  - agent_sdk/core/spec/
outputs:
  - agent_sdk/events/
  - agent_sdk/result/
  - agent_sdk/session/
  - agent_sdk/stores/memory/
checks:
  - id: ers-tests
    cmd: go test ./agent_sdk/events/... ./agent_sdk/result/... ./agent_sdk/session/... ./agent_sdk/stores/memory/...
    description: events/result/session tests pass
mode: task
---
# Rung 03 — events + result + session

Python → Go:
- `agent_sdk/events.py` → `events` (RunStart, PathResolved, StageStart, TextDelta, ToolCall, ToolResult, CitationFound, MetaAction, StageEnd, Final; `stamp`; wire-stable JSON).
- `agent_sdk/result.py` → `result` (AgentResult, AgentStream, Trace, Usage, Refusal, MemoryUpdate, Optimization, ActivationSnapshot).
- `agent_sdk/session.py` → `session` (Session, SessionState, Turn, SNAPSHOT_VERSION; `ToJSON`/`FromJSON` forward/backward tolerant — the stateless-serving contract).
- `agent_sdk/stores/{memory,session}.py` → `stores/memory` (SessionStore protocol + in-memory default).

Translate: `tests/test_events_result.py`, `tests/test_memory_session.py`, and the spec/session round-trip parts of `tests/test_spec.py` / `tests/test_stateless_snapshot.py` (schema versioning + tolerance: unknown future key ignored, missing keys default).

PARITY.md: all 10 event types, AgentResult, AgentStream, Trace, Usage, Refusal, MemoryUpdate, Optimization, ActivationSnapshot, Session, SessionState, Turn.
