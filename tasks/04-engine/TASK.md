---
id: 04-engine
title: Engine kernel, stages, flows, selection
group: engine
inputs:
  - agent_sdk/core/activate/
  - agent_sdk/events/
  - agent_sdk/session/
outputs:
  - agent_sdk/engine/
  - agent_sdk/flows/
checks:
  - id: engine-tests
    cmd: go test ./agent_sdk/engine/... ./agent_sdk/flows/...
    description: engine + flow tests pass
mode: task
---
# Rung 04 — engine

Port the generic PreAct turn kernel — one turn = deterministic core (recognize
flow → activate lobes → resolve stages → build per-stage prompt) wrapped around
the I/O seams (LlmCall/ToolRuntime/Memory), streaming typed events, assembling
AgentResult + Trace.

Python → Go:
- `agent_sdk/engine.py` → `engine` (the kernel; `RunTurn`, agentic tool loop with forced tool-free final hop, `Inspect`/`Probe`, segmented `ComposeSystem` emitting `[]SystemSegment{Source,Stability,Start,End}` + `<subject>` threading, `tool_loop`).
- `agent_sdk/{engine_context,selection,stages,stage_overrides,flow_def}.py` → `engine` + `flows` (Stage/StageRegistry/`stage` builder, Flow/`flow`, registries, overrides).
- `agent_sdk/flows/{flow,registry,defaults,compat,stages/*}.py` → `flows` (stage implementations: act/catalog/cite/enrich/filter/research/respond/synthesize/common).

Translate: `tests/test_engine_robustness.py`, `tests/test_stages.py`, `tests/test_stage_overrides.py`, `tests/test_stall_break.py`, `tests/test_fanout_robustness.py`, `tests/test_reply_flow.py`. Use a deterministic FakeClient (port from rung 09 stub or a local fake) to drive turns.

PARITY.md: Engine, Stage, StageRegistry, stage, Flow, flow, tool_loop.
