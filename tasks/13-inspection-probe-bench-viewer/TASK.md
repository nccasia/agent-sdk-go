---
id: 13-inspection-probe-bench-viewer
title: Probe, bench harness, report + viewer (cosmetics), blocks
group: dev-tools
inputs:
  - agent_sdk/engine/
  - agent_sdk/agent/
outputs:
  - agent_sdk/probe/
  - agent_sdk/bench/
  - agent_sdk/report/
  - agent_sdk/viewer/
  - agent_sdk/blocks/
checks:
  - id: devtools-tests
    cmd: go test ./agent_sdk/probe/... ./agent_sdk/bench/... ./agent_sdk/report/... ./agent_sdk/viewer/... ./agent_sdk/blocks/...
    description: probe/bench/report/viewer tests pass
mode: task
---
# Rung 13 — inspection + probe + bench + viewer

Python → Go:
- `agent_sdk/probe.py` → `probe` (Probe, ProbeRecord — async exec trace of stages/tools/status/answer).
- `agent_sdk/bench.py` → `bench` (Harness, Scenario, ScenarioResult, Report; lobe_recall summary).
- `agent_sdk/report.py` + `assets/*.html` → `report` (RenderHTML/WriteHTML single-file report; port the HTML templates via go:embed).
- `agent_sdk/viewer.py` → `viewer` (RenderViewerHTML/WriteViewer/ToRecord).
- `agent_sdk/_blocks.py` → `blocks` (flat re-export aggregation of contracts/lobe primitives).

Translate: `tests/test_probe_report.py`, `tests/test_viewer.py`.

PARITY.md: Harness, Scenario, ScenarioResult, Report, probe, ProbeRecord, render_html, write_html, render_viewer_html, write_viewer, to_viewer_record, and the remaining _blocks primitives.
