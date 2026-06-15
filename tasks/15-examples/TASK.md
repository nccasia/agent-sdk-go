---
id: 15-examples
title: Examples (coding-agent + subagents-analytics)
group: examples
inputs:
  - agent_sdk/agent/
  - agent_sdk/plugins/
outputs:
  - examples/codingagent/
  - examples/subagents-analytics/
checks:
  - id: examples-tests
    cmd: go test ./examples/...
    description: example tests pass
mode: task
---
# Rung 15 — examples

Python → Go (`examples/` → `examples`):
- `examples/coding-agent/` → `examples/codingagent` (triage→explore→plan→implement→verify on a real FS; one CodingPlugin; tools Read/Write/Edit/LS/Glob/Grep/Bash). Translate `test_coding_agent.py` + `conftest.py` helpers; `demo.py` offline-deterministic.
- `examples/subagents-analytics/` → `examples/subagents-analytics` (plan→supervise→fanout→fanin over a SQLite fixture → executive summary; subagent fan-out with deps-independent todos). SQLite fixture via `modernc.org/sqlite`.

Both must run offline-deterministic via FakeClient.
