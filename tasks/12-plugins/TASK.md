---
id: 12-plugins
title: Plugin framework + all 11 plugins
group: plugins
inputs:
  - agent_sdk/engine/
  - agent_sdk/tools/
  - agent_sdk/skills/
outputs:
  - agent_sdk/plugins/
checks:
  - id: plugins-tests
    cmd: go test ./agent_sdk/plugins/...
    description: plugin tests pass
mode: task
---
# Rung 12 — plugins

Port the extension surface. Each plugin contributes a full capacity surface
(lobes/stages/flows/skills/tools + finalize/tool-result hooks).

Python → Go (`agent_sdk/plugins/` → `plugins`):
- `base.py` (AgentSetup), `registry.py` (PluginRegistry, builtin_registry), `__init__.py`.
- Default-on: `safety/` (filter), `format/` (styling). Opt-in: `rag/` (cite + citation contract: extraction/backfill/strip/ground-or-refuse).
- `tasks/` (lobes/path/stages/todos), `metacognition/` (lobes/path/stages/tool — MetaControlToolRuntime; enables corgiction plugin_surface mode), `planning/` (lobes/path/stages/tool), `mcp/` (resolver), `otel/` (**`go.opentelemetry.io/otel`** telemetry), `guardrails/` (errors, leak_guard), `workspace/` (drivers, tools), `support_triage/` (lobes/skills/stages/tools).

Translate: `tests/test_plugin_registry.py`, `tests/test_plugins_full_surface.py`, `tests/test_task_integration.py`, `agent_sdk/plugins/tasks/tests/*`, `agent_sdk/plugins/rag/tests/*`, `agent_sdk/plugins/metacognition/tests/*`.

Default-network parity: a no-plugin agent must be byte-identical to the default network.

PARITY.md: (plugins not in `__all__` directly).
