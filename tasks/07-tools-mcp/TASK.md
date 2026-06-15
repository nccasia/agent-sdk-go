---
id: 07-tools-mcp
title: Tools, MCP, doc guards
group: domains
inputs:
  - agent_sdk/contracts/
  - agent_sdk/engine/
outputs:
  - agent_sdk/tools/
  - agent_sdk/mcp/
  - agent_sdk/react/docguard.go
  - agent_sdk/react/docworkspace.go
checks:
  - id: tools-tests
    cmd: go test ./agent_sdk/tools/... ./agent_sdk/mcp/... ./agent_sdk/react/...
    description: tools/mcp/react-doc tests pass
mode: task
---
# Rung 07 — tools + mcp

Python → Go:
- `agent_sdk/tools/` → `tools` (`Tool` builder → spec + `MissingRequired` + `Requires`; hand-rolled object schema, pydantic substitute; `FunctionToolRuntime`, `CompositeToolRuntime` + `ExternalNames`; `tools/lobes/tool_select.py` → `ToolSelectLobe.Select` with essentials firewall + relevance trim + budget + parity-dark activation).
- `agent_sdk/mcp.py` → `mcp` (`MCPToolRuntime`, `MCPServerSpec`, `MCPError`) over an in-proc JSON-RPC `Transport func(map[string]any) map[string]any` (no network); `resolve`/`get_tool_specs`/`call_tool`/`external_names`.
- `agent_sdk/react/{docguard,docworkspace}.py` → `react` (DocWriteGuard, DocWorkspace heavy-doc capability).

Translate: `tests/test_tools.py`, `tests/test_tool_selection.py`, `tests/test_mcp.py`, `tests/test_docguard.py`.

PARITY.md: tool, Tool, FunctionToolRuntime, CompositeToolRuntime, MCPToolRuntime, MCPServerSpec, MCPError, DocWriteGuard.
