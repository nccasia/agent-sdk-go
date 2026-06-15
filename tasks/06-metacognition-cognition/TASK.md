---
id: 06-metacognition-cognition
title: Metacognition, cognition lobes, guards, expression
group: domains
inputs:
  - agent_sdk/core/spec/
  - agent_sdk/inspection/
  - agent_sdk/engine/
outputs:
  - agent_sdk/metacognition/
  - agent_sdk/cognition/
  - agent_sdk/expression/
  - agent_sdk/guards/
  - agent_sdk/inspection/
checks:
  - id: metacog-tests
    cmd: go test ./agent_sdk/metacognition/... ./agent_sdk/cognition/... ./agent_sdk/expression/... ./agent_sdk/guards/... ./agent_sdk/inspection/...
    description: metacognition/cognition/guards tests pass
mode: task
---
# Rung 06 — metacognition + cognition + guards + expression

Python → Go:
- `agent_sdk/inspection.py` → `inspection` (LobeInspection, LobeAxisSnapshot, FlowStepInspection, FlowAxisSnapshot, EngineSnapshot).
- `agent_sdk/metacognition/{model,monitor,regulator,controller}.py` + `metacognition_facade.py` → `metacognition` (MetaObservation/MetaDecision/MetaAction, `Monitor`, `Regulate` decision table, `MetaController`, `Metacognition` with pinned-unskippable guard, `CompileStatePlan`; `_TRIMMABLE_LOBES`, `_APPLY_CAPABLE_ACTIONS`, `_DEFAULT_APPLY_ACTIONS`). All PURE — port the decision tables 1:1.
- `agent_sdk/cognition/lobes/{classify,condense,plan,research,scope_check,synthesize}.py` → `cognition` (LLM-driven via seams; activation signals deterministic).
- `agent_sdk/expression/lobes/respond.py` → `expression`.
- `agent_sdk/guards/{refusal,safety,answer_guard}.py` → `guards`.

Translate: `tests/test_navigator.py`, `tests/test_planning.py`, `tests/test_refusal_gate.py`, `tests/test_answer_leak_guard.py`, `tests/test_grounding.py`, and `agent_sdk/plugins/metacognition/tests/*` (the pure monitor/regulate/pinned cases).

PARITY.md: Metacognition.
